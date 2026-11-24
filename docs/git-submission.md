# Git提交范围

## 应提交

- `frontend/`：Vue源码、样式、HTML入口、Vite配置。
- `internal/`：Go核心模块、同目录单元测试；`webassets/assets.go`与`static/README.txt`要提交，`static/dist/`不用提交。
- `main.go`、`cmd/hash-password/`：Go入口与哈希工具。
- `go.mod`、`go.sum`：固定Go依赖。
- `package.json`、`pnpm-lock.yaml`：前端依赖与统一命令。
- `.gitignore`、`.dockerignore`：忽略规则。
- `config.example.yaml`：无默认密码/Key的配置模板。
- `deploy/`：Dockerfile、发布与部署说明、无秘密的环境示例。
- `tests/`：浏览器测试、Go模拟服务、数据排序测试；固定测试凭据仅用于测试固件，不能用于生产。
- `docs/`：说明文档、`docs/headscale/docker-compose.yml`、`docs/headscale/examples/`脱敏模板。
- `README.md`。

## 不提交

| 类型 | 路径/文件 |
|---|---|
| 实际管理配置 | 根config.yaml、config.local.yaml |
| 真实Headscale配置 | docs/headscale/config/config.yaml |
| 真实管理部署配置 | docs/headscale/web/config.yaml、config.docker.yaml |
| 真实Caddy站点路由 | docs/headscale/Caddyfile |
| 凭据 | secrets/、*.secret、*.hash、*.key、*.pem、.env |
| 数据库与数据 | docs/headscale/data/、*.sqlite及WAL/SHM、*.db |
| 构建与测试产物 | build/、internal/webassets/static/dist/、测试报告 |
| 依赖与IDE | node_modules/、.idea/、*.iml |
| 历史实验 | archive/，包括旧Node、原型、浏览器缓存 |

实际配置已经留在原位置，没有删除；Git只用examples模板。不要将真实Key或哈希挪到一个不在忽略规则中的文件。

## 提交前流程

在项目根目录：

```sh
git status --short
git status --ignored --short
go test ./...
go vet ./...
pnpm build
pnpm test:ui
```

查看候选文件，不执行提交：

```sh
git ls-files --others --exclude-standard
```

确认后可以添加明确范围：

```sh
git add .gitignore .dockerignore README.md main.go go.mod go.sum package.json pnpm-lock.yaml config.example.yaml frontend internal cmd deploy docs tests
git diff --cached --stat
git diff --cached --name-only
# 在本机仔细检查暂存内容，不把含秘密的diff贴到聊天
# git diff --cached
```

当前工程初始为空索引，先检查再提交，不需要archive/build/node_modules。`.gitignore`只影响未跟踪文件；如果以后某个秘密已被跟踪，新增忽略规则并不能从历史删除它，应先停止传播并轮换凭据，再按需要清理索引/历史。

检查某个文件的忽略原因：

```sh
git check-ignore -v config.yaml docs/headscale/secrets/headscale-api-key.secret docs/headscale/web/config.yaml
```

## 干净克隆可运行性

- `go test ./...`不需要真实部署文件：部署测试使用examples模板。
- `internal/webassets/static/README.txt`确保没有dist时Go测试能编译。
- 正式Go二进制编译前执行`pnpm build`，嵌入生成的前端。
- 运行真实模式前从模板创建配置并设置自己的密码哈希/Key；不会自动提供默认生产凭据。

本次整理不会执行git add、commit或push，最终是否公开部署域名和文档内容由你审查。
