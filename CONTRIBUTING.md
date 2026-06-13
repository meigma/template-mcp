# Contributing

Thank you for your interest in contributing.
This repository is a Go MCP server template, so changes should keep the generated-project path simple and predictable.
For private vulnerability reporting, use [SECURITY.md](SECURITY.md) instead of public channels.

## Reporting Bugs

Report non-security bugs through GitHub issues.
Include the following details when possible:

- version, commit, or environment details
- steps to reproduce
- expected behavior
- actual behavior
- logs, screenshots, or a minimal reproduction

If you are reporting a security issue, stop and follow [SECURITY.md](SECURITY.md) instead.

## Pull Requests

Contributors should:

1. Keep changes focused and scoped to a single problem.
2. Add or update tests when behavior changes.
3. Update documentation when user-facing behavior changes.
4. Use Conventional Commit subjects, such as `feat: add config loader` or `fix: handle empty input`.
5. Make sure `moon run root:check` passes before requesting review.

## Local Setup

Install the toolchain with [proto](https://moonrepo.dev/proto) and
[Moon](https://moonrepo.dev/moon) (proto is what Moon uses to provision the
pinned Go toolchain):

```sh
curl -fsSL https://moonrepo.dev/install/proto.sh | bash   # install proto
proto install moon                                        # install moon
proto install                                             # provision Go, golangci-lint, moon
```

Then run the full check. Note it also builds the docs, so it needs Python and uv
(provisioned by proto):

```sh
moon run root:check
```

Useful project commands:

```sh
moon run root:format       # check formatting
moon run root:format-fix   # apply formatting
moon run root:lint
moon run root:build
moon run root:test
moon run docs:serve        # preview the docs at http://127.0.0.1:8000
go run ./cmd/template-mcp --version
```

A few environment notes:

- macOS has no `timeout`/`gtimeout` by default; install coreutils or use a
  different mechanism when scripting time-bounded runs.
- The `stdio` subcommand is a server: it blocks until the client closes its
  input stream or the process is signaled. That is expected, not a hang.

## Release Changes

Release Please reads Conventional Commit subjects to build changelogs and release PRs.
Keep release-impacting commits clear; routine docs, CI, and maintenance commits should use the appropriate non-release type.
