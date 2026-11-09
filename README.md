## PaneNest

> **二次开发说明：PaneNest 基于开源项目 [Kero（egoist/kero）](https://github.com/egoist/kero) 二次开发并独立维护。**
>
> 基于原项目 v2.1.27（`4ebd0de`），沿用 MIT 许可证并保留原始版权声明，详见 [LICENSE](LICENSE)。

面向个人长期使用的终端与编程代理工作台。每个项目可以保存多个会话、分屏窗格和浏览器页面；后台服务负责保留会话。


## 开发

需要 Go 1.27.3。启用 `GOTOOLCHAIN=auto` 时，现有 Go 会自动获取所需工具链。macOS 本地构建使用 Xcode 工具。

```sh
make dev
```

开发应用显示为 **PaneNest Dev**。修改 Go 代码、翻译或资源后会自动重新编译、重启。桌面 UI 使用 Go / MyGo；`website/` 和 `docs/` 是保留的上游网站、文档源码，使用 Bun 单独开发。

```sh
make check
make build
```

Apple 芯片 Mac 的构建结果是 `build/darwin-arm64/PaneNest.app`。构建默认使用本地签名，适合本机使用。开发版构建位于 `.mygo/dev/`。

## 使用

将构建的 `PaneNest.app` 放入「应用程序」，然后打开。按 `⌘O` 添加项目，选择 **PaneNest → Install Command Line Tool…** 安装命令行。

```sh
panenest new -n dev-server -d /path/to/project -- npm run dev
panenest ls
panenest config path
```

## 源码位置

- `side.go`：左侧项目、会话列表。
- `view.go`、`panes.go`：主界面和窗格布局。
- `settings_view.go`、`internal/settings/`：设置界面和配置文件。
- `internal/daemon/`：会话生命周期、持久化和后台服务。
- `internal/cli/`：命令行、编程代理集成。
- `locales/zh-Hans.json`：中文文案；`catalog_gen.go` 由 MyGo 生成。
- `mygo.json`：名称、标识、版本、签名和打包配置。

## 更新与上游同步

更新 `mygo.json` 的版本号并推送到 `main` 后，GitHub Actions 会自动构建 macOS、Windows、Linux 的 x64 和 ARM64 安装包并发布 Release；也可通过 `make build` 构建本机应用。桌面与服务端均未配置自动更新仓库、公钥，应用内自动更新默认停用。发布流程和更新配置见 [RELEASING.md](RELEASING.md)。

Git 的 `upstream` 指向原始项目。可以用 `git fetch upstream` 查看上游更新，在审阅差异后自行合并。应用名称、数据目录、环境变量、服务标识和更新配置是独立维护的边界。

`docs/`、`website/` 和历史 `CHANGELOG.md` 保留上游资料，其中的产品名和安装命令对应原始项目。
