# HTTP API 文档

本文档基于后端路由定义整理（见 [internal/server/routes.go](../internal/server/routes.go) 与 [internal/auth/auth.go](../internal/auth/auth.go)）。

## 认证与安全要求

- 当 `auth.enabled=true` 时，除 `/api/auth`、`/api/login`、`/api/logout` 及登录页静态资源外，其余 `/api/*` 接口均需携带有效的 `vm_manager_session` 会话 Cookie 才可访问。
- 开启认证后，所有非只读方法（`POST` / `PUT` / `PATCH` / `DELETE`）都会校验同源请求头（`Origin` / `Referer` 与 `Host` / `X-Forwarded-Proto`）。通过 Nginx / 宝塔等反向代理部署时，请务必透传 `Host` 与 `X-Forwarded-Proto`。
- 所有响应均带 `Cache-Control: no-store`，敏感字段（如密码、Cloudflare Token、代理密码）不会在列表接口明文返回。

---

## 1. 认证与概览

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/auth` | 查询当前认证开关与登录状态 |
| POST | `/api/login` | 登录（触发连续失败限速保护） |
| POST | `/api/logout` | 退出登录并清除会话 Cookie |
| GET | `/api/overview` | 获取运行概览统计（基于后端持久化缓存） |

## 2. 配置、系统设置与在线更新

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/config/status` | 查询配置文件路径及加载状态 |
| POST | `/api/config/reload` | 手动热重载配置文件 |
| POST | `/api/config/upload-key` | 上传云厂商密钥文件至 `config/keys/` |
| GET | `/api/settings/auth` | 查询登录认证配置状态 |
| POST | `/api/settings/auth` | 修改并热重载登录认证配置（自动 bcrypt 加密） |
| POST | `/api/settings/update` | 保存更新下载加速源 |
| GET | `/api/update/status` | 检查 GitHub Release 最新版本与更新说明 |
| POST | `/api/update/apply` | 下载对应平台压缩包、校验并应用更新重启 |
| POST | `/api/update/reset` | 重置更新状态并回退到内置版本 |

## 3. 云账号与代理池管理

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/accounts` | 获取已配置的云账号列表 |
| GET | `/api/accounts/:provider/:account` | 获取指定云账号详情（供编辑回显） |
| POST | `/api/accounts` | 新增或更新云账号并自动写入配置与热重载 |
| DELETE | `/api/accounts/:provider/:account` | 删除指定云账号 |
| GET | `/api/proxies` | 获取代理池列表与账号代理绑定 |
| POST | `/api/proxies` | 添加单条代理 |
| POST | `/api/proxies/import` | 逐行批量导入 HTTP / HTTPS / SOCKS5 代理 |
| PUT | `/api/proxies/:id` | 修改代理地址或备注 |
| DELETE | `/api/proxies/:id` | 删除代理 |
| POST | `/api/proxies/:id/test` | 测试代理连通性 |
| PUT | `/api/proxy-bindings` | 新增或更新账号的主代理 / Fallback 代理绑定 |
| DELETE | `/api/proxy-bindings` | 删除账号代理绑定（Query：`provider`、`account`） |

## 4. 通用实例操作（Azure / GCP / OCI）

> 实例标识 `:name` 说明：Azure 为 VM 名称；GCP 为 `zone|instance-name`；OCI 为 Instance OCID。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/vms` | 加载指定账号下的虚拟机列表（Query：`provider`、`account`） |
| GET | `/api/vm/:provider/:account/:name` | 获取单台虚拟机详情 |
| GET | `/api/refresh/:provider/:account/:name` | 实时刷新单台虚拟机详情 |
| POST | `/api/vm/:provider/:account/:name/start` | 实例开机 |
| POST | `/api/vm/:provider/:account/:name/stop` | 实例关机 |
| POST | `/api/vm/:provider/:account/:name/restart` | 实例重启 |
| POST | `/api/vm/:provider/:account/:name/change-ip` | 更换实例公网 IP（支持按选项同步更新 DNS） |
| POST | `/api/vm/:provider/:account/:name/update-dns` | 使用当前公网 IP 手动更新已绑定 DNS |

