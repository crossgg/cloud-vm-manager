# 更新日志

所有显著的功能新增、优化与问题修复均记录于此文件。

## [Unreleased]

## [v1.2.2] - 2026-11-26

### 新增
- **全功能多云 DNS 管理控制台**：
  - 在原有 Cloudflare 基础上新增对 **阿里云 DNS（Alidns）**、**腾讯云 DNS（API 3.0，区分 `cn` 国内版 / `intl` 国际版）** 与 **DNSPod（Legacy Token，区分 `cn` 国内版 / `intl` 国际版）** 的完整接入。
  - 网页端新增「解析控制台 / VM 联动绑定 / DNS 账号配置」三栏横向工作台，移除原只读脱敏 `dns.conf` 预览面板。
  - 原生记录管理支持按账号与域名按需懒加载查询、记录增删改、阿里云/腾讯云/DNSPod 记录启用与暂停、解析线路与 MX/SRV 优先级、Cloudflare Proxied 开关、备注、关键词实时搜索、记录类型筛选胶囊与记录值一键复制。
  - 安全边界控制：不枚举账号全量域名，仅管理账号配置中显式指定的根域名（Cloudflare 未填域名时仅通过所填单个 Zone ID 解析）。
- **BIND Zone（`.zone` / `.txt`）导入预览与导出**：支持导出当前域名全部记录为标准 `.zone` 文件；导入前先与现网对比展示「新增 / 冲突 / 系统跳过（SOA 与根域 `@` NS）」预览表格，**默认跳过同名同类型冲突记录**，支持按需勾选覆盖。
- **跨服务商 VM 联动绑定**：VM 卡片的「DNS 绑定」弹窗支持选用 Cloudflare / 阿里云 / 腾讯云 / DNSPod 账号（非 Cloudflare 自动隐藏 CDN 代理开关），并在后端严格校验仅更新明确匹配云厂商、账号、VM 实例与记录类型（`A` / `AAAA`）的绑定记录。

### 优化与修复
- 重构「DNS 账号配置」为现代左右分栏工作台，支持服务商可视化选卡、国内版/国际版分段切换、域名标签列表与敏感凭据脱敏保存（编辑留空保持原密钥）。
- 修复暗色模式下 DNS 管理页及弹窗内输入框、下拉框与 `<option>` 选项白底白字对比度问题，统一明暗主题视觉体验。
- 优化「解析控制台」加载时机，进入 DNS 管理页面时不直接请求远端 API，选定服务商账号与目标域名后才拉取解析记录。
- 同步更新 `README.md`、`CONFIGURATION.md`、`docs/api.md` 与 `docs/manual-config.md` 中的多云 DNS 说明与接口文档。

## [v1.2.1] - 2026-11-19

### 优化
- 重构通知中心与 Telegram Bot 配置界面，优化账号管理与发布工作流。

## [v1.2.0] - 2026-11-19

### 新增
- **多渠道单向通知中心**：参考 `webhook-notify` 接入 `bark`、`wecom_bot`（企业微信机器人）、`wecom_app`（企业微信应用）、`feishu`（飞书机器人）、`telegram`、`pushplus` 与 `custom`（自定义 Webhook）7 种推送渠道；支持每个渠道独立绑定代理池节点与 Fallback、冷却防刷屏、账号过滤及连通性测试，并在面板展示最近推送记录。
- **可手动配置的通知触发点**：支持在全局及各渠道/Bot 维度手动勾选 6 类触发事件：`oci_task_succeeded`（OCI 抢购成功，失败重试不通知）、`oci_dt_alert`（OCI 流量告警与自动停机）、`azure_credit_alert`（Azure 赠金/费用阈值告警与自动动作）、`vm_change_ip`（更换公网 IP）、`vm_power`（实例开关机/重启）及 `auth_login`（面板登录成功与异常锁定告警）；配置独立保存在 `config/notify.conf`（附 [notify.example.conf](config/notify.example.conf)）并支持 WebUI 可视化保存与热重载。
- **可走代理的交互式 Telegram Bot**：支持绑定代理池（HTTP / HTTPS / SOCKS5）与备用回退策略进行长轮询交互，强制 `allowedUserIds` / `allowedChatIds` 白名单鉴权与敏感操作二次确认；支持 `/help`、`/oracle`、`/azure`、`/gcp` 菜单式交互，开放实例查看/开关机重启/换 IP（若绑定 Cloudflare DNS，换 IP 成功后提供一键「同步更新 DNS」按钮）、OCI 抢购任务启停与日志查看、OCI 流量与 Azure 赠金监控状态/日志查看及立即检测；开启 `auto_notify` 后自动复用该 Bot 接收触发点通知。
- **Azure 与 GCP 更多管理能力**：新增跨云集中式 IP 管理（Azure / GCP / OCI 统一查看与更换 IPv4/IPv6、保留 IP 与 DNS 联动）；新增 Azure 创建实例、默认网络初始化、成本与赠金监控（告警 / 自动停机 / 孤立资源或计费资源清理）；新增 GCP 创建实例与闲置资源扫描清理、OCI 闲置资源扫描清理。
- **OCI 启动盘 (Boot Volume) 管理**：支持查看实例关联启动盘及账号下全部启动盘、调整容量与性能单元、附加/卸载及删除未挂载启动盘。
- **公网部署安全加固与文档**：新增 `netguard` SSRF 防护与 [公网部署安全指南](docs/public-deployment.md)（`BLOCK_PRIVATE_PROXY`、`TRUSTED_PROXIES`、`DISABLE_SELF_UPDATE` 等）。

