# Headscale Docker 部署

本目录提供一份 Compose，运行 Headscale 和管理端。镜像构建与发布见 [构建说明](<../../deploy/README.md>)。

## 配置约定

| 项目 | 设置 |
|---|---|
| Headscale | `headscale/headscale:0.29.7` |
| 管理端镜像 | `promptness/headscale-console`，可用 `HEADSCALE_CONSOLE_TAG` 指定标签 |
| 客户端地址 | `https://headscale.holmes.cn` |
| 管理页面 | [Headscale Console](https://headscale.holmes.cn/admin/) |
| 代理 | 宿主 Caddy；Headscale 上游 9981，管理端上游 9983 |
| STUN | UDP 3478 |
| 访问策略 | `database`，通过 API 校验与发布 |
| DNS / MagicDNS | 共享配置文件，只读查看 |

更换域名时，同步修改 Headscale `server_url`、管理端 `public_url`、`server.origin` 和 Caddy。Origin 不包含 `/admin`。

## 文件用途

```text
headscale/
├─ docker-compose.yml          唯一部署入口
├─ config/config.yaml          Headscale 配置
├─ web/config.docker.yaml      Docker 管理端配置
├─ web/config.yaml             本机调试配置
├─ Caddyfile                   宿主站点内的路由片段
├─ examples/                   可提交的脱敏模板
├─ secrets/                    管理 Key 与密码哈希，不提交
└─ data/                       Headscale 持久化数据，不提交
```

新克隆先从 `examples/` 创建实际配置并替换示例域名、DERP 公网 IP。初始化步骤见 [项目启动指南](<../../README.md>)；已有配置不要覆盖。

## 1. 准备配置与权限

凭据文件：

- `secrets/console-password.hash`：通过 `pnpm hash-password` 生成的控制台密码哈希。
- `secrets/headscale-api-key.secret`：Headscale 管理 API Key，不是设备预授权密钥。

管理端以 UID/GID 65532 运行，Secrets 和挂载配置必须可读。Linux 可采用专用读组：

```sh
sudo chown root:65532 secrets/console-password.hash secrets/headscale-api-key.secret
sudo chmod 0640 secrets/console-password.hash secrets/headscale-api-key.secret
```

按服务器权限环境调整父目录访问权限；不要把秘密设为所有用户可读。保留原 `data/` 及其身份权限，不清空数据库。

可在本目录 `.env` 中指定：

```dotenv
HEADSCALE_CONSOLE_TAG=latest
CONSOLE_ORIGIN=https://headscale.holmes.cn
```

使用已经发布到 Docker Hub 的镜像标签，生产建议固定版本。

## 2. 检查与启动

以下命令在本目录执行：

```sh
docker compose config --quiet
docker compose pull headscale-console
docker compose run --rm --no-deps headscale configtest
```

### 只更新管理端

```sh
docker compose up -d --no-deps headscale-console
```

### 首次部署或切换 Headscale 配置

先备份数据库、配置与已有策略，再在维护窗口执行：

```sh
docker compose up -d --force-recreate headscale headscale-console
```

修改挂载配置文件不会自动触发容器重启。SQLite WAL 模式下不要运行中只复制 `db.sqlite` 当作完整备份；使用一致性备份或停服备份整个数据目录。

已有 Headscale 来自另一 Compose 时，先处理项目网络和同名容器，不直接启动第二套。不要执行 `docker compose down -v` 删除原数据。

## 3. Caddy 路由

将 [路由模板](<examples/Caddyfile.example>)或本机实际 Caddy 片段合入宿主完整站点配置：

- `/admin` 跳转到 `/admin/`。
- `/admin/*` 剥离前缀后转发到 `127.0.0.1:9983`。
- 其余注册、Noise、`/api/v1`、DERP 请求转发到 `127.0.0.1:9981`。

```sh
caddy validate --config /实际路径/Caddyfile
caddy reload --config /实际路径/Caddyfile
```

片段不能单独作为完整 Caddyfile。若 Caddy 运行在容器内，必须调整网络和上游地址，不能使用宿主方案的 `127.0.0.1`。

## 4. 验收

```sh
docker compose ps
docker compose logs --tail=100 headscale
docker compose logs --tail=100 headscale-console
```

打开 [管理页面](https://headscale.holmes.cn/admin/)，确认：

- 账号密码和验证码登录正常。
- 设备、用户、路由为真实数据。
- 访问策略可读取、校验、确认发布；空策略不代表默认隔离。
- DNS 配置可读，保存禁用。当前镜像未提供主配置写入和跨容器重启工具。

客户端仍连接 `https://headscale.holmes.cn`，不使用 `/admin`。控制台健康不代表设备服务、DERP 或 DNS 一定可达，需在客户端验证。

## 本机调试

项目根目录执行：

```sh
pnpm build
go run . -config docs/headscale/web/config.yaml
```

打开 [本地控制台](http://127.0.0.1:3080/)。Vite 调试时把 `server.origin` 改成 `http://127.0.0.1:5173`，运行 `pnpm dev` 后打开 [前端开发页面](http://127.0.0.1:5173/)。

本机 DNS 页面读取的是本地配置副本，不自动同步远端。真实部署文件和凭据不提交 Git，具体见 [提交清单](<../git-submission.md>)。
