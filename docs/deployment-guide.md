# Deployment Guide

## Quick Start

### Prerequisites

- Docker 24+ and Docker Compose v2
- At least 2 GB RAM and 2 CPU cores available

### Steps

```bash
git clone https://github.com/agentorbit-tech/agentorbit.git
cd agentorbit
cp .env.example .env
```

Edit `.env` and replace all `changeme_` values with secure random strings:

```bash
# Generate secrets (Linux/macOS)
openssl rand -hex 16   # for JWT_SECRET, HMAC_SECRET, INTERNAL_TOKEN (min 32 chars)
openssl rand -hex 32   # for ENCRYPTION_KEY (exactly 64 hex chars)
```

Start the stack:

```bash
docker compose up -d
```

Dashboard: http://localhost:8081

## Configuration

All configuration is via environment variables in `.env`.

### Required Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `POSTGRES_PASSWORD` | Database password | Random string |
| `DATABASE_URL` | PostgreSQL connection string | `postgres://user:pass@postgres:5432/agentorbit?sslmode=disable` |
| `JWT_SECRET` | JWT signing key (min 32 chars) | `openssl rand -hex 16` |
| `HMAC_SECRET` | API key digest secret (min 32 chars) | `openssl rand -hex 16` |
| `ENCRYPTION_KEY` | AES-256 key for provider keys (64 hex chars) | `openssl rand -hex 32` |
| `INTERNAL_TOKEN` | Proxy-to-processing auth (min 32 chars) | `openssl rand -hex 16` |

### Optional Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PROCESSING_PORT` | `8081` | Processing service port |
| `PROXY_PORT` | `8080` | Proxy service port |
| `DEPLOYMENT_MODE` | `self_host` | Deployment mode (`cloud`, `self_host`) |
| `JWT_TTL_DAYS` | `30` | JWT token lifetime in days |
| `ALLOWED_ORIGINS` | (empty) | Comma-separated frontend URLs for CORS |
| `PROVIDER_TIMEOUT_SECONDS` | `120` | Timeout for LLM provider requests |
| `SPAN_WORKERS` | `3` | Number of async span dispatch workers |
| `SMTP_HOST` | (none) | SMTP server for email delivery |
| `SMTP_PORT` | (none) | SMTP port (typically 587) |
| `SMTP_USER` | (none) | SMTP username |
| `SMTP_PASS` | (none) | SMTP password |
| `SMTP_FROM` | (none) | Sender address for emails |
| `DATA_RETENTION_DAYS` | `0` (keep forever) | Auto-delete spans/sessions older than N days |
| `LOG_LEVEL` | `INFO` | Log verbosity (`DEBUG`, `INFO`, `WARN`, `ERROR`) |
| `PROCESSING_LLM_BASE_URL` | (none) | LLM API URL for narratives/clustering |
| `PROCESSING_LLM_API_KEY` | (none) | LLM API key |
| `PROCESSING_LLM_MODEL` | (none) | LLM model name |

### CORS Configuration

Set `ALLOWED_ORIGINS` to the URL(s) where your frontend is served:

```bash
# Single origin
ALLOWED_ORIGINS=https://agentorbit.example.com

# Multiple origins
ALLOWED_ORIGINS=https://agentorbit.example.com,https://app.example.com
```

If empty, CORS headers are not sent. Browsers will block cross-origin requests to the API.

## Security Checklist

Before exposing to the internet:

- [ ] Replace all `changeme_` values in `.env`
- [ ] Set `ALLOWED_ORIGINS` to your frontend URL(s)
- [ ] Place behind a reverse proxy (nginx, Caddy) with HTTPS
- [ ] Ensure `.env` is not accessible from the web
- [ ] Keep `POSTGRES_PORT` bound to `127.0.0.1` (default)
- [ ] Review Docker resource limits in `docker-compose.yml`

## Upgrading

```bash
git pull
docker compose build
docker compose up -d
```

### Database Migrations

Migrations run as a one-shot `migrate` service in `docker-compose.yml`. The service uses the `agentorbit-migrate` binary (built from `processing/cmd/migrate`) and exits before `processing` starts. Compose enforces `depends_on: condition: service_completed_successfully` so processing will not boot until migrations succeed.

Manual operations:

