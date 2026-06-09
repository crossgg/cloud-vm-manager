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
- 网页可视化管理 DNS 绑定，每个 VM 实例可单独配置
- DNS 管理页面：查看 Cloudflare 账号、DNS 绑定列表、导入/导出 dns.conf
- 可选登录认证，适合需要暴露到公网的场景
- 网页管理页修改登录账号密码，密码落盘前自动 bcrypt 加密
- 网页管理页手动重载配置，修改配置后无需重启服务

## 目录结构

```text
config/                  # 配置目录（Docker 映射 /app/config）
├── config.conf          # 主配置：云账号 + 认证
├── dns.conf             # DNS 配置：Cloudflare 账号 + DNS 绑定
└── keys/                # 密钥文件目录
    ├── gcp01.pem
    └── oci.pem
```

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

程序读取顺序：

```text
config/config.conf -> config/config.ini -> config/config.yaml -> config.conf -> config.ini -> config.yaml
```

初始化：

```bash
mkdir -p config/keys
cp config.example.conf config/config.conf
```

DNS 配置（Cloudflare 账号和绑定）放在 `config/dns.conf`，也可以在网页 DNS 管理页面导入或通过 VM 实例页面可视化配置。

密钥文件放到 `config/keys/`，配置文件里使用容器内路径 `/app/config/keys/xxx.pem`。

详细字段说明见 [CONFIGURATION.md](CONFIGURATION.md)。

## 宝塔反向代理

如果项目需要公网访问，建议先在 `config/config.conf` 开启认证，并使用 HTTPS：

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

4. 在反向代理的"配置文件"或"Nginx 高级配置"里确认增加这些字段：

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

准备配置目录：

```bash
mkdir -p config/keys
cp config/config.example.conf config/config.conf
```

把 GCP/OCI 等 PEM 或 JSON 密钥文件放入 `config/keys/`，配置文件里使用容器内路径 `/app/config/keys/xxx.pem`。

### 方式一：Docker 自构建

```bash
docker build -t cloud-vm-manager:local .

docker run -d \
  --name cloud-vm-manager \
  -p 3000:3000 \
  -v $(pwd)/config:/app/config \
  -v $(pwd)/runtime:/app/runtime \
  --restart unless-stopped \
  cloud-vm-manager:local
```

### 方式二：Docker Run 使用镜像

```bash
docker pull ghcr.io/crossgg/cloud-vm-manager:latest

docker run -d \
  --name cloud-vm-manager \
  -p 3000:3000 \
  -v $(pwd)/config:/app/config \
  -v $(pwd)/runtime:/app/runtime \  
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
  - ./config:/app/config
  - ./runtime:/app/runtime
```

## DNS 管理

### 网页管理

- **DNS 管理页面**：查看 Cloudflare 账号（仅显示名称和备注，隐藏敏感信息）、DNS 绑定列表（支持删除）、导入 dns.conf、预览当前配置（api_token 已脱敏）
- **VM 实例页面**：每个 VM 卡片有「DNS 绑定」按钮，点击弹出可视化配置面板，自动填充 provider/account/vm 信息

### 配置文件

DNS 配置独立存放在 `config/dns.conf`：

```ini
cloudflare=begin
[cf01]
remark=主站 Cloudflare
api_token=your-cloudflare-api-token
zone_id=your-zone-id
cloudflare=end

dns=begin
[my-binding]
cloudflare=cf01
provider=azure
account=az001
vm=myvm
domain=vm.example.com
type=A
ttl=1
proxied=false
dns=end
```

### 行为

- 没有绑定：网页不显示"更新 DNS"按钮，换 IP 后 DNS 开关不可用。
- 有绑定：机器卡片显示"更新 DNS"按钮，可以不换 IP 直接用当前公网 IP 更新 Cloudflare。
- 换 IP 时：只有勾选"换 IP 后更新 DNS"才会在换 IP 成功后更新 Cloudflare。

Cloudflare 使用 API Token（不使用 Global API Key）。

## 安全说明

- Cloudflare API Token 和 Zone ID 不会在网页前端显示，仅在导入/保存时写入配置文件
- dns.conf 原始预览自动脱敏 `api_token`、`client_secret`、`password` 等敏感字段
- 认证开启后，所有 API 需要登录后才能访问
- 配置文件、密钥文件不要提交到 Git 仓库

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
| GET | `/api/accounts` | 获取本地配置账号列表 |
| GET | `/api/vms?provider=&account=` | 加载指定账号机器列表 |
| GET | `/api/vm/:provider/:account/:name` | 获取单台机器详情 |
| POST | `/api/vm/:provider/:account/:name/start` | 开机 |
| POST | `/api/vm/:provider/:account/:name/stop` | 关机 |
| POST | `/api/vm/:provider/:account/:name/restart` | 重启 |
| POST | `/api/vm/:provider/:account/:name/change-ip` | 换 IP |
| POST | `/api/vm/:provider/:account/:name/update-dns` | 更新 DNS |
| GET | `/api/account/:provider/:account/balance` | Azure 余额 |
| GET | `/api/dns/cloudflare` | Cloudflare 账号列表（脱敏） |
| GET | `/api/dns/bindings` | DNS 绑定列表 |
| GET | `/api/dns/raw` | 预览 dns.conf（脱敏） |
| POST | `/api/dns/delete-binding` | 删除 DNS 绑定 |
| GET | `/api/vm/:provider/:account/:name/dns` | VM 的 DNS 绑定 |
| POST | `/api/vm/:provider/:account/:name/dns` | 保存 VM 的 DNS 绑定 |

## 注意

- GCP 机器 ID 使用 `zone|instance-name`。
- OCI 机器 ID 使用 instance OCID。
- Cloudflare Token 建议只授予目标 Zone 的 DNS edit 权限。
- `config/` 目录下的所有文件（config.conf、dns.conf、keys/）不要提交到仓库。

## License

MIT
