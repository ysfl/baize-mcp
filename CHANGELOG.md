# Changelog

本文件记录 Baize MCP 已发布版本中用户可以直接使用的变化。

This file records user-visible changes in published Baize MCP versions.

## Unreleased

### 中文

- 未发布候选增加直接远程任务入口；模板只是可选快捷方式，也可以使用服务端允许的精确自定义命令，权限、风险确认、审批和审计仍由白泽处理。
- 未发布候选增加 `baize_overview_get`，组合运行态总览和重点异常节点查询；结果对缓存不可用、空异常列表和部分请求失败做显式标记，不把空结果解释为整体健康。
- 增加按 profile 保存的 `multi` / `single` 工作流模式，旧配置和新登录默认使用多人模式。
- 增加工作流状态与命令计划取消工具；模式只影响 AI 工作流偏好，白泽后端仍负责审批策略、权限和审计。
- 普通账号无审批策略查看权限时，工作流状态仍返回本地模式并标记策略不可见；会话失效和服务异常继续显式失败。
- 审批策略摘要只保留公开风险等级枚举，未知值不会进入 AI 结果。

### English

- Adds an unreleased direct remote-task candidate. Templates remain optional shortcuts, while exact custom commands are accepted only when Baize allows them; Baize still handles permissions, risk confirmation, approval, and audit.
- Adds the unreleased `baize_overview_get` candidate, combining the runtime summary with highlighted abnormal nodes. The result marks unavailable caches, empty abnormal lists, and partial request failures instead of treating an empty result as proof of overall health.
- Adds profile-scoped `multi` and `single` workflow modes; existing profiles and new sign-ins default to multi-user mode.
- Adds workflow-status and command-plan cancellation tools. The mode changes AI workflow preference only; Baize still owns approval policy, permissions, and audit.
- Keeps workflow status useful for accounts that cannot view approval policies by marking the policy summary as `not_visible`; invalid sessions and service failures still return errors.
- Keeps only the published risk-level enum in approval-policy summaries; unknown values are omitted from AI results.

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
