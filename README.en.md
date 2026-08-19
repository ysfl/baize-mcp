# Baize MCP

[中文](README.md) | [Baize](https://github.com/ysfl/baize)

Baize MCP is the open-source MCP connector for Baize. It lets MCP-compatible AI clients connect to a Baize instance owned and operated by the user. It does not install the Baize server, console, or Agent; product deployment and upgrades remain in [Baize](https://github.com/ysfl/baize).

Use [GitHub Releases](https://github.com/ysfl/baize-mcp/releases) as the source for runnable versions. To give an AI client both MCP tools and Baize usage guidance, use the [AI access installer](https://github.com/ysfl/baize/blob/main/scripts/install-ai-access.sh) and install the [Baize AI Skill](https://github.com/ysfl/baize/blob/main/skills/baize-ai/SKILL.md).

## Available Capabilities

- Check whether a saved Baize session is valid.
- List server agents with pagination and filters for name, alias, system, region, architecture, Agent version, status, or groups, with selectable stable sort fields and direction.
- Read the basic status of one server agent.
- List available command templates and parameter constraints, and preview a template for selected agents without creating a plan.
- Create and inspect command plans; creating a plan does not dispatch a command to an agent.
- Execute command plans, inspect task progress, and request task cancellation.

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

After installing the [Baize AI Skill](https://github.com/ysfl/baize/blob/main/skills/baize-ai/SKILL.md), an AI can prefer a configured controlled API tool when the user mentions Baize, server nodes, or status queries, and use these MCP tools when no API tool is available. It still asks the user to choose when multiple nodes match. See the [AI Access and Remote Task Guide](https://github.com/ysfl/baize/blob/main/docs/en/ai-remote-tasks.md) for the complete workflow.

## MCP Tools

| Tool | Purpose |
|---|---|
| `baize_connection_status` | Verify that the current profile has a valid session |
| `baize_agents_list` | Read a paginated list of agents with privacy-protected status information |
| `baize_agent_get` | Read the basic status of one agent |
| `baize_command_templates_list` | List command template summaries and parameter constraints allowed for the signed-in account |
| `baize_command_template_preview` | Preview template rendering and prechecks for selected agents without creating a plan |
| `baize_command_plan_create` | Create a Baize-validated command plan without dispatching it |
| `baize_command_plan_get` | Read command plan status, risk, and precheck information |
| `baize_command_plan_execute` | Request plan execution; Baize handles permissions, confirmation, approval, and audit |
| `baize_command_plan_approval_create` | Request approval for a command plan without executing it |
| `baize_command_plan_approvals_list` | List visible command-plan approvals with pagination |
| `baize_command_plan_approval_get` | Read one approval and its redacted plan snapshot |
| `baize_command_plan_approval_decide` | Submit an approval or rejection decision for a command plan |
| `baize_exec_task_get` | Read overall and per-agent remote task progress |
| `baize_exec_task_cancel` | Request cancellation of a pending or running remote task |

Command-plan approval tools were released in `v0.1.3`. Approval still requires the backend permission of the signed-in account and never executes a plan automatically.

### Next-release candidate (not in the stable release yet)

- `baize_exec_task_direct`: creates one traceable remote task through Baize's direct-task entry. A template is an optional shortcut; an exact custom command may be used when Baize allows it. Baize still decides permissions, risk confirmation, approval requirements, and audit.
- `baize_overview_get`: reads the account-scoped runtime summary and a bounded set of highlighted abnormal nodes. Missing resource caches, an empty abnormal list, and failed sections are marked in the result so an AI must not treat an empty list as proof of health; addresses, credentials, and backend ranking weights are excluded.
- `baize_workflow_status`: reads the local profile workflow mode and a minimal summary of the Baize server approval policies; if the signed-in account cannot view policies, it still returns the local mode and marks `approvalPolicyAccess` as `not_visible`.
- `baize_command_plan_cancel`: cancels a command plan that has not been executed.
- `baize_agent_observe`: reads one bounded observation view for an agent, including health, metrics, processes, storage, Docker, Nginx, host-profile status, and control-plane status. Sensitive bodies, credentials, environment values, and complete histories are excluded.
- `baize_exec_task_output_get`: reads bounded task output by target, cursor, and page window after the user explicitly asks for it. The result states whether it is summarized, truncated, or conservatively redacted; missing output does not mean the task failed and must not trigger a duplicate task submission.
- Profiles support `multi` (the default) and `single`. Single-user mode changes the workflow preference only; Baize still decides whether self-approval, approval, or audit is required.

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
