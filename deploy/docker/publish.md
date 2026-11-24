# Docker Hub 发布

仓库：`docker.io/promptness/headscale-console`。

项目根目录执行，先确认Docker引擎运行、测试通过，并对仓库拥有推送权限：

```sh
docker login docker.io
docker build -f deploy/docker/Dockerfile -t promptness/headscale-console:0.2.0 -t promptness/headscale-console:latest .
docker push promptness/headscale-console:0.2.0
docker push promptness/headscale-console:latest
```

版本0.2.0是示例，应与实际发布版本一致。镜像未push前Compose无法从Docker Hub拉取。凭据由Docker交互/凭据助手处理，不写到命令参数/项目文件。

多架构发布（可选；需要buildx与相应构建环境）：

```sh
docker buildx build --platform linux/amd64,linux/arm64 -f deploy/docker/Dockerfile -t promptness/headscale-console:0.2.0 -t promptness/headscale-console:latest --push .
```

Go无需CGO，前端由镜像构建后embed。生产镜像无Node运行时。所有真实配置、data、docs、secrets由.dockerignore排除，发布前确认未将秘密复制进其它源码文件。

服务器使用唯一 `docs/headscale/docker-compose.yml`，设置HEADSCALE_CONSOLE_TAG为已发布版本后：

```sh
docker compose -f docs/headscale/docker-compose.yml pull headscale-console
docker compose -f docs/headscale/docker-compose.yml up -d --no-deps headscale-console
```

仅更新UI，不自动升级或重启Headscale。首次完整部署与database迁移见 [最终配置说明](<../../docs/headscale/README.md>)。

本任务没有执行镜像推送；要公开发布，需用户确认并提供已登录的Docker环境。Dockerfile/schema检查不能代替实际构建和容器运行验证。
