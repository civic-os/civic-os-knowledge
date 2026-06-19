# Service Architecture

> Status: **Decisions resolved** — all architectural choices finalized June 19, 2026.

## Overview

The civic-os-knowledge service consists of three components:

1. **MCP Server** — Streamable HTTP endpoint serving OKF concepts to all Claude surfaces
2. **OKF Bundle** — Markdown files with YAML frontmatter, cached locally with S3 as source of truth
3. **viz.html** — Static HTML knowledge graph viewer, regenerated from the bundle

## Component Diagram

```
                  ┌──────────────────────────┐
                  │     All Claude Surfaces   │
                  │  (Code, Desktop, Web,     │
                  │   Cowork, Mobile)         │
                  └────────────┬──────────────┘
                               │ Streamable HTTP
                               │ OAuth 2.1 (Keycloak)
                  ┌────────────▼──────────────┐
                  │    DO K8s Cluster Ingress  │
                  │    kb.civic-os.org         │
                  ├───────────┬───────────────┤
                  │  /mcp     │  /            │
                  │  ↓        │  ↓            │
                  │  MCP      │  viz.html     │
                  │  Server   │  (static)     │
                  └─────┬─────┴───────────────┘
                        │ reads/writes
                  ┌─────▼─────────────────────┐
                  │  /data/knowledge/ (ephemeral)
                  │  ├── bundle/    (OKF files)│
                  │  ├── .versions/ (snapshots)│
                  │  └── viz.html  (generated) │
                  └─────┬─────────────────────┘
                        │ sync on every write
                        │ pull on startup
                  ┌─────▼─────┐
                  │ DO Spaces  │
                  │ (S3 SoT)  │
                  └────────────┘
```

## Architectural Decisions

### 1. Hosting: DigitalOcean Kubernetes Cluster

The MCP server runs as a single-replica Deployment in the existing DO K8s cluster (same cluster hosting NEH and ICGF). The central-os VPS is mentally deprecated and not suitable for new services.

Single replica is sufficient: this is a single-writer knowledgebase for one user (accessed via Claude surfaces). No concurrent-write coordination needed. K8s handles restarts (~5s) if the pod crashes. Brief downtime during rolling updates is acceptable.

### 2. Storage: S3 as Source of Truth, Local Disk as Cache

DO Spaces (S3-compatible) is the persistent store. The pod's local filesystem is an ephemeral cache. No PVC needed.

**Lifecycle:**
1. Pod starts → pulls full bundle from S3 to `/data/knowledge/`
2. Reads served from local disk (fast, no S3 latency)
3. Writes go to local disk first, then sync to S3 (see below)
4. Pod dies → no data loss, new pod pulls from S3 on startup
5. In-memory search index and viz.html rebuild from local disk after pull

### 3. S3 Sync: On Every Write

Every `kb_create` and `kb_update` call writes to local disk, then uploads the changed file(s) to S3. ~100-200ms added per write is imperceptible — writes are infrequent (Claude updating knowledge during conversations).

S3 upload failures are logged as warnings but do not fail the MCP tool call. The local write succeeds regardless; S3 consistency is retried on the next write or can be recovered by a full sync.

### 4. viz.html Regeneration: On Every Write

Regenerated after every `kb_create` and `kb_update`. At ~50 concepts, regeneration takes 1-5s. Writes are infrequent, so always-current beats the operational overhead of a cron job.

### 5. Search: In-Memory Index

~50 concepts × ~2KB average = ~100KB of text. Trivially fits in memory. Built on startup after S3 pull, rebuilt incrementally after writes.

Implementation: array of `{path, frontmatter, bodyText}` objects. Filter by `type`/`tag` from frontmatter, case-insensitive substring match on body text for queries. Upgrade path to SQLite FTS5 if the bundle grows past ~200 concepts.

### 6. Implementation Language: Go

Single static binary, ~10-20MB container image (distroless), no runtime dependencies. The MCP Go SDK (Tier 2, v1.6.1, co-maintained by Google) supports Streamable HTTP with stateless mode. Goroutines naturally handle the MCP server, HTTP server, and S3 sync concurrently.