```bash
# apply all pending migrations
docker compose run --rm migrate up

# print version, dirty flag, and pending count
docker compose run --rm migrate status

# roll back N steps (destructive — guarded)
AGENTORBIT_ALLOW_DESTRUCTIVE=1 \
  docker compose run --rm \
    -e AGENTORBIT_ALLOW_DESTRUCTIVE=1 \
    migrate down --steps=1 --i-know-what-im-doing

# force a specific version (destructive — only after manual cleanup)
AGENTORBIT_ALLOW_DESTRUCTIVE=1 \
  docker compose run --rm \
    -e AGENTORBIT_ALLOW_DESTRUCTIVE=1 \
    migrate force 27 --i-know-what-im-doing
```

The processing service refuses to start when `schema_migrations` is missing, behind the embedded version, or marked dirty — the runbook scenario "Deploy failed mid-migration" walks through recovery.

## Backup and Restore

### Creating a Backup

Use `pg_dump` from the PostgreSQL container (or any host with `pg_dump` installed):

```bash
# From the Docker host — dumps the database inside the postgres container
docker compose exec postgres pg_dump \
  -U ${POSTGRES_USER:-agentorbit} \
  -d ${POSTGRES_DB:-agentorbit} \
  --format=custom \
  --compress=zstd \
  > agentorbit_$(date +%Y%m%d_%H%M%S).dump
```

The `--format=custom` flag produces a compressed archive that supports selective restore and parallel jobs. The `--compress=zstd` flag uses zstd compression (PostgreSQL 16+; omit for older versions or use `--compress=9` for gzip).

For automated daily backups, add a cron entry on the Docker host:

```cron
0 3 * * * docker compose -f /path/to/docker-compose.yml exec -T postgres pg_dump -U agentorbit -d agentorbit --format=custom --compress=zstd > /backups/agentorbit_$(date +\%Y\%m\%d).dump 2>> /var/log/agentorbit-backup.log
```

### Restoring a Backup

To restore onto a fresh instance (new `docker compose up` with empty database):

```bash
# 1. Start only postgres (processing would auto-migrate and create tables)
docker compose up -d postgres

# 2. Wait for postgres to be healthy
docker compose exec postgres pg_isready -U agentorbit

# 3. Restore the dump (--clean drops existing objects first)
docker compose exec -T postgres pg_restore \
  -U ${POSTGRES_USER:-agentorbit} \
  -d ${POSTGRES_DB:-agentorbit} \
  --clean --if-exists \
  --no-owner \
  --no-privileges \
  < agentorbit_20260330.dump

# 4. Start the full stack — processing will run any newer migrations on top
docker compose up -d
```

Key flags:
- `--clean --if-exists`: drops existing objects before restoring, ignores missing ones
- `--no-owner --no-privileges`: avoids errors when the target user differs from the original

### Verifying a Restore

After restoring, verify data integrity:

```bash
docker compose exec postgres psql -U agentorbit -d agentorbit -c "
  SELECT 'users' AS table_name, COUNT(*) FROM users
  UNION ALL SELECT 'organizations', COUNT(*) FROM organizations
  UNION ALL SELECT 'sessions', COUNT(*) FROM sessions
  UNION ALL SELECT 'spans', COUNT(*) FROM spans
  UNION ALL SELECT 'schema_migrations', MAX(version)::text FROM schema_migrations;
"
```

Check the migrate job logs and that processing started without complaining about missing or dirty migrations:

```bash
docker compose logs migrate
docker compose logs processing | grep -E 'migrations verified|migration check failed'
```

### Restore + Migration on Fresh Instance

When restoring a backup from an older version onto a newer AgentOrbit release:

1. The dump contains the schema at the time of backup (e.g., migration 17)
2. `pg_restore --clean` recreates that exact schema
3. Run `docker compose run --rm migrate up` — `golang-migrate` detects the current version from `schema_migrations` and applies any newer migrations on top
4. Start the rest of the stack: `docker compose up -d`

This is safe because all migrations are idempotent and use `IF NOT EXISTS` / `IF EXISTS` guards.

## Backups (Cloud)

AgentOrbit cloud uses managed PostgreSQL 17 (AWS RDS today; GCP Cloud SQL is the documented fallback) with the following baseline:

- Point-in-time recovery (PITR) enabled, 7-day retention.
- Daily automated snapshots retained for 30 days.
- Snapshots are cross-region replicated to the secondary recovery region.
- Quarterly restore drill: ops owns a checklist that provisions a throw-away instance from the most recent snapshot, runs `agentorbit-migrate status`, and validates the dashboard against a known fixture user. Drill timestamp + outcome are recorded in the ops log.

Restore SLO targets:

