# Skill Sources

Skills are copied from upstream without modification (except added `LICENSE` files noted below). To update a skill, replace its directory from a newer upstream revision and update this table. `.claude/skills/<name>` holds relative symlinks pointing to these directories.

| Skill | Source | Revision | License | Purpose |
| --- | --- | --- | --- | --- |
| `mygo-maintenance` | `mygo init -template native` (`github.com/egoist/mygo`) | mygo v0.8.3 (unchanged since v0.7.4) | MIT (mygo) | mygo rules: state, threading, UI lifetime, testing, builds |
| `sketch-design-to-code` | `github.com/sketch-hq/agents` | `2b9f716abcb433b2f1b7d18270d0c1141eaaf16e` (2026-06-25) | see `LICENSE.txt` in skill dir | Implementing UI from Sketch frames via Sketch MCP |
| `sketch-design-from-reference` | `github.com/sketch-hq/agents` | `2b9f716abcb433b2f1b7d18270d0c1141eaaf16e` (2026-06-25) | see `LICENSE.txt` in skill dir | Designing new screens in Sketch from references |
| `golang-testing` | `github.com/samber/cc-skills-golang`, v1.4.0 | `bb1d43d98a3f8ab639a0042f90d0ec524e39a477` (2026-11-01) | MIT | Table-driven tests, fixtures, fakes, integration tags |
| `golang-concurrency` | `github.com/samber/cc-skills-golang`, v1.2.1 | `bb1d43d98a3f8ab639a0042f90d0ec524e39a477` (2026-11-01) | MIT | Parallel scanner, cancellation, errgroup, race safety |

To update `mygo-maintenance` after upgrading `mygo`: run `go tool mygo install-skills` (overwrites only its own files under `.agents/skills`).

## Notes

- A `LICENSE` file from the upstream repository root was added to each `samber` skill directory to satisfy MIT redistribution terms.
- On Windows, symlinks under `.claude/skills` require `git config core.symlinks true` and Developer Mode enabled; otherwise read skills directly from `.agents/skills`.

## Considered and Omitted

- Other `samber/cc-skills-golang` skills (error handling, project layout, naming, etc.) overlap with `AGENTS.md` and `mygo-maintenance`.
- Skills targeting `testify`, `gotests`, or `mockery`: Hearth tests use Go stdlib and manual fakes.
