# OKF Research & Knowledgebase Architecture

> Research conducted June 19, 2026. Based on OKF v0.1 (published June 12, 2026).

## Table of Contents

- [What Is OKF](#what-is-okf)
- [OKF Specification Summary](#okf-specification-summary)
- [The Problem We're Solving](#the-problem-were-solving)
- [Knowledge Inventory](#knowledge-inventory)
- [Backing Store Analysis](#backing-store-analysis)
- [MCP Transport & Auth](#mcp-transport--auth)
- [Visualization Layer](#visualization-layer)
- [Architecture Decision](#architecture-decision)
- [Open Questions](#open-questions)
- [Effort Estimate](#effort-estimate)
- [Sources](#sources)

---

## What Is OKF

Google Cloud published the Open Knowledge Format (OKF) v0.1 on June 12, 2026 — a vendor-neutral markdown spec for packaging organizational knowledge so AI agents can consume it without re-interpreting scattered internal docs every session.

Key properties:
- **Markdown files with YAML frontmatter** in a directory
- **One required field**: `type` in frontmatter
- Concepts linked via standard markdown links
- Reserved files: `index.md` (table of contents), `log.md` (changelog)
- Designed to be loaded into LLM context directly — not RAG, not vector search
- Spec fits on one page: https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md

OKF is intentionally minimal. There is no registry, no SDK, no compression scheme. "Readable by humans without tooling, parseable by agents without bespoke SDKs, and diffable in version control."

## OKF Specification Summary

### Concept Document Structure

```yaml
---
type: BigQuery Table           # Required: concept kind (not centrally registered)
title: Orders                  # Recommended: human-readable name
description: One row per order # Recommended: one-sentence summary
resource: https://...          # Recommended: URI to underlying asset
tags: [sales, orders]          # Recommended: cross-cutting categories
timestamp: 2026-06-19T00:00Z  # Recommended: last modification (ISO 8601)
---

# Markdown body with optional sections

## Schema
| Column | Type | Description |
...

## Examples
...

## Citations
[1] [Source](https://...)
```

### Reserved Files

- **`index.md`**: No frontmatter. Lists available concepts for progressive disclosure. Can be auto-generated.
- **`log.md`**: Chronological change history. ISO 8601 date headings. Newest first.

### Cross-Linking

- Absolute (bundle-relative): `[customers](/tables/customers.md)` — recommended
- Relative: `[other](./other.md)`
- Broken links must be tolerated (may represent not-yet-written knowledge)

### Conformance (v0.1)

A bundle conforms if:
1. Every non-reserved `.md` file has parseable YAML frontmatter
2. Every frontmatter has non-empty `type`
3. Reserved filenames follow their specified structures

Unknown types, unknown keys, missing optional fields, and broken links are all explicitly tolerated.

### Ecosystem Tooling

| Tool | Type | What It Does |
|------|------|-------------|
| Enrichment Agent | CLI | Generates OKF bundles from BigQuery schemas + web crawling |
| viz.html | Static HTML | Force-directed concept graph with search, filtering, backlinks |
| kcmd | CLI + MCP Server | Bidirectional sync with Google Cloud Knowledge Catalog |

## The Problem We're Solving

Civic OS L3C operates 4 customer instances (3 pilots + 1 full customer) and an internal CRM. Knowledge about these clients is scattered across:

1. **Deployments Git repo** (`../deployments/`) — instance configs, SQL scripts, Keycloak realms, deployment runbooks
2. **OneDrive** (`OneDrive-CivicOSL3C/`) — business plans, proposals, contracts, research, presentations
3. **Central OS CRM database** — 5 clients, 4 projects, 46 time entries, contact logs
4. **Undocumented knowledge** — relationships, decisions, and context in the founder's head

Much of this knowledge doesn't map well onto tables (system configurations, ADRs, design documents, client relationship context). The CRM captures structured transactional data but not the narrative knowledge surrounding it.

### Claude Surfaces That Need Access

| Surface | Access Model | Current KB Access |
|---------|-------------|-------------------|
| Claude Code (civic-os-frontend) | Local filesystem + MCP | CLAUDE.md + docs/ |
| Claude Code (deployments) | Local filesystem + MCP | CLAUDE.md + instances/ |
| Claude Desktop / Cowork | Local MCP + Remote MCP | Nothing persistent |
| claude.ai web / mobile | Remote MCP only | Nothing persistent |

## Knowledge Inventory

### Deployments Repo

| Category | Count | Knowledge Type |
|----------|-------|---------------|
| Customer instances | 5 active (FFSC, Mott Park, NEH, ICGF, CFI) | Config, SQL, Keycloak realms |
| Central OS CRM | 6 entities, 62 records | Client lifecycle, time tracking |
| Deployment runbooks | DEPLOYMENT.md + UPDATES.md per instance | Operational procedures |
| Infrastructure | VPS templates, Caddy, deploy scripts | Infrastructure-as-code |
| Archived K8s | Full Kubernetes deployment (retired) | Historical reference |

### OneDrive

| Category | Files | Knowledge Type |
|----------|-------|---------------|
| Business plans | 3 (v13, v14, Theory of Change) | Strategy |
| Client proposals | 8 (DOCX + PDF pairs) | Sales |
| Contracts/agreements | 6 (templates + signed pilots) | Legal |
| NEH project docs | 26 (requirements, geodata, SOPs) | Requirements |
| Presentations | 24 PPTX + slide index | Stakeholder comms |
| Market research | 18 (nonprofit analysis, IRS data) | Market intelligence |

### Central OS CRM Database

| Entity | Records | Nature |
|--------|---------|--------|
| Clients | 5 | Lead → Active lifecycle |
| Projects | 4 | Per-client work tracking |
| Time Entries | 46 | Billable hours |
| Design Docs | 1 | Architecture specs |
| Contact Log | 3 | Interaction history |
| Scheduled Contacts | 3 | Upcoming touchpoints |

### Civic OS Framework (civic-os-frontend)

| Category | Count |
|----------|-------|
| Documentation files | 89 markdown |
| SQL migrations | 88 versioned triplets |
| Example projects | 9 complete systems |
| Frontend services | 32 |
| Property types | 20+ |
| RPC functions | 50+ |

## Backing Store Analysis

We evaluated four backing store options. The analysis evolved through several rounds as requirements crystallized.

### Round 1: Git as Backing Store

**Initial assumption**: OKF files in a Git repo, since they're markdown.

**Problems identified**:
- Remote writes (from claude.ai) require a PR workflow — 3-step async process to change one field
- Push/pull sync between local and remote adds operational burden
- Git optimizes for collaborative text editing with conflict resolution — overkill for a single-writer KB with 5 clients
- Versioning is valuable but not worth the write-path complexity

**Verdict**: Git's complexity tax buys insurance we may never cash in.

### Round 2: PostgreSQL as Backing Store

**Rationale**: We already run managed PostgreSQL, PostgREST, and Keycloak on central-os.

**Advantages**:
- Zero new infrastructure
- Symmetric writes from every Claude surface (just API calls)
- No sync needed — one source of truth
- SQL full-text search
- Audit trigger for version history
- Auth already solved (Keycloak → PostgREST JWT)

**Disadvantages**:
- Data isn't in OKF format natively — requires export script
- Loses "readable without tooling" property
- Can't grep concept files on a bare server

**Verdict**: Strong option. Minimum infrastructure, maximum write simplicity.

### Round 3: Outline Wiki as Backing Store

**Rationale**: Wiki gives you human-readable UI + built-in versioning + existing MCP server (30+ tools).

**Advantages**:
- Full web UI for browsing, searching, reading
- Automatic revision history with side-by-side diffs
- Keycloak OAuth is Outline's required auth model (alignment)
- Community MCP server supports stdio + SSE + HTTP
- PostgreSQL-backed (can share existing managed DB)

**Disadvantages**:
- Requires Redis + S3/MinIO (new infrastructure)
- Vendor lock-in — migration out is a project
- Structured metadata (YAML frontmatter) lost — body text only
- OKF compatibility only via export

**Verdict**: Strong if the UI is the primary value. But the UI was doing all the heavy lifting.

### Round 4: What If We Build a UI Regardless?

**Key insight**: If we assume a visualization layer exists for any backend (viz.html from OKF tooling, or a custom viewer), then the wiki's UI advantage — its load-bearing wall — is neutralized.

With UI as a constant, the comparison shifts to structural properties:

| Factor | OKF Files + viz.html | PostgreSQL | Outline |
|--------|---------------------|-----------|---------|
| Format alignment | **Native** — files ARE OKF | Export needed | Export needed |
| Structured metadata | **YAML frontmatter** | SQL columns | Weak (body text) |
| Portability | **Maximum** | Export needed | Export + vendor lock-in |
| No-tooling fallback | **cat/grep** | psql | Need Outline running |
| New infrastructure | Node process | Nothing | Redis + MinIO |
| Versioning | Build (~80 LOC) | Build (~30 LOC) | Free |
| MCP server | Build (~300 LOC) | Build (~200 LOC) | Existing |

**Verdict**: OKF files win on structural properties when UI is no longer a differentiator.

### Decision: OKF Files as Backing Store

**OKF markdown files with YAML frontmatter**, served by a custom MCP server, visualized by viz.html.

Rationale:
1. **Format alignment**: The source of truth, consumption format, visualization source, and export format are all the same thing. Zero translation layers.
2. **Structured metadata**: YAML frontmatter is directly parseable for filtering, search, and viz.html consumption.
3. **Portability**: If OKF takes off, we're natively compliant. If it doesn't, we have perfectly good markdown files.
4. **Simplicity**: Files on disk + a Node process + a static HTML page. No Redis, no MinIO, no wiki to maintain.
5. **Fallback**: SSH in and `cat`/`grep` works. Always.

The bundle lives at runtime in a container volume, NOT in Git. Git contains the service code, documentation, and templates.

## MCP Transport & Auth

### Unified Transport: Streamable HTTP

Rather than maintaining separate stdio (local) and HTTP (remote) transports, we use **Streamable HTTP for all Claude surfaces**. The MCP server runs as a service; all Claudes connect over HTTPS.

**Rationale**: Latency of HTTPS to VPS is ~50-100ms — imperceptible. One transport means one auth model, one write path, no local/remote divergence.

**Tradeoff**: No offline access. But if you're offline, you're not talking to Claude anyway.

### Auth: OAuth 2.1 via Keycloak

The MCP spec (draft, June 2025 revision) defines OAuth 2.1 as the standard auth mechanism for HTTP transport:

- OAuth 2.1 with PKCE (required)
- RFC 8414: Authorization Server Metadata (Keycloak's `/.well-known/openid-configuration`)
- RFC 9728: Protected Resource Metadata (MCP server serves `/.well-known/oauth-protected-resource`)
- RFC 7591: Dynamic Client Registration (Keycloak supports natively)
- RFC 8707: Resource Indicators (audience binding)

We already run Keycloak at `auth.civic-os.org`. Creating a `civic-os-kb` client is the same workflow as creating clients for Civic OS instances. The MCP server validates JWT tokens against Keycloak's JWKS endpoint.

### MCP Tools

| Tool | Purpose | Read/Write |
|------|---------|------------|
| `kb_read(path)` | Read a specific concept | Read |
| `kb_search(query, type?, tag?)` | Full-text search + frontmatter filtering | Read |
| `kb_list(type?, tag?)` | List concepts with optional filters | Read |
| `kb_create(path, type, content)` | Create a new concept file | Write |
| `kb_update(path, content)` | Update concept + create version snapshot | Write |
| `kb_history(path)` | List timestamped versions of a concept | Read |
| `kb_diff(path, v1, v2)` | Compare two versions of a concept | Read |

### Claude Surface Configuration

All surfaces use the same remote MCP connector:

| Surface | Configuration |
|---------|--------------|
| claude.ai web | Connectors → Add custom connector → MCP URL |
| Claude Desktop / Cowork | `claude_desktop_config.json` remote MCP entry |
| Claude Code (both repos) | `~/.claude/settings.json` MCP server entry |

### Writing Culture: Claude Project Prompt

On claude.ai, a Project called "Civic OS Operations" with the KB MCP connector attached should include instructions that:

1. **Read first**: Before answering questions about clients/ops, search the KB
2. **Write after**: After conversations that produce actionable knowledge, update the KB
3. **Specify collections**: Clients, Instances, Projects, Strategy, Decisions, Runbooks

This makes note-taking a default behavior across Claude surfaces, not a separate maintenance task. The KB grows as a side-effect of normal work.

## Visualization Layer

### viz.html (OKF Ecosystem Tool)

A self-contained static HTML file using Cytoscape.js that provides:

- **Force-directed concept graph** — nodes = concepts, edges = cross-links
- **Full-text search** — by title, concept ID, tags
- **Type-based filtering** — toggle concept types on/off
- **Backlink computation** — "what links to this concept?" (automatic)
- **Side panel** — click a node to see rendered markdown + frontmatter
- **Multiple layouts** — force-directed, hierarchical, circular
- **Zero runtime** — static HTML, serve from any web server

Regenerated automatically when concepts are written/updated. Served via Caddy (or any reverse proxy) as the human-facing read UI.

### Relationship to the MCP Server

viz.html and the MCP server both read the same OKF files. viz.html is the human interface; the MCP server is the agent interface. Neither needs to know about the other — they share a data format, not a runtime.

## Architecture Decision

### What Lives Where

```
Git repo (civic-os-knowledge/):     Code + docs + templates
├── server/                          MCP server source
├── scripts/                         Seed, export, viz generation
├── templates/                       Example concept files
├── docs/                            Research, architecture
├── Dockerfile
└── CLAUDE.md

Container volume (/data/knowledge/): Live data (NOT in Git)
├── bundle/                          OKF concept files
│   ├── index.md
│   ├── clients/
│   ├── instances/
│   └── ...
├── .versions/                       Concept snapshots
└── viz.html                         Auto-generated viewer
```

### Versioning Without Git

Copy-on-write file snapshots managed by the MCP server:

1. On `kb_update`: copy current file to `.versions/{path}/{ISO-timestamp}.md`
2. Write new content to the concept file (with updated `timestamp` in frontmatter)
3. Regenerate viz.html
4. `kb_history` lists version timestamps; `kb_diff` compares any two

Automatic cleanup: keep last N versions per concept (configurable).

### S3 Sync for Durability

The live bundle syncs to S3 (DigitalOcean Spaces) for backup/recovery. Mechanism TBD — options include periodic rsync, inotify-triggered upload, or application-level sync in the MCP server's write path.

## Open Questions

1. **Hosting location**: The MCP server doesn't have to run on central-os. Could be its own droplet, a container on any VPS, or even a Cloudflare Worker with R2 storage. Depends on latency, cost, and operational simplicity tradeoffs.

2. **Container strategy**: How do the OKF files live at runtime? Options:
   - Docker volume on VPS (simple, but tied to one host)
   - Container with mounted S3 via s3fs or goofys
   - Application-level S3 read/write (files in container, sync to S3)
   - R2 + Cloudflare Worker (serverless, globally distributed)

3. **S3 sync mechanism**: Push on every write? Periodic batch? Bidirectional (for disaster recovery)?

4. **viz.html regeneration**: On every write (adds latency) vs. periodic cron vs. on-demand endpoint?

5. **Seed data pipeline**: How do we initially populate client profiles from the existing deployments repo, OneDrive, and CRM? One-time scripts or ongoing sync?

6. **Search implementation**: In-memory index (fast, limits bundle size) vs. grep-based (simple, slower) vs. SQLite FTS sidecar (powerful, one more dependency)?

7. **Bundle size limits**: At what point does a single-file viz.html become unwieldy? Likely hundreds of concepts, not a concern for our ~50-concept bundle.

## Effort Estimate

| Phase | Work | Estimate |
|-------|------|----------|
| MCP server (7 tools, frontmatter parser, file I/O, versioning) | TypeScript | 6 hrs |
| OAuth 2.1 middleware (Keycloak JWKS validation) | TypeScript | 2 hrs |
| viz.html generation (OKF visualizer integration) | Script | 1 hr |
| Container + deployment (Dockerfile, compose, Caddy, DNS) | DevOps | 2 hrs |
| Seed content (5 clients, 5 instances, 3 runbooks) | Content | 4 hrs |
| Claude configuration (MCP connector, settings, Project prompt) | Config | 1 hr |
| **Total** | | **~16 hrs** |

## Sources

- [OKF v0.1 Specification](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md)
- [Google Cloud Blog — OKF Introduction](https://cloud.google.com/blog/products/data-analytics/how-the-open-knowledge-format-can-improve-data-sharing/)
- [OKF Ecosystem Tools](https://okf.md/tools/)
- [MCP Authorization Specification](https://modelcontextprotocol.io/specification/draft/basic/authorization)
- [MCP Stdio vs Streamable HTTP](https://www.truefoundry.com/blog/mcp-stdio-vs-streamable-http-enterprise)
- [Outline Wiki](https://www.getoutline.com/)
- [Outline MCP Server](https://mcpservers.org/servers/Vortiago/mcp-outline)
- [Outline Revision History](https://docs.getoutline.com/s/guide/doc/revision-history-AiL6p22Ssq)
- [KB MCP Server Comparison 2026](https://mcp.directory/blog/best-knowledge-base-mcp-servers-2026)
- [MCP OAuth 2.1 Explained](https://www.prefect.io/resources/mcp-oauth)
- [Claude Cowork Product Page](https://www.anthropic.com/product/claude-cowork)
- [Remote MCP Connectors](https://support.claude.com/en/articles/11503834-build-custom-connectors-via-remote-mcp-servers)
