# Registry Format

A **tap** is a git repository (or local directory) containing an `index.json` at its root and one or more package manifests. mcper ships with the `official` tap pointing at `https://github.com/sarjann/mcp-registry.git`.

## Repository layout

```
index.json
packages/
  vercel-mcp/
    1.0.0/
      manifest.json
    1.1.0/
      manifest.json
  other-pkg/
    0.2.0/
      manifest.json
```

Manifest paths are relative to the repository root and declared in `index.json`.

## index.json

The index lists every package and version available in the tap.

```json
{
  "schema_version": 1,
  "generated_at": "2026-02-14T00:00:00Z",
  "packages": {
    "vercel-mcp": {
      "description": "Vercel MCP server for deployment management",
      "versions": {
        "1.0.0": {
          "manifest": "packages/vercel-mcp/1.0.0/manifest.json",
          "sha256": "a1b2c3..."
        },
        "1.1.0": {
          "manifest": "packages/vercel-mcp/1.1.0/manifest.json",
          "sha256": "d4e5f6..."
        }
      }
    }
  }
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `schema_version` | yes | Always `1`. |
| `generated_at` | no | ISO 8601 timestamp of last generation. |
| `packages` | yes | Map of package name to package entry. |
| `packages.<name>.description` | no | Short description shown in `mcper search`. |
| `packages.<name>.versions` | yes | Map of semver string to version entry. |
| `packages.<name>.versions.<ver>.manifest` | yes | Relative path to the manifest file. |
| `packages.<name>.versions.<ver>.sha256` | no | SHA-256 hex digest of the manifest file. If present, mcper verifies it on install. |

## Package manifest

Each version has a standalone JSON manifest that fully describes what to install.

```json
{
  "schema_version": 1,
  "name": "vercel-mcp",
  "version": "1.0.0",
  "description": "Vercel MCP server for deployment management",
  "mcp_servers": {
    "vercel": {
      "transport": "stdio",
      "command": "npx",
      "args": ["-y", "@vercel/mcp"],
      "env_required": ["VERCEL_TOKEN"]
    }
  },
  "setup_commands": {
    "VERCEL_TOKEN": {
      "run": ["npx", "@vercel/mcp", "auth"],
      "pattern": "token: (.+)",
      "description": "Authenticate with Vercel to obtain an API token"
    }
  },
  "compatibility": {
    "os": ["darwin", "linux"],
    "arch": ["amd64", "arm64"]
  }
}
```

### Top-level fields

| Field | Required | Description |
|-------|----------|-------------|
| `schema_version` | yes | Always `1`. |
| `name` | yes | Package name. Must be non-empty. |
| `version` | yes | Semver version string. |
| `description` | no | Human-readable description. |
| `mcp_servers` | yes | Map of server name to server spec. At least one entry required. |
| `setup_commands` | no | Map of env var name to setup command. See [Setup commands](#setup-commands). |
| `compatibility` | no | Platform constraints. See [Compatibility](#compatibility). |

### Server spec (`mcp_servers.<name>`)

Each entry defines one MCP server that gets written to the client's config.

| Field | Required | Description |
|-------|----------|-------------|
| `transport` | yes | `"stdio"` or `"http"`. |
| `command` | stdio only | Binary to execute (e.g., `"npx"`). |
| `args` | no | Arguments passed to the command. |
| `url` | http only | Endpoint URL for HTTP transport. |
| `env_required` | no | List of environment variable names the server needs at runtime. Used by `mcper doctor` to check for missing secrets. |

### Setup commands

Setup commands run interactively after a successful install to help users obtain API tokens or other secrets. Each key is the environment variable name that the command produces.

| Field | Required | Description |
|-------|----------|-------------|
| `run` | yes | Command as an argv array (no shell). First element must be the binary name. |
| `pattern` | no | Regex with one capture group to extract the value from stdout. If omitted, the full trimmed stdout is used. |
| `description` | no | Shown to the user before prompting. Explain what the command does. |

**Install-time flow:**

1. If the secret already exists in the keyring, the command is skipped.
2. The user is prompted: `Run "npx @vercel/mcp auth" to obtain VERCEL_TOKEN? [yes/skip]`
3. On success, the extracted value is shown masked: `Extracted value: vrc...k9f2`
4. The user chooses: `Store as VERCEL_TOKEN? [yes/edit/skip]`
   - **yes** stores the extracted value as-is.
   - **edit** lets the user type or paste a different value.
   - **skip** moves on without storing.
5. All failures are non-fatal — the install already succeeded. A fallback command is shown: `mcper secret set <pkg> <ENV_VAR>`.

In non-interactive mode (piped stdin), all setup commands are silently skipped.

### Compatibility

Optional platform constraints. Currently informational — mcper does not enforce them.

| Field | Description |
|-------|-------------|
| `os` | Allowed operating systems (`"darwin"`, `"linux"`, `"windows"`). |
| `arch` | Allowed architectures (`"amd64"`, `"arm64"`). |
| `codex_min` | Minimum Codex CLI version. |
| `claude_min` | Minimum Claude Code version. |

## Versioning

mcper uses [semver](https://semver.org/) for all version resolution.

- `mcper install vercel-mcp` installs the latest version.
- `mcper install vercel-mcp@1.0.0` pins to an exact version.
- `mcper upgrade` resolves the highest version within the same major (e.g., `1.x.x`).
- `mcper upgrade --major` allows crossing major version boundaries.

Constraint expressions follow the [Masterminds/semver](https://github.com/Masterminds/semver) syntax: `>=1.2.0`, `>=1.0.0, <2.0.0`, etc.

## Integrity

When the index entry includes a `sha256` field, mcper computes the SHA-256 of the manifest file after download and rejects it on mismatch. This guards against tampering between index generation and install time.

Taps use hash-based trust by default. Signature-based verification is not currently supported.

## Taps

A tap is any git repository or local directory that contains a valid `index.json`.

```bash
# List configured taps
mcper tap list

