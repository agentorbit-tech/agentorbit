package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/agentorbit-tech/agentorbit/proxy/internal/auth"
	"github.com/agentorbit-tech/agentorbit/proxy/internal/config"
	"github.com/agentorbit-tech/agentorbit/proxy/internal/errtrack"
	"github.com/agentorbit-tech/agentorbit/proxy/internal/handler"
	"github.com/agentorbit-tech/agentorbit/proxy/internal/span"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

func main() {
	// Initialize structured logging (JSON for production, per D-19)
	var logLevel slog.LevelVar
	logLevel.Set(slog.LevelInfo)
	if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		if err := logLevel.UnmarshalText([]byte(lvl)); err != nil {
			slog.Error("invalid LOG_LEVEL, using INFO", "value", lvl, "error", err)
		}
	}
	logHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:       &logLevel,
		ReplaceAttr: errtrack.SlogReplaceAttr,
	})
	slog.SetDefault(slog.New(logHandler))

	cfg, err := config.LoadProxy()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if err := errtrack.Init(errtrack.Config{
		DSN:         cfg.SentryDSN,
		Environment: cfg.SentryEnvironment,
		Release:     cfg.SentryRelease,
		Service:     "proxy",
		SampleRate:  cfg.SentrySampleRate,
	}); err != nil {
		slog.Error("sentry init failed, continuing without error tracking", "error", err)
	}

	// Signal-only context: triggers the shutdown sequence on SIGINT/SIGTERM.
	// Deliberately NOT used as parent for upstream provider calls or workers —
	// otherwise SIGTERM would abort in-flight requests instead of letting the
	// http.Server drain them.
	shutdownSig, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Long-lived context for background goroutines (cache eviction, per-key
	// rate-limiter cleanup, dispatcher workers). Canceled AFTER HTTP shutdown
	// completes so handlers in flight don't see abrupt cancellation of their
	// dependencies.
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	// Auth cache — validates AgentOrbit API keys via Processing
	cache := auth.NewAuthCache(
		workerCtx,
		cfg.ProcessingURL,
		cfg.InternalToken,
		cfg.HMACSecret,
		cfg.AuthCacheTTL,
		&http.Client{Timeout: 5 * time.Second},
		cfg.CacheEvictInterval,
	)

	// Span dispatcher — async send to Processing
	dispatcher := span.NewSpanDispatcher(
		cfg.ProcessingURL,
		cfg.InternalToken,
		cfg.SpanBufferSize,
		&http.Client{Timeout: cfg.SpanSendTimeout},
		cfg.SpanSendTimeout,
		cfg.DrainTimeout,
		cfg.SpanWorkers,
	)
	dispatcher.Start(workerCtx)

	// Proxy handler. The ctx passed here is used for the per-key rate-limiter
	// cleanup goroutine — NOT for upstream provider calls (those use r.Context()).
	proxyHandler := handler.NewProxyHandler(workerCtx, cache, dispatcher, cfg.ProviderTimeout, cfg.DefaultAnthropicVersion, cfg.AllowPrivateProviderIPs, cfg.PerKeyRateLimit)

	// Router
	r := chi.NewRouter()
	r.Use(errtrack.Middleware)
	r.Use(chiMiddleware.RequestID)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":        "ok",
			"spans_dropped": dispatcher.Dropped(),
			"goroutines":    runtime.NumGoroutine(),
			"heap_alloc_mb": m.HeapAlloc / 1024 / 1024,
		})
	})

	// Proxy endpoints
	r.Post("/v1/chat/completions", proxyHandler.ServeHTTP)
	r.Post("/v1/messages", proxyHandler.ServeHTTP)

	slog.Info("proxy listening", "port", cfg.Port)
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	// Wait for either a shutdown signal or the server to exit unexpectedly.
	select {
	case <-shutdownSig.Done():
		slog.Info("shutdown signal received, beginning graceful shutdown")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err)
			os.Exit(1) //nolint:gocritic // exitAfterDefer: process is terminating, defers are for graceful shutdown path only
		}
		return
	}

	// Phase 1: stop accepting new connections, wait for in-flight HTTP handlers
	// (and their upstream provider calls). Provider calls are bound by
	// r.Context(), which Go cancels per-request when the response writes
	// complete or the agent disconnects — NOT by shutdownSig.
	httpCtx, httpCancel := context.WithTimeout(context.Background(), cfg.ShutdownHTTPTimeout)
	if err := srv.Shutdown(httpCtx); err != nil {
		slog.Error("http shutdown error", "error", err)
	}
	httpCancel()

	// Phase 2: drain dispatcher. Handlers may have just dispatched the final
	// spans for completed requests — give them time to ship to Processing.
	dispatcher.Drain(cfg.ShutdownDrainTimeout)

	// Phase 3: cancel long-lived workers (cache eviction, rate-limiter sweep,
	// any dispatcher backstop) and flush sentry.
	workerCancel()
	errtrack.Flush(5 * time.Second)
}
