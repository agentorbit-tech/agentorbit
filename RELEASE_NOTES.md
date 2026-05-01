# Release Notes

- Fix: Pro request submission now succeeds in cloud — auth cookie uses `SameSite=None; Secure` when shared across subdomains so it travels to the billing service on cross-site fetch. Self-host stays on `SameSite=Lax`.
