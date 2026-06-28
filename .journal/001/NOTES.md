---
id: 001
title: Reproduce go-template-api session 015 tooling changes
started: 2026-06-28
---

## 2026-06-28 07:31 — Kickoff
Goal for the session: Review the full scope of session 015 in
`~/code/meigma/go-template-api` (which was originally sourced from this repo,
`template-mcp` / "go-template") and fully reproduce its tooling changes here, so
far as they concern what is present in this repo. The flagged changes are:
adopting Mise, adopting Chainguard tooling, and release-flow updates.

Current state of the world:
- This repo is `meigma/template-mcp` on branch `master` at b806a95, currently
  3 commits behind `origin/master`.
- Session journal system was just initialized (session-setup) on
  `journal/jmgilman`; this is session 001, the first session.
- Investigation of `go-template-api` session 015 has not started yet. Need to
  read that repo's `.journal/015/` (SUMMARY.md + NOTES.md) and diff its tooling
  state against this repo to scope what applies here.
- Open question already raised with the user: `go-template-api` is a Go service
  while `template-mcp` is an MCP template repo, so part of the work is mapping
  which changes are language-agnostic vs. Go-specific.

Plan (rough):
1. Read `~/code/meigma/go-template-api/.journal/015/` to understand the full
   scope of what changed and why.
2. Inventory the Mise / Chainguard / release-flow artifacts in that repo.
3. Inventory the equivalent tooling currently present in this repo.
4. Produce an assessment: what to reproduce, what does not apply, and the
   ordering/risks — then wait for direction before implementing.

## 2026-06-28 07:45 — Investigation complete; assessment formed
Source repo is `template-go-api` (user said "go-template-api"); the journal lives
at `template-go-api/.wt/journal-jmgilman/.journal/015/`. Read SUMMARY + full NOTES
(423 lines) and ran two parallel Explore inventories (source post-015 state +
this repo's current state).

**Session 015 = three merged PRs + a forced-release shakeout:**
- #24 `7aac1e1` Proto→mise + moon on `system` toolchain. New `mise.toml`
  (go/python + aqua CLIs; `[settings] lockfile+locked`; `GOTOOLCHAIN=local`) +
  committed `mise.lock`; `moon.yml` bare commands + `toolchains.default: system`
  + fileGroups track mise files; `.moon/toolchains.yml`/`docs/moon.yml`→system;
  CI `moonrepo/setup-toolchain`→`jdx/mise-action`; deleted `.prototools`,
  `.moon/proto/*`, `.nvmrc`.
- #25 `4098277` Dockerfile→melange/apko. New `melange.yaml` (signed Wolfi apk,
  go/build, ldflags) + `apko.yaml` (Wolfi base, `accounts` nonroot 65532,
  amd64+arm64, entrypoint); keyless cosign + syft SBOM + provenance; native-runner
  multi-arch; rewired `release.yml`/`release-dry-run.yml`/`security-scan.yml`;
  `compose.yaml`→prebuilt image + mise `image-local`/`stack-up`;
  `release-please-config.json` extra-files = [melange.yaml, apko.yaml]; deleted
  `Dockerfile`/`.dockerignore`/`.go-version`; gitignore `*.spdx.json`. NO
  apko.lock.json (float Wolfi, rely on SBOM+provenance).
- #26 `8d5007d` SLSA L3 via reusable `.github/workflows/attest.yml` (actions/attest
  binaries + attest-build-provenance image); release.yml gains `attest-binaries`/
  `attest-image` caller jobs; `ghd.toml` signer + stage script expected_signer +
  dry-run signer all repointed to attest.yml.
- Release fixes baked in (the 3 tag-only-path bugs): caller grants `packages: write`
  to attest-binaries; `mkdir -p sbom` before apko publish; attest.yml needs its OWN
  `docker/login-action` for `--push-to-registry`.

