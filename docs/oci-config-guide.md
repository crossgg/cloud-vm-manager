# Oracle Cloud Infrastructure (OCI 甲骨文云) 配置获取指南

本文档指导您如何获取甲骨文云 (OCI) 的 API 认证凭据，并在 **Cloud VM Manager** 面板中完成配置。

---

## 一、所需配置参数清单

在配置 OCI 账号时，您需要准备以下 6 个关键参数：

| 参数名称 | 配置项 Key | 说明与示例 |
| :--- | :--- | :--- |
| **用户 OCID** | `user` | 以 `ocid1.user.oc1..` 开头的唯一标识符 |
| **租户 OCID** | `tenancy` | 以 `ocid1.tenancy.oc1..` 开头的唯一标识符 |
| **隔间 OCID** | `compartment_id` | 实例所在的隔间 OCID（通常与租户 OCID 相同） |
| **区域代码** | `region` | OCI 区域标识符，例如 `ap-tokyo-1`, `us-ashburn-1`, `ap-seoul-1` |
| **API 指纹** | `fingerprint` | API 密钥指纹，例如 `12:34:56:78:90:ab:cd:ef:12:34:56:78:90:ab:cd:ef` |
| **私钥文件** | `key_file` | API 私钥 `.pem` 在容器内的路径，例如 `/app/config/keys/oci.pem` |

---

## 二、详细获取步骤

### 第 1 步：获取租户 OCID (Tenancy OCID)
1. 登录 [Oracle Cloud 控制台](https://cloud.oracle.com/)。
2. 点击右上角的用户头像/设置图标，在下拉菜单中点击 **“租户: [您的租户名] (Tenancy: xxx)”**。
3. 在打开的“租户详细信息”页面中，找到 **OCID** 项。
4. 点击旁边的 **“复制 (Copy)”** 按钮，记录该值（对应 `tenancy`）。

---

### 第 2 步：获取用户 OCID (User OCID)
1. 再次点击右上角的用户头像，在下拉菜单中点击 **“用户设置 (User Settings)”**（或点击自己的用户名）。
2. 在打开的“用户详细信息”页面中，找到 **OCID** 项。
3. 点击旁边的 **“复制 (Copy)”** 按钮，记录该值（对应 `user`）。

---

### 第 3 步：生成并下载 API 私钥，获取指纹 (Fingerprint)
1. 在上述“用户详细信息”页面中，滚动到页面下方的 **“资源 (Resources)”** 栏目。
2. 在左侧资源列表中点击 **“API 密钥 (API Keys)”**。
3. 点击顶部的 **“添加 API 密钥 (Add API Key)”** 按钮：
   * 在弹出的对话框中，选择 **“生成 API 密钥对 (Generate API Key Pair)”**。
   * 点击 **“下载私有密钥 (Download Private Key)”**，将 `.pem` 私钥文件下载保存到本地电脑（请妥善保管好此文件）。
4. 点击底部的 **“添加 (Add)”** 按钮。
5. 添加成功后，页面会弹出一个“配置文件预览”窗口：
   * 在预览代码块中找到 `fingerprint = xx:xx:xx:...`。
   * 复制该指纹字符串，记录为 `fingerprint`。

---

### 第 4 步：确认区域标识 (Region) 与隔间 (Compartment)
1. **区域代码 (Region)**：
   * 在控制台顶部右侧可查看当前所在的区域名称。
   * 对应的标准区域代码示例：
     * 日本东部 (东京)：`ap-tokyo-1`
     * 韩国中部 (首尔)：`ap-seoul-1`
     * 韩国南部 (春川)：`ap-chuncheon-1`
     * 美国东部 (阿什本)：`us-ashburn-1`
     * 美国西部 (圣何塞)：`us-sanjose-1`
     * 德国中部 (法兰克福)：`eu-frankfurt-1`
2. **隔间 OCID (Compartment ID)**：
   * 如果您的虚拟机建在根隔间下，则 **隔间 OCID 与租户 OCID 完全一致**（在面板填入租户 OCID 即可）。
   * 若建在专属子隔间下，可通过控制台左上角导航菜单 $\rightarrow$ **“身份和安全”** $\rightarrow$ **“隔间 (Compartments)”** 查看并复制子隔间的 OCID。

---

## 三、在面板中配置与使用

### 方式 1：通过 Web 页面可视化添加（强烈推荐）
1. 打开 Cloud VM Manager 网页控制台，进入 **“系统设置”** 页面。
2. 在 **“云账号与配置管理”** 区块，点击 **“+ 添加云账号”**。
3. 服务商选择 **Oracle Cloud Infrastructure (OCI)**。
4. 输入 `账号标识名称`（如 `oci01`）、`用户 OCID`、`租户 OCID`、`指纹`、`区域`。
5. **上传私钥**：在“API 私钥文件路径”旁，直接点击 **“上传 PEM 密钥”** 按钮，选择刚才在 OCI 控制台下载的 `.pem` 私钥文件：
   * 面板后台会自动将私钥安全保存至容器的 `/app/config/keys/` 目录；
   * 输入框会自动填充正确的容器内路径（如 `/app/config/keys/oci01.pem`）。
6. 点击 **“保存云账号”**，系统自动完成写入并热重载生效！

### 方式 2：手动存入密钥并编辑配置文件 (`config.conf`)
1. 将下载的私钥 `.pem` 文件上传到容器挂载目录 `config/keys/oci.pem`（权限建议设为 `chmod 600`）。
2. 在服务器的 `config/config.conf` 中追加以下配置段：

```ini
oci=begin
[oci01]
group=oci
user=ocid1.user.oc1..aaaaaaaaxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
fingerprint=12:34:56:78:90:ab:cd:ef:12:34:56:78:90:ab:cd:ef
tenancy=ocid1.tenancy.oc1..aaaaaaaaxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
compartment_id=ocid1.tenancy.oc1..aaaaaaaaxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
region=ap-tokyo-1
key_file=/app/config/keys/oci.pem
oci=end
```
保存后，在 Web 页面“系统设置”中点击 **“重载配置”** 即可。