# Add a custom tap (git URL)
mcper tap add my-team https://github.com/my-team/mcp-registry.git

# Add a local tap (directory path)
mcper tap add local /path/to/registry

# Remove a tap (cannot remove the default "official" tap)
mcper tap remove my-team

# Install from a specific tap
mcper install vercel-mcp --tap my-team
```

mcper clones git taps to a local cache (`~/.cache/mcper/taps/<name>`) and does a `git pull --ff-only` on subsequent syncs. Local/file taps are read directly.

## Direct URL installs

Manifests can also be installed from a URL or file path without a tap:

```bash
mcper install-url https://example.com/my-mcp/manifest.json
mcper install-url ./local-manifest.json
```

Direct installs require explicit trust approval on first use (or `--yes` to skip the prompt). The trust decision is persisted per URL.

## Detected AI clients

When installing, mcper auto-detects which AI clients are present and writes server configs to all of them. Use `--target` to limit to specific clients:

```bash
# Install to all detected clients (default)
mcper install vercel-mcp

# Install to specific clients only
mcper install vercel-mcp --target claude,cursor
```

Supported clients:

| Target | Client | Config file | Server key |
|--------|--------|-------------|------------|
| `claude` | Claude Code | `~/.claude/settings.local.json` | `mcpServers` |
| `claude-desktop` | Claude Desktop | `claude_desktop_config.json` | `mcpServers` |
| `codex` | Codex CLI | `~/.codex/config.toml` | `[mcp_servers]` |
| `cursor` | Cursor | `~/.cursor/mcp.json` | `mcpServers` |
| `vscode` | VS Code | `mcp.json` | `servers` |
| `gemini` | Gemini CLI | `~/.gemini/settings.json` | `mcpServers` |
| `zed` | Zed | `settings.json` | `context_servers` |
| `opencode` | OpenCode | `opencode.json` | `mcp` |

## Validation rules

mcper validates every manifest on load. A manifest is rejected if:

- `name` or `version` is empty.
- `mcp_servers` is empty.
- A server name is empty.
- A `stdio` server is missing `command`.
- An `http` server is missing `url`.
- A server has an unsupported transport (not `stdio` or `http`).
- A `setup_commands` entry has an empty `run`.
- A `setup_commands` entry has a `pattern` that doesn't compile as a valid regex.

## Full example

**index.json:**

```json
{
  "schema_version": 1,
  "packages": {
    "vercel-mcp": {
      "description": "Vercel deployment management",
      "versions": {
        "1.0.0": {
          "manifest": "packages/vercel-mcp/1.0.0/manifest.json",
          "sha256": "a1b2c3d4e5f6..."
        }
      }
    }
  }
}
```

**packages/vercel-mcp/1.0.0/manifest.json:**

```json
{
  "schema_version": 1,
  "name": "vercel-mcp",
  "version": "1.0.0",
  "description": "Vercel deployment management",
  "mcp_servers": {
    "vercel": {
      "transport": "stdio",
      "command": "npx",
      "args": ["-y", "@vercel/mcp"],
      "env_required": ["VERCEL_TOKEN"]
    }
  },
  "setup_commands": {
    "VERCEL_TOKEN": {
      "run": ["npx", "@vercel/mcp", "auth"],
      "pattern": "token: (.+)",
      "description": "Authenticate with Vercel to obtain an API token"
    }
  }
}
```

**HTTP-only server (no setup):**

```json
{
  "schema_version": 1,
  "name": "my-http-mcp",
  "version": "0.1.0",
  "mcp_servers": {
    "my-server": {
      "transport": "http",
      "url": "https://mcp.example.com/v1"
    }
  }
}
```
