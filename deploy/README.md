# Go 管理端部署

## 1. 构建与访问

```sh
pnpm install --frozen-lockfile
pnpm build
pnpm build:server
```

Vue产物嵌入二进制。生产Node只用于构建，不参与运行。根路径访问使用`/api`；Caddy同域admin部署需先将`/admin`重定向`/admin/`，再剥离前缀转发。前端资源相对地址，API使用`/admin/api`，Go收到`/api`；Origin只填域名。参考 [Caddy配置](<../docs/headscale/Caddyfile>)，不能使用不剥离前缀的旧反代。

本机预览 `go run . preview`，仅127.0.0.1:3080，随机口令。公网必须独立配置账号和TLS。

## 2. 密码与Secrets

```sh
go run ./cmd/hash-password
```

交互无回显并确认，或安全stdin单行，拒绝明文argv。输出Node兼容`scrypt$16384$8$1$...`。保存UTF-8哈希文件到构建目录外，控制权限，不提交秘密。

Go本机默认读取根 `config.yaml`，或使用 `-config` 指定；环境变量覆盖文件配置。Docker部署统一使用 [唯一Compose](<../docs/headscale/docker-compose.yml>)，只读挂载 `docs/headscale/web/config.docker.yaml` 到 `/app/config.yaml`。

凭据放在 `docs/headscale/secrets/`：`console-password.hash`与`headscale-api-key.secret`。后端内网地址、公网设备接入地址和账号设置在Docker YAML配置中，不再需要另一套CONSOLE_CONFIG_FILE/SECRET_FILE环境配置。

项目根目录执行：

```sh
docker compose -f docs/headscale/docker-compose.yml config --quiet
docker compose -f docs/headscale/docker-compose.yml pull headscale-console
# Headscale已由本Compose运行时，仅启动/更新管理端
docker compose -f docs/headscale/docker-compose.yml up -d --no-deps headscale-console
# 全新部署，经审查配置和备份后再启动全部服务
# docker compose -f docs/headscale/docker-compose.yml up -d
```

同一Compose网络中，管理端请求`http://headscale:9981`；宿主Caddy将`https://headscale.holmes.cn/admin/`转发到127.0.0.1:9983，Headscale保留9981与UDP3478。Caddy继续由宿主运行，不新增冲突的80/443容器。完整配置、Secrets与Caddy说明见 [配置目录说明](<../docs/headscale/README.md>)。

运行镜像为distroless nonroot（65532），secrets必须对其可读。配置、Key和密码不进入前端构建。

## 3. 核心环境变量

- APP_ENV默认production；只有development且监听/访问为loopback才允许不Secure。
- HOST默认127.0.0.1，PORT默认3080。
- CONSOLE_ORIGIN精确HTTPS origin（不含路径/尾斜杠），COOKIE_SECURE=true。
- CONSOLE_USERNAME、CONSOLE_PASSWORD_HASH及对应_FILE：demo也必填。
- HEADSCALE_URL、HEADSCALE_PUBLIC_URL、HEADSCALE_API_KEY及_FILE：live必填；同一变量VALUE与VALUE_FILE不可同时配置。
- HEADSCALE_POLICY_MODE=file/database/unknown。版本和服务端返回还会再次限制操作，不以手工配置绕过Headscale。
- SESSION_TTL_SECONDS=3600，绝对期限60–86400；重启会话失效。
- LOGIN_RATE_LIMIT=10，LOGIN_RATE_WINDOW_SECONDS=900。按socket IP，不信任任意X-Forwarded-For；反代后聚合限流，必要时在可信网关补充。

## 4. 登录接口

| 请求 | 说明 |
|---|---|
| GET /api/health | 公共最小控制台健康状态 |
| POST /api/auth/captcha | 精确Origin；返回captchaId、GIF data URL、expiresAt；设置HttpOnly预认证cookie |
| POST /api/auth/login | JSON username/password/captchaId/captcha；预cookie绑定；返回会话用户、CSRF和过期时间 |
| GET /api/auth/session | 会话恢复 |
| POST /api/auth/logout | 会话+Origin+X-CSRF-Token；注销服务端状态 |

