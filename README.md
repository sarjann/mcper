# mcper

`mcper` is a Homebrew-style MCP package manager that auto-detects your AI clients and wires up MCP servers for you.

Supports Claude Code, Claude Desktop, Codex, Cursor, VS Code, Gemini CLI, Zed, and OpenCode.

## Install

```bash
go install github.com/sarjann/mcper/cmd/mcper@latest
```

Or build from source:

```bash
git clone https://github.com/sarjann/mcper.git
cd mcper
go build -o mcper ./cmd/mcper
```

## Quick Start

```bash
# search for packages
mcper search vercel

# install — auto-detects your AI clients
mcper install vercel-mcp

# install to specific clients only
mcper install vercel-mcp --target claude,cursor

# manage secrets
mcper secret set vercel-mcp VERCEL_TOKEN

# health check
mcper doctor
```

## Features

- Auto-detects installed AI clients and writes configs to all of them
- Post-install setup commands to obtain API tokens interactively
- Registry model with default tap plus custom taps (`tap add/remove/list`)
- Direct URL installs with explicit trust approval (`install-url`)
- Hash-pinned manifest verification
- Keychain-backed secrets (`secret set/unset`)
- Health checks (`doctor`) and export (`export --format lock|sbom`)

## Integrity Model

- Curated taps: verified using hash pins from each tap `index.json`.
- Direct URLs: blocked until explicit trust (`--yes` or interactive approval).

## Registry Layout

Tap root should include `index.json` and package manifests:

```text
index.json
packages/<name>/<version>/manifest.json
```

See `docs/registry.md` for schema details.
