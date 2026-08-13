# Baize MCP

[中文](README.md) | [Baize](https://github.com/ysfl/baize)

Baize MCP is the open-source MCP connector for Baize. It lets MCP-compatible AI clients connect to a Baize instance owned and operated by the user. It does not install the Baize server, console, or Agent; product deployment and upgrades remain in [Baize](https://github.com/ysfl/baize).

Use [GitHub Releases](https://github.com/ysfl/baize-mcp/releases) as the source for runnable versions. To give an AI client both MCP tools and Baize usage guidance, use the [AI access installer](https://github.com/ysfl/baize/blob/main/scripts/install-ai-access.sh) and install the [Baize AI Skill](https://github.com/ysfl/baize/blob/main/skills/baize-ai/SKILL.md).

## Available Capabilities

- Check whether a saved Baize session is valid.
- List server agents with pagination and filters for name, alias, system, region, architecture, Agent version, status, groups, or tags, with selectable stable sort fields and direction.
- Read the basic status of one server agent.

Results contain only the agent ID, display name, status, operating system, architecture, Agent version, and last heartbeat. Connection addresses, IP addresses, fingerprints, capability lists, and credentials are excluded.

## Install

The recommended path is the [Baize AI access entry](https://github.com/ysfl/baize/blob/main/README.en.md#connect-an-ai-client), which installs MCP and the Skill and registers MCP when the selected client supports it. This entry is independent from the Baize product installer and does not deploy or modify a Baize instance.

For a manual MCP installation, download and fully extract the archive for your operating system and architecture from [GitHub Releases](https://github.com/ysfl/baize-mcp/releases). The program automatically checks the executable SHA-256 using the integrity file shipped beside it; keep the archive contents together. This startup check detects corrupted or incomplete installations, while `SHA256SUMS` remains available as an optional release-file verification entry point. You can also build from source with Go 1.25.12 or later:

```bash
go build -trimpath -o baize-mcp ./cmd/baize-mcp
```

## Sign In

Sign in from an interactive local terminal. The username is used only for that sign-in request and is not saved in the profile. The password is never placed in command arguments or configuration files:

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

After installing the [Baize AI Skill](https://github.com/ysfl/baize/blob/main/skills/baize-ai/SKILL.md), an AI can prefer these MCP tools when the user mentions Baize, server nodes, or status queries, and ask the user to choose when multiple nodes match. See the [AI Access and Remote Task Guide](https://github.com/ysfl/baize/blob/main/docs/en/ai-remote-tasks.md) for the complete workflow.

## MCP Tools

| Tool | Purpose |
|---|---|
| `baize_connection_status` | Verify that the current profile has a valid session |
| `baize_agents_list` | Read a paginated list of agents with privacy-protected status information |
| `baize_agent_get` | Read the basic status of one agent |

## Versions and Updates

- Structured update history: [releases/changelog.json](releases/changelog.json)
- Full update history: [CHANGELOG.md](CHANGELOG.md)

Every published version provides its current version manifest, platform executables, `release-assets.json`, and `SHA256SUMS` on [GitHub Releases](https://github.com/ysfl/baize-mcp/releases). Each archive also contains the project license, third-party license notices, and Chinese and English guides.

## Security Boundary

- Connection settings stay in the current operating-system user's configuration directory.
- Usernames are not saved in profiles, and command results report only whether authentication succeeded.
- Login passwords are not written to configuration files.
- Login sessions are protected by the operating system's credential store and are never returned by MCP tools.
- MCP access remains limited to resources already allowed for the signed-in Baize user.
- All currently available tools are read-only and non-destructive.
- Release archives verify executable integrity at startup and refuse to run when the integrity file is missing or does not match.

Follow [SECURITY.md](SECURITY.md) to report security issues privately. Do not post credentials, server addresses, or runtime logs in public issues.

## Future Direction

This version is read-only. Future releases will map published Baize write capabilities into explicit task tools. They will use the signed-in account's existing permission scope and Baize's existing confirmation, audit, and rollback flows instead of adding a second control layer in MCP. Formal release notes will define the actual scope.

## License

This project is licensed under the [Apache License 2.0](LICENSE). The Baize platform remains subject to its respective license terms.
