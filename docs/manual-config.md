# 手动配置说明

> **提示**：目前项目绝大部分配置均已支持在 WebUI 中直接添加、修改、上传密钥并自动持久化与热重载：
> - **云账号与密钥文件**：WebUI 左侧「系统设置」→「云账号与配置管理」（可直接上传 `.pem` / `.json` 密钥到 `config/keys/`）
> - **登录认证与系统更新**：WebUI 左侧「系统设置」→「登录认证」「程序更新」
> - **代理池与账号代理绑定**：WebUI 左侧「代理池」
> - **多云 DNS 账号与绑定**：WebUI 左侧「DNS 管理」或实例卡片上的「DNS 绑定」
> - **通知中心与交互式 Telegram Bot**：WebUI 左侧「通知与 Bot」
>
> 以下内容适用于**无界面初始化预置、批量迁移或手动排查配置文件**场景。完整字段细节亦可参阅 [CONFIGURATION.md](../CONFIGURATION.md)。

---

## 目录结构与读取顺序

程序按以下顺序查找主配置文件：

```text
config/config.conf -> config/config.ini -> config/config.yaml -> config.conf -> config.ini -> config.yaml
```

推荐将所有配置文件与密钥统一放在 `config/` 目录（Docker 部署会将其映射到 `/app/config`）：

```text
config/
├── config.conf          # 主配置：云账号（Azure/GCP/OCI）、监控参数与登录认证
├── dns.conf             # DNS 配置：多云 DNS 账号（CF/阿里云/腾讯云/DNSPod）+ DNS 绑定（A / AAAA）
├── proxy.conf           # 代理池 + 账号代理绑定（由 WebUI 或手动维护）
├── notify.conf          # 通知触发点、交互式 Telegram Bot 与单向推送渠道配置
├── vm_cache.json        # 实例列表概览持久化缓存
├── oci_create_tasks.json# OCI 创建/抢购任务持久化记录
└── keys/                # 密钥文件目录（Docker 路径为 /app/config/keys/xxx.pem）
    ├── gcp01.pem
    └── oci.pem
```

初始化示例：

```bash
mkdir -p config/keys
cp config/config.example.conf config/config.conf
```

> **注意**：`config/` 下的真实配置与密钥文件切勿提交到 Git 仓库。

---

## 登录认证（`config/config.conf`）

如需将服务暴露到公网，建议开启认证并配合 HTTPS 反向代理使用：

```ini
auth=begin
[main]
enabled=true
username=admin
password_hash=$2a$12$your-bcrypt-password-hash
session_secret=your-random-secret-at-least-32-chars
session_hours=12
cookie_secure=true
auth=end
```

- `password_hash`：使用 bcrypt 生成的密码哈希（cost 建议 `12`），不存明文密码；如果在 WebUI「系统设置」修改密码，后端会自动转为 bcrypt 哈希并重置 `session_secret`。
- `session_secret`：长度至少 32 个字符的随机字符串。
- `cookie_secure`：公网 `https://` 访问设 `true`，本地 `http://localhost:3000` 调试设 `false`。

在 PowerShell 快速生成 `session_secret`：

```powershell
-join ((48..57 + 65..90 + 97..122) | Get-Random -Count 48 | ForEach-Object {[char]$_})
```

---

## 云账号配置（`config/config.conf`）

关于各云厂商控制台如何申请凭据与生成密钥，请参考：
- [Azure 配置获取指南](azure-config-guide.md)
- [OCI (Oracle Cloud) 配置获取指南](oci-config-guide.md)
- [GCP 配置获取指南](gcp-config-guide.md)

### Azure 示例

```ini
azure=begin
[az001]
group=azure
subscription_id=xxx
tenant_id=xxx
client_id=xxx
client_secret=xxx
resource_group=rg-name
location=koreacentral
azure=end
```

### GCP 示例

```ini
gcp=begin
[gcp01]
group=gcp
project_id=my-project
client_email=sa@project.iam.gserviceaccount.com
key_file=/app/config/keys/gcp.pem
gcp=end
```

### OCI 示例（含数据传输用量监控参数）

```ini
oci=begin
[oci-jp]
group=oci
user=ocid1.user.oc1...
fingerprint=xx:xx:xx
tenancy=ocid1.tenancy.oc1...
compartment_id=ocid1.compartment.oc1...
region=ap-tokyo-1
key_file=/app/config/keys/oci.pem
dt_monitor_enabled=false
dt_monitor_interval=300
dt_monitor_threshold=9000
dt_monitor_auto_stop=false
dt_monitor_stop_method=soft
oci=end
```

