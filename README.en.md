# Baize MCP

[中文](README.md) | [Baize](https://github.com/ysfl/baize)

Baize MCP is the open-source MCP connector for Baize. It lets MCP-compatible AI clients connect to a Baize instance owned and operated by the user. It does not install the Baize server, console, or Agent; product deployment and upgrades remain in [Baize](https://github.com/ysfl/baize).

Use [GitHub Releases](https://github.com/ysfl/baize-mcp/releases) as the source for runnable versions. To give an AI client both MCP tools and Baize usage guidance, use the [AI access installer](https://github.com/ysfl/baize/blob/main/scripts/install-ai-access.sh) and install the [Baize AI Skill](https://github.com/ysfl/baize/blob/main/skills/baize-ai/SKILL.md).

## Available Capabilities

- Check whether a saved Baize session is valid.
- List server agents with pagination and filters for name, alias, system, region, architecture, Agent version, status, or groups; read privacy-protected basic status, bounded observation views for one agent, and the account-scoped runtime overview with highlighted abnormal nodes.
- Read fixed semantic observation views: alerts, asset inventory, certificate status, scheduled tasks, logs, Nginx, Runbooks, security exposure, subscription status, and component version status.
- Start a read-only runtime diagnosis for one agent and read its bounded summary and privacy-reduced AI context.
- List available command templates and parameter constraints, and preview a template for selected agents without creating a plan.
- Create, inspect, cancel, and approve command plans; creating a plan does not dispatch a command, and approval decisions are finally judged by Baize permissions and rules.
- Create and dispatch remote tasks, inspect progress, read bounded output on demand, and request cancellation; a direct-task entry also accepts template tasks or exact custom commands allowed by Baize.
- Read the local workflow mode and a minimal Baize approval-policy summary.

Template, plan, and task results contain only bounded fields needed for the current decision. They exclude command bodies, working directories, environment values, operator identity, task output, and credentials. Preview and list results are bounded by item count, text length, and complete UTF-8 boundaries.

## Install

The recommended path is the [Baize AI access entry](https://github.com/ysfl/baize/blob/main/README.en.md#connect-an-ai-client), which installs MCP and the Skill and registers MCP when the selected client supports it. This entry is independent from the Baize product installer and does not deploy or modify a Baize instance.

For a manual MCP installation, download and fully extract the archive for your operating system and architecture from [GitHub Releases](https://github.com/ysfl/baize-mcp/releases). The program automatically checks the executable SHA-256 using the integrity file shipped beside it; keep the archive contents together. This startup check detects corrupted or incomplete installations, while `SHA256SUMS` remains available as an optional release-file verification entry point. You can also build from source with Go 1.25.13 or later:

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

### Session reload after sign-in (next stable release)

The release containing this improvement reloads the locally saved session before authenticated requests, so signing in normally does not require restarting the AI client. If a read-only request receives a 401 while a session has just changed, MCP retries it once with the new session; requests that may have side effects are never replayed automatically.

To manually reload the saved session and check the connection, run:

```bash
baize-mcp retry --profile default
```

This command does not ask for the password again; it only validates the saved session. You can also call `baize_connection_status` again in the AI client. After updating the MCP executable, tool definitions, or connection configuration, reconnect it according to the client instructions.

## Connect an MCP Client

The recommended path is the [Baize AI access installer](https://github.com/ysfl/baize/blob/main/README.en.md#connect-an-ai-client), which detects installed clients — Codex CLI, Claude Code, ZCode, Gemini CLI, Qwen Code, Cursor, Windsurf, VS Code (GitHub Copilot), Cline, Trae, and DeepSeek Harness (DSH) — writes the MCP registration into each one, and installs the Skill for Codex CLI, Claude Code, ZCode, and DSH.

For manual installation, add the following configuration to a client that supports MCP over stdio. Replace the command with the absolute path to the executable on your computer:

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

DeepSeek Harness (DSH) does not use `mcpServers` JSON. It registers a generic MCP client plugin row in the user patch layer instead: `$DSH_HOME/cordis.patch.yml` applies to every DSH configuration on the machine, while `$DSH_HOME/profiles/<name>/cordis.patch.yml` affects only that configuration.

```yaml
- insert:
    - id: mcp-baize
      name: '@deepseek-ai/dsh-mcp-client'
      config:
        serverName: baize
        transport: stdio
        command: /absolute/path/to/baize-mcp
        args: [serve, --profile, default]
```

Configuration locations for common clients: Codex CLI uses `codex mcp add`; Claude Code uses `claude mcp add`; ZCode reads `mcp.servers` in `~/.zcode/cli/config.json`; Gemini CLI uses `~/.gemini/settings.json`; Qwen Code uses `~/.qwen/settings.json`; Cursor uses `~/.cursor/mcp.json`; Windsurf uses `~/.codeium/windsurf/mcp_config.json`; VS Code uses `mcp.json` in the user profile (top-level `servers` key); Cline uses its own `cline_mcp_settings.json`; Trae uses `~/.trae/mcp.json`. Client updates may change these locations or formats; after registering, confirm in the client that Baize MCP is connected.

The client configuration contains no Baize address, username, password, or session credential. To connect to more than one instance, use a different `--profile` name for both the sign-in and `serve` commands.

After installing the [Baize AI Skill](https://github.com/ysfl/baize/blob/main/skills/baize-ai/SKILL.md), an AI can prefer a configured controlled API tool when the user mentions Baize, server nodes, or status queries, and use these MCP tools when no API tool is available. It still asks the user to choose when multiple nodes match. See the [AI Access and Remote Task Guide](https://github.com/ysfl/baize/blob/main/docs/en/ai-remote-tasks.md) for the complete workflow.

## MCP Tools

| Tool | Purpose |
|---|---|
| `baize_connection_status` | Reload the saved session and verify that the current profile can connect |
| `baize_agents_list` | Read a paginated list of agents with privacy-protected status information |
| `baize_agent_get` | Read privacy-protected basic status for one agent |
| `baize_agent_observe` | Read one bounded observation view for an agent (health, metrics, processes, storage, Docker, Nginx, host profile, control plane); sensitive bodies, credentials, environment values, and complete histories are excluded |
| `baize_overview_get` | Read the account-scoped runtime summary and a bounded set of highlighted abnormal nodes; missing caches, an empty abnormal list, and failed sections are marked so an empty list is not treated as proof of health |
| `baize_runtime_diagnosis_start` | Create a bounded, read-only runtime diagnosis probe for one agent; Baize validates the target, checks account permission and agent capability, and keeps diagnosis and execution separate |
| `baize_runtime_diagnosis_get` | Read diagnosis status and evidence counts for one probe; commands, paths, ports, credentials, and raw agent output are excluded |
| `baize_runtime_diagnosis_ai_context_get` | Read the privacy-reduced AI context for one diagnosis; commands, paths, evidence values, operator identity, and credentials are excluded |
| `baize_alerts_list` | List alerts visible to the signed-in account with bounded, privacy-reduced messages |
| `baize_alert_change` | Request acknowledgement or resolution for one alert; Baize remains responsible for permissions, state rules, and audit, and callers must query the alert again to confirm the final status |
| `baize_assets_query` | Read one fixed asset inventory view (list, summary, expiring, detail); IP addresses, notes, links, and credentials are excluded |
| `baize_certificates_list` | List certificate monitoring targets and their latest bounded status; file paths, private keys, and credentials are excluded |
| `baize_cron_jobs_query` | Read one fixed scheduled-task view (list, detail, execution logs); the tool never runs or changes a task |
| `baize_logs_query` | Read one fixed log view (recent server logs, on-demand agent logs, aggregate overview); correlation identifiers and raw source paths are redacted |
| `baize_nginx_observe` | Read one fixed Nginx observation view; client addresses, complete URLs, configuration contents, and credentials are excluded |
| `baize_runbooks_query` | Read one fixed Runbook view (definitions, bounded step metadata, definition audit events); inputs, bindings, instructions, and operator identity are excluded |
| `baize_security_observe` | Read one fixed exposure or network-entry security view; addresses, paths, process details, and raw evidence are excluded |
| `baize_subscription_get` | Read the account-visible subscription plan, license state, feature modes, limits, usage counters, and telemetry policy |
| `baize_system_release_get` | Read current and latest component versions, update availability, and bounded release notes; images, digests, upgrade commands, and source URLs are excluded |
| `baize_workflow_status` | Read the local profile workflow mode and a minimal approval-policy summary; when policies are not viewable it still returns the local mode and marks `not_visible` |
| `baize_command_templates_list` | List command template summaries and parameter constraints allowed for the signed-in account |
| `baize_command_template_preview` | Preview template rendering and prechecks for selected agents without creating a plan |
| `baize_command_plan_create` | Create a Baize-validated command plan without dispatching it |
| `baize_command_plan_get` | Read command plan status, risk, and precheck information |
| `baize_command_plan_cancel` | Cancel a command plan that has not been executed |
| `baize_command_plan_execute` | Request plan execution; Baize handles permissions, confirmation, approval, and audit |
| `baize_command_plan_approval_create` | Request approval for a command plan without executing it |
| `baize_command_plan_approvals_list` | List visible command-plan approvals with pagination |
| `baize_command_plan_approval_get` | Read one approval and its redacted plan snapshot |
| `baize_command_plan_approval_decide` | Submit an approval or rejection decision for a command plan |
| `baize_exec_task_direct` | Create one traceable remote task through Baize's direct-task entry; a template is an optional shortcut and an exact custom command is accepted when Baize allows it. Baize decides permissions, risk confirmation, approval requirements, and audit |
| `baize_exec_task_get` | Read overall and per-agent remote task progress |
| `baize_exec_task_dispatch` | Push an existing pending task into the Baize execution chain without resubmitting the command or changing targets |
| `baize_exec_task_output_get` | Read bounded task output by target, cursor, and page window after the user explicitly asks for it; the result states summary, truncation, and conservative redaction, and missing output does not mean the task failed |
| `baize_exec_task_cancel` | Request cancellation of a pending or running remote task |

Command-plan approval tools were released in `v0.1.3`; runtime overview, agent observation, fixed semantic observation, runtime diagnosis, the direct-task dispatch loop, alerts, workflow status, and plan cancellation were released in `v0.1.4`. Approval still requires the backend permission of the signed-in account and never executes a plan automatically.

Local profiles support `multi` (the default) and `single` workflow modes. Single-user mode changes the workflow preference only; Baize still decides whether self-approval, approval, or audit is required.

Switch the mode in the local profile:

```bash
baize-mcp config set --profile default --workflow-mode single
baize-mcp config get --profile default
```

The stable MCP release mainly provides the command-plan workflow. The ordinary remote-task API does not require `templateId`, and using the API does not bypass permissions or audit. Both API and MCP use a role-bearing Baize account; Baize keeps the operation history and security audit, and its policy decides whether approval is required. MCP has no independent audit store.

## Versions and Updates

- Structured update history: [releases/changelog.json](releases/changelog.json)
- Full update history: [CHANGELOG.md](CHANGELOG.md)

Every published version provides its current version manifest, platform executables, `release-assets.json`, and `SHA256SUMS` on [GitHub Releases](https://github.com/ysfl/baize-mcp/releases). Each archive also contains the project license, third-party license notices, and Chinese and English guides.

### Update an installed version

From the previously cloned [Baize public entry](https://github.com/ysfl/baize), use the [AI access upgrader](https://github.com/ysfl/baize/blob/main/scripts/upgrade-ai-access.sh):

```bash
bash scripts/upgrade-ai-access.sh --lang en
```

It updates the public access entry, installs the current stable MCP, and refreshes the Skill; the archive and executable are verified automatically. The upgrade does not delete the local profile or operating-system credential store. Close AI clients that use MCP before upgrading, then reopen them afterward. On Windows, use `upgrade-ai-access.ps1`; manual installations can download the target platform archive from Releases and replace the executable and `baize-mcp.sha256` in the same directory.

## Security Boundary

- Connection settings stay in the current operating-system user's configuration directory.
- Usernames are not saved in profiles, and command results report only whether authentication succeeded.
- Login passwords are not written to configuration files.
- Login sessions are protected by the operating system's credential store and are never returned by MCP tools.
- MCP access remains limited to resources already allowed for the signed-in Baize user.
- Write tools only map to published Baize command workflows. Baize remains responsible for permissions, risk confirmation, approval, audit, and task state; MCP does not add a second control layer.
- Creating a plan does not dispatch a command, and execution or cancellation can still be rejected by Baize permissions, risk checks, or task state.
- Release archives verify executable integrity at startup and refuse to run when the integrity file is missing or does not match.

Follow [SECURITY.md](SECURITY.md) to report security issues privately. Do not post credentials, server addresses, or runtime logs in public issues.

## Next Direction

Future releases will continue to cover stable published capabilities for observation, diagnostics, alerts, assets, scheduled tasks, and Runbooks, using pagination, chunking, and on-demand views to control context size. New write tools will keep explicit task semantics, account permission boundaries, and traceable results; formal release notes define the actual scope.

## License

This project is licensed under the [Apache License 2.0](LICENSE). The Baize platform remains subject to its respective license terms.
