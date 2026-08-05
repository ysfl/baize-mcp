# Baize MCP

[English](README.en.md) | [Baize](https://github.com/ysfl/baize)

Baize MCP 是白泽的开源 MCP 接入组件，用于让支持 MCP 的 AI 客户端连接用户自己的白泽实例。

可运行版本以 [GitHub Releases](https://github.com/ysfl/baize-mcp/releases) 为准。白泽的部署与升级入口请访问 [Baize](https://github.com/ysfl/baize)。

## 当前能力

- 检查已保存的白泽会话是否可用。
- 分页查询服务器节点，并按名称、系统、架构、版本或状态筛选。
- 查询单个服务器节点的基础状态。

返回结果只包含节点 ID、显示名称、状态、操作系统、架构、Agent 版本和最后心跳时间，不返回连接地址、IP、指纹、能力清单或凭据。

## 安装

从 [GitHub Releases](https://github.com/ysfl/baize-mcp/releases) 下载与你的系统和架构匹配的压缩包，校验 `SHA256SUMS` 后解压。也可以使用 Go 1.25.12 或更高版本从源码构建：

```bash
go build -trimpath -o baize-mcp ./cmd/baize-mcp
```

## 登录

登录必须在本机交互式终端完成，密码不会出现在命令参数或配置文件中：

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

## MCP 工具

| 工具 | 作用 |
|---|---|
| `baize_connection_status` | 验证当前 profile 的会话是否可用 |
| `baize_agents_list` | 分页查询经过裁剪的节点列表 |
| `baize_agent_get` | 查询单个节点的基础状态 |

## 版本与更新

- 结构化更新记录：[releases/changelog.json](releases/changelog.json)
- 完整更新记录：[CHANGELOG.md](CHANGELOG.md)

每个已发布版本都会在 [GitHub Releases](https://github.com/ysfl/baize-mcp/releases) 提供当前版本清单、各平台可执行文件、`release-assets.json` 和 `SHA256SUMS`。发布包同时包含项目许可证、第三方许可证清单和中英文说明。

## 安全边界

- 连接信息保存在当前系统用户的配置目录。
- 登录密码不会写入配置文件。
- 登录会话由系统凭据存储保护，不通过 MCP 工具返回。
- MCP 只能访问当前登录用户在白泽中已有权限覆盖的资源。
- 所有当前工具均为只读、非破坏性工具。
- 当前发布内容只描述已经提供的能力，不包含未发布功能。

安全问题请按 [SECURITY.md](SECURITY.md) 中的方式私下报告，不要在公开 Issue 中提交凭据、服务器地址或运行日志。

## 许可证

本项目使用 [Apache License 2.0](LICENSE)。白泽平台本身适用其各自的许可条款。
