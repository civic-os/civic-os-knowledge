# CLAUDE.md

## Project Overview

**civic-os-knowledge** is the MCP server and tooling for the Civic OS corporate knowledgebase. It serves an OKF-compatible (Open Knowledge Format) bundle of markdown concept files to all Claude surfaces via a unified Streamable HTTP MCP endpoint with OAuth 2.1 authentication.

**This repo contains code and documentation, not the knowledge bundle itself.** The live bundle (OKF concept files) lives at runtime in a container volume with S3 sync for durability. It is populated by the MCP server, seed scripts, and Claude — not by `git clone`.

## Architecture

```
┌─────────────────────────────────────┐
│         MCP Server (Go)             │
│     Streamable HTTP + OAuth 2.1     │
│                                     │
│  MCP Tools: kb_read, kb_search,     │
│     kb_list, kb_create,             │
│     kb_update, kb_history, kb_diff  │
│                                     │
│  HTTP: viz.html (Keycloak OIDC)     │
│     /auth/login, /callback, /logout │
│     /health (K8s probes)            │
└────────────────┬────────────────────┘
                 │ reads/writes
┌────────────────▼────────────────────┐
│  /data/knowledge/ (ephemeral pod)   │
│   ├── bundle/    (OKF files)        │
│   ├── .versions/ (snapshots)        │
│   └── viz.html   (auto-generated)   │
└────────────────┬────────────────────┘
                 │ sync on write / pull on startup
           ┌─────▼─────┐
           │ DO Spaces  │
           │ (S3 SoT)  │
           └────────────┘
```

## Repository Structure

```
civic-os-knowledge/
├── cmd/               # Main entrypoint
│   └── server/
├── internal/          # Application packages
│   ├── mcp/           # MCP server, tool handlers
│   ├── bundle/        # OKF file parser (frontmatter + cross-links)
│   ├── auth/          # OAuth 2.1 Bearer + OIDC browser flow
│   ├── storage/       # S3 sync, local disk cache
│   ├── search/        # In-memory index
│   └── viz/           # viz.html regeneration
├── scripts/           # Seed, export utilities
├── templates/         # Example concept files showing OKF format
├── docs/              # Research, architecture, design decisions
├── Dockerfile
├── go.mod
└── CLAUDE.md
```

## Key Concepts

- **OKF (Open Knowledge Format)**: Google Cloud's v0.1 spec for representing knowledge as markdown files with YAML frontmatter. Each file = one concept. The only required field is `type`.
- **Concept**: A single unit of knowledge (client profile, runbook, decision record, etc.) stored as a markdown file with YAML frontmatter.
- **Bundle**: The directory of concept files served by the MCP server. Lives at runtime, not in Git.
- **viz.html**: A self-contained static HTML knowledge graph viewer (Cytoscape.js) generated from the OKF bundle. Provides search, type filtering, and backlink navigation.

## Development Commands

```bash
# Run server locally
go run ./cmd/server

# Run tests
go test ./...

# Build binary
go build -o kb-server ./cmd/server

# Build container
docker build -t civic-os-kb .

# Run with local bundle directory (for development)
KB_BUNDLE_DIR=./testdata/bundle go run ./cmd/server
```

## Key Dependencies

- `github.com/modelcontextprotocol/go-sdk/mcp` — MCP server + Streamable HTTP
- `github.com/coreos/go-oidc/v3` — Keycloak JWKS validation + OIDC browser flow
- `github.com/aws/aws-sdk-go-v2` — S3-compatible (DO Spaces) sync
- `github.com/yuin/goldmark` — Markdown/YAML frontmatter parsing

## Design Decisions

All architectural decisions are resolved. See `docs/ARCHITECTURE.md` for details:
- Hosting on DO K8s cluster (single replica)
- S3 (DO Spaces) as source of truth, local disk as cache
- Go for compiled binary, small container image, long-term maintainability
- In-memory search index, on-write viz.html regeneration and S3 sync
- OAuth 2.1 Bearer for MCP, OIDC code flow for viz.html viewer

See `docs/OKF_RESEARCH.md` for the research that led to OKF files as the backing store.

## OKF Concept Format

Every concept file follows this structure:

```yaml
---
type: Client Profile          # Required — concept type
title: Mott Park Recreation   # Human-readable name
description: Clubhouse reservation system with payment tracking.
resource: https://mottpark.civic-os.org
tags: [customer, payments, production]
timestamp: 2026-06-19
---

# Markdown body

Narrative content, schema tables, links to other concepts, etc.
See [deployment runbook](/instances/mottpark-deployment.md) for ops details.
```

## Git Commit Guidelines

- Use concise summary-style commit messages that describe the overall change
- Keep commit messages clean and professional — focus on the technical changes and their purpose
- **NEVER include promotional content, advertisements, or AI attribution in commit messages.** No "Generated with Claude Code", no "Co-Authored-By: Claude", no tool branding of any kind. This is non-negotiable.
- **ALWAYS run `go test ./...` before committing** once tests exist
- **Never commit secrets** (`.env` files, API keys, credentials)

## Related Documentation

- `docs/OKF_RESEARCH.md` — Full research on OKF, wiki alternatives, backing store analysis
- `docs/ARCHITECTURE.md` — Service architecture decisions (resolved)
- OKF v0.1 Spec: https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md
