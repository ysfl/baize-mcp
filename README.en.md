# Baize MCP

[中文](README.md) | [Baize](https://github.com/ysfl/baize)

Baize MCP is the open-source MCP connector for Baize. It lets MCP-compatible AI clients connect to a Baize instance owned and operated by the user.

Use [GitHub Releases](https://github.com/ysfl/baize-mcp/releases) as the source for runnable versions, installation instructions, and the currently available tools. Visit [Baize](https://github.com/ysfl/baize) for deployment and upgrade guidance.

## Security Boundary

- Connection settings stay in the current operating-system user's configuration directory.
- Login passwords are not written to configuration files.
- Login sessions are protected by the operating system's credential store and are never returned by MCP tools.
- MCP access remains limited to resources already allowed for the signed-in Baize user.
- Published content describes available behavior only and does not include unreleased features.

Follow [SECURITY.md](SECURITY.md) to report security issues privately. Do not post credentials, server addresses, or runtime logs in public issues.

## License

This project is licensed under the [Apache License 2.0](LICENSE). The Baize platform remains subject to its respective license terms.

