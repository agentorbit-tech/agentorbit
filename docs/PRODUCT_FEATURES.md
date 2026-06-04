# AgentOrbit — Implemented Product Features

This document lists all implemented product features, grouped by category. When developing a new feature, add it here upon completion.

**Tiers:** Free (cloud), Pro (cloud, paid), Self-host (open-source)

---

## Proxy

- **OpenAI-compatible endpoint** (`POST /v1/chat/completions`) — forwards to OpenAI-compatible providers
- **Anthropic-compatible endpoint** (`POST /v1/messages`) — forwards to Anthropic
- **SSE streaming pass-through** — chunks forwarded without buffering
- **Custom provider base URLs** — per-key override of provider endpoint
- **Provider timeout** — 120s, returns 504 on timeout
- **Fail-open design** — proxy continues if Processing is down (spans lost, acceptable)
- **API key validation caching** — 30-60s TTL, reduces Processing load
- **Rate limiting per API key** — sliding window, configurable
- **Request masking** — automatic PII masking in logs (email, phone, SSN, credit card)
- **Session grouping headers** — `X-AgentOrbit-Session` and `X-AgentOrbit-Agent` for explicit grouping

---

## Authentication & Authorization

- **Registration** — email + password, optional email verification gate
- **Login / logout** — JWT HS256, HttpOnly cookie, configurable TTL (default 30 days)
- **Password reset** — email-based token flow
- **Profile** — update name and password
- **API keys** — create `ao-<32 hex>` keys; HMAC-SHA256 digest-only storage; masked display; deactivation
- **Provider key encryption** — AES-256-GCM at rest
- **Multi-org membership** — users can belong to multiple organizations
- **Roles** — Owner, Admin, Member, Viewer with per-endpoint gates
- **Invitations** — email invites with role pre-assignment; token-based acceptance; revocation
- **Ownership transfer** — owner can transfer to another member
- **Leave organization** — members can leave (owner cannot without transfer)

---

## Dashboard & Observability

- **Session list** — filter by status, API key, agent, date range, provider; sort by date / cost / span count; cursor pagination
- **Session detail** — full span list, narrative, metadata
- **KPI cards** — total sessions, spans, cost (USD), avg latency, error rate
- **Daily activity chart** — sessions and cost per day
- **Finish reason distribution** — breakdown by finish_reason across spans
- **Date range filtering** — custom windows, default last 30 days
- **Per-agent statistics** — sessions / spans / cost breakdown by agent name
- **Real-time updates** — WebSocket push for sessions, spans, KPIs, and alerts

---

## Sessions & Spans

- **Auto-grouping** — spans grouped by API key + idle timeout (default 60s)
- **Explicit grouping** — via `X-AgentOrbit-Session` header
- **Session statuses** — `in_progress`, `completed`, `failed`, `abandoned`, `completed_with_errors` (terminal, no reverse transitions)
- **Session closure cron** — closes idle sessions every 30s
- **Cost calculation** — per-token pricing (hardcoded by model + provider), input/output differential

---

## System Prompts

- **Automatic extraction** — detects explicit `system:` blocks and longest common prefix across spans (≥100 chars)
- **Deduplication** — SHA-256 content hash
- **Listing and detail view** — frequency, linked sessions, short UIDs (SP-1, SP-2…)

---

## Narratives *(Free: concatenated summaries; Pro/Self-host: LLM-generated)*

- **LLM-generated session narratives** — summarizes what the agent did
- **Locale support** — narratives in org language (EN, RU)
- **Fallback summaries** — concatenated input text for Free tier

---

## Failure Clustering *(Free: deterministic; Pro/Self-host: LLM-powered)*

- **Failure detection** — HTTP errors and span anomalies
- **Cluster grouping** — related failures grouped into named categories
- **Clusters view** — list with count, drill into sessions per cluster
- **Anomaly detection** — content, timing, token anomalies

---

## Alerts *(Pro/Self-host only)*

- **Alert types** — `failure_rate`, `anomalous_latency`, `new_failure_cluster`, `error_spike`
- **Configurable thresholds** — evaluation window (1–1440 min), cooldown (15–1440 min)
- **Role-based notifications** — specify which roles receive emails
- **Cron evaluation** — runs on schedule; `new_failure_cluster` fires immediately
- **Alert events log** — history of all fired alerts
- **Enable/disable** — temporary toggle without deletion

---

## Privacy & Masking *(Pro/Self-host only)*

- **Content storage toggle** — `store_span_content` flag to disable storing span text
- **Custom masking rules** — up to 10 regex-based rules per org
- **Masking modes** — regex masking or full content suppression
- **Masking map** — records which rules matched per span

---

## CSV Export

- **Session and span export** — CSV download with same filters as dashboard
- **Streaming export** — handles large result sets without buffering
- **Row cap** — configurable limit with `#TRUNCATED` indicator
- **Dedicated DB pool** — 180s timeout vs 30s standard to handle long exports

---

## Organization Settings

- **Name and locale** — editable org name; UI language EN / RU
- **Session timeout** — configurable idle threshold (default 60s)
- **Member management** — list, update roles, remove members; audit log
- **Org deletion** — 14-day grace period with restoration; cascade hard-delete by cron
- **Data retention** — configurable span retention (days; 0 = keep forever)

---

## Email & SMTP

- **Transactional emails** — password reset, verification, invites, alert notifications
- **Rate limiting** — per-address, per-time-window to prevent spam
- **Optional SMTP** — system continues without it (emails not sent)

---

## Support *(cloud only)*

- **Support tickets** — users open tickets from the dedicated "Support" sidebar section
- **Per-ticket chat** — back-and-forth messaging with the support team; staff replies are visually distinguished from user messages
- **Billing-gated** — available on cloud plans only, surfaced based on the org's billing entitlement

---

## Infrastructure & Ops

- **Health endpoints** — `/health` (liveness), `/readyz` (DB check)
- **Metadata endpoint** — `/meta` (version, config)
- **JSON structured logging** — production-ready, no secrets logged
- **Request ID correlation** — tracked across logs
- **Sentry integration** — optional error reporting
- **Docker Compose stack** — self-host: proxy + processing + postgres
- **DB migrations** — golang-migrate, run at Processing startup

---

## Tier Summary

| Feature | Free | Pro | Self-host |
|---|---|---|---|
| Sessions, Spans, KPIs | ✓ | ✓ | ✓ |
| System Prompts | ✓ | ✓ | ✓ |
| CSV Export | ✓ | ✓ | ✓ |
| Multi-org, Roles, Invites | ✓ | ✓ | ✓ |
| Real-time WebSocket | ✓ | ✓ | ✓ |
| Narratives | Summaries | LLM | LLM |
| Failure Clustering | Deterministic | LLM | LLM |
| Alerts | ✗ | ✓ | ✓ |
| Privacy / Masking | ✗ | ✓ | ✓ |
| Support chat | ✓ | ✓ | ✗ |
| Span quota | 3 000/month | Unlimited | Unlimited |
