ALTER TABLE organizations
    DROP COLUMN IF EXISTS spans_this_month_count,
    DROP COLUMN IF EXISTS spans_count_period_start;
