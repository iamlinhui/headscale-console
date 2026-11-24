# 构建与运行

本页面向开发者，介绍二进制和镜像构建。服务器配置见 [Headscale 部署指南](<../docs/headscale/README.md>)。

## 本地运行

需要 Go 1.26+、Node 22+、pnpm 10。项目根目录执行：

```sh
pnpm install --frozen-lockfile
pnpm build
pnpm preview
```

打开 [本地预览](http://127.0.0.1:3080/)，使用终端输出的随机账号密码。这是演示模式，不连接真实 Headscale。

真实模式：从 [配置模板](<../config.example.yaml>)创建自己的配置，填写密码哈希、Headscale 地址与管理 Key，然后执行：

```sh
go run . -config config.yaml
```

开发热更新：将 `server.origin` 设为 `http://127.0.0.1:5173`，分别运行 `go run .` 和 `pnpm dev`，打开 [前端开发页面](http://127.0.0.1:5173/)。

## 二进制构建

```sh
pnpm build:all
```

前端先构建，再通过 Go embed 嵌入二进制；产物位于 `build/bin/`。运行只需要二进制、配置和秘密文件，不需要 Node。

## Docker 镜像

```sh
pnpm docker:build
```

本地镜像标签为 `docker.io/promptness/headscale-console:latest`。版本标签、多架构和推送步骤见 [镜像发布说明](<docker/publish.md>)。

服务器使用唯一 [Compose](<../docs/headscale/docker-compose.yml>)拉取镜像，不在服务器上构建源码。

## 验证

```sh
go test ./...
go vet ./...
pnpm test:ui
```

浏览器测试需要本机 Chrome，使用隔离的模拟服务，不操作生产 Headscale。

## 运行要求

- 生产使用 HTTPS，`server.origin` 必须与浏览器实际域名一致，不含路径。
- 真实 Key、密码哈希和数据库不提交 Git、不放进镜像。
- 默认镜像以 UID/GID 65532 运行，挂载配置与 Secrets 必须可读。
- 默认 Docker 部署仅支持 DNS 只读；没有跨容器重启或配置写入工具。