---

## 代理池配置（`config/proxy.conf`）

`proxy.conf` 通常由 WebUI「代理池」页面自动生成与维护；如需手动编写：

```ini
proxy=begin
[proxy-example]
url=http://127.0.0.1:8080
remark=东京出口
proxy=end

proxy_binding=begin
[binding-1]
provider=azure
account=az001
proxy=proxy-example
fallback=direct
proxy_binding=end
```

- 账号绑定按 `provider/account` 精确匹配；`fallback` 可选 `direct`、`none`（不回退）或池内其他代理 ID。

---

## 多云 DNS 配置（`config/dns.conf`）

支持在 WebUI「DNS 管理 -> DNS 账号配置」直接增改并热重载，也可手动在 `config/dns.conf` 中维护；支持 **Cloudflare**（API Token + Zone ID）、**阿里云 DNS (`alidns`)**、**腾讯云 DNS (`tencent`，`cn`/`intl`)** 与 **DNSPod (`dnspod`，`cn`/`intl`)**：

```ini
cloudflare=begin
[cf01]
remark=主站 Cloudflare
api_token=your-cloudflare-api-token
zone_id=your-zone-id
domains=example.com
cloudflare=end

dns_accounts=begin
[ali-prod]
provider=alidns
access_key_id=LTAI...
access_key_secret=your-ak-secret
domains=example.com,example.org
dns_accounts=end

dns=begin
[my-binding]
dns_account=cf01
provider=oci
account=oci-jp
vm=ocid1.instance.oc1...
domain=vm.example.com
type=A
ttl=1
proxied=false
dns=end
```

- 非 Cloudflare 账号必须在 `domains` 中明确指定要管理的根域名，程序不会枚举账号全量域名。
- `vm` 字段规范：Azure 填 VM 名称；GCP 填 `zone|instance-name`；OCI 填 Instance OCID（所有绑定字段均需精确匹配，不支持留空通配）。
- `type` 支持 `A`（绑定 IPv4）与 `AAAA`（绑定 IPv6）。

---

## 通知与 Telegram Bot 配置（`config/notify.conf`）

推荐在 WebUI 左侧 **「通知与 Bot」** 页面可视化维护并一键热重载；如需预置可参考仓库内的 [config/notify.example.conf](../config/notify.example.conf)：

```ini
triggers=begin
[global]
enabled_events=oci_task_succeeded,oci_dt_alert,azure_credit_alert,vm_change_ip,auth_login
triggers=end

telegram_bot=begin
[default]
enabled=true
bot_token=123456:ABC-DEF
api_host=https://api.telegram.org
proxy=proxy-example
fallback=none
allowed_user_ids=123456789
allowed_chat_ids=
auto_notify=true
notify_chat_ids=
notify_events=oci_task_succeeded,oci_dt_alert,azure_credit_alert,vm_change_ip,auth_login
notify_cooldown_sec=300
telegram_bot=end

notify_channel=begin
[bark-1]
name=iPhone Bark
type=bark
enabled=true
proxy=none
fallback=none
cooldown_sec=300
events=oci_task_succeeded,azure_credit_alert
param.bark_push=https://api.day.app/your_key
notify_channel=end
```

- **支持的触发点（`enabled_events` / `events`）**：`oci_task_succeeded`（OCI 抢购成功）、`oci_dt_alert`（OCI 流量告警）、`azure_credit_alert`（Azure 赠金/费用告警）、`vm_change_ip`（更换公网 IP）、`vm_power`（开关机/重启）、`auth_login`（面板登录提醒/锁定告警）。
- **支持的单向渠道类型（`type`）**：`bark`、`wecom_bot`、`wecom_app`、`feishu`、`telegram`、`pushplus`、`custom`。
- **TG Bot 交互指令**：`/help`、`/oracle`、`/azure`、`/gcp`（需配置 `allowed_user_ids` 或 `allowed_chat_ids` 白名单后才会响应）。

---

## 热重载配置文件

程序不会主动轮询磁盘修改。手动修改 `config/config.conf`、`config/dns.conf`、`config/proxy.conf` 或 `config/notify.conf` 后，请在 WebUI 左侧「系统设置」右上角点击 **「重载配置」** 使其生效；若配置解析报错，程序会继续沿用上一次成功加载的配置。
