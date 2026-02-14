# mcper

`mcper` is a Homebrew-style MCP package manager for Codex and Claude Code.

It manages install, upgrade, removal, integrity checks, and local config wiring for MCP server definitions.

## Features (v1)

- CLI-first package lifecycle (`install`, `upgrade`, `remove`, `list`)
- Registry model with default tap plus custom taps (`tap add/remove/list`)
- Direct URL installs with explicit trust approval (`install-url`)
- Codex + Claude Code local config integration
- Hash-pinned registry verification
- Keychain-backed secrets (`secret set/unset`)
- Health checks (`doctor`) and export (`export --format lock|sbom`)

## Build

```bash
go build -o mcper ./cmd/mcper
```

## Quick Start

```bash
# show taps
./mcper tap list

# search/install from default official tap
./mcper search convex
./mcper install convex

# add a local tap (optional)
./mcper tap add local /path/to/registry

# inspect and health-check
./mcper list
./mcper doctor
```

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
