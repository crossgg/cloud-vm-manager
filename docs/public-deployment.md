# 公网部署安全指南

如果仅在可信局域网或本机使用，默认配置即可满足便利性；但若需要将面板开放到公网（例如绑定域名远程访问），请务必按本指南完成网络隔离、认证启用与环境变量加固，防止未授权接管、凭据泄露或 SSRF 风险。

---

## 一、公网部署核心检查清单

1. **禁止将明文 `3000` 端口直接暴露到公网**：
   - 将容器端口绑定限制在 `127.0.0.1:3000:3000`，仅允许本机反向代理访问，并在云服务器安全组/宿主机防火墙放行 `80`/`443`（关闭 `3000` 外部入站）。
   - 必须通过 **HTTPS（TLS）** 访问面板，避免登录密码、Session Cookie 和云厂商配置在公网上明文传输。
2. **上线前开启登录认证与 HTTPS Cookie**：
   - 在放开公网域名访问前，务必启用登录认证（`enabled=true`），设置高强度用户名与密码、至少 32 位随机 `session_secret`，并将 `cookie_secure` 设为 `true`（WebUI 勾选 **HTTPS Cookie**）。
   - 具体预置方式可参考 [配置说明 - 登录认证](../CONFIGURATION.md#登录认证)。
   - 提示：已配置密码后，后续修改密码、修改用户名、关闭认证或取消 HTTPS Cookie 时都需要输入**当前密码**验证。
3. **按部署拓扑配置环境变量**：
   - 建议开启 `BLOCK_PRIVATE_PROXY=true` 与 `DISABLE_SELF_UPDATE=true`。
   - 根据是否经过反代配置 `TRUSTED_PROXIES`（见下文）。
4. **遵循云账号最小权限原则**：
   - 为 Azure Service Principal、OCI 用户、GCP Service Account 和 Cloudflare API Token 仅授予必需的资源组/隔间/Zone 权限，避免直接使用全局管理员主密钥。
   - 宿主机上的 `config/` 目录（含 `config.conf`、`dns.conf`、`proxy.conf`、`keys/`）请保持严格权限（仅属主可读写），切勿提交到公开仓库。

---

## 二、公网安全环境变量说明

| 环境变量 | 默认值 | 公网推荐值 | 说明 |
| :--- | :--- | :--- | :--- |
| `GIN_MODE` | 镜像内为 `release` | `release` | 关闭 Gin 调试日志与路由警告。非 Docker 二进制运行未显式设置时程序也会默认以 `release` 模式启动。 |
| `BLOCK_PRIVATE_PROXY` | 未设置（允许本地代理） | `true` | 开启后，代理池与更新代理在 DNS 解析后、TCP 建连前会禁止连接回环（`127.0.0.0/8`、`::1`）和私有网段（`10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16`、`fc00::/7`、`100.64.0.0/10`）。**注意**：云元数据与链路本地地址（`169.254.0.0/16`、`fe80::/10`、`fd00:ec2::254`）在所有模式下均永久封禁。 |
| `TRUSTED_PROXIES` | 回环与常见私网段（`127.0.0.1/32,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fc00::/7`） | `127.0.0.1/32,::1/128`（本机反代）或 `none`（无反代直连） | 控制程序是否采信反向代理传入的 `X-Forwarded-For`、`X-Real-IP`、`X-Forwarded-Host`、`X-Forwarded-Proto`。若使用本机 Nginx/Caddy，建议收窄为 `127.0.0.1/32,::1/128`（Docker 反代则填反代容器 IP/网段）；若无反代且直接暴露端口，必须设为 `none`，防止攻击者伪造请求头绕过登录失败锁定与同源（CSRF）校验。 |
| `DISABLE_SELF_UPDATE` | `false` | `true` | 公网生产容器建议开启，禁用面板内「在线热更新」与「恢复镜像版」重启接口，统一通过拉取/重建 Docker 镜像更新。 |

---

## 三、Docker Compose 公网部署示例

在公网服务器上，建议基于仓库的 [docker-compose.yaml](../docker-compose.yaml) 做如下调整：

```yaml
version: '3.8'

services:
  cloud-vm-manager:
    image: ghcr.io/crossgg/cloud-vm-manager:latest
    container_name: cloud-vm-manager
    ports:
      # 仅绑定宿主机回环地址，防止绕过反代直接访问 HTTP 明文端口
      - "127.0.0.1:3000:3000"
    environment:
      - GIN_MODE=release
      - BLOCK_PRIVATE_PROXY=true
      - TRUSTED_PROXIES=127.0.0.1/32,::1/128
      - DISABLE_SELF_UPDATE=true
    volumes:
      - ./config:/app/config
      - ./runtime:/app/runtime
    restart: unless-stopped
```

> **提示**：如果你在宿主机上同时运行了供面板使用的本地代理（且确实需要面板通过 `127.0.0.1` 或 Docker 网桥代理访问云 API），则不要设置 `BLOCK_PRIVATE_PROXY=true`；若面板公网可访问，请务必配合强认证与前置访问控制（见第五节）。

如果先在本地初始化认证配置后再部署到公网，推荐流程：
1. 在 `config/config.conf` 中提前写好 `auth=begin ... auth=end`（`enabled=true`、`cookie_secure=true`），或先通过 SSH 端口转发（`ssh -L 3000:127.0.0.1:3000 user@server`）访问 `http://127.0.0.1:3000` 完成认证配置；
2. 确认登录认证已生效后，再启动反向代理对外提供 HTTPS 访问。

---

## 四、反向代理与 HTTPS 配置示例

### Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name vm.example.com;

    ssl_certificate     /path/to/fullchain.pem;
    ssl_certificate_key /path/to/privkey.pem;

    # 仅允许特定管理 IP 访问（可选，强烈建议）
    # allow 203.0.113.0/24;
    # deny all;

    # 配合 OCI 对象存储最大 128MB 上传限制
    client_max_body_size 130m;

    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

> **注意**：`Host` 与 `X-Forwarded-Proto` 必须准确转发，否则开启认证后的同源校验（CSRF 防护）会拒绝 `POST`/`PUT`/`DELETE` 请求。如果 Nginx 与容器位于不同 Docker 网络（例如经由 Docker 网桥而非 `127.0.0.1` 访问），请确保反代容器网段包含在 `TRUSTED_PROXIES` 中。

### Caddy

```caddyfile
vm.example.com {
    reverse_proxy 127.0.0.1:3000
}
```

Caddy 会自动签发 TLS 证书并转发 `Host`、`X-Forwarded-For` 与 `X-Forwarded-Proto`。

---

## 五、额外纵深防御建议

1. **前置零信任 / 隧道访问**：如无需随时从未知 IP 登录，优先使用 **Cloudflare Tunnel + Cloudflare Access**、**Tailscale**、**WireGuard** 或反向代理 Basic Auth 作为外层准入门槛，使管理面板不直接裸露在公网扫描下。
2. **文件权限检查**：宿主机上建议对配置与私钥设置严格权限：
   ```bash
   chmod 700 config config/keys
   chmod 600 config/*.conf config/keys/*
   ```
3. **审计日志与备份**：反向代理开启访问日志并配合 Fail2ban 限制异常扫描；定期备份 `config/` 目录中的配置文件与 `.pem` 私钥（离线加密保存）。
