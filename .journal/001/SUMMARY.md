---
id: 001
title: Reproduce go-template-api session 015 tooling changes (mise + melange/apko + SLSA L3)
date: 2026-06-28
status: complete
repos_touched: [template-mcp]
related_sessions: []
---

## Goal
Reproduce, in `template-mcp`, the tooling migration from `template-go-api`'s
session 015 — **so far as it concerns what is present in this repo**: adopt
**mise** (replacing Proto), adopt **Chainguard melange/apko** (replacing the
Dockerfile), and the **release-flow updates** (reusable provenance workflow / SLSA
L3). Investigate first, deliver an assessment, then implement.

## Outcome
**Met in full and proven end-to-end.** Reproduced as three squash-merged PRs and
validated with a real release rehearsal that passed clean on the first attempt
(the sibling needed four versions to shake out the same path):
- **#11** `7a…`→`51625ca` — Proto → mise + moon on the `system` toolchain.
- **#12** `d0f4f5c` — Dockerfile → melange/apko (+ keyless cosign, syft SBOM, in-job provenance).
- **#16** `dc616c9` — provenance moved to a reusable `attest.yml` (SLSA Build L3).
- **Rehearsal:** forced **v0.1.4** (v0.1.3 was a pre-existing rolled-back release);
  `release.yml` ran green across all 8 jobs; the published image
  `ghcr.io/meigma/template-mcp:v0.1.4` (linux/amd64+arm64) is cosign-verified and
  carries SLSA-provenance attestations whose signer is `attest.yml@refs/tags/v0.1.4`
  (verified — confirms L3 isolation). The GitHub release is a **draft** awaiting a
  human publish.

## Key Decisions
- **Three-PR structure mirroring 015** (developer choice) — mise → melange/apko →
  reusable attest.yml, each provable via `moon run root:check` + a dispatched
  `release-dry-run`/`security-scan`. PR2 deliberately keeps attestation **in-job**
  and adds explicit `actions/attest-build-provenance` (dropping buildx removed its
  `provenance: mode=max`); PR3 extracts attestation into the reusable workflow.
  The sibling's current `release.yml` is already post-L3, so PR2 could not copy it
  verbatim — the in-job intermediate had to be reconstructed.
- **mockery → mise (aqua)** (developer choice) — moved off the `tools/proxy/go.mod`
  `tool` directive to a single mise-pinned source; added a `proxy:mockery` regen
  task. **Dropped the sibling's `mockery-check` drift task**: this repo generates
  mocks **in-package** (`mocks_test.go`, `package reloader`), so a
  generate-to-tempdir-and-diff check yields package-qualified false drift; and that
  check was never a 015 change. Staleness still surfaces via `proxy:test`.
