.PHONY: dev build check

# 启动具有独立数据目录的开发版，源码变化后自动重编译。
dev:
	go tool mygo dev

# 使用本机架构和本地签名构建应用。
build:
	go tool mygo build -skip-dmg

# 检查 MyGo 界面约束并运行已有测试。
check:
	go tool mygo vet .
	CGO_ENABLED=0 go test ./...