**Key libraries:**
- `github.com/modelcontextprotocol/go-sdk/mcp` — MCP server + Streamable HTTP transport
- `github.com/coreos/go-oidc/v3` — Keycloak JWKS validation + OIDC browser flow for viz.html
- `github.com/aws/aws-sdk-go-v2` — S3-compatible (DO Spaces) sync
- `github.com/yuin/goldmark` + frontmatter extension — markdown/YAML parsing

OAuth 2.1 Bearer token validation for the MCP endpoint is ~50 lines of middleware using `go-oidc` (verify JWT signature against Keycloak JWKS, check audience/issuer/expiry). The Go SDK provides Protected Resource Metadata structs but not validation middleware — acceptable tradeoff for the benefits of a compiled, dependency-free binary.

**Why not TypeScript?** TypeScript (Tier 1 SDK) has the best built-in auth support and would be faster to prototype. But for a containerized service intended to run unattended: larger images (~150MB Node alpine), `node_modules` supply chain surface, runtime type coercion, and Node.js version churn make it less suitable. TypeScript would be the better choice if stdio transport were needed (runs anywhere Node does), but this service is Streamable HTTP only.

## Other Settled Decisions

### Transport: Streamable HTTP Only

All Claude surfaces connect via HTTPS. No stdio/HTTP split. Rationale: one transport = one auth model = one write path. ~50-100ms network latency is imperceptible.

### Auth: OAuth 2.1 via Existing Keycloak

MCP server validates JWTs against `auth.civic-os.org`. Create a `civic-os-kb` client in the Keycloak realm. Same pattern as all other Civic OS services.

### Backing Store: OKF Files (Not Git, Not Database, Not Wiki)

Markdown files with YAML frontmatter on the filesystem. See `docs/OKF_RESEARCH.md` for the full analysis of alternatives.

### Versioning: Copy-on-Write File Snapshots

On every `kb_update`, copy the current file to `.versions/{path}/{ISO-timestamp}.md` before overwriting. No Git, no database — just files.

### Bundle Location: Ephemeral Pod Storage, Not Git

The live concept files live in the pod's local filesystem at runtime, pulled from S3 on startup. The Git repo contains service code, documentation, and templates — not the knowledge content.

## Security Considerations

- MCP endpoint requires valid Keycloak JWT (OAuth 2.1 Bearer token)
- viz.html served by authenticated HTTP server with role-based access (see below)
- DO Spaces bucket must not be publicly readable (contains business knowledge)
- Keycloak client should use confidential client type with PKCE
- No PII in concept files (client profiles reference contacts, don't store them inline)
- K8s NetworkPolicy should restrict pod egress to S3 and Keycloak endpoints

### viz.html: Authenticated HTTP Server with Role-Based Access

A lightweight HTTP server (same Go process as the MCP server) serves viz.html and handles Keycloak authentication. This is separate from the MCP transport — the MCP server handles Claude surfaces via OAuth 2.1, while the HTTP server handles human users via browser-based OIDC.

**Auth flow:**
1. User visits `kb.civic-os.org` → redirected to Keycloak login
2. Keycloak issues JWT with realm roles → redirect back with token
3. HTTP server validates JWT and checks roles before serving viz.html
4. Session maintained via httpOnly cookie (standard OIDC code flow)

**Role configuration** via environment variables:

```
KB_VIZ_ROLES=kb-viewer,kb-admin    # comma-separated Keycloak realm roles
KB_KEYCLOAK_URL=https://auth.civic-os.org
KB_KEYCLOAK_REALM=central-os
KB_KEYCLOAK_CLIENT_ID=civic-os-kb
```

Any user with at least one of the roles listed in `KB_VIZ_ROLES` can access viz.html. Roles are managed in Keycloak — no application-level user management.

**Endpoints:**

| Path | Auth | Purpose |
|------|------|---------|
| `/` | Keycloak OIDC | Serves viz.html (role-gated) |
| `/auth/login` | — | Initiates OIDC code flow |
| `/auth/callback` | — | Handles Keycloak redirect |
| `/auth/logout` | — | Clears session, redirects to Keycloak logout |
| `/mcp` | OAuth 2.1 Bearer | MCP server (Claude surfaces) |
| `/health` | None | K8s liveness/readiness probe |
