# Headscale 最终部署配置

## 最终约定

| 项目 | 配置 |
|---|---|
| Headscale | `headscale/headscale:0.29.7` |
| 管理端镜像 | `docker.io/promptness/headscale-console:<发布标签>`，默认latest |
| 客户端接入 | `https://headscale.holmes.cn` |
| 管理页面 | `https://headscale.holmes.cn/admin/` |
| 宿主代理 | 已有Caddy，保留现有TLS/站点 |
| Headscale HTTP | 宿主127.0.0.1:9981；同Compose内headscale:9981 |
| 管理端HTTP | 宿主127.0.0.1:9983 → 容器3080 |
| STUN | 公网UDP3478，映射保持不变 |
| 访问策略 | database，Web校验/发布 |
| DNS / MagicDNS | Headscale配置文件，管理端只读 |

**这是最终部署目标，不表示服务器已更新。** 本次未连接或重启远端，没有发布ACL，没有推送Docker Hub镜像。

## 文件职责

```text
headscale/
├─ docker-compose.yml          唯一Docker部署入口
├─ Caddyfile                   原Caddy站点内部路由片段
├─ config/config.yaml          Headscale v0.29.7服务端配置
├─ web/config.docker.yaml      容器Go管理端配置（生产）
├─ web/config.yaml             本机Go管理端调试配置
├─ secrets/                    API Key / 密码哈希，私密，不提交
└─ data/                       部署服务器现有Headscale持久化数据，不提交
```

部署到Linux时复制Compose、config/、web/config.docker.yaml、secrets/；保留服务器原data/。Caddy片段合入实际完整站点，不能单独拿片段启动Caddy。

## Git模板与首次克隆

`config/config.yaml`、`web/config*.yaml`、实际Caddyfile、secrets/、data/都是本地部署文件，已加入忽略规则。可提交的是 `examples/` 下的脱敏模板和本Compose/说明。域名与公网IP虽不是密码，也不应作为通用模板的默认值。

新克隆仓库需要先创建实际配置（已存在的部署文件不要覆盖）：

```powershell
New-Item -ItemType Directory -Force docs/headscale/config, docs/headscale/web, docs/headscale/secrets
Copy-Item docs/headscale/examples/headscale.config.example.yaml docs/headscale/config/config.yaml
Copy-Item docs/headscale/examples/console.docker.example.yaml docs/headscale/web/config.docker.yaml
Copy-Item docs/headscale/examples/console.local.example.yaml docs/headscale/web/config.yaml
Copy-Item docs/headscale/examples/Caddyfile.example docs/headscale/Caddyfile
```

将模板域名、公网IP、代理信任范围改成自己的实际值；生成或提供自己的Secrets。Go测试只读取模板和临时测试凭据，不要求私密文件存在。镜像构建同样不需要真实部署配置。

## 1. 发布管理端镜像（开发电脑）

需Docker引擎运行，并拥有Docker Hub promptness命名空间推送权限。仓库名为 `promptness/headscale-console`，实际页面通常是 `https://hub.docker.com/r/promptness/headscale-console`。

项目根目录：

```sh
docker login docker.io
# 替换0.2.0为本次正式发布标签；不要推送未验证版本
# Dockerfile会构建Vue，再将资源嵌入Go二进制
# 如需多架构见deploy/docker/publish.md
docker build -f deploy/docker/Dockerfile -t promptness/headscale-console:0.2.0 -t promptness/headscale-console:latest .
docker push promptness/headscale-console:0.2.0
docker push promptness/headscale-console:latest
```

**当前没有执行这些push命令，不能确认镜像已可拉取。** 不把本目录secret/config私密信息放入构建上下文；Dockerfile只复制有效源码，.dockerignore排除docs与secrets。

## 2. 部署前检查（服务器）

进入此目录。`.env` 可选：

```dotenv
HEADSCALE_CONSOLE_TAG=0.2.0
CONSOLE_ORIGIN=https://headscale.holmes.cn
```

标签必须是你真正推送过的版本。Headscale固定0.29.7，不随HEADSCALE_CONSOLE_TAG改变。Origin不带/admin路径。

```sh
docker compose config --quiet
docker exec headscale headscale version
# 先拉取已经发布的管理端镜像；不是在服务器上构建源码
docker compose pull headscale-console
# Headscale配置校验使用指定0.29.7镜像；读取配置，不启动serve
# 注意检查data目录中的密钥/数据库文件属于原环境
docker compose run --rm --no-deps headscale configtest
```

Docker Hub仓库尚未发布、secret权限错误或挂载路径缺失时，先处理再启动。

### Secrets与文件权限

- `secrets/console-password.hash`：现有控制台密码哈希，不是明文。
- `secrets/headscale-api-key.secret`：现有Headscale管理Key，不是设备Auth key。
- YAML只引用secret文件，整理未轮换原凭据。
- 运行镜像用户为65532，必须能读Secrets和两份共享YAML。Compose本地secrets采用bind，不要假定mode声明会修改宿主文件权限。
- 建议宿主secret文件归root、组65532，权限0640；父目录限制为root与该组可访问。根据你的Linux权限/SELinux/rootless环境调整，避免简单公开全员读取。

