# Technical Notes

- Use hexagonal architecture at all times. Keep business logic isolated from CLI, filesystem, network, storage, and other external adapters.
- Prefer functional testing before calling any feature complete. Unit tests are useful, but they do not prove the tool works the way the design intends.
- Take an agile approach to development. Avoid waterfall: underspecify when useful, prototype early, learn from the result, and refine from working behavior.

## Tooling (as of session 001)
- Toolchain is mise (`mise.toml` + committed `mise.lock`, fail-closed via `settings.locked`); moon runs tasks on the `system` toolchain (tools come from mise on PATH), it manages no language toolchain. Local dev: `mise install`, then `moon run root:check`; build the container image with `mise run image-local`.
- Container image is built by melange (signed Wolfi apk) + apko (`melange.yaml`/`apko.yaml`) — no Dockerfile. Release provenance is generated in the isolated reusable workflow `.github/workflows/attest.yml` (SLSA Build L3), so `gh attestation verify` uses `--signer-workflow .../attest.yml`; the keyless cosign image signature is issued by `release.yml`.
- `release.yml` only runs off a release-please draft (`resolve-release` waits for one) — a bare tag will not trigger it; rehearse by forcing a real version through release-please, and check for pre-existing tags/releases first.
- Gotchas: `mise lock` drops moon's `macos-x64` entry (hand-add it); under moon's `system` toolchain use the space form `go test -coverprofile coverage.out` (the `=` form writes a file named `=coverage.out`); after `wt remove`, run `golangci-lint cache clean` before linting in a sibling worktree.