**this repo (template-mcp) current state — shares the ancestry, diverges 4 ways:**
- Proto-pinned (`.prototools`: golangci-lint 2.12.2, moon 2.1.4) + moon v2
  (`.moon/proto/golangci-lint.toml`, `.moon/toolchains.yml` go 1.26.4/python
  3.14.3/uv 0.11.0). Has Dockerfile (distroless static-debian12:nonroot, port
  8080, CMD `http --addr 0.0.0.0:8080 --insecure`), GoReleaser v2, release-please,
  ghd.toml (signer=release.yml), stage_ghd script+test, same 6 workflows. release.yml
  attests IN-JOB (actions/attest@v4.1.0), not via reusable workflow.
- DIVERGENCE 1: NO database — no sqlc/goose, and crucially NO `sqlc-verify` task to
  remove. mise tool set shrinks to: go, python, uv, golangci-lint, moon, melange,
  apko, cosign (8 not 11).
- DIVERGENCE 2: NO `compose.yaml`/local stack (no DB) → `stack-up`/compose part is
  N/A; only an optional `image-local` mise task is relevant.
- DIVERGENCE 3: nested `tools/proxy/` Go module (MCP dev proxy) with its own
  moon.yml. **mockery is a Go `tool` directive** in `proxy/go.mod` (v3.7.0) +
  `.mockery.yaml`, NO moon task — already pinned via go.mod/go.sum, so it does NOT
  need to enter mise (sibling made it aqua; here it stays a go tool). Recommend
  leave as-is.
- DIVERGENCE 4: container is an MCP server → apko.yaml needs entrypoint
  `/usr/bin/template-mcp` PLUS `cmd: http --addr 0.0.0.0:8080 --insecure` (sibling's
  API image had entrypoint only).

**Confirmed details that shape the plan:**
- ldflag vars are `main.version/commit/date` (from .goreleaser.yaml) → melange ldflags
  match directly.
- `release.yml:98` + `release-dry-run.yml:36` both `go-version-file: .go-version`;
  ci.yml cache keys hash `.go-version` → all must repoint to go.mod / mise.lock before
  deleting `.go-version` (the 015 adversarial blocker applies here too).

Verdict: all three PRs apply to template-mcp with the four divergences above; this
is a faithful, slightly-smaller reproduction. Next: deliver assessment + surface the
real forks (mockery handling, forced-release rehearsal appetite, PR structure) before
implementing.

## 2026-06-28 07:55 — Decisions + prerequisites confirmed; starting PR1
Developer answered the three forks (AskUserQuestion):
1. **Mockery → move to mise (aqua).** Add `aqua:vektra/mockery` to mise.toml + a
   `proxy:mockery`/`mockery-check` moon task; drop the `tool github.com/vektra/
   mockery/v3` directive from `tools/proxy/go.mod` so mise is the single version
   source (then `go mod tidy` the proxy). So mise tool set = 9: go, python, uv,
   golangci-lint, mockery, moon, melange, apko, cosign.
2. **Release rehearsal → YES.** After the 3 PRs land, cut a throwaway/prerelease
   tag to exercise the tag-only publish→cosign→attest path (how 015 found 3 bugs).
3. **PR structure → three PRs mirroring 015**, fixes folded in.

Prereqs (this machine): mise 2026.6.14 ✓ (== sibling CI pin), moon 2.3.5 (via
proto) ✓, proto 0.58.1, docker 29.4.0 ✓ (melange --runner docker), cosign (nix) +
syft (go bin) present; melange/apko absent → mise provides. NOTE memory: bare `go`
is broken on this box (goenv shim) — use moon tasks / `mise exec`.

