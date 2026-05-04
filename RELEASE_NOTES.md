# Release Notes

- Fix release image build: drop unused `canMutate` import from `SettingsPage.tsx` so the prod `tsc -b` build (used by Docker / Release workflow) compiles. v0.1.7 published green CI but its release pipeline failed to push images.