### 优化与安全修复
- 修复若干安全隐患：强化代理、在线更新下载、登录认证、同源（CSRF）校验及路径/配置输入校验，完善敏感凭据脱敏回显与还原逻辑。
- 修复 `custom` Webhook 在 JSON 模板中使用 `$title` / `$content` 时的转义问题，避免含引号或换行内容破坏 JSON 结构。
- 优化多云防火墙、IP 管理与监控工作区的前端交互与错误提示。

## [v1.1.4] - 2026-11-17

### 新增
- **OCI 控制台工作区**：在「实例管理」选中 OCI 账号后展示横向功能标签页，整合「概述」「实例管理」「云监控」「用户管理（IAM）」「创建实例」「任务管理」「对象存储」。
- **OCI 账户概述与配额查询**：展示 Tenancy 基本信息、Home Region 计费周期开销（无 Usage 权限时优雅降级提示）以及 AMD / ARM / 存储 / VCN / 保留 IP 等 Limits 配额使用情况。
- **OCI 集中式 IPv4 / IPv6 全生命周期管理**：支持统一查看 VNIC 主/辅私有 IP、更换/附加/删除 IPv4（临时与保留公网 IP），并提供独立的**开启子网 IPv6 向导**（预检 VCN `/56`、子网 2 位 Hex `/64`、路由表 `::/0 -> IGW` 与安全列表状态，默认不强制改写防火墙规则，支持按需勾选添加出站 `::/0`）、分配 IPv6（`/80`–`/128`）及先删后建更换 `/128` IPv6，同时支持与 Cloudflare DNS 联动更新 `A` / `AAAA` 记录。
- **OCI 创建实例与自动抢购任务**：内置 AMD 免费 / ARM 免费（2C 12G） / 自定义模板，支持一键创建默认 VCN、公有子网与 IGW（可选同步开启 IPv6 与默认安全列表 `::/0` 出站），支持单次立即创建或按随机秒数区间在后台持续自动抢购（本地 JSON 持久化并在重启后自动恢复，仅在创建成功或手动停止时终止）。
- **OCI IAM 与策略管理**：支持 IAM 用户与用户组增删、成员关系维护、重置控制台一次性密码、API Key 查看/上传/自动生成 RSA 密钥对，以及按区间管理 Policy（内置官方 Policy Builder 场景模板与语句构造器）。
- **OCI 对象存储 (Object Storage) 管理**：支持 Bucket 创建与删除、目录浏览、新建文件夹、文件上传与下载、生成预认证链接（PAR）与对象删除，并在切换 Bucket 时按需查询统计信息与访问权限。
- **OCI 网络安全组 (NSG) 增强与实例安全删除**：支持在实例侧选择已有 NSG 进行绑定或解绑，在「防火墙管理」重命名或删除 NSG；支持安全删除 OCI 实例（可选保留引导卷，并需输入实例名称进行二次确认）。
- **在线更新说明展示与发布流程**：检查更新时同步获取并展示 GitHub Release 更新日志正文与发布页链接；发布工作流支持从 `CHANGELOG.md` 自动提取对应版本更新日志。

### 优化与修复
- 修复 OCI `availabilityDomains` 错误请求 `iaas` 域名导致返回 `404` 的问题（改为请求 `identity.<region>`），并修正 VCN/子网启用 IPv6 的官方 API 端点（`actions/addIpv6Cidr`）及 `/56` 前缀异步轮询。
- 修复 OCI 对象存储 `ListBuckets` 传入非法 `fields` 参数（`approximateCount` / `approximateSize`）导致 `400 InvalidBucketProperty` 的问题，改为切换 Bucket 时按需调用 `GetBucket`。
- 修复无公网 IP 实例误触发换 IP 的问题，在后端明确校验并拒绝无公网 IPv4 时的换 IP 请求。
- 修复 Cloudflare DNS 绑定与更新对 IPv4（`A` 记录）与 IPv6（`AAAA` 记录）的校验及联动逻辑，支持选定指定 IPv6 更新 DNS。
- 修复深色主题下创建实例表单多行文本框可读性与 `.form-grid` 底部对齐错位问题，并优化监控面板高度与任务按钮排版。
