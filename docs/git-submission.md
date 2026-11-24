# Git 提交范围

## 提交

| 内容 | 路径 |
|---|---|
| 前端源码 | `frontend/` |
| Go 源码与测试 | `main.go`、`internal/`、`cmd/`、`tests/` |
| 依赖与锁定文件 | `go.mod`、`go.sum`、`package.json`、`pnpm-lock.yaml` |
| 构建与部署 | `deploy/`、`docs/headscale/docker-compose.yml` |
| 脱敏模板 | `config.example.yaml`、`docs/headscale/examples/` |
| 文档与忽略规则 | `README.md`、`docs/`、`.gitignore`、`.dockerignore` |

Go 前端资源包中的 `assets.go` 和 `static/README.txt`要提交；生成的 `static/dist/` 不提交。

## 不提交

- 实际配置：根 `config.yaml`、`config.local.yaml`，Headscale 的 `config/config.yaml`、`web/config*.yaml`、实际 `Caddyfile`。
- 凭据与数据库：`secrets/`、`.env`、`*.secret`、`*.hash`、`*.key`、`*.pem`、数据库及 WAL/SHM。
- 生成与历史文件：`build/`、`internal/webassets/static/dist/`、`node_modules/`、测试报告、`archive/`。
- IDE 文件：`.idea/`、`*.iml`。

实际部署文件留在本机，Git 提交脱敏模板。新克隆通过模板创建自己的配置；测试不依赖真实 Key。

## 提交前检查

项目根目录执行：

```sh
git status --short
git ls-files --others --exclude-standard
git check-ignore -v config.yaml docs/headscale/secrets/headscale-api-key.secret
```

确认源码范围后添加并检查暂存区：

```sh
git add .gitignore .dockerignore README.md main.go go.mod go.sum package.json pnpm-lock.yaml config.example.yaml frontend internal cmd deploy docs tests
git diff --cached --stat
git diff --cached --name-only
```

提交前在本机检查 `git diff --cached`，不要将包含秘密的内容贴到聊天或工单。

注意：`.gitignore` 不能保护已经跟踪的秘密文件。若 Key 曾被提交，应先轮换凭据，再清理索引或历史。
