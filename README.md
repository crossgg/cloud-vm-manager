# Cloud VM Manager

![界面示例](pic/示例.jpg)

一个轻量级网页工具，用于管理 Azure、GCP、OCI 虚拟机实例。

当前支持：

- 按配置账号手动加载机器列表
- 开机、关机、重启
- 更换公网 IP
- Azure 账号余额查询
- Cloudflare DNS 更新，使用 API Token
- 手动更新 DNS，或在换 IP 后按开关决定是否同步 DNS
- 可选登录认证，适合需要暴露到公网的场景
- 网页管理页修改登录账号密码，密码落盘前自动 bcrypt 加密
- 网页管理页手动重载配置，修改配置后无需重启服务

## 本地运行

```bash
go mod tidy
go run .
```

访问：

```text
http://localhost:3000
```

## 配置

推荐使用 `config.conf`。程序读取顺序：

```text
config.conf -> config.ini -> config.yaml
```

复制示例：

```bash
cp config.example.conf config.conf
mkdir -p keys
```

密钥文件建议统一放到 `./keys`，容器内路径对应 `/app/keys`。例如：

```ini
key_file=/app/keys/gcp01.pem
```

详细字段说明见 [CONFIGURATION.md](CONFIGURATION.md)。

## 宝塔反向代理

如果项目需要公网访问，建议先在 `config.conf` 开启认证，并使用 HTTPS：

```ini
auth=begin
[main]
enabled=true
username=admin
password_hash=your-bcrypt-hash
session_secret=your-random-secret-at-least-32-chars
session_hours=12
cookie_secure=true
auth=end
```

宝塔面板操作：

1. 网站 -> 添加站点，绑定你的域名并申请 SSL。
2. 网站设置 -> 反向代理 -> 添加反向代理。
3. 目标 URL 填：

```text
http://127.0.0.1:3000
```

4. 在反向代理的“配置文件”或“Nginx 高级配置”里确认增加这些字段：

```nginx
proxy_set_header Host $host;
proxy_set_header X-Real-IP $remote_addr;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection "upgrade";
```

完整 location 示例：

```nginx
location / {
    proxy_pass http://127.0.0.1:3000;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
}
```

其中 `Host` 和 `X-Forwarded-Proto` 很重要：项目开启认证后会校验 POST 请求来源，缺少这两个头可能导致登录、保存配置、开关机等 POST 操作被拒绝。

## Docker 部署

下面三种方式都需要准备：

```bash
cp config.example.conf config.conf
mkdir -p keys
```

把 GCP/OCI 等 PEM 或 JSON 密钥文件放入 `./keys`，配置文件里使用容器内路径 `/app/keys/xxx.pem`。

### 方式一：Docker 自构建

```bash
docker build -t cloud-vm-manager:local .

docker run -d \
  --name cloud-vm-manager \
  -p 3000:3000 \
  -v $(pwd)/config.conf:/app/config.conf \
  -v $(pwd)/keys:/app/keys \
  --restart unless-stopped \
  cloud-vm-manager:local
```

### 方式二：Docker Run 使用镜像

```bash
docker pull ghcr.io/crossgg/cloud-vm-manager:latest

docker run -d \
  --name cloud-vm-manager \
  -p 3000:3000 \
  -v $(pwd)/config.conf:/app/config.conf \
  -v $(pwd)/keys:/app/keys \
  --restart unless-stopped \
  ghcr.io/crossgg/cloud-vm-manager:latest
```

### 方式三：Docker Compose

使用仓库里的 [docker-compose.yaml](docker-compose.yaml)：

```bash
docker compose up -d
```

当前 compose 会映射：

```yaml
volumes:
  - ./config.conf:/app/config.conf
  - ./keys:/app/keys
```

## DNS 更新

DNS 更新只会在配置了 `dns` 绑定时启用。

- 没有绑定：网页不显示“更新 DNS”按钮，换 IP 后 DNS 开关不可用。
- 有绑定：机器卡片显示“更新 DNS”按钮，可以不换 IP 直接用当前公网 IP 更新 Cloudflare。
- 换 IP 时：只有勾选“换 IP 后更新 DNS”才会在换 IP 成功后更新 Cloudflare。

Cloudflare 使用 API Token：

```http
Authorization: Bearer <api_token>
```

不使用 Global API Key。

## API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/auth` | 查询认证状态 |
| POST | `/api/login` | 登录 |
| POST | `/api/logout` | 退出登录 |
| GET | `/api/config/status` | 查询配置加载状态 |
| POST | `/api/config/reload` | 手动重载配置 |
| GET | `/api/settings/auth` | 查询认证配置状态 |
| POST | `/api/settings/auth` | 修改认证配置 |
| GET | `/api/accounts` | 获取本地配置账号列表，不访问云 API |
| GET | `/api/vms?provider=&account=` | 加载指定账号机器列表 |
| GET | `/api/vm/:provider/:account/:name` | 获取单台机器详情 |
| POST | `/api/vm/:provider/:account/:name/start` | 开机 |
| POST | `/api/vm/:provider/:account/:name/stop` | 关机 |
| POST | `/api/vm/:provider/:account/:name/restart` | 重启 |
| POST | `/api/vm/:provider/:account/:name/change-ip?update_dns=true` | 换 IP，可选更新 DNS |
| POST | `/api/vm/:provider/:account/:name/update-dns` | 用当前公网 IP 手动更新 DNS |
| GET | `/api/account/:provider/:account/balance` | Azure 账号余额 |

## 注意

- GCP 赠金余额和 OCI 余额查询已移除。
- GCP 机器 ID 使用 `zone|instance-name`。
- OCI 机器 ID 使用 instance OCID。
- Cloudflare Token 建议只授予目标 Zone 的 DNS edit 权限。
- `config.conf`、密钥文件、PEM 文件不要提交到仓库。

## License

MIT
