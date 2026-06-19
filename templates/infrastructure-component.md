---
type: Infrastructure Component
title: Example Component
description: One-sentence summary of what this component does in the stack.
resource: https://cloud.digitalocean.com/droplets/12345
tags: [vps, production, caddy]
timestamp: 2026-01-01
---

# Role

What this component does in the Civic OS infrastructure stack.

# Current Configuration

| Property | Value |
|----------|-------|
| Provider | DigitalOcean / AWS / Cloudflare |
| Region | nyc1 / sfo3 |
| Size/Tier | s-1vcpu-1gb / $12/mo |
| Version | Caddy 2, PostgreSQL 17, etc. |
| Domain | component.civic-os.org |

# Services Hosted

| Service | Port | Purpose |
|---------|------|---------|
| ... | ... | ... |

# Access

- SSH: `ssh alias-name`
- Admin UI: https://...
- API: https://...

# Instances Served

Which Civic OS instances depend on this component.

- [Mott Park](/instances/mottpark-deployment.md)
- [FFSC](/instances/ffsc-deployment.md)

# Backup & Recovery

Backup schedule, retention policy, restore procedure.

# Cost

Monthly cost breakdown and what drives it.

# Maintenance

Recurring maintenance tasks (certificate renewal, version upgrades, log rotation).
