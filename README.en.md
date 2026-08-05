# Baize MCP

[中文](README.md) | [Baize](https://github.com/ysfl/baize)

Baize MCP is the open-source MCP connector for Baize. It lets MCP-compatible AI clients connect to a Baize instance owned and operated by the user.

Use [GitHub Releases](https://github.com/ysfl/baize-mcp/releases) as the source for runnable versions. Visit [Baize](https://github.com/ysfl/baize) for deployment and upgrade guidance.

## Available Capabilities

- Check whether a saved Baize session is valid.
- List server agents with pagination and filters for name, system, architecture, version, or status.
- Read the basic status of one server agent.

Results contain only the agent ID, display name, status, operating system, architecture, Agent version, and last heartbeat. Connection addresses, IP addresses, fingerprints, capability lists, and credentials are excluded.

## Install

Download the archive for your operating system and architecture from [GitHub Releases](https://github.com/ysfl/baize-mcp/releases), verify it with `SHA256SUMS`, and extract it. You can also build from source with Go 1.25.12 or later:

```bash
go build -trimpath -o baize-mcp ./cmd/baize-mcp
```

## Sign In

Sign in from an interactive local terminal. The password is never placed in command arguments or configuration files:

```bash
baize-mcp login \
  --api-url https://baize.example.com/api/v1 \
  --username your-user
```

HTTPS is required by default. HTTP is accepted for loopback addresses; any other HTTP address requires the user to add `--allow-http` explicitly. Run the sign-in command again when the session expires.

## Connect an MCP Client

Add the following configuration to a client that supports MCP over stdio. Replace the command with the absolute path to the executable on your computer:

```json
{
  "mcpServers": {
    "baize": {
      "command": "/absolute/path/to/baize-mcp",
      "args": ["serve", "--profile", "default"]
    }
  }
}
```

The client configuration contains no Baize address, username, password, or session credential. To connect to more than one instance, use a different `--profile` name for both the sign-in and `serve` commands.

## MCP Tools

| Tool | Purpose |
|---|---|
| `baize_connection_status` | Verify that the current profile has a valid session |
| `baize_agents_list` | Read a paginated, privacy-reduced agent list |
| `baize_agent_get` | Read the basic status of one agent |

## Versions and Updates

- Structured update history: [releases/changelog.json](releases/changelog.json)
- Full update history: [CHANGELOG.md](CHANGELOG.md)

Every published version provides its current version manifest, platform executables, `release-assets.json`, and `SHA256SUMS` on [GitHub Releases](https://github.com/ysfl/baize-mcp/releases). Each archive also contains the project license, third-party license notices, and Chinese and English guides.

## Security Boundary

- Connection settings stay in the current operating-system user's configuration directory.
- Login passwords are not written to configuration files.
- Login sessions are protected by the operating system's credential store and are never returned by MCP tools.
- MCP access remains limited to resources already allowed for the signed-in Baize user.
- All currently available tools are read-only and non-destructive.
- Published content describes available behavior only and does not include unreleased features.

Follow [SECURITY.md](SECURITY.md) to report security issues privately. Do not post credentials, server addresses, or runtime logs in public issues.

## License

This project is licensed under the [Apache License 2.0](LICENSE). The Baize platform remains subject to its respective license terms.