- **mockery pinned at 3.7.0** (the repo's current version) to avoid spurious mock
  regeneration vs. the sibling's 3.7.1.
- **No DB / no compose** → no sqlc/goose, no `sqlc-verify` to remove, no `stack-up`;
  only an `image-local` mise task. mise tool set = 9 (vs. the sibling's 11).
- **apko gains `cmd: http --addr 0.0.0.0:8080 --insecure`** so the MCP image default
  matches the former Dockerfile CMD (the sibling's API image had an entrypoint only).
- **Dropped the dead `docker` dependabot ecosystem** — no Dockerfile to track; the
  Wolfi base floats and is recorded in the per-build SBOM + provenance. (Deliberate
  deviation; the sibling left this stale entry.)
- **Rehearsed via release-please, not a bare tag** — `release.yml` requires a
  release-please draft (its `resolve-release` waits for one), so a throwaway tag
  cannot trigger it; the rehearsal must force a real version through release-please.

## Changes
- `mise.toml` + `mise.lock` (new): 9 aqua/core tools, `GOTOOLCHAIN=local`,
  `lockfile`+`locked`; `image-local` task. moon `macos-x64` lock entry hand-added.
- `moon.yml` (root + `tools/proxy` + `docs`): bare commands, `toolchains.default:
  system`, fileGroups track mise files; `proxy:mockery` task. `.moon/toolchains.yml`
  emptied; deleted `.prototools`, `.moon/proto/*`.
- `tools/proxy/go.mod`: dropped the mockery `tool` directive (+ `go mod tidy`).
- `melange.yaml` + `apko.yaml` (new); deleted `Dockerfile`/`.dockerignore`/`.go-version`.
- `.github/workflows/`: `attest.yml` (new reusable, SLSA L3); `release.yml`,
  `release-dry-run.yml`, `security-scan.yml` rewired to melange/apko + the reusable
  attest jobs; `ci.yml`/`docs-pages.yml` → `jdx/mise-action`.
- `ghd.toml` + `stage_ghd_release_assets.py` (+ test): signer → `attest.yml`.
- `release-please-config.json`: `extra-files` = [melange.yaml, apko.yaml].
- `.gitignore`, README, CONTRIBUTING, DELETE_ME, docs prose updated.
- **Bug fix exposed by the migration:** moon's `system` toolchain mangles
  `go test -coverprofile=coverage.out` into a file named `=coverage.out`; switched
  both test tasks to the space form `-coverprofile coverage.out`.

## Open Threads
- **`v0.1.4` GitHub release is a DRAFT** — awaiting a human publish (image already
  live on GHCR). The developer was asked and has not yet published it.
- **Cosmetic residue:** the force-release path bumped master 0.1.2→0.1.3→0.1.4;
  `CHANGELOG.md` carries a `0.1.3 (force release #17)` entry plus the `0.1.4` entry.
  Manifest now `0.1.4`.
- **Pre-existing `v0.1.3`** (released `#4`, 2026-06-14, then manifest rolled back to
  0.1.2) left untouched — not created this session; its GHCR image/tag/release were
  deliberately not deleted.
- Dependabot PRs #10/#13/#14/#15 (action bumps incl. `actions/attest` 4.1.0→4.1.1)
  are unrelated to this session and left for normal triage.

## References
- PRs: #11 (mise), #12 (melange/apko), #16 (SLSA L3); rehearsal #17/#18 (blocked at
  0.1.3), #20/#19 (v0.1.4). release.yml run `28329365824` = full success.
- Released (draft): `v0.1.4` — `ghcr.io/meigma/template-mcp:v0.1.4`
  (index `sha256:99fb728bd58fbcc73f14c6f736b57b4b127ca5f1a011b241601c700f875cedc1`).
- Source: `template-go-api/.wt/journal-jmgilman/.journal/015/` (SUMMARY + NOTES).
- Session log: `.journal/001/NOTES.md`.

## Lessons
- **The three tag-only-path fixes from the sibling's shakeout, pre-applied, made the
  first real tag pass clean:** (1) a reusable attest workflow can't request more
  permissions than its caller grants — every caller (incl. the binary one) must grant
  `packages: write` to match the shared job; (2) `apko publish --sbom-path <dir>`
  needs the dir to pre-exist (`mkdir -p`); (3) `attest.yml` needs its OWN
  `docker/login-action` — the build job's GHCR login doesn't cross the
  reusable-workflow boundary. The dry-run reaches none of these; only a real tag does.
- **`release.yml` can't be rehearsed with a bare/throwaway tag** — `resolve-release`
  waits for a release-please draft, so the rehearsal must go through release-please
  and burns a real version. Check for pre-existing tags/releases first (v0.1.3 had
  been released then rolled back, which silently blocked the first attempt).
- **mise `lockfile=true` does not create `mise.lock`** (touch it / `mise lock` first);
  enforcement key is `settings.locked`; `mise lock` drops moon's `macos-x64` entry
  (hand-add from moon's published checksum). `.wt/` worktrees nest under the repo, so
  `mise trust` both the worktree and the parent.
- **Local multi-worktree gotcha:** after `wt remove` of a sibling worktree,
  golangci-lint's shared cache holds stale entries pointing at the deleted path and
  fails the next lint with spurious findings — `golangci-lint cache clean` fixes it.
  CI is unaffected (fresh runner).
