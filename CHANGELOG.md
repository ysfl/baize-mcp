# Changelog

本文件记录 Baize MCP 已发布版本中用户可以直接使用的变化。

This file records user-visible changes in published Baize MCP versions.

## [0.1.6] - 2026-09-24

### 中文

- 新增 `baize_exec_task_create` 工具：一次性命令可直接创建普通远程任务，不需要模板、也不受直连白名单约束；权限、资源范围、命令安全引擎与审计由服务端统一生效。任务默认以待派发状态创建（`autoDispatch` 缺省关闭），可先审查再用 `baize_exec_task_dispatch` 派发。
- 远程任务错误信息更可行动：取消任务现在直接返回取消后的任务摘要（含各目标的失败原因与预计截止时间），不再返回空结果；失败原因经过保守脱敏与限长。
- 409 冲突提示按远程执行场景细分：任务已终态、缺少风险确认、调试会话失效、模板未就绪、批次未收尾、成功率低于门禁各自给出对应的下一步指引，并携带当前状态、任务标识、计数等结构化参数。
- 任务参数超过 4096 字符的提示现在带上限值与当前长度，并明确建议精简或拆分，而不是笼统的"超出允许长度"。

### English

- New `baize_exec_task_create` tool: one-off commands can now create a regular remote execution task directly, with no template and no allowlist required; permissions, agent scope, the command safety engine and audit are enforced by the server. Tasks are created pending by default (`autoDispatch` off), so they can be reviewed and dispatched later with `baize_exec_task_dispatch`.
- Remote task errors are now actionable: cancelling a task returns the post-cancellation task summary (per-target failure reasons and expected deadlines) instead of an empty result; failure reasons are conservatively redacted and length-capped.
- 409 conflict messages are now subdivided for remote execution scenarios: terminal-state tasks, missing risk confirmation, invalid debug sessions, templates not ready, unfinished batches, and below-gate success rates each point to their matching next step, with structured parameters such as current status, task id, and counts.
- The error for task parameters longer than 4096 characters now states the limit and the current length, and clearly suggests shortening or splitting instead of a generic "exceeds the allowed length".

## 0.1.5 - 2026-09-13

### 中文

- 已运行的 MCP 进程会在认证请求前重新读取本机保存的会话；只读请求遇到会话刚更新导致的 401 时会自动重试一次。
- 增加 `baize-mcp retry` 手动检查命令，不会再次要求输入密码；可能产生副作用的写请求不会自动重放。
- 修复 Nginx 站点观察的分页，`sites` 视图现在使用服务端真实页码并返回总数与继续标记。
- 409 冲突提示按原因分族：授权中心拒绝、权益限制与任务状态冲突不再共用同一句话，提示直接指向核对激活码、License、安装绑定、订阅或清理卡住任务等对应下一步。
- 修正目标参数校验失败时错误文案的标签推导，目标相关提示表述保持一致。
- 通过 MCP 进行的管理操作会在请求中声明客户端来源，审计可以区分 AI 经手与人工直接操作。

### English

- Running MCP processes reload the locally saved session before authenticated requests, and retry a read-only request once when a session change causes a 401 response.
- Adds `baize-mcp retry` for a manual session check without asking for the password again; requests that may have side effects are never replayed automatically.
- Fixes pagination for the Nginx sites observation so the `sites` view uses server-side pages and returns totals and continuation metadata.
- Groups 409 conflict messages by reason: authorization-center rejections, entitlement limits, and task-state conflicts no longer share one sentence; guidance now points to the matching next step such as checking the activation code, license, installation binding, subscription, or clearing a stuck task.
- Fixes label derivation in validation messages for invalid target lists so wording stays consistent.
- Management operations made through MCP now declare the client source in requests, so audits can tell AI-handled actions from direct human operations.

## 0.1.4 - 2026-08-29

### 中文

- 增加运行概览工具，读取账号可见范围内的平台运行摘要和有限数量的重点异常节点；缓存缺失、异常列表为空和分区失败会显式标记。
- 增加节点有界观察、固定语义观察（告警、资产、证书、定时任务、日志、Nginx、Runbook、安全、订阅、组件版本）和只读运行态诊断工具；结果只返回完成判断所需字段，敏感正文、凭据、环境变量、关联标识和完整历史不返回。
- 补齐远程任务闭环：直接任务入口、待决任务派发、按需有界输出读取，并保留进度查询与取消。
- 增加告警确认/解决、工作流状态与审批策略摘要、命令计划取消工具；错误信息保留稳定的原因、可重试标记、消息键和下一步动作键。
- AI 接入说明覆盖 DeepSeek Harness（DSH）；最低兼容白泽版本更新为 `0.2.2`。

