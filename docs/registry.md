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
          "sha256": "<sha256-of-manifest>"
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

## Integrity checks

- `mcper` validates manifest hashes pinned in `index.json`.
- Signature-based verification is not part of this registry format.

## Adding a tap

```bash
mcper tap add local /tmp/registry
```