| Target | Value |
|--------|-------|
| RPO (point-in-time recovery) | 5 minutes |
| RTO (single-region restore) | 30 minutes |

## Backups (Self-host)

Self-host deployments are responsible for their own backups. The recommended setup uses [WAL-G](https://github.com/wal-g/wal-g) for continuous archiving + base backups against any S3-compatible storage (AWS S3, MinIO, Backblaze B2, etc.).

1. **Install the WAL-G binary on the postgres host** — pre-built releases are at https://github.com/wal-g/wal-g/releases. Place it at `/usr/local/bin/wal-g`.
2. **Required environment variables** (see `examples/backups/wal-g.env.example`):
   - `WALG_S3_PREFIX` — destination bucket and path, e.g. `s3://my-bucket/agentorbit/wal-g`
   - `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` — bucket credentials
   - `AWS_REGION` — bucket region (or `AWS_ENDPOINT` for non-AWS S3)
   - `WALG_GPG_KEY_ID` — encryption-at-rest GPG key fingerprint (mandatory; do not skip)
   - `WALG_COMPRESSION_METHOD` — `lz4` (default) or `zstd`
3. **Postgres configuration** (`postgresql.conf` snippet at `examples/backups/postgres-walg.conf`):
   ```conf
   wal_level = replica
   archive_mode = on
   archive_command = '/usr/local/bin/wal-g wal-push %p'
   archive_timeout = 60
   ```
4. **Base-backup cron** (postgres user crontab):
   ```cron
   0 3 * * * /usr/local/bin/wal-g backup-push /var/lib/postgresql/data
   ```
5. **Retention** (run weekly):
   ```bash
   wal-g delete retain FULL 7 --confirm
   ```
6. **Restore drill** — run quarterly on a clean host:
   ```bash
   # 1. Stop postgres on the target host.
   # 2. Empty the data directory.
   # 3. Fetch the most recent base backup.
   wal-g backup-fetch /var/lib/postgresql/data LATEST
   # 4. Configure recovery.signal + restore_command, then start postgres.
   echo "restore_command = '/usr/local/bin/wal-g wal-fetch %f %p'" >> /var/lib/postgresql/data/postgresql.auto.conf
   touch /var/lib/postgresql/data/recovery.signal
   systemctl start postgresql
   # 5. Once recovery completes, validate: connect with psql, run the
   #    integrity query from "Verifying a Restore" above, then promote.
   ```

Cross-link: the runbook scenario "Full disk on prod DB" references this section for emergency WAL trimming.

## Data Retention

Set `DATA_RETENTION_DAYS` to automatically purge old data. When set to a positive integer, a daily cron inside the processing service deletes:

1. **Spans** older than N days (by `created_at`)
2. **Sessions** that are closed and older than N days, with no remaining spans
3. **Alert events** older than N days

Deletion runs in batches of 1000 rows to avoid long locks. Organizations, users, API keys, and alert rules are never affected.

| Use Case | Recommended Setting |
|----------|---------------------|
| Development / testing | `DATA_RETENTION_DAYS=7` |
| Production (cost-conscious) | `DATA_RETENTION_DAYS=90` |
| Production (compliance) | `DATA_RETENTION_DAYS=365` |
| Keep everything | Omit or set to `0` |

Combined with backups, a typical strategy is: daily `pg_dump` retained for 30 days + `DATA_RETENTION_DAYS=90` for live data.

## Resource Requirements

| Service | Memory | CPU | Purpose |
|---------|--------|-----|---------|
| PostgreSQL | 512 MB | 1.0 | Database |
| Processing | 512 MB | 1.0 | API, WebSocket, workers |
| Proxy | 256 MB | 0.5 | Request forwarding |
| **Total** | **~1.3 GB** | **2.5** | Minimum for the full stack |

For production workloads, increase limits based on your agent traffic volume.

## Metrics

Metrics scraping (Prometheus / Grafana) is not shipped in MVP. Roadmap: post-launch.

## Reverse Proxy Setup

### nginx

```nginx
server {
    listen 443 ssl;
    server_name agentorbit.example.com;

    location / {
        proxy_pass http://localhost:8081;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /cable {
        proxy_pass http://localhost:8081;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

### Agent Proxy

For agents, point at the proxy service (default port 8080):

```nginx
server {
    listen 443 ssl;
    server_name proxy.agentorbit.example.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_http_version 1.1;
        proxy_buffering off;  # Required for SSE streaming
    }
}
```
