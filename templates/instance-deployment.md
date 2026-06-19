---
type: Instance Deployment
title: Example Instance
description: Deployment runbook and current status for the example instance.
resource: https://client.civic-os.org
tags: [vps, production]
timestamp: 2026-01-01
---

# Current State

- **Version**: v0.X.0
- **Host**: VPS / Kubernetes namespace
- **Database**: Managed PostgreSQL (provider, region)
- **Auth**: Keycloak realm name
- **Storage**: S3 bucket name

# SQL Scripts

| # | File | Purpose |
|---|------|---------|
| 01 | schema.sql | Base schema from examples/... |
| 02 | permissions.sql | RBAC configuration |
| ... | ... | ... |

# Deployment Procedure

Steps to deploy a new version or apply patches.

# Recent Updates

- **YYYY-MM-DD**: Description of change
- **YYYY-MM-DD**: Description of change

# Keycloak Configuration

Realm name, client IDs, service account setup, identity providers.

# Backup & Recovery

Backup schedule, retention policy, restore procedure.
