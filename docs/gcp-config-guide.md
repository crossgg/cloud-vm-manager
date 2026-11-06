# Google Cloud Platform (GCP 谷歌云) 配置获取指南

本文档指导您如何获取 Google Cloud Platform (GCP) 的服务账号 API 凭据，并在 **Cloud VM Manager** 面板中完成配置。

---

## 一、所需配置参数清单

在配置 GCP 账号时，您需要准备以下 3 个关键参数：

| 参数名称 | 配置项 Key | 说明与示例 |
| :--- | :--- | :--- |
| **项目 ID** | `project_id` | GCP 项目唯一标识符，例如 `my-project-12345`（注意不是项目显示名称） |
| **服务账号邮箱** | `client_email` | 服务账号生成的邮箱地址，例如 `vm-manager@my-project-12345.iam.gserviceaccount.com` |
| **密钥凭据文件** | `key_file` | 服务账号私钥在容器内的路径，例如 `/app/config/keys/gcp01.pem` 或 `/app/config/keys/gcp01.json` |

---

## 二、详细获取步骤

### 第 1 步：获取 GCP 项目 ID (Project ID)
1. 登录 [Google Cloud Console (谷歌云控制台)](https://console.cloud.google.com/)。
2. 查看控制台顶部导航栏正中央的项目下拉选择器。
3. 点击下拉菜单，在弹出的项目列表中，查看并复制对应项目的 **“项目 ID (ID)”**（对应 `project_id`）。
   > **注意**：必须是小写的系统分配 ID，而不是您自己起的项目显示名称。

---

### 第 2 步：确认启用 Compute Engine API
1. 在左侧导航菜单中，进入 **“API 和服务 (APIs & Services)”** $\rightarrow$ **“已启用的 API 和服务”**。
2. 检查是否已包含 **“Compute Engine API”**。
3. 如果未启用，请在顶部搜索框输入 `Compute Engine API` 并点击 **“启用 (Enable)”**。

---

### 第 3 步：创建服务账号 (Service Account)
1. 在左侧主导航菜单中，点击 **“IAM 和管理 (IAM & Admin)”** $\rightarrow$ **“服务账号 (Service Accounts)”**。
2. 点击页面上方的 **“+ 创建服务账号 (+ Create Service Account)”**：
   * **服务账号名称**：例如 `cloud-vm-manager`。
   * **服务账号 ID**：系统会自动填充。
   * **说明**：可填 `用于多云控制面板运维管理`。
3. 点击 **“创建并继续 (Create and continue)”**。

---

### 第 4 步：授予管理权限 (Roles)
在“向此服务账号授予对项目的访问权限”步骤中，为该账号配置所需权限：
1. **虚拟机运维权限**：
   * 点击“选择角色”，搜索并选择 **“Compute 实例管理员 (v1)” (Compute Instance Admin (v1))**。
2. **防火墙规则管理权限（可选，若需管理防火墙请添加）**：
   * 点击 **“+ 添加其他角色 (+ Add another role)”**。
   * 搜索并选择 **“Compute 网络管理员” (Compute Network Admin)**。
   > 💡 **快速简便选项**：亦可直接选择 **“Compute 管理员 (Compute Admin)”** 角色，该角色集成了实例管理与网络/防火墙管理权限。
3. 点击 **“继续 (Continue)”**，最后点击 **“完成 (Done)”**。
4. 返回服务账号列表，找到刚创建的账号，复制其 **“电子邮件 (Email)”**（对应 `client_email`）。

---

### 第 5 步：创建并下载服务账号私钥 (Key File)
1. 在服务账号列表中，点击刚创建的服务账号名称进入详情页。
2. 切换到顶部的 **“密钥 (Keys)”** 标签页。
3. 点击 **“添加密钥 (Add key)”** $\rightarrow$ **“创建新密钥 (Create new key)”**。
4. **密钥类型**：选择 **JSON**（推荐，系统原生支持解析 JSON 凭据，或者选择/提取 PEM 私钥格式）。
5. 点击 **“创建 (Create)”**。
6. 浏览器会自动将密钥文件下载保存到您的本地电脑。

---

## 三、在面板中配置与使用

### 方式 1：通过 Web 页面可视化添加（强烈推荐）
1. 打开 Cloud VM Manager 网页控制台，进入 **“系统设置”** 页面。
2. 在 **“云账号与配置管理”** 区块，点击 **“+ 添加云账号”**。
3. 服务商选择 **Google Cloud Platform (GCP)**。
4. 填写 `账号标识名称`（如 `gcp01`）、`GCP 项目 ID`、`服务账号邮箱`。
5. **上传凭据/私钥**：
   * 在“私钥/凭据路径”旁，点击 **“上传凭据/私钥”** 按钮；
   * 选择刚刚在 GCP 下载的 JSON 凭据文件（或 PEM 密钥）；
   * 面板后台会自动将其存入容器的 `/app/config/keys/` 目录并自动填好路径。
6. 点击 **“保存云账号”**，系统自动完成写入并热重载生效！

### 方式 2：手动存入密钥并编辑配置文件 (`config.conf`)
1. 将下载的密钥文件保存到容器映射的 `config/keys/gcp01.pem` 或 `config/keys/gcp01.json`。
2. 在服务器的 `config/config.conf` 中追加以下配置段：

```ini
gcp=begin
[gcp01]
group=gcp
project_id=my-project-12345
client_email=cloud-vm-manager@my-project-12345.iam.gserviceaccount.com
key_file=/app/config/keys/gcp01.pem
gcp=end
```
保存后，在 Web 页面“系统设置”中点击 **“重载配置”** 即可。
