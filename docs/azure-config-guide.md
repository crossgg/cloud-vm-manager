# Azure (微软云) 配置获取指南

本文档指导您如何获取 Azure 云账号的凭据参数，并在 **Cloud VM Manager** 面板中完成配置。

---

## 一、所需配置参数清单

在配置 Azure 账号时，您需要准备以下 6 个关键参数：

| 参数名称 | 配置项 Key | 说明与示例 |
| :--- | :--- | :--- |
| **订阅 ID** | `subscription_id` | 您的 Azure 订阅 UUID，例如 `12345678-abcd-1234-ef01-123456789abc` |
| **租户 ID** | `tenant_id` | Microsoft Entra ID 目录 ID，例如 `87654321-dcba-4321-10fe-cba987654321` |
| **客户端 ID** | `client_id` | 已注册应用程序的 Application ID，例如 `abcdef01-2345-6789-abcd-ef0123456789` |
| **客户端机密** | `client_secret` | 应用程序创建的密码密钥 Value（注意不是 Secret ID） |
| **资源组** | `resource_group` | 您的虚拟机所在的资源组名称，例如 `my-vm-rg` |
| **区域代码** | `location` | 虚拟机所在区域代码，例如 `koreacentral`, `eastasia`, `japaneast` |

---

## 二、详细获取步骤

### 第 1 步：获取订阅 ID (Subscription ID)
1. 登录 [Azure Portal (微软云控制台)](https://portal.azure.com/)。
2. 在顶部搜索框中输入并选择 **“订阅 (Subscriptions)”**。
3. 在订阅列表中，找到您正在使用的订阅，复制并记录 **“订阅 ID (Subscription ID)”**。

---

### 第 2 步：注册应用程序以获取 Client ID 和 Tenant ID
1. 在顶部搜索框中输入并进入 **“Microsoft Entra ID”**（原 Azure Active Directory）。
2. 在左侧菜单中点击 **“应用注册 (App registrations)”**。
3. 点击页面顶部的 **“+ 新注册 (+ New registration)”**：
   * **名称**：可输入自定义名称，例如 `cloud-vm-manager`。
   * **受支持的帐户类型**：选择 **“仅此组织目录中的帐户 (仅单一租户)”**。
   * **重定向 URI**：留空即可，无需填写。
4. 点击底部 **“注册 (Register)”**。
5. 注册成功后，页面会自动跳转到该应用的“概述 (Overview)”页面：
   * 复制 **应用程序(客户端) ID (Application (client) ID)** $\rightarrow$ 对应 `client_id`。
   * 复制 **目录(租户) ID (Directory (tenant) ID)** $\rightarrow$ 对应 `tenant_id`。

---

### 第 3 步：创建客户端密码 (Client Secret)
1. 在刚注册的应用管理页面，点击左侧菜单的 **“证书和密码 (Certificates & secrets)”**。
2. 切换到 **“客户端密码 (Client secrets)”** 标签页，点击 **“+ 新客户端密码 (+ New client secret)”**：
   * **说明**：可输入说明文字，例如 `vm-manager-key`。
   * **截止期限**：根据需要选择（建议选择最长的 24 个月）。
3. 点击 **“添加 (Add)”**。
4. ⚠️ **极其重要**：生成后，请**立即复制“值 (Value)”一列的内容**（对应 `client_secret`）。
   > **注意**：离开此页面或刷新后，“值”将被永久隐藏，请务必立即复制保存。

---

### 第 4 步：为应用程序授予订阅访问权限 (RBAC 角色分配)
为了让面板能够管理该订阅下的虚拟机、网络安全组及公共 IP，需要将该应用赋予订阅的权限：

1. 返回 Azure 门户主页，进入 **“订阅 (Subscriptions)”**，点击您的订阅名称。
2. 在左侧菜单中点击 **“访问控制 (IAM) (Access control)”**。
3. 点击顶部的 **“+ 添加 (+ Add)”** $\rightarrow$ **“添加角色分配 (Add role assignment)”**：
   * **角色 (Role)**：搜索并选择 **“参与者 (Contributor)”**（若需最小权限，可分别分配 Virtual Machine Contributor + Network Contributor）。
   * 点击页面下方的 **“下一步 (Next)”**。
   * **分配访问权限的目标**：选择“用户、组或服务主体”。
   * 点击 **“+ 选择成员 (+ Select members)”**，在右侧滑出的搜索框中输入第 2 步注册的应用名称（如 `cloud-vm-manager`），选中该应用。
4. 点击 **“查看并分配 (Review + assign)”**，完成授权。

---

### 第 5 步：确认资源组 (Resource Group) 与区域 (Location)
1. 在 Azure 门户中进入 **“虚拟机 (Virtual machines)”**。
2. 查看您的 VM 列表，记录：
   * **资源组名称**：例如 `my-resource-group`。
   * **区域代码 (Location)**：界面可能显示中文“韩国中部”，对应的英文标准代码为 `koreacentral`；“东亚”对应 `eastasia`；“日本东部”对应 `japaneast` 等。

---

## 三、在面板中配置与使用

### 方式 1：通过 Web 页面可视化添加（推荐）
1. 打开 Cloud VM Manager 网页控制台，进入 **“系统设置”**。
2. 在 **“云账号与配置管理”** 区块，点击 **“+ 添加云账号”**。
3. 选择服务商为 **Microsoft Azure**。
4. 填入刚才获取的 `账号标识名称`、`订阅 ID`、`租户 ID`、`客户端 ID`、`客户端机密`、`资源组`、`区域`。
5. 点击 **“保存云账号”**，系统将自动写入配置文件并热重载生效。

### 方式 2：直接编辑配置文件 (`config.conf`)
您也可以直接在服务器的 `config/config.conf` 中追加以下内容：

```ini
azure=begin
[az01]
group=azure
subscription_id=12345678-abcd-1234-ef01-123456789abc
tenant_id=87654321-dcba-4321-10fe-cba987654321
client_id=abcdef01-2345-6789-abcd-ef0123456789
client_secret=your-azure-client-secret-value
resource_group=my-resource-group
location=koreacentral
azure=end
```
保存后，在 Web 界面的“系统设置”中点击 **“重载配置”** 即可。