验证码4字母，4帧每帧只见3个，一轮读全。2分钟、一次性消费，刷新使旧挑战失效。领取20/min/IP，有界1000挑战；HMAC答案不返回、不记录。captcha不是MFA，动画多帧可被采集，不承诺防OCR。

HttpOnly、SameSite=Strict，生产Secure与__Host前缀，密码哈希与管理Key从不返回。CSRF只保存在页面内存，不用JWT/localStorage。所有业务/配置API统一先鉴权，所有写操作另校验Origin+CSRF。

## 5. Headscale API适配

基准契约：0.28.x与0.29.x。设备/用户/预授权密钥/路由；0.29 database模式支持policy/check和发布。未知/不兼容版本拒绝，不以演示数据替代。OS/客户端版本没有此API字段则显示未采集。

客户端固定目标、禁重定向、默认不继承HTTP代理、8秒超时、4MiB响应上限、输入白名单和uint64字符串ID。上游401/403映射502，不错误注销控制台会话。节点内嵌preAuthKey及machineKey等不会发送前端；列表密钥只返回脱敏后缀，创建响应一次展示明文。

策略/路由有本进程锁和旧内容compare，但上游没有原子CAS。最后写入窗口仍可能被外部管理员并发修改，建议不要同时用多个UI更新同一资源。

## 6. 受控DNS配置

默认CONFIG_WRITE_ENABLED=false，默认镜像不包含Docker socket/Headscale CLI，因此只读。

读取：HEADSCALE_CONFIG_PATH为已挂载的**绝对文件路径**，前端只接收DNS允许字段，不返回原始配置/私钥路径。

启用写入需要受控集成镜像及：

```dotenv
HEADSCALE_CONFIG_PATH=/etc/headscale/config.yaml
CONFIG_WRITE_ENABLED=true
CONFIG_TEST_EXECUTABLE=/usr/local/bin/headscale
APPLY_EXECUTABLE=/usr/local/bin/restart-headscale-only
```

- 校验器调用固定参数 `--config <candidate> configtest`；应用器不接受浏览器参数。操作器必须在同环境中能校验目标配置并仅重启指定Headscale。
- 共享挂载整个配置目录，不使用单文件bind：atomic rename需要目录写入权限。
- Linux要求原配置UID/GID与控制台进程一致，否则拒绝写入，防止atomic替换后Headscale失去读取权限。Windows需运维保证ACL。
- 新配置只改MagicDNS/base_domain/global nameservers/override字段，保留其它字段和分流nameservers；旧nameservers:[]归一化global。
- POST /api/config/dns/preview携带revision和dns，先校验候选，再返回差异、短时确认token和重启提示。此时未写原文件。
- POST /api/config/dns/apply携带token与confirmed:true；绑定账号、一次性、2分钟；再核对revision、备份、atomic replace、固定应用器、Headscale健康验证。
- 失败时仅当原文件仍为本次候选才回滚；有外部修改则返回rollback_conflict，不覆盖他人改动。恢复使用独立有限ctx并再次验证健康。收到取消且尚未写入则没有副作用。
- 应用成功只是控制面与数据库健康，不承诺DNS实际解析或业务网络可达。按目标版本与运维流程核对configtest能力、TLS及重启行为。
- Compose默认只读容器。启用写入需明确修改配置目录volume、工具挂载和身份；不能只改开关。不要直接挂全权限Docker socket。

## 7. 现有部署注意

用户提供配置使用latest镜像、file空策略路径，实际运行版本未确认。先读版本、备份数据库和策略。迁移database不可自动发布演示规则，不改DERP/STUN/客户端server_url。

Headplane挂载源名config.yaml与本地config.yml不一致需核对；不直接修改远端配置。

## 8. 验证与限制

```sh
go test ./...
go vet ./...
pnpm build
pnpm test:ui
```

本机Go与Chrome、独立测试固件和模拟上游。Docker镜像构建需Docker daemon可用，未跑镜像时不能以Compose语法验证代替构建。

当前会话、挑战、确认token为内存单实例；多副本需要共享状态。完整审计、多人管理员RBAC、配置通用编辑器、Agent/SSH、真实远端部署验证尚未覆盖。
