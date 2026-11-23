## Summary

<!-- Describe what this PR changes and why. Link related issues if any (e.g. Fixes #123). -->

## Verification

- [ ] `make check` passes (`golangci-lint`, `mygo vet`, `mygo i18n check`, tests with `-race` + `./internal/ui`)
- [ ] New or updated tests cover the changed behavior
- [ ] UI changes checked in light & dark mode and at `1000x680` (attach screenshots if applicable)
- [ ] User-facing strings added to `locales/` (and `locales/catalog_gen.go` regenerated if needed)
- [ ] If `website/` changed: `pnpm --dir website check` passes
