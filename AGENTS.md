# PaneNest 协作规范

## 项目身份

- 产品名称 PaneNest，命令行 `panenest`，后台服务 `panenestd`。
- 应用标识 `local.abc.panenest`，环境变量前缀 `PANENEST_`。
- 开发版和正式版使用独立的数据、配置、服务和应用标识。
- 发布仓库和公钥留空时停用自动更新。不得重新接入上游更新源、签名证书或数据目录。
- 保留 MIT 许可证及原始版权声明。

## 开发与验证

- 说明、回答和新增注释优先使用中文；Go 注释遵循 Go 的文档注释规范。
- 直接在当前分支工作；未经明确要求，不创建或切换分支，不创建 Git 提交。
- 保留用户已有改动。未经明确要求，不新建测试文件。
- `make dev` 启动开发版，`make build` 构建本机应用。
- 根据改动范围执行已有检查；涉及 MyGo 时参考 `.agents/skills/mygo-maintenance/SKILL.md`。
