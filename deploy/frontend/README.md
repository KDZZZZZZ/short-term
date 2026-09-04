# 前端部署运行手册

把 `frontend/` 构建产物(Vite `dist/`)发布到生产 ECS,由 rootless Podman 里的
nginx 提供静态托管,并把 `/api/v1` 与 `/media` 反向代理到同机的 Gateway。
与后端部署同一惯例:仅限部署账号、无状态静态卷、systemd 用户服务。

## 拓扑

| 单元 | 容器 | 网络/端口 | 内容 |
| --- | --- | --- | --- |
| Web 前端 | `nginx:1.29-alpine`(固定摘要) | 公网 `0.0.0.0:18084 -> 80` | 只读挂载:`current/html`(dist)与 `current/nginx.conf` |

- 上游:Gateway `host.containers.internal:18083`(与后端 README 的 Gateway 行一致)。
- SPA 回退:未命中路径回退 `index.html`;`/assets/*` 永久缓存(文件名带内容哈希),`index.html` 不缓存。
- 上传:`client_max_body_size 25m`,略宽于网关自身限制,让网关的 413 语义先生效。

## 目录与状态

```text
/opt/short-term/frontend/
├─ releases/<git-sha>/   # html/ 与 nginx.conf
├─ current               # -> releases/<git-sha>(原子替换)
└─ previous              # -> 上一版(供回滚)
/opt/short-term/state/frontend-release.env
/opt/short-term/bin/short-term-frontend
~/.config/systemd/user/short-term-frontend.service
```

## 触发与验收

`.github/workflows/deploy-frontend.yml`:

1. 触发:`main` 上 `frontend/**`、`deploy/frontend/**` 或本 workflow 变更的 push;亦可手动 `workflow_dispatch`。
2. Runner 上 `npm ci && npm run build`,打包 `dist` 并附 SHA-256。
3. 上传到 `/opt/short-term/incoming/`,校验摘要后落到 `releases/<sha>/`。
4. `short-term-frontend deploy <release>`:换 `current` 链、重启用户服务,并在本机验证
   `GET /`(含 `<div id="root">`)与 `GET /api/v1/products`(网关 JSON 错误,证明代理生效)。
5. 公网验收:从 runner 直接请求 `http://<DEPLOY_HOST>:18084/` 与 `/api/v1/products`。

## 前置条件

- 阿里云安全组放行入方向 `TCP/18084`(与 `18082`/`18083` 同一模式);未放行时公网验收步骤会显式失败。
- 部署账号已由 `deploy/backend/bootstrap-host.sh` 初始化(`/opt/short-term`、linger)。

## 运维命令

```bash
/opt/short-term/bin/short-term-frontend status
/opt/short-term/bin/short-term-frontend rollback
journalctl --user -u short-term-frontend.service -n 100
```

## 当前限制

- 与后端一致:单节点、明文 HTTP、以 IP 访问。接入真实用户或长期凭据前,应配置域名与
  可信 TLS 终止层,并按后端 README 的既定方向收敛公网直连端口。
- 前端发版与后端发版相互独立;`current` 链由 podman 在容器启动时解析,因此每次发版都会重启
  nginx 容器(秒级)。