**PR1 plan (mirror #24 — Proto→mise + moon system):** new `mise.toml`
([tools] go 1.26.4/python 3.14.3/uv 0.11.0 + aqua golangci-lint 2.12.2/mockery/
moon 2.3.5/melange/apko/cosign; [env] GOTOOLCHAIN=local; [settings] lockfile+locked;
no [tasks] yet — image-local lands in PR2); committed `mise.lock`
(`touch mise.lock` + `mise lock --platform linux-x64,linux-arm64,macos-x64,macos-arm64`,
watch the macos-x64 moon-entry persist quirk). `moon.yml` (root + tools/proxy +
docs) → bare commands, `toolchains.default: system`, fileGroups track mise.toml/
mise.lock; add proxy mockery tasks. `.moon/toolchains.yml`→empty;
`docs/moon.yml`→system. `ci.yml`+`docs-pages.yml`: `moonrepo/setup-toolchain`→
`jdx/mise-action` (SHA-pinned, mise 2026.6.14), cache keys `.go-version`/`.prototools`/
`.moon/proto/*`→`mise.lock`. Remove proxy go.mod tool directive + tidy. Delete
`.prototools`, `.moon/proto/*`. KEEP `.go-version` + Dockerfile (PR2 removes; release.yml:98
+ release-dry-run.yml:36 still `go-version-file: .go-version`). Prove via `moon ci` +
fail-closed lock (checksum tamper). Branch `build/proto-to-mise` off origin/master.

## 2026-06-28 08:00 — PR1 shipped (PR #11, open, awaiting CI)
**PR #11** `build(tooling): replace proto with mise and run moon on system
binaries` — branch `build/proto-to-mise` (commit `271bcba`) off origin/master.
https://github.com/meigma/template-mcp/pull/11

What landed: `mise.toml` (go 1.26.4/python 3.14.3/uv 0.11.0 + aqua golangci-lint
2.12.2/mockery 3.7.0/moon 2.3.5/melange 0.54.0/apko 1.2.19/cosign 3.1.1; [env]
GOTOOLCHAIN=local; [settings] lockfile+locked) + committed `mise.lock` (9 tools ×
4 platforms = 36 entries). `moon.yml` (root + proxy + docs) → bare commands,
`toolchains.default: system`, fileGroups track mise files; proxy gains a
`mockery` regen task. `.moon/toolchains.yml` emptied; `docs/moon.yml`→system.
`ci.yml`/`docs-pages.yml` → `jdx/mise-action@v4.2.0` (mise 2026.6.14), GOTOOLCHAIN
env, cache keys → mise.lock. Removed proxy go.mod `tool` directive + tidied
(96 go.sum deletions). Deleted `.prototools` + `.moon/proto/*`. Prose (README/
CONTRIBUTING/getting-started/DELETE_ME) → mise.

Verified locally: `moon run root:check` green (12 tasks); `proxy:mockery`
regenerates committed mocks byte-for-byte; fail-closed proven by checksum-tamper
(`Checksum mismatch`).

GOTCHAs hit + resolved:
1. **moon macos-x64 lock quirk** (015 lesson confirmed) — `mise lock` resolved
   moon for only 3 platforms; hand-added macos-x64 from moon's official v2.3.5
   checksum (matches sibling's mise.lock exactly).
2. **In-package mockery drift-check is infeasible** — committed mocks are
   generated in-package (`package reloader`, unqualified `CallToolFunc`); a
   generate-to-tempdir-and-diff check produces package-qualified
   `reloader.CallToolFunc` → false drift. The sibling's mockery-check works only
   because its mocks live in a separate `mocks/` package. Dropped the check
   (it was pre-existing sibling infra, NOT a 015 change); kept the regen task.
3. **NEW BUG found + fixed: `=coverage.out`** — under moon's `system` toolchain,
   `go test -coverprofile=coverage.out` writes a file literally named
   `=coverage.out` (moon's old `go` toolchain handled the `=` form; system does
   not). Fixed root + proxy test tasks to space form `-coverprofile coverage.out`.
   This is a template-mcp-specific find (sibling's test task has no coverprofile).

Next: watch PR #11 CI (ci/docs-pages/CodeQL/Kusari). Then PR2 (melange/apko) off
master after #11 merges (consumes PR1's mise tools).

## 2026-06-28 09:25 — PR2 shipped (PR #12, open)
**PR #12** `build(release): build the container image with melange + apko` —
branch `build/melange-apko` (commit `7533ea3`) off master (post-#11).
https://github.com/meigma/template-mcp/pull/12

What landed: `melange.yaml` (go/build ./cmd/template-mcp, ldflags) + `apko.yaml`
(Wolfi base, nonroot 65532, amd64+arm64, entrypoint /usr/bin/template-mcp +
`cmd: http --addr 0.0.0.0:8080 --insecure`). Rewired `release.yml`
(melange-build matrix → apko publish + cosign keyless sign + syft SBOM +
attest-sbom + in-job attest-build-provenance), `release-dry-run.yml`
(melange/apko dry-run), `security-scan.yml` (melange/apko → Trivy). Kept binary
attestation IN-JOB (PR3 moves to reusable attest.yml). setup-go .go-version→go.mod.
`release-please-config.json` extra-files [melange.yaml, apko.yaml]. mise `image-local`
task. Deleted Dockerfile/.dockerignore/.go-version; gitignored melange/apko artifacts.
README/DELETE_ME/docs prose → melange/apko.

KEY in-job-vs-reusable decision: sibling's CURRENT release.yml is post-L3 (reusable
attest.yml). To keep the 3-PR split, PR2 keeps attestation in-job and ADDS explicit
`actions/attest-build-provenance` (dropping buildx removed `provenance: mode=max`);
PR3 extracts to the reusable workflow.

Verified locally: `mise run image-local` → 13 MB nonroot image; `--version` stamps
`dev (51625ca) ...`; image Entrypoint/Cmd/User EXACTLY mirror the old Dockerfile;
default run logs `listening addr=[::]:8080`; workflows actionlint-clean; root:check
green (12 tasks).

Deviation from sibling (noted in PR): dropped the dead `docker` dependabot ecosystem
(no Dockerfile; Wolfi floats) — sibling left it stale.

GOTCHA (local only, not CI): after `wt remove` of the PR1 worktree, golangci-lint's
shared cache held stale entries pointing at the deleted `.wt/build-proto-to-mise`,
causing spurious lint failures across the sibling worktree. Fixed by
`golangci-lint cache clean`; CI uses a fresh runner so unaffected.

Next: dispatch release-dry-run + security-scan on the branch to exercise the
melange/apko CI path (dry-run skips on normal branches). Then PR3 (reusable attest.yml,
SLSA L3) off master after #12 merges.

## 2026-06-28 09:35 — PR2 CI green (dispatched dry-run + scan); merging
PR #12 standard checks: CI + GitHub Pages pass. Dispatched on the branch (melange/
apko jobs skip on normal branches): release-dry-run → SUCCESS (Melange Build Dry Run
amd64 + arm64, Binary Release Dry Run, Container Image Dry Run all green) and
security-scan → SUCCESS (Trivy clean on the apko image). Tag-only publish/cosign/
attest path still unexercised (rehearsal after PR3). Merging PR #12, then PR3.

## 2026-06-28 09:50 — PR3 shipped (PR #16, open) — SLSA L3
**PR #16** `ci(release): generate provenance in an isolated reusable workflow (SLSA L3)`
— branch `ci/slsa-l3-provenance` (commit `b6e00ea`) off master (post-#12).
https://github.com/meigma/template-mcp/pull/16

New `.github/workflows/attest.yml` (reusable, workflow_call): actions/attest (binary
checksums) + attest-build-provenance (image). release.yml: binary-release-assets
uploads checksums artifact (drops in-job attest + id-token/attestations perms); new
attest-binaries + attest-image caller jobs; container-image-release keeps cosign +
syft SBOM attest in-job, drops in-job provenance. Signer ripple → attest.yml:
ghd.toml, stage_ghd_release_assets.py expected_signer (+ test), inspection-summary
gh-attestation-verify signer-workflow (cosign cert-identity stays release.yml).

The 3 tag-only-path fixes baked in from the start: shared attest job declares
packages:write so attest-binaries caller grants it too; mkdir -p sbom (PR2);
attest.yml has its own docker/login-action.

Verified: actionlint clean (release.yml + attest.yml); stage_ghd test 6/6 green
(now asserts attest.yml signer); root:check green (12 tasks). attest.yml only fires
on a real tag → exercised by the rehearsal next.

GOTCHA repeat: golangci-lint cache pollution from the removed PR2 worktree again;
`golangci-lint cache clean` before root:check. (Local-only; CI fresh.)

Next: PR #16 CI + dispatched release-dry-run, then merge. Then the throwaway-tag
rehearsal (the user-approved fork) to exercise publish→cosign→L3-attest end-to-end.
