# civic-os-knowledge

MCP server and tooling for the Civic OS corporate knowledgebase. Serves an [OKF-compatible](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md) bundle of markdown concept files to Claude via a unified Streamable HTTP endpoint with OAuth 2.1 authentication.

## What This Repo Contains

- `cmd/` / `internal/` — MCP server source (Go)
- `scripts/` — Seed and export utilities
- `templates/` — Example concept files showing the OKF format
- `docs/` — Research, architecture decisions, and design documentation

## What This Repo Does NOT Contain

The live knowledge bundle (OKF concept files) lives at runtime in a container volume, not in Git. See `docs/ARCHITECTURE.md` for the deployment model.

## Documentation

- [OKF Research](docs/OKF_RESEARCH.md) — Full research on format, alternatives, and design rationale
- [Architecture](docs/ARCHITECTURE.md) — Service architecture decisions
- [CLAUDE.md](CLAUDE.md) — AI assistant context for working in this repo

## Status

**Early development.** Architecture decisions are resolved; implementation is next. See `docs/ARCHITECTURE.md`.

## License

Copyright (C) 2026 Civic OS, L3C. Licensed under the [GNU Affero General Public License v3.0](LICENSE).
