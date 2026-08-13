# Baize MCP

[English](README.en.md) | [Baize](https://github.com/ysfl/baize)

Baize MCP 是白泽的开源 MCP 接入组件，用于让支持 MCP 的 AI 客户端连接用户自己的白泽实例。它不会安装白泽中心服务、控制台或 Agent；白泽产品的部署与升级入口仍在 [Baize](https://github.com/ysfl/baize)。

可运行版本以 [GitHub Releases](https://github.com/ysfl/baize-mcp/releases) 为准。希望让 AI 客户端同时获得 MCP 工具和白泽使用方法时，推荐使用 [AI 接入安装器](https://github.com/ysfl/baize/blob/main/scripts/install-ai-access.sh)，并安装 [Baize AI Skill](https://github.com/ysfl/baize/blob/main/skills/baize-ai/SKILL.md)。

## 当前能力

- 检查已保存的白泽会话是否可用。
- 分页查询服务器节点，并按名称、别名、系统、地区、架构、Agent 版本、状态或分组筛选，也可以选择稳定的排序字段和方向。
- 查询单个服务器节点的基础状态。

返回结果只包含节点 ID、显示名称、状态、操作系统、架构、Agent 版本和最后心跳时间，不返回连接地址、IP、指纹、能力清单或凭据。

## 安装

推荐从 [Baize AI 接入入口](https://github.com/ysfl/baize#ai-客户端接入) 安装 MCP、Skill，并按客户端能力自动注册 MCP。这个入口与白泽产品安装器相互独立，不会部署或修改白泽实例。

如需手动安装 MCP，从 [GitHub Releases](https://github.com/ysfl/baize-mcp/releases) 下载与你的系统和架构匹配的压缩包并完整解压。程序启动时会自动校验压缩包随附的可执行文件 SHA-256；请保留压缩包内的校验文件。这个自检用于发现文件损坏或安装不完整，`SHA256SUMS` 仍作为可选的发布文件校验入口。也可以使用 Go 1.25.12 或更高版本从源码构建：

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

## 连接 MCP 客户端

在支持 stdio MCP 的客户端中添加以下配置，并把命令替换为本机可执行文件的绝对路径：

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

这份客户端配置不包含白泽地址、用户名、密码或会话凭据。需要连接多个实例时，可以在登录和 `serve` 命令中使用不同的 `--profile` 名称。

安装 [Baize AI Skill](https://github.com/ysfl/baize/blob/main/skills/baize-ai/SKILL.md) 后，AI 可以在用户提到白泽、服务器节点或状态查询时优先选择这些 MCP 工具，并在多节点匹配时先让用户确认。完整使用方式见 [AI 接入与远程任务指南](https://github.com/ysfl/baize/blob/main/docs/ai-remote-tasks.md)。

## MCP 工具

| 工具 | 作用 |
|---|---|
| `baize_connection_status` | 验证当前 profile 的会话是否可用 |
| `baize_agents_list` | 分页查询经过隐私保护的节点状态信息 |
| `baize_agent_get` | 查询单个节点的基础状态 |

## 版本与更新

- 结构化更新记录：[releases/changelog.json](releases/changelog.json)
- 完整更新记录：[CHANGELOG.md](CHANGELOG.md)

每个已发布版本都会在 [GitHub Releases](https://github.com/ysfl/baize-mcp/releases) 提供当前版本清单、各平台可执行文件、`release-assets.json` 和 `SHA256SUMS`。发布包同时包含项目许可证、第三方许可证清单和中英文说明。

## 安全边界

- 连接信息保存在当前系统用户的配置目录。
- 用户名不写入 profile，命令结果只返回是否已认证。
- 登录密码不会写入配置文件。
- 登录会话由系统凭据存储保护，不通过 MCP 工具返回。
- MCP 只能访问当前登录用户在白泽中已有权限覆盖的资源。
- 所有当前工具均为只读、非破坏性工具。
- 发布包启动时会自动校验可执行文件完整性；校验文件缺失或不匹配时不会继续运行。

安全问题请按 [SECURITY.md](SECURITY.md) 中的方式私下报告，不要在公开 Issue 中提交凭据、服务器地址或运行日志。

## 未来方向

当前版本只提供只读能力。后续版本会把白泽已经发布的写能力映射为语义明确的任务工具，并继续使用当前登录账号的权限范围以及白泽已有的确认、审计和回滚流程，不在 MCP 中另建一套控制逻辑。具体能力以正式版本说明为准。

## 许可证

本项目使用 [Apache License 2.0](LICENSE)。白泽平台本身适用其各自的许可条款。
