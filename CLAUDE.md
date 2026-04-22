# Cvalent

Go project. See `/opt/projects/CLAUDE.md` for cross-project protocol and workspace layout.

## Area Identity

This project's pln area is `cvalent`. All pln work for this repo uses `--area cvalent`.

## Release Model

**Target: trunk+tag with release-please bot.**
**Current: trunk+tag (tag-push triggered); alignment work in progress.**

Flow (once fully aligned):
- Feature branches cut from `main`
- PRs target `main`; use conventional-commit PR titles (`feat:`, `fix:`, `chore:`, etc.)
- release-please opens release PRs automatically; merging them tags + triggers `release.yml` (goreleaser)
- First release target: `v0.1.0`

**Do not:** `git tag` manually — release-please owns tag creation post-alignment.

Canonical playbook: `/opt/projects/base/docs/release-models.md` (tracked as pln AH-0380.2).
Alignment work: pln AH-0380.4. Decision log: pln epic AH-0380.
