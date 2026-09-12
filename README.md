# Baize MCP

[English](README.en.md) | [Baize](https://github.com/ysfl/baize)

Baize MCP 是白泽的开源 MCP 接入组件，用于让支持 MCP 的 AI 客户端连接用户自己的白泽实例。它不会安装白泽中心服务、控制台或 Agent；白泽产品的部署与升级入口仍在 [Baize](https://github.com/ysfl/baize)。

可运行版本以 [GitHub Releases](https://github.com/ysfl/baize-mcp/releases) 为准。希望让 AI 客户端同时获得 MCP 工具和白泽使用方法时，推荐使用 [AI 接入安装器](https://github.com/ysfl/baize/blob/main/scripts/install-ai-access.sh)，并安装 [Baize AI Skill](https://github.com/ysfl/baize/blob/main/skills/baize-ai/SKILL.md)。

## 当前能力

- 检查已保存的白泽会话是否可用。
- 分页查询服务器节点并按名称、别名、系统、地区、架构、Agent 版本、状态或分组筛选；查询单个节点通过隐私保护的基础状态、有界观察摘要，以及账号可见的运行概览和重点异常节点。
- 读取固定语义观察视图：告警、资产清单、证书状态、定时任务、日志、Nginx、Runbook、安全暴露面、订阅状态和组件版本状态。
- 针对单个节点发起只读运行态诊断，并在脱敏后读取诊断摘要和 AI 上下文。
- 查询可用的命令模板及参数约束，并对指定节点执行不落地的模板预览。
- 创建、查询、取消和审批命令计划；创建计划不会直接向节点派发命令，审批决定由白泽后端权限与规则最终判断。
- 创建和派发远程任务、查询任务进度、按需读取有界输出并请求取消；也可以通过直接任务入口提交模板任务或服务端允许的精确自定义命令。
- 读取本机工作流模式和白泽审批策略摘要。

命令模板、计划和任务结果只返回完成当前判断所需的有限字段，不返回命令正文、工作目录、环境变量、操作者身份、任务输出或凭据。预览结果和列表结果会按数量、文本长度和 UTF-8 边界限制大小。

参数与输出约束：单个参数值和自定义命令不超过 4096 字符；任务输出按页读取，单次返回有总量上限，按返回的分页游标继续读取，不要重复提交任务。错误结果附带稳定的错误原因标识（如权限、状态冲突、需要风险确认）；遇到这类原因时按返回的提示处理，不要更换工具重试同一动作。

## 安装

推荐从 [Baize AI 接入入口](https://github.com/ysfl/baize#ai-客户端接入) 安装 MCP、Skill，并为检测到的 AI 客户端自动注册 MCP。这个入口与白泽产品安装器相互独立，不会部署或修改白泽实例。

如需手动安装 MCP，从 [GitHub Releases](https://github.com/ysfl/baize-mcp/releases) 下载与你的系统和架构匹配的压缩包并完整解压。程序启动时会自动校验压缩包随附的可执行文件 SHA-256；请保留压缩包内的校验文件。这个自检用于发现文件损坏或安装不完整，`SHA256SUMS` 仍作为可选的发布文件校验入口。也可以使用 Go 1.25.13 或更高版本从源码构建：

```bash
go build -trimpath -o baize-mcp ./cmd/baize-mcp
```

## 登录

登录必须在本机交互式终端完成。用户名只用于本次登录请求，不会写入 profile；密码不会出现在命令参数或配置文件中：

```bash
baize-mcp login \
  --api-url https://baize.example.com/api/v1 \
  --username your-user
```

默认要求 HTTPS。本机回环地址可以使用 HTTP；其它 HTTP 地址必须由用户显式增加 `--allow-http`。会话失效后，重新执行登录命令即可。

### 登录后的会话重读（下一正式版本）

包含此改进的版本会在认证请求前重新读取本机保存的会话，因此登录后通常不需要重启 AI 客户端。只读请求如果在会话刚更新时收到 401，MCP 会自动使用新会话重试一次；可能产生副作用的写请求不会自动重放。

需要手动读取当前会话并检查连接时，可以运行：

```bash
baize-mcp retry --profile default
```

该命令不会再次要求密码，只会验证已保存的会话。也可以在 AI 客户端再次调用 `baize_connection_status`。更新 MCP 可执行文件、工具定义或连接配置后，仍需按客户端说明重新连接。

## 连接 MCP 客户端

推荐使用 [Baize AI 接入入口](https://github.com/ysfl/baize#ai-客户端接入) 自动完成注册：安装器会识别本机已安装的 Codex CLI、Claude Code、ZCode、Gemini CLI、Qwen Code、Cursor、Windsurf、VS Code（GitHub Copilot）、Cline、Trae 和 DeepSeek Harness（DSH），并写入对应客户端的 MCP 配置，同时为 Codex CLI、Claude Code、ZCode 和 DSH 安装 Skill。

手动安装时，在支持 stdio MCP 的客户端中添加以下配置，并把命令替换为本机可执行文件的绝对路径：

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

DeepSeek Harness（DSH）不使用 `mcpServers` JSON，而是在用户插件层插入一行通用 MCP 客户端插件：写入 `$DSH_HOME/cordis.patch.yml` 对本机所有 DSH 配置生效，写入 `$DSH_HOME/profiles/<名称>/cordis.patch.yml` 则只影响对应配置。

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

常见客户端的配置位置：Codex CLI 使用 `codex mcp add`；Claude Code 使用 `claude mcp add`；ZCode 写入 `~/.zcode/cli/config.json` 的 `mcp.servers`；Gemini CLI 写入 `~/.gemini/settings.json`；Qwen Code 写入 `~/.qwen/settings.json`；Cursor 使用 `~/.cursor/mcp.json`；Windsurf 使用 `~/.codeium/windsurf/mcp_config.json`；VS Code 使用用户配置中的 `mcp.json`（顶层 `servers` 键）；Cline 使用自己的 `cline_mcp_settings.json`；Trae 使用 `~/.trae/mcp.json`。客户端版本更新可能调整位置或格式，注册后请在客户端中确认 Baize MCP 已连接。

这份客户端配置不包含白泽地址、用户名、密码或会话凭据。需要连接多个实例时，可以在登录和 `serve` 命令中使用不同的 `--profile` 名称。

安装 [Baize AI Skill](https://github.com/ysfl/baize/blob/main/skills/baize-ai/SKILL.md) 后，AI 可以在用户提到白泽、服务器节点或状态查询时优先选择已配置的受控 API 工具；没有 API 工具时使用这些 MCP 工具，并在多节点匹配时先让用户确认。完整使用方式见 [AI 接入与远程任务指南](https://github.com/ysfl/baize/blob/main/docs/ai-remote-tasks.md)。

## MCP 工具

| 工具 | 作用 |
|---|---|
| `baize_connection_status` | 重新读取已保存会话并验证当前 profile 是否可用 |
| `baize_agents_list` | 分页查询经过隐私保护的节点状态信息 |
| `baize_agent_get` | 查询单个节点通过隐私保护的基础状态 |
| `baize_agent_observe` | 读取单个节点指定维度的有界观察摘要（健康、指标、进程、存储、Docker、Nginx、主机画像、控制面）；敏感正文、凭据、环境变量和完整历史不返回 |
| `baize_overview_get` | 读取当前账号可见范围内的运行摘要和有限数量的重点异常节点；缓存缺失、异常列表为空、分区失败均明确标记，不把空列表当作整体健康 |
| `baize_runtime_diagnosis_start` | 针对单个节点创建有界、只读的运行态诊断探测；目标校验、账号权限和能力检查由白泽处理 |
| `baize_runtime_diagnosis_get` | 查询一次诊断的状态和证据数量；命令、路径、端口、凭据和原始输出不返回 |
| `baize_runtime_diagnosis_ai_context_get` | 读取隐私脱敏后的诊断 AI 上下文；命令、路径、证据值、操作者身份和凭据不返回 |
| `baize_alerts_list` | 分页查询当前账号可见的告警，消息经过限幅和隐私精简 |
| `baize_alert_change` | 确认或解决一条告警；服务端负责权限、状态规则和审计，成功后需再次查询告警确认最终状态 |
| `baize_assets_query` | 读取固定的资产清单视图（列表、摘要、到期、详情）；地址、备注、链接和凭据不返回 |
| `baize_certificates_list` | 查询证书监控目标及最近的有界状态；文件路径、私钥和凭据不返回 |
| `baize_cron_jobs_query` | 读取固定的定时任务视图（列表、详情、执行日志）；不运行也不修改任务 |
| `baize_logs_query` | 读取固定的日志视图（服务端最近日志、按需节点日志、聚合概览）；关联标识和原始路径已脱敏 |
| `baize_nginx_observe` | 读取固定的 Nginx 观察视图；客户端地址、完整 URL、配置内容和凭据不返回 |
| `baize_runbooks_query` | 读取固定的 Runbook 视图（定义、有界步骤元数据、审阅审计）；输入、绑定、指令和操作者身份不返回 |
| `baize_security_observe` | 读取固定的暴露面或网络入口安全视图；地址、路径、进程细节和原始证据不返回 |
| `baize_subscription_get` | 读取当前账号可见的订阅计划、授权状态、功能模式、限制、用量和遥测策略 |
| `baize_system_release_get` | 读取当前与最新组件版本、更新可用性和有界发布说明；镜像、摘要、升级命令和源码地址不返回 |
| `baize_workflow_status` | 读取本机 profile 的工作流模式和服务端审批策略摘要；无策略查看权限时仍返回本地模式并标记 `not_visible` |
| `baize_command_templates_list` | 查询当前账号可用的命令模板摘要和参数约束 |
| `baize_command_template_preview` | 对指定节点预览模板渲染和预检，不创建计划 |
| `baize_command_plan_create` | 创建经白泽校验的命令计划，不直接派发 |
| `baize_command_plan_get` | 查询命令计划的状态、风险和预检结果 |
| `baize_command_plan_cancel` | 取消尚未执行的命令计划 |
| `baize_command_plan_execute` | 请求执行命令计划，由白泽处理权限、确认、审批和审计 |
| `baize_command_plan_approval_create` | 为需要审批的命令计划申请审批，不执行计划 |
| `baize_command_plan_approvals_list` | 分页查询当前账号可见的命令计划审批单 |
| `baize_command_plan_approval_get` | 查询单个审批单及脱敏计划快照 |
| `baize_command_plan_approval_decide` | 提交命令计划审批通过或驳回决策 |
| `baize_exec_task_direct` | 通过服务端直接任务入口创建一条可追踪远程任务；模板可选，也可提交服务端允许的精确自定义命令；权限、风险确认、审批要求和审计由白泽决定 |
| `baize_exec_task_get` | 查询远程任务整体及目标进度 |
| `baize_exec_task_dispatch` | 将已创建且等待中的远程任务推入白泽执行链路；不重新提交命令或修改目标 |
| `baize_exec_task_output_get` | 在用户明确要求后按目标、游标和页窗口读取有限任务输出；结果明确标记摘要、截断和保守替换，未返回内容不代表任务失败 |
| `baize_exec_task_cancel` | 请求取消等待中或运行中的远程任务 |

命令计划审批工具在 `v0.1.3` 提供；运行概览、节点观察、固定语义观察、运行态诊断、直接任务派发闭环、告警、工作流状态和计划取消工具在 `v0.1.4` 提供。审批通过仍需当前账号具备后端权限，且不会自动执行计划。

本机 profile 支持 `multi`（默认）和 `single` 工作流模式；单人模式只改变工作流偏好，是否允许自审批、是否需要审批以及审计仍由白泽服务端策略决定。

可以在本机 profile 中切换模式：

```bash
baize-mcp config set --profile default --workflow-mode single
baize-mcp config get --profile default
```

当前稳定 MCP 版本主要提供命令计划工作流；普通远程任务 API 不要求 `templateId`，也不因 API 调用而绕过权限或审计。API 和 MCP 都使用带角色的白泽账号，操作历史和安全审计由白泽服务端保留；是否需要审批由服务端策略决定。MCP 没有独立的审计存储。

## 文件推送参考（结合 REST 能力）

MCP 工具本身不传输文件，但结合 `baize_exec_task_direct` 与公开 REST 任务接口，可以把中小文件（建议 ≤ 20MB）以"分片远程任务"方式推送到只有 Agent 可达、未开放 FTP/SSH 的节点：本地 gzip + base64 切片（约 96KB/片，受 Linux `MAX_ARG_STRLEN` 限制），逐片串行下发写盘任务，远端拼接解码后做 sha256 双端校验。每个分片都是一条受审计的远程任务，权限与风险确认由白泽处理。

完整的通道选择（FTP / 制品仓库 / 分片）、约束与可直接使用的参考脚本见 [baize 公开仓的《远程文件传输》](https://github.com/ysfl/baize/blob/main/docs/remote-file-transfer.md) 与 [`scripts/baize-file-push.sh`](https://github.com/ysfl/baize/blob/main/scripts/baize-file-push.sh)。

## 版本与更新

- 结构化更新记录：[releases/changelog.json](releases/changelog.json)
- 完整更新记录：[CHANGELOG.md](CHANGELOG.md)

每个已发布版本都会在 [GitHub Releases](https://github.com/ysfl/baize-mcp/releases) 提供当前版本清单、各平台可执行文件、`release-assets.json` 和 `SHA256SUMS`。发布包同时包含项目许可证、第三方许可证清单和中英文说明。

### 更新已安装版本

推荐在之前克隆的 [Baize 公开入口](https://github.com/ysfl/baize) 目录中运行 [AI 接入升级器](https://github.com/ysfl/baize/blob/main/scripts/upgrade-ai-access.sh)：

```bash
bash scripts/upgrade-ai-access.sh --lang zh
```

它会更新公开接入入口，安装当前正式 MCP 并同步 Skill；下载包和可执行文件会自动校验。升级不会删除本机 profile 或系统凭据存储。升级前请退出正在使用 MCP 的 AI 客户端，完成后再重新打开。Windows 使用对应的 `upgrade-ai-access.ps1`；手动安装用户也可以从 Releases 下载目标平台归档，替换同一目录中的程序和 `baize-mcp.sha256`。

## 安全边界

- 连接信息保存在当前系统用户的配置目录。
- 用户名不写入 profile，命令结果只返回是否已认证。
- 登录密码不会写入配置文件。
- 登录会话由系统凭据存储保护，不通过 MCP 工具返回。
- MCP 只能访问当前登录用户在白泽中已有权限覆盖的资源。
- 写工具只映射已发布的白泽命令工作流；权限、风险确认、审批、审计和任务状态由白泽处理，MCP 不另建一套控制逻辑。
- 创建计划不会直接派发命令；执行和取消请求仍可能被白泽权限、风险或任务状态拒绝。
- 发布包启动时会自动校验可执行文件完整性；校验文件缺失或不匹配时不会继续运行。

安全问题请按 [SECURITY.md](SECURITY.md) 中的方式私下报告，不要在公开 Issue 中提交凭据、服务器地址或运行日志。

## 后续方向

后续版本会继续覆盖已经稳定发布的观察、诊断、告警、资产、定时任务和 Runbook 能力，并通过分页、分块和按需视图控制上下文体积。新增写工具仍需保持明确的任务语义、账号权限边界和可追踪结果；具体能力以正式版本说明为准。

## 许可证

本项目使用 [Apache License 2.0](LICENSE)。白泽平台本身适用其各自的许可条款。
