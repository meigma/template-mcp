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
