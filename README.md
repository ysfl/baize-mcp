# Baize MCP

[English](README.en.md) | [Baize](https://github.com/ysfl/baize)

Baize MCP 是白泽的开源 MCP 接入组件，用于让支持 MCP 的 AI 客户端连接用户自己的白泽实例。

可运行版本、安装方式和当前可用工具以 [GitHub Releases](https://github.com/ysfl/baize-mcp/releases) 为准。白泽的部署与升级入口请访问 [Baize](https://github.com/ysfl/baize)。

## 安全边界

- 连接信息保存在当前系统用户的配置目录。
- 登录密码不会写入配置文件。
- 登录会话由系统凭据存储保护，不通过 MCP 工具返回。
- MCP 只能访问当前登录用户在白泽中已有权限覆盖的资源。
- 当前发布内容只描述已经提供的能力，不包含未发布功能。

安全问题请按 [SECURITY.md](SECURITY.md) 中的方式私下报告，不要在公开 Issue 中提交凭据、服务器地址或运行日志。

## 许可证

本项目使用 [Apache License 2.0](LICENSE)。白泽平台本身适用其各自的许可条款。

