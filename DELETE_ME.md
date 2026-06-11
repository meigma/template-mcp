# Welcome to the Meigma MCP Server Template

This repository was generated from `template-mcp`, the standard starter for Meigma [Model Context Protocol](https://modelcontextprotocol.io) servers.
It gives a new MCP server a working baseline on day one: a transport-agnostic server built on the official `modelcontextprotocol/go-sdk`, two ready-to-use transports (STDIO and Streamable HTTP), a single demo tool, Moon task orchestration, pinned CI, dependency automation, repository security defaults, and an enabled release pipeline that has already been exercised by the template application.

Delete this file after you finish the first-repository setup checklist below.
It is only here to orient the initial project owner.

## What This Template Provides

- A minimal Go module at `github.com/meigma/template-mcp`.
- A transport-agnostic MCP server in `internal/mcpserver` with one demo tool, `random_int`.
- A Cobra/Viper CLI under `cmd/template-mcp` and `internal/cli`, with two transport subcommands: `stdio` and `http`.
- Moon tasks for `format`, `lint`, `build`, `test`, and `check`.
- A hot-reloading dev loop: a checked-in `.mcp.json` wires Claude Code to the dev proxy in `tools/proxy`, which rebuilds the server on save and swaps it behind the live session.
- `golangci-lint` wired through Proto and Moon.
- CI that delegates to `moon ci --summary minimal` with pinned actions, dependency caches, and minimal token permissions.
- A scheduled container vulnerability scan that uploads SARIF results to GitHub code scanning.
- Dependabot coverage for GitHub Actions, Docker base images, Go modules, and the docs uv project.
- MkDocs Material docs scaffolding under `docs/`, with GitHub Pages as the default publishing target.
- Repository settings for signed commits, squash-only merges, immutable releases, private vulnerability reporting, and protected tags.
- Release workflows for Release Please, GoReleaser binary assets, GHCR container images, checksums, SBOMs, and GitHub artifact attestations.
- A root `ghd.toml` package manifest so released binaries can be installed with `ghd`.

## How It Works

The package layout keeps the server independent of any transport:

- `cmd/template-mcp` — thin entrypoint that wires signal handling into the CLI.
- `internal/cli` — builds the Cobra command tree. `root.go` registers the subcommands; `stdio.go` and `http.go` each own one transport.
- `internal/mcpserver` — constructs the MCP server and registers the `random_int` tool. It knows nothing about transports.
- `internal/templateinfo` — the single source of truth for the application name and title, and the derived `TEMPLATE_MCP_*` environment-variable prefix. Renaming the app to your project starts here. (Build metadata — version, commit, date — is separate: GoReleaser injects it via ldflags into `cmd/template-mcp/main.go`.)

Both subcommands call `mcpserver.New(...)` and differ only in how they connect it to a transport, so swapping or deleting a transport never touches the tool or server code.

Developing the server with Claude Code needs no setup: the checked-in `.mcp.json` builds the dev proxy (`tools/proxy`) through Moon's cached `proxy:build` task and launches it.
Start `claude` in the repository root, approve the project-scoped `dev` server, and edit the server source — changed tools appear on the next conversation turn with no reconnect.
See `tools/proxy/README.md` for how the proxy works.

Moon is the main entrypoint for local development and CI:

```sh
moon run root:check
```

That aggregate check runs the Go formatter/linter/build/tests plus the docs build.
The GitHub Actions CI workflow runs the same path through:

```sh
moon ci --summary minimal
```

The workflow caches Go modules, Go build artifacts, golangci-lint state, and uv's download cache through GitHub Actions. If that is not enough for a larger generated repository, add Moon remote caching later with Depot or another Bazel Remote Execution-compatible backend and repository credentials.

The `GitHub Pages` workflow builds the MkDocs site on pull requests and deploys the default-branch `docs/build` output to Pages. The repository settings manifest defaults Pages to workflow-based publishing with HTTPS enforcement.

The release machinery is intentionally enabled in the template repository so the starter app proves Release Please, GoReleaser binary releases, native-runner container image builds, artifact validation, and attestations before generated projects inherit the setup.
The nominal generated-project path is a server with both a downloadable binary and a container image. If the new project is binary-only, container-only, trim the release files as described below before the first release.

## First Setup Checklist

1. Rename the Go module:

   ```sh
   go mod edit -module github.com/meigma/YOUR_REPO
   ```

2. Rename the binary directory:

   ```sh
   mv cmd/template-mcp cmd/YOUR_BINARY
   ```

   The dev proxy's zero-config default builds `./cmd/template-mcp`, so update `defaultBuildCommand` in `tools/proxy/internal/cli/defaults.go` to the new path (or pass explicit `--build` and child arguments in `.mcp.json`).

3. Choose one transport.

   The template ships both the STDIO and Streamable HTTP transports so you can compare them. Most servers keep one:

   - **STDIO** for a server the client launches as a local subprocess.
   - **Streamable HTTP** for a remote or containerized server.

   To keep only one transport, delete the unused subcommand file and remove its single registration line in `internal/cli/root.go`:

   - Keeping STDIO: delete `internal/cli/http.go` and its registration in `root.go`.
   - Keeping HTTP: delete `internal/cli/stdio.go` and its registration in `root.go`.

   The `internal/mcpserver` server and the `random_int` tool do not change when you drop a transport.

4. Replace the demo tool.

   `random_int` in `internal/mcpserver` is a placeholder that exists to prove the end-to-end tool path. Replace it with your own tool (typed input/output structs plus a handler registered via the SDK), or add more tools alongside it; each tool lives in its own file (`randomint.go`) with a matching test file (`randomint_test.go`). The transport subcommands stay the same.

5. Replace template placeholders:

   ```sh
   rg "template-mcp|TEMPLATE_MCP|github.com/meigma/template-mcp"
   ```

   Update Go imports, Moon metadata, README text, docs text, the MCP server `Implementation` name/title, and CLI environment variable prefixes. For release-bearing projects, also update `.goreleaser.yaml`, `release-please-config.json`, `ghd.toml`, `Dockerfile`, and `.github/workflows/release*.yml` as applicable.
   Update `docs/mkdocs.yml` with the generated repository's GitHub Pages URL, usually `https://OWNER.github.io/REPO/`.

6. Refresh module metadata:

   ```sh
   go mod tidy
   ```

7. Configure releases for the chosen shape.

   For the nominal binary plus container case:

   - Update `.goreleaser.yaml`: `project_name`, build `id`, `main`, binary name, archive name template, and any linked package paths.
   - Update `ghd.toml`: `provenance.signer_workflow`, package name, description, asset patterns, and installed binary path.
   - Update `Dockerfile`: binary path, labels, default `SOURCE`, base-image tags/digests, and the default subcommand to match the transport you kept (containers usually run `http`).
   - Update `.github/workflows/release.yml`: `IMAGE_NAME`, binary validation names, container labels, summary commands, and verification examples.
   - Update `.github/workflows/release-dry-run.yml`: binary validation names, local container image name, and smoke-test commands.
   - Update `.github/workflows/security-scan.yml`: local container image name and scan category.
   - Update `.github/repository-settings.toml` only if required status-check names change.

   For binary-only projects:

   - Keep `.goreleaser.yaml`, `ghd.toml`, `Release Please`, `Binary Release Dry Run`, and the binary asset portions of `release.yml`.
   - Remove the `container-image-release` job, container verification summary text, and `Container Image Dry Run`.
   - Remove `Dockerfile`, `.dockerignore`, and `.github/workflows/security-scan.yml` if no container build remains.
   - Remove `Container Image Dry Run` from required branch checks.

   For container-only projects:

   - Keep `Release Please`, `Container Image Dry Run`, `container-image-release`, `Dockerfile`, and `.dockerignore`.
   - Remove `.goreleaser.yaml`, `ghd.toml`, `binary-release-assets`, binary verification summary text, and `Binary Release Dry Run`.
   - Change `container-image-release` so it depends only on `resolve-release`.
   - Remove `Binary Release Dry Run` from required branch checks.

   In every release-bearing project, configure the release app credentials, protected-tag bypass, and repository package permissions before the first release. Run the release dry-run workflow after these edits and before merging the first release PR.

8. Run the full local check:

   ```sh
   moon run root:check
   ```

9. Update project-facing docs:

   - Rewrite `README.md` for the actual server, including its real tools and the transport you kept.
   - Review `CONTRIBUTING.md` and `SECURITY.md`.
   - Add a real license before publishing the repository.

10. Delete this file:

    ```sh
    rm DELETE_ME.md
    ```