## 5. OCI 实例编辑、安全规则与 IP 管理

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/vm/:provider/:account/:name/edit-options` | 获取 OCI 实例可选规格与 OCPU/内存约束范围 |
| POST | `/api/vm/:provider/:account/:name/edit` | 保存 OCI 实例名称或规格/配置修改 |
| POST | `/api/vm/:provider/:account/:name/terminate` | 删除 OCI 实例（支持 `preserveBootVolume` 参数） |
| GET | `/api/vm/:provider/:account/:name/security-lists` | 获取实例主 VNIC 关联的安全列表 |
| POST | `/api/vm/:provider/:account/:name/security-lists/:listID/rules` | 保存安全列表入站/出站规则 |
| GET | `/api/vm/:provider/:account/:name/network-security-groups` | 获取实例主 VNIC 关联及可用的网络安全组 (NSG) |
| POST | `/api/vm/:provider/:account/:name/network-security-groups` | 新建并关联网络安全组 |
| POST | `/api/vm/:provider/:account/:name/network-security-groups/attach` | 为实例关联已有网络安全组 |
| POST | `/api/vm/:provider/:account/:name/network-security-groups/:groupID/detach` | 解绑实例上的指定网络安全组 |
| POST | `/api/vm/:provider/:account/:name/network-security-groups/:groupID/rules` | 保存网络安全组规则 |
| GET | `/api/vm/:provider/:account/:name/ips` | 获取 VNIC 下所有 IPv4/IPv6、保留 IP 与子网 IPv6 就绪状态 |
| POST | `/api/vm/:provider/:account/:name/ips/change-ipv4` | 更换指定私有 IP 的公网 IPv4 |
| POST | `/api/vm/:provider/:account/:name/ips/attach-ipv4` | 附加辅助私有 IP / 临时公网 IPv4 |
| POST | `/api/vm/:provider/:account/:name/ips/delete-ipv4` | 删除辅助私有 IP 或解绑/释放公网 IPv4 |
| POST | `/api/vm/:provider/:account/:name/ips/attach-reserved` | 为私有 IP 分配或绑定保留公网 IPv4 |
| POST | `/api/vm/:provider/:account/:name/ips/attach-ipv6` | 为 VNIC 分配 IPv6 地址（`/80`–`/128`） |
| POST | `/api/vm/:provider/:account/:name/ips/change-ipv6` | 先删再建更换 `/128` IPv6 地址 |
| POST | `/api/vm/:provider/:account/:name/ips/delete-ipv6` | 移除指定 IPv6 地址 |
| POST | `/api/vm/:provider/:account/:name/ips/enable-ipv6` | 一键为 VCN/子网启用 IPv6 并按需补充路由与出站规则 |

## 6. OCI 创建实例、抢购任务与流量监控

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/oci/:account/create-options` | 获取可用域、Shape 列表、VCN/子网/NSG 选项 |
| GET | `/api/oci/:account/create-images` | 根据 Shape 获取兼容系统镜像列表 |
| POST | `/api/oci/:account/default-network` | 一键创建默认 VCN、公有子网、IGW（可选启用 IPv6） |
| POST | `/api/oci/:account/launch-instance` | 单次立即发起 OCI 实例创建 |
| GET | `/api/oci/create-tasks` | 获取持久化抢购任务列表 |
| POST | `/api/oci/create-tasks` | 新建后台自动抢购任务 |
| PUT | `/api/oci/create-tasks/:taskID` | 修改抢购任务配置 |
| DELETE | `/api/oci/create-tasks/:taskID` | 删除抢购任务 |
| POST | `/api/oci/create-tasks/:taskID/start` | 启动或恢复抢购任务 |
| POST | `/api/oci/create-tasks/:taskID/stop` | 手动停止抢购任务 |
| GET | `/api/oci/:account/data-transfer` | 手动查询 OCI 当月 VCN 数据传输用量 |
| GET | `/api/oci/:account/data-transfer/config` | 查询 OCI 数据传输监控配置 |
| POST | `/api/oci/:account/data-transfer/config` | 保存 OCI 数据传输监控配置 |
| POST | `/api/oci/:account/data-transfer/start` | 启动 OCI 数据传输周期监控 |
| POST | `/api/oci/:account/data-transfer/stop` | 停止 OCI 数据传输周期监控 |
| GET | `/api/oci/:account/data-transfer/status` | 查询 OCI 数据传输监控运行状态 |

