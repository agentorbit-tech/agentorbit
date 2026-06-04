# Release Notes

- Raise auth rate limit from 5 to 30 req/min/IP — the old limit broke legitimate accept-invite flows after logout+login and surfaced a misleading "invite expired" error.
