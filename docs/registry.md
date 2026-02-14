# Registry Format

`mcper` expects a tap to provide `index.json` at repository root.

## `index.json`

```json
{
  "schema_version": 1,
  "generated_at": "2026-02-14T00:00:00Z",
  "packages": {
    "example": {
      "description": "Example MCP package",
      "versions": {
        "1.0.0": {
          "manifest": "packages/example/1.0.0/manifest.json",
          "sha256": "<sha256-of-manifest>",
          "sig": "packages/example/1.0.0/manifest.sig",
          "cert": "packages/example/1.0.0/manifest.pem"
        }
      }
    }
  }
}
```

## Manifest format

```json
{
  "schema_version": 1,
  "name": "example",
  "version": "1.0.0",
  "description": "Example MCP",
  "mcp_servers": {
    "example": {
      "transport": "stdio",
      "command": "npx",
      "args": ["-y", "example-mcp"],
      "env_required": ["EXAMPLE_API_KEY"]
    }
  }
}
```

## Trust modes

- `cosign`: requires trusted signer identity configured in local tap config.
  - Tap index should include `index.json.sig` and `index.json.pem`.
  - Each manifest should include signature and certificate files.
- `hash`: validates only hash pins from `index.json`.

## Adding a tap

```bash
# cosign trust mode
mcper tap add official git@github.com:org/registry.git \
  --trust-mode cosign \
  --issuer https://token.actions.githubusercontent.com \
  --subject https://github.com/org/registry/.github/workflows/release.yml@refs/heads/main

# hash mode for local/dev
mcper tap add local /tmp/registry --trust-mode hash
```