## 7. OCI 控制台：概览、IAM 与对象存储

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/oci/:account/overview` | 获取 Tenancy 信息、当月开销及 Limits 配额 |
| GET | `/api/oci/:account/iam` | 查询 IAM 用户、用户组、成员关系与可用区间 |
| POST | `/api/oci/:account/iam/users` | 创建 IAM 用户 |
| DELETE | `/api/oci/:account/iam/users/:userID` | 删除 IAM 用户 |
| POST | `/api/oci/:account/iam/users/:userID/password` | 重置用户控制台一次性密码 |
| GET | `/api/oci/:account/iam/users/:userID/api-keys` | 查看用户已绑定的 API Key 列表 |
| POST | `/api/oci/:account/iam/users/:userID/api-keys` | 为用户上传 API 公钥 |
| DELETE | `/api/oci/:account/iam/users/:userID/api-keys/:fingerprint` | 删除用户 API Key |
| POST | `/api/oci/:account/iam/groups` | 创建 IAM 用户组 |
| DELETE | `/api/oci/:account/iam/groups/:groupID` | 删除 IAM 用户组 |
| POST | `/api/oci/:account/iam/memberships` | 将用户添加到用户组 |
| DELETE | `/api/oci/:account/iam/memberships/:membershipID` | 移除用户组成员关系 |
| GET | `/api/oci/:account/iam/policies` | 按区间列出 IAM Policy |
| POST | `/api/oci/:account/iam/policies` | 创建 IAM Policy |
| PUT | `/api/oci/:account/iam/policies/:policyID` | 更新 IAM Policy 语句或描述 |
| DELETE | `/api/oci/:account/iam/policies/:policyID` | 删除 IAM Policy |
| GET | `/api/oci/:account/os/buckets` | 获取对象存储 Bucket 列表 |
| POST | `/api/oci/:account/os/buckets` | 创建 Bucket |
| DELETE | `/api/oci/:account/os/buckets/:bucket` | 删除 Bucket |
| GET | `/api/oci/:account/os/buckets/:bucket/objects` | 列出对象/目录（支持 `?includeBucket=1` 获取当前 Bucket 统计） |
| POST | `/api/oci/:account/os/buckets/:bucket/folder` | 在 Bucket 中新建文件夹占位对象 |
| POST | `/api/oci/:account/os/buckets/:bucket/upload` | 上传文件到 Bucket |
| GET | `/api/oci/:account/os/buckets/:bucket/download` | 下载 Bucket 中的对象 |
| DELETE | `/api/oci/:account/os/buckets/:bucket/objects` | 删除对象 |
| POST | `/api/oci/:account/os/buckets/:bucket/par` | 创建对象预认证访问链接 (PAR) |
| GET | `/api/vm/:provider/:account/:name/boot-volume` | 获取指定 OCI 实例的启动盘及同可用域可附加启动盘 |
| GET | `/api/oci/:account/boot-volumes` | 列出 OCI 账号全部启动盘及实例挂载信息 |
| PUT | `/api/oci/:account/boot-volumes/:bootVolumeID` | 调整启动盘大小 (`sizeInGBs`) 或性能单元 (`vpusPerGB`) |
| DELETE | `/api/oci/:account/boot-volumes/:bootVolumeID` | 删除已卸载的启动盘 |
| POST | `/api/oci/:account/boot-volumes/:bootVolumeID/attach` | 将未挂载的启动盘附加到指定实例 |
| POST | `/api/oci/:account/boot-volume-attachments/:attachmentID/detach` | 从已停止实例卸载启动盘 |

## 8. 账号级防火墙管理（OCI / GCP / Azure）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/oci/:account/security-lists` | 获取 OCI 账号下所有安全列表 |
| PUT | `/api/oci/:account/security-lists/:listID/rules` | 保存 OCI 安全列表规则 |
| GET | `/api/oci/:account/network-security-groups` | 获取 OCI 账号下所有网络安全组 |
| PUT | `/api/oci/:account/network-security-groups/:groupID` | 重命名 OCI 网络安全组 |
| DELETE | `/api/oci/:account/network-security-groups/:groupID` | 删除 OCI 网络安全组 |
| PUT | `/api/oci/:account/network-security-groups/:groupID/rules` | 保存 OCI 网络安全组规则 |
| GET | `/api/oci/:account/route-tables` | 获取 OCI 账号下所有路由表及可用网关实体 |
| PUT | `/api/oci/:account/route-tables/:routeTableID/rules` | 保存 OCI 路由表规则 |
| GET | `/api/gcp/:account/firewalls` | 获取 GCP VPC 防火墙规则列表 |
| POST | `/api/gcp/:account/firewalls` | 创建 GCP 防火墙规则 |
| PUT | `/api/gcp/:account/firewalls/:name` | 更新 GCP 防火墙规则 |
| PATCH | `/api/gcp/:account/firewalls/:name/toggle` | 启用/禁用 GCP 防火墙规则 |
| DELETE | `/api/gcp/:account/firewalls/:name` | 删除 GCP 防火墙规则 |
| GET | `/api/azure/:account/security-groups` | 获取 Azure 网络安全组 (NSG) 列表 |
| POST | `/api/azure/:account/security-groups/:nsg/rules` | 新增或更新 Azure NSG 规则 |
| DELETE | `/api/azure/:account/security-groups/:nsg/rules/:rule` | 删除 Azure NSG 规则 |

## 9. Cloudflare DNS 管理

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/dns/cloudflare` | 获取 Cloudflare 账号列表（敏感字段脱敏） |
| POST | `/api/dns/cloudflare` | 保存 Cloudflare 账号 |
| GET | `/api/dns/bindings` | 获取所有 DNS 绑定记录 |
| POST | `/api/dns/bindings` | 批量保存 DNS 绑定记录 |
| GET | `/api/dns/raw` | 预览脱敏后的 `config/dns.conf` |
| POST | `/api/dns/delete-binding` | 删除指定 DNS 绑定 |
| GET | `/api/vm/:provider/:account/:name/dns` | 获取指定 VM 的 DNS 绑定（`A` / `AAAA`） |
| POST | `/api/vm/:provider/:account/:name/dns` | 保存指定 VM 的 DNS 绑定 |
