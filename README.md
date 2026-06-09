# template-go

`template-go` is the reusable Go repository starter for Meigma projects.
It includes a small Go CLI skeleton, Moon tasks, pinned CI, Dependabot, baseline repository security settings, and an enabled Release Please plus GoReleaser release layer.

## Local Bootstrap

Prerequisites:

- Go 1.26.4
- Moon 2.x
- Python 3.14.3 and uv 0.11.0 for the MkDocs documentation project

After creating a new repository from this template, replace the placeholder names before doing feature work:

```sh
go mod edit -module github.com/meigma/YOUR_REPO
mv cmd/template-go cmd/YOUR_BINARY
```

Then update `template-go` references in the Moon tasks, GoReleaser config, `ghd.toml`, README, and package docs.

## Common Tasks

Moon is the standard task front door:

```sh
moon run root:format
moon run root:lint
moon run root:build
moon run root:test
moon run root:check
```

CI runs the same aggregate check:

```sh
moon ci --summary minimal
```

The starter CLI is intentionally small:

```sh
go run ./cmd/template-go --version
go run ./cmd/template-go --message "hello from cobra"
go test ./...
```

The CLI entrypoint uses Cobra and Viper in the same shape as other Meigma CLIs: `cmd/template-go` stays thin, `internal/cli` owns command construction, and Viper-backed flags can also be supplied through `TEMPLATE_GO_*` environment variables.

## Container Image

The included Dockerfile builds a static Linux binary and copies it into a non-root distroless runtime image:

```sh
docker build --target test .
docker build -t template-go:dev .
docker run --rm template-go:dev --version
```

The Dockerfile pins the builder and runtime images by digest and verifies that the selected Go builder image matches `.go-version`. When bumping Go, update `.go-version` and the builder `FROM` tag/digest together.

Release builds can pass the same binary metadata injected by GoReleaser:

```sh
docker build \
  --build-arg VERSION="$(git describe --tags --always --dirty)" \
  --build-arg COMMIT="$(git rev-parse HEAD)" \
  --build-arg DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t template-go:dev .
```

## CI and Security

The default CI workflow keeps permissions minimal, pins external actions, disables checkout credential persistence, and delegates checks to Moon.
It uses GitHub-hosted dependency caches for Go, golangci-lint, and uv download artifacts while leaving Moon remote caching as an optional follow-up for repositories that need a shared task-output cache.
The docs workflow builds the MkDocs site on pull requests and deploys `docs/build` to GitHub Pages from the default branch.
The scheduled security scan workflow builds the local container image weekly, scans it for high/critical fixed vulnerabilities, and uploads SARIF results to GitHub code scanning.
Dependabot covers GitHub Actions, Docker base images, the root Go module, and the docs uv project.

Repository settings live in `.github/repository-settings.toml`.
They default to immutable releases, private vulnerability reporting, signed commits, squash-only merges, GitHub Pages workflow publishing, and protected tags.

## Release Layer

Release automation is enabled for the template application so this repository proves the full binary and container release lifecycle before generated projects inherit it.
Repositories generated from the template should update the release app credentials, package names, asset patterns, container image name, and `ghd.toml` signer workflow before cutting their first release.

The release path is:

- Release Please opens and maintains the release PR.
- Release Please creates a draft GitHub release and tag after merge.
- Release Dry Run rehearses the GoReleaser binary path and native-runner Docker container build path on pull requests.
- GoReleaser builds binaries, checksums, and SBOMs without publishing directly.
- The release workflow uploads assets to the draft release and creates a GitHub-hosted attestation for `checksums.txt`.
- The release workflow builds amd64 and arm64 container images on native GitHub-hosted runners, publishes `ghcr.io/meigma/template-go:vX.Y.Z` as a multi-platform manifest, attaches BuildKit provenance and SBOM metadata, and creates a GitHub-native attestation for the manifest digest.
- A human inspects the draft release before publication.

The root `ghd.toml` matches the default GoReleaser output so generated projects can be installed with `ghd` once the release workflow runs.
After cloning this template, update `provenance.signer_workflow`, package names, asset patterns, binary paths, and image names to match the new repository and binary name.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines, local setup expectations, and pull request workflow.

## Security

See [SECURITY.md](SECURITY.md) for supported versions and the private vulnerability reporting path.

## License

Add the repository license before publishing a project generated from this template.
