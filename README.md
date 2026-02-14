# mcper

`mcper` is a Homebrew-style MCP package manager for Codex and Claude Code.

It manages install, upgrade, removal, trust, and local config wiring for MCP server definitions.

## Features (v1)

- CLI-first package lifecycle (`install`, `upgrade`, `remove`, `list`)
- Registry model with default tap plus custom taps (`tap add/remove/list`)
- Direct URL installs with explicit trust approval (`install-url`)
- Codex + Claude Code local config integration
- Tap trust policy (`cosign` or `hash`) with per-tap trust roots
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

# add a dev tap with hash mode (no signatures)
./mcper tap add local /path/to/registry --trust-mode hash

# search/install
./mcper search chroma
./mcper install chroma --tap local

# inspect and health-check
./mcper list
./mcper doctor
```

## Trust Model

- Curated taps: verified according to tap trust mode.
- `cosign` mode requires configured trusted identity (`--issuer` + `--subject`).
- Direct URLs: blocked until explicit trust (`--yes` or interactive approval).

## Registry Layout

Tap root should include `index.json` and package manifests:

```text
index.json
packages/<name>/<version>/manifest.json
```

See `docs/registry.md` for schema details.
