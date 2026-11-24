# Headscale Console

基于 **Vue 3 + Naive UI + Go** 的 Headscale 管理控制台。前后端分离开发，生产将前端资源嵌入 Go 二进制，一个镜像提供页面和同源 API。

## 功能与适用范围

- 设备、用户、路由和设备预授权密钥管理。
- ACL / Grants 源码编辑、服务端校验、差异预览与发布。
- 独立账号密码登录、动态 GIF 验证码、HttpOnly 会话、CSRF 与限流。
- 桌面折叠侧栏和 H5 设备卡片。
- DNS / MagicDNS **默认只读**，不宣称通过普通 API 热更新主配置。

当前 API 适配 Headscale **0.28.x / 0.29.x**，部署模板固定 **0.29.7**；策略在线发布要求 **0.29.x + database 模式**。真实模式连接失败不会回退到模拟数据。

## 一、快速启动看效果

需要 Go **1.26+**、Node **22+**、pnpm **10**。先将本仓库克隆到本机，再在项目根目录执行：

```sh
pnpm install --frozen-lockfile
pnpm build
pnpm preview
```

打开 [本地预览页面](http://127.0.0.1:3080/)。终端显示临时账号与随机密码，填写图形验证码后登录。

这是**演示模式**，不连接真实 Headscale。重启会更换临时密码；验证码仍由 Go 后端校验。停止服务使用 `Ctrl+C`。

## 二、接入你自己的 Headscale

### 1. 生成控制台配置与密码哈希

项目根目录执行：

```powershell
Copy-Item config.example.yaml config.yaml
pnpm hash-password
```

哈希工具会交互输入密码，不回显。将输出填入 `auth.password_hash`；也可保存至受保护文件并使用 `password_hash_file`。**不要同时设置两者**。

### 2. 准备 Headscale 管理 API Key

在部署 Headscale 的服务器上执行，容器名按你的环境替换：

```sh
docker exec headscale headscale version
docker exec headscale headscale apikeys create --expiration 90d
```

保存输出的 Key。它是**管理端后端凭据**，不要给普通设备或放进前端代码。此命令不是设备 Auth key 的创建命令。

### 3. 修改 config.yaml

下面是本机使用真实数据的最小配置，替换域名和密码哈希：

```yaml
environment: development
demo: false
server:
  host: 127.0.0.1
  port: 3080
  origin: http://127.0.0.1:3080
  secure_cookie: false
auth:
  username: admin
  password_hash: "填写生成的scrypt哈希"
  password_hash_file: ""
headscale:
  url: https://headscale.example.com
  public_url: https://headscale.example.com
  api_key: ""
  api_key_file: ./secrets/headscale-api-key.secret
  policy_mode: database
configuration:
  path: ""
  write_enabled: false
  test_executable: ""
  apply_executable: ""
```

将真实管理 Key 保存为 `secrets/headscale-api-key.secret` 的一行文本，并限制文件读取权限。所有相对文件路径以 **YAML 所在目录** 为基准。

- `demo: false` 才请求真实数据；**不要用 preview 启动真实模式**。
- `headscale.url` 是 Go 后端能访问的地址；同 Docker 网络可用 `http://headscale:9981`。
- `public_url` 是设备客户端能访问的 HTTPS 地址，不能使用容器名、管理页面地址或 `/admin`。
- `policy_mode` 必须与 Headscale 实际运行的模式一致。Headscale 仍是 file 模式时就填 file；只改管理端不会切换服务端。
- DNS 配置未挂载时会显示未配置，不妨碍设备、用户和路由 API。不要把本地备份当作远端实时配置。

### 4. 启动

```sh
pnpm build
go run .
# 或指定配置位置
go run . -config /path/to/config.yaml
```

浏览器打开 [本地控制台页面](<http://127.0.0.1:3080/>)（`http://127.0.0.1:3080/`），使用你设置的控制台账号密码登录。连接成功后查看真实设备、用户、路由；后端不会将管理 Key 返回浏览器。

## 三、开发调试

1. 配置文件的 `server.origin` 改为 `http://127.0.0.1:5173`。
2. 终端一启动 Go：`go run .`。
3. 终端二启动前端：`pnpm dev`。
4. 浏览器打开 [前端开发页面](<http://127.0.0.1:5173/>)（`http://127.0.0.1:5173/`）。

Vite 将 `/api` 代理到 Go 的 3080 端口。前端修改热更新；Go 修改后需重启后端。Vite 固定 5173，端口冲突会报错而不会自动换端口。

验证码出现 403 时，检查实际浏览器 Origin 与配置一致，不混用 `localhost` 和 `127.0.0.1`，并确认旧 preview 服务已经停止。浏览器 DevTools 不等于前端热更新；访问 Go 的嵌入页面时需要重新构建并重启才能看到前端变更。

GoLand 调试根包 `meshconsole`，工作目录设项目根目录，程序参数可用 `-config config.yaml`。环境变量仍可覆盖 YAML，优先级：**环境变量 > YAML > 默认值**。Go不自动读取 `.env`。

## 四、Docker 部署

镜像：`docker.io/promptness/headscale-console`。生产运行不需要 Node 或独立前端目录。

唯一部署入口为 [docker-compose.yml](<docs/headscale/docker-compose.yml>)。它同时定义 Headscale 与管理端，Caddy 沿用宿主服务。**这个文件是模板，不要直接覆盖已有服务器数据或套用别人的域名。**

### 1. 从 Git 模板创建你的部署配置

实际配置与 Secrets 不提交 Git，因此其他人克隆后需先生成自己的文件：

```powershell
New-Item -ItemType Directory -Force docs/headscale/config, docs/headscale/web, docs/headscale/secrets
Copy-Item docs/headscale/examples/headscale.config.example.yaml docs/headscale/config/config.yaml
Copy-Item docs/headscale/examples/console.docker.example.yaml docs/headscale/web/config.docker.yaml
Copy-Item docs/headscale/examples/console.local.example.yaml docs/headscale/web/config.yaml
Copy-Item docs/headscale/examples/Caddyfile.example docs/headscale/Caddyfile
```

**已有文件先备份，不要执行覆盖复制。** 修改模板里的 `headscale.example.com`、DERP 公网 IP 和代理设置，再准备：

```text
secrets/console-password.hash       你自己的控制台密码哈希
secrets/headscale-api-key.secret    你自己的Headscale管理API Key
```

默认生产容器 UID/GID 为 `65532`，Secrets 和挂载配置必须可读。具体权限说明见 [部署配置指南](<docs/headscale/README.md>)。

### 2. 调整 Compose 与 Caddy

- Compose 的 `CONSOLE_ORIGIN` 改成你的实际 HTTPS 域名，或通过 `.env` 覆盖；**不要沿用模板默认的示例部署域名**。
- Docker管理端配置的 `public_url` 与 Headscale `server_url` 使用你的公网地址。
- Docker管理端请求地址为 `http://headscale:9981`；若接入另一Compose中的Headscale，必须明确共享网络，防止同名容器冲突。
- 宿主 Caddy 将 `/admin` 重定向到 `/admin/`，`handle_path /admin/*` 剥离前缀后转发管理端9983，其余请求转发Headscale9981。
- `server.origin` 只填写 `https://你的域名`，**不带 `/admin`**。
- 提供的 Caddyfile 是站点内片段，需合入完整配置后 validate/reload；若 Caddy 在容器中，不能把容器的127.0.0.1当作宿主。

### 3. 拉取与启动

在部署服务器的配置目录中设置已发布镜像标签，例如 `.env`：

```dotenv
HEADSCALE_CONSOLE_TAG=latest
CONSOLE_ORIGIN=https://headscale.example.com
```

项目根目录命令：

```sh
docker compose -f docs/headscale/docker-compose.yml config --quiet
docker compose -f docs/headscale/docker-compose.yml pull headscale-console
# 现有Headscale已由此Compose运行时，仅启动/更新管理端
docker compose -f docs/headscale/docker-compose.yml up -d --no-deps headscale-console
```

全新环境或需要切换 Headscale 配置时，先备份并校验，再按 [部署配置指南](<docs/headscale/README.md>)启动完整环境。修改 bind-mounted 配置不会自动重启已有容器；切换策略模式须明确重新加载。**不要执行 `down -v` 清理原数据。**

镜像只有真正发布后才能从 Docker Hub 拉取。开发者构建、标签与推送见 [镜像发布说明](<deploy/docker/publish.md>)；不会因为运行上述命令自动创建管理 API Key或默认密码。

## 五、普通用户如何接入设备

管理页面账号密码只用于控制台。普通设备安装官方 **Tailscale 客户端**，接入你的 **Headscale 公网地址**，不填控制台 `/admin` 地址，也不使用管理 API Key。

### 有设备预授权密钥

管理员在“认证密钥”中创建短时、单次设备 Key，或在“添加设备”向导生成；也可以用已有设备 Key生成接入命令。演示模式生成的 Key 无效。

Windows / macOS 终端：

```sh
tailscale up --login-server=https://headscale.example.com --auth-key=YOUR_DEVICE_AUTH_KEY
```

命令参数以本机 `tailscale up --help` 为准。密钥可能进入终端历史，不在共享电脑执行，不把真实Key贴到聊天/工单。不要随意使用 `--reset` 或重新认证，以免中断现有SSH/RDP。

### 浏览器注册

| 系统 | 自定义服务器入口 |
|---|---|
| Android | Tailscale → 设置 → Accounts → 三点菜单 → Use an alternate server |
| iOS / iPadOS | 账号 → Log in → 选项菜单 → Use custom coordination server |
| macOS | Option点击菜单栏图标 → Debug → Custom Login Server → Add Account；或下面CLI |
| Windows | 安装官方客户端后执行下面CLI |

```sh
tailscale login --login-server=https://headscale.example.com
```

打开注册页面后按该实例指引完成；未配置OIDC时通常需要Headscale管理员批准。管理员在**服务器**核对 `headscale auth register --help` 后按注册页给出的 Auth ID 执行批准。本管理端目前没有完整的交互注册批准页面，可以使用服务器CLI。客户端允许VPN配置后会接入并出现在设备列表。

Android也可在设置自定义服务器后从 Accounts 菜单选择 Use an auth key。iOS菜单可能因版本变化，不假定有同样的Key输入入口。官方操作参考：[Android](https://headscale.net/stable/usage/connect/android/)、[Apple](https://headscale.net/stable/usage/connect/apple/)、[Windows](https://headscale.net/stable/usage/connect/windows/)。

接入完成可以使用 `tailscale status`、`tailscale ip -4` 和实际业务服务验证。在线不代表ACL、防火墙、子网路由都已放行。MagicDNS默认可提供 `设备名.你配置的基础域名`；退出节点需独立批准并由客户端选用。

## 六、策略与 DNS 的生效方式

| 设置 | 当前方式 |
|---|---|
| ACL / Grants | Headscale数据库；页面校验、差异、确认后API发布，不需要每次重启 |
| 切换file/database模式 | 主配置改变，先备份并明确重启/重建Headscale |
| MagicDNS、基础域名、全局DNS | 主配置；默认Docker部署仅只读查看，不可只改开关就动态保存 |
| 自定义A/AAAA外置记录 | Headscale可监视外置记录文件，但当前页面尚未实现完整记录编辑功能 |

首次database尚无记录时可以在源码创建已审查策略；空策略不代表默认隔离。不要直接把示例允许全部或拒绝全部发布到生产。

## 七、构建与测试

```sh
pnpm build:all       # 前端先构建，再嵌入Go二进制
go test ./...
go vet ./...
pnpm test:ui         # 需要本机Chrome
```

前端生成到 `internal/webassets/static/dist/`；二进制、截图和报告进入 `build/`。浏览器测试使用隔离的本地Go固件与模拟Headscale，不使用你的真实Key或远端服务器。

## 工程结构

```text
frontend/                 Vue源码、样式、Vite配置
internal/                 Go认证、验证码、配置、Headscale API、HTTP路由
main.go                   Go服务入口
cmd/hash-password/        密码哈希工具
config.example.yaml       配置模板；实际config.yaml不提交
tests/                    单元测试、浏览器测试与模拟服务
deploy/docker/            Dockerfile、发布说明
docs/headscale/           唯一Compose、说明、脱敏examples模板
build/                    生成产物，不提交
archive/                  历史实验，不参与构建、不提交
```

Git提交范围与排除Secrets/数据库规则见 [Git提交说明](<docs/git-submission.md>)。新克隆不包含你的实际配置和凭据，必须按模板设置后再启用真实模式。

## 当前边界

- 上游Headscale没有原子CAS；本进程锁与内容比对减少冲突，但不要多个UI同时写同一策略/路由。
- 会话、验证码和确认令牌为单实例内存，重启失效；多副本需共享状态。
- 生产默认不挂Docker socket，不提供跨容器重启权限。DNS主配置写入联动尚未作为开箱功能。
- 完整操作审计、多管理员RBAC、Agent、浏览器SSH、端到端监控未实现。
- 示例与测试不等于目标服务器已部署、镜像已发布或四平台真机已逐一验收。