### English

- Adds a runtime overview tool with the account-scoped platform summary and highlighted abnormal nodes; missing caches, empty abnormal lists, and failed sections are marked explicitly.
- Adds bounded agent observation, fixed semantic observation (alerts, assets, certificates, scheduled tasks, logs, Nginx, Runbooks, security, subscription, components), and read-only runtime diagnosis tools; results carry only the fields needed for the current decision and exclude sensitive bodies, credentials, environment values, correlation identifiers, and complete histories.
- Closes the remote-task loop with the direct-task entry, pending-task dispatch, and on-demand bounded output reads, while keeping progress lookup and cancellation.
- Adds alert acknowledgement/resolution, workflow status with the approval-policy summary, and command-plan cancellation; errors keep stable reason, retryable, message-key, and next-action fields.
- AI access documentation now covers DeepSeek Harness (DSH); the minimum compatible Baize version is raised to `0.2.2`.

## 0.1.3 - 2026-08-16

### 中文

- 增加命令计划审批申请、查询和通过/驳回决策工具；决策仍由白泽后端权限、自审批策略、快照和过期规则最终判断。
- 增加分页继续标记、写生命周期验收和 64 KiB 工具目录/结构化结果门禁。
- 源码构建最低版本更新为 Go 1.25.13，以使用当前标准库安全修复。

### English

- Adds command-plan approval request, listing, and approve/reject decision tools; Baize still makes the final permission, self-approval, snapshot, and expiry decisions.
- Adds pagination continuation markers, write-lifecycle coverage, and 64 KiB limits for the tool catalog and structured results.
- Raises the minimum Go version for source builds to Go 1.25.13 so builds use the current standard-library security fixes.

## 0.1.2 - 2026-08-14

### 中文

- 增加命令模板列表和模板预览工具，帮助 AI 在创建任务前确认可用模板、参数约束、目标和预检结果。
- 增加命令计划创建、查询和执行工具；创建计划不会直接向服务器节点派发命令。
- 增加远程任务查询和取消工具，支持查看整体及目标进度，并请求停止等待中或运行中的任务。
- 写工具只映射白泽已发布的命令工作流，权限、风险确认、审批、审计和任务状态继续由白泽后端处理。
- 计划和任务结果采用有限字段、数量和文本长度边界，不返回命令正文、工作目录、环境变量、操作者身份、任务输出或凭据。

### English

- Added command-template listing and preview tools so an AI can confirm available templates, parameter constraints, targets, and prechecks before creating a task.
- Added command-plan creation, inspection, and execution tools; creating a plan does not dispatch a command to a server agent.
- Added remote-task inspection and cancellation tools for overall and per-agent progress, including cancellation requests for pending or running tasks.
- Write tools only map to published Baize command workflows; Baize continues to handle permissions, risk confirmation, approval, audit, and task state.
- Plan and task results use bounded fields, item counts, and text lengths, and exclude command bodies, working directories, environment values, operator identity, task output, and credentials.

## 0.1.1 - 2026-08-13

### 中文

- `baize_agents_list` 支持按别名、系统、地区、Agent 版本、架构和分组进一步定位节点。
- 节点列表支持按创建时间、更新时间、最后心跳、注册时间、名称、状态、版本或系统排序。
- 仅提供已经验证可用的筛选条件，保持三个只读工具名称、登录方式和隐私保护结果兼容。

### English

- `baize_agents_list` can further locate nodes by alias, system, region, Agent version, architecture, and group.
- Agent lists can be sorted by creation time, update time, last heartbeat, registration time, name, status, version, or operating system.
- Only verified filters are exposed, while the three read-only tool names, sign-in flow, and privacy-protected outputs remain compatible.

## 0.1.0 - 2026-08-12

### 中文

- 提供本机安全登录、会话检查、退出登录和多 profile 支持。
- 提供连接检查、节点分页列表和单节点基础状态三个只读 MCP 工具。
- 工具结果仅包含经过隐私保护的必要状态信息，底层错误不会透传连接、身份或追踪信息。
- 提供 Linux、macOS、Windows 的 amd64 与 arm64 发布包，并在启动时自动校验可执行文件完整性。

### English

- Added secure local sign-in, session checks, sign-out, and multiple profiles.
- Added three read-only MCP tools for connection checks, paginated agent lists, and basic status for one agent.
- Tool results expose only necessary, privacy-protected status information, while lower-level errors exclude connection, identity, and trace details.
- Added amd64 and arm64 release archives for Linux, macOS, and Windows with automatic executable integrity checks at startup.
