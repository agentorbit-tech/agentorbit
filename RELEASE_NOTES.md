# Release Notes

- Fix invite links — `/auth/invite?token=...` now opens an accept-invite page instead of a 404. Visitors are prompted to sign in or register, and the invite is auto-accepted afterwards.
- Onboarding now surfaces the correct proxy `base_url` (e.g. `https://api.agentorbit.tech/v1`). The processing service exposes a new `PROXY_URL` env via `/api/meta`; cloud deployments must set `PROXY_URL=https://api.agentorbit.tech` in `compose/processing/.env`.
- API key creation: the raw-key dialog now shows the ready-to-paste `base_url=…` snippet and an "Open dashboard" button that activates after the first copy.
- Dashboard empty state: the "Create your first key" panel now disappears as soon as the first session arrives. WebSocket invalidation was extended to agent stats and finish reasons, and a slow poll covers the case where the WebSocket is disconnected.
