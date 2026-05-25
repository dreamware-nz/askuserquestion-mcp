# Changelog

All notable changes to this project will be documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## v0.1.2 - 2026-05-26

### Added

- Optional `context` field on the `AskUserQuestion` request, plumbed
  through to the browser picker. When the agent supplies it, the HTML
  form renders it as a subtle muted banner above the questions so the
  user can see what work the questions belong to (project name, repo
  path, current task, whatever the agent finds useful). Opaque to the
  server -- not validated, never appears in the canonical answer
  string. Bumps the schema dependency to
  [`askuserquestion-go v0.1.2`](https://github.com/dreamware-nz/askuserquestion-go/releases/tag/v0.1.2).

## v0.1.0 - 2026-05-19

### Added

- Initial stdio MCP server exposing the `AskUserQuestion` tool, backed
  by [`dreamware-nz/askuserquestion-go`](https://github.com/dreamware-nz/askuserquestion-go)
  for schema, validation, and canonical answer formatting.
- Browser-based Resolver: ephemeral loopback HTTP server with an
  embedded zero-dependency HTML form. Auto-opens the user's default
  browser; `--no-open` falls back to printing the URL on stderr.
- CLI flags `--host` (bind address) and `--no-open` (suppress browser
  launch).
- Cancellation handling: `ErrResolverCancelled` surfaces as a non-error
  tool result `[cancelled by user]` so the conversation stays valid.
- CI workflow (`build`, `vet`, `test -race`) on push and PR.
- Release workflow producing cross-platform binaries for
  darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64,
  plus `SHA256SUMS`.
