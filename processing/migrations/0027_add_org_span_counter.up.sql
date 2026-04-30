-- Step 1: add columns NULL-able with constant defaults — instant on Postgres 11+
-- (no table rewrite). spans_this_month_count defaults to 0 literal. Period start
-- gets a placeholder epoch date that the next IngestSpan will reset to current
-- month via the period-rollover branch in TryIncrementOrgSpanCounter.
ALTER TABLE organizations
    ADD COLUMN spans_this_month_count   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN spans_count_period_start DATE    NOT NULL DEFAULT '1970-01-01';

-- Step 2: backfill counter from spans for the current UTC month. Uses the
-- existing index idx_spans_org_created_at(organization_id, created_at) — one
-- index range scan per org. Period start is set to current UTC month start so
-- the first ingest after deploy uses the existing counter value rather than
-- treating it as stale and resetting to 1.
UPDATE organizations o
SET
    spans_this_month_count = COALESCE((
        SELECT count(*) FROM spans s
        WHERE s.organization_id = o.id
          AND s.created_at >= date_trunc('month', NOW() AT TIME ZONE 'UTC')
    ), 0),
    spans_count_period_start = (date_trunc('month', NOW() AT TIME ZONE 'UTC'))::date;
