# Release Notes

- Fix: "Deactivate" button on API Keys page now works (frontend was calling the wrong route).
- Viewer role: mutating buttons (create/deactivate key, invite/remove member, add/edit/delete alert, save settings, save privacy, delete org) are now disabled with a tooltip explaining the missing permission.
- Viewers can now read alert rules and events (was blocked with 403); creating, updating, and deleting alerts still requires owner/admin.
- Privacy masking rules: added `email` as a second preset; preset rules are now editable and deletable. Presets are seeded once for new orgs and never re-injected on save.
- API key creation dialog now shows an encryption notice ("provider keys are stored encrypted with AES-256-GCM") so users understand how their keys are handled.
- Security: `GET /api/orgs/{orgID}/spans/{spanID}/masking-maps` now scopes the lookup by organization. Previously a member of one org could read masking maps for spans owned by another org by passing a foreign span ID through their own org route.
- Login now restores the user's last-used organization instead of always landing on the first org returned by the API.
- Privacy settings: regex compile for user-supplied masking patterns is now bounded by a 250 ms deadline so a pathological pattern cannot pin the request goroutine.