Linux可按实际账号权限执行：

```sh
sudo chown root:65532 secrets/console-password.hash secrets/headscale-api-key.secret
sudo chmod 0640 secrets/console-password.hash secrets/headscale-api-key.secret
# 不更改Headscale data数据库身份；配置共享YAML需对各自容器身份可读
```

必要时在服务器检查两容器日志确认配置能读，日志/截图不要暴露Key。旧明文配置若曾公开，应由你轮换Key。

## 3. 策略database迁移与启动

本地三份配置已统一为database。部署前备份原配置、数据库、现有策略。SQLite WAL模式不能运行中只拷贝db.sqlite作为完整备份；使用一致性SQLite备份或维护窗口停服备份data/。

- 域名、容器内端口、DERP区域、公网IP、IP分配段和原data路径保持不变。
- HTTP宿主端口收敛到loopback，外部只走Caddy；若还有其它机器直接访问9981，应先调整访问方式。
- 主配置切换policy.mode需要由你确认重启Headscale。
- 首次database模式若无记录，管理端识别官方明确“acl policy not found”后显示空编辑器，允许服务端校验后的首次发布；不把其它500/鉴权错误当作空策略。
- 空策略不意味着隔离，不直接发布demo规则或拒绝全部规则。也可在服务器用`headscale policy check --file /etc/headscale/已审核策略.hujson`后`headscale policy set --file ...`初始化，操作前确认实际用户和标签。
- 修改bind-mounted YAML内容不会自动触发Compose重启；迁移时明确使用下面的force-recreate，让两服务读取最终配置。

确认备份与校验后，按环境选择：

```sh
# 本Compose管理整个环境时，应用Headscale配置和新管理端
docker compose up -d --force-recreate headscale headscale-console
# 仅升级管理端，不改变Headscale配置时
docker compose up -d --no-deps headscale-console
```

原Headscale来自别的Compose项目时，先明确迁移/网络项目名和已存在container_name，不能直接造同名新容器。原Headplane占9983时由你停止旧UI，避免冲突。没有自动删除data或docker compose down -v的步骤。

## 4. Caddy同域路由

[Caddyfile](<Caddyfile>) 是已有站点内部片段：

1. `/admin` → 308重定向 `/admin/`。
2. `handle_path /admin/*`剥离/admin，转发127.0.0.1:9983。
3. `/admin/api/...`在Go中收到`/api/...`，不会混入Headscale `/api/v1`。
4. 其余注册/Noise/DERP请求转发127.0.0.1:9981，保留flush_interval -1。
5. Caddy默认处理Host与X-Forwarded链；Go允许的Origin是`https://headscale.holmes.cn`。

本文件不是完整Caddy配置。合入现有完整站点后：

```sh
caddy validate --config /实际路径/Caddyfile
# 验证成功后由你按服务管理方式reload，不自动执行
```

公网443用于Headscale和DERP，UDP3478用于STUN，不将HTTP3080/9983公开。Docker Caddy不在本Compose中：当前设计要求Caddy运行在宿主；若你的Caddy实际是容器，127.0.0.1含义会不同，必须先改网络和上游地址。

## 5. 启动后验收

```sh
docker compose ps
docker compose logs --tail=100 headscale
docker compose logs --tail=100 headscale-console
```

- 打开`https://headscale.holmes.cn/admin/`，看到Headscale Console与动态验证码。
- 独立管理员账号登录，读取真实设备/用户/路由；没有demo fallback。
- 访问策略页可读取；编辑 → 服务端检查 → 差异 → 确认发布。
- DNS页显示当前共享配置，保存禁用；可读不意味着可写。
- 客户端继续使用`https://headscale.holmes.cn`，不填/admin。
- 端到端设备连通、DERP/STUN和DNS解析需真机验证，控制面健康不代表业务可达。

## 6. 本机调试配置

项目根目录：

```sh
pnpm build
go run . -config docs/headscale/web/config.yaml
```

打开http://127.0.0.1:3080。想用Vite，修改该文件server.origin到http://127.0.0.1:5173后另开`pnpm dev`。

本机REST连接公网服务；DNS页读取本地config副本，不自动同步远端。若远端尚未切database，本机policy_mode声明不能绕过远端限制，发布会被Headscale拒绝。

## 验证边界

配置已按 [v0.29.7官方示例](https://github.com/juanfont/headscale/blob/v0.29.7/config-example.yaml)逐项核对。可验证YAML、Go配置加载、secret存在、Compose与Caddy路由的模拟浏览器测试。

本机Docker daemon/Caddy工具不可用时，不能宣称完整镜像启动或完整Caddy配置验证通过。原完整Caddy站点/TLS不在提供文件中，仍需服务器validate和真机验收。
