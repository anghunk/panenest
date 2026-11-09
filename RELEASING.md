# PaneNest 的构建与发布

当前以本地构建为主。`mygo.json` 使用本地签名（`-`），未配置自动更新源、上游开发者证书或公证凭据。

## 本机使用

1. 修改 `mygo.json` 中的版本号。
2. 执行 `make check` 和 `make build`。
3. 退出 PaneNest，使用构建结果替换「应用程序」中的同名应用。
4. 重新打开；如果提示后台服务版本变化，按界面提示重启服务。

开发期间使用 `make dev`，其后台服务和会话数据均独立保存。

## GitHub 自动打包与发布

修改 `mygo.json` 的 `version`（例如从 `0.1.4` 改为 `0.1.5`），提交并推送到 `main` 分支后，Release 工作流会自动：

1. 检查版本号是否变化，并运行已有的内部测试。
2. 构建 macOS、Windows、Linux 的 x64 和 ARM64 版本。
3. 收集 macOS `.dmg`、Windows 安装程序 `.exe`、Linux `.deb` 和 `.tar.gz`，生成 `SHA256SUMS`。
4. 创建对应的 `v版本号` 标签和 Release，上传全部安装包后公开发布。

只修改配置而没有修改版本号时跳过发布。也可推送与 `mygo.json` 版本一致的 `v*` 标签，或在 Actions 中手动运行 Release 工作流。已公开版本不会被覆盖；失败后可重新运行，未发布的草稿会继续上传。

流程使用仓库自带的 `GITHUB_TOKEN`，无需设置 `PANENEST_RELEASES_ENABLED` 或额外的签名密钥。仓库需要允许 Actions 运行，且仓库或组织策略允许工作流使用 `contents: write` 权限。macOS 沿用本地签名，不执行 Developer ID 签名或 Apple 公证；Windows 安装包也没有代码签名。下载后的运行验证仍需在各自平台完成。

自动发布仅分发桌面应用，不上传远程服务端独立包，也不启用应用内自动更新。Server build 工作流继续用于独立服务端的构建与验证。旧的 Publish release 手动流程仅用于维护历史草稿，不属于新流程的必要步骤。

## 配置自己的应用内自动更新

如果后续需要应用内更新，应单独配置自有 GitHub 仓库与签名凭据：

- 在 `mygo.json` 设置自有 `updates.github` 和更新公钥。
- 同步设置 `internal/update/update.go` 的 `Repo` 与 `PublicKey`。
- 同步设置 `scripts/install-server.sh` 和 `scripts/install-server.ps1` 的发布仓库。
- 增加更新归档及服务端程序的签名、上传和验证步骤。
- 对外分发需要签名、公证的 macOS 应用时，配置自己的 Developer ID 和公证信息。

这些配置均应使用自己的仓库和密钥。更新归档和服务端程序使用 `panenest`、`panenest-server` 命名。

原始源码版本为 Kero v2.1.27，历史上游变更记录保留在 `CHANGELOG.md`。
