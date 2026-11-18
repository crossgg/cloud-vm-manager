package azure

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type AzureImagePreset struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Publisher string `json:"publisher"`
	Offer     string `json:"offer"`
	SKU       string `json:"sku"`
	Version   string `json:"version"`
}

type AzureVMSizeOption struct {
	Name          string `json:"name"`
	NumberOfCores int    `json:"numberOfCores"`
	MemoryInMB    int    `json:"memoryInMb"`
	Label         string `json:"label"`
}

type AzureSubnetOption struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	VNetName   string   `json:"vnetName"`
	VNetID     string   `json:"vnetId"`
	Location   string   `json:"location"`
	RG         string   `json:"resourceGroup"`
	IPv4CIDR   string   `json:"ipv4Cidr"`
	IPv6CIDRs  []string `json:"ipv6Cidrs"`
	HasIPv6    bool     `json:"hasIpv6"`
}

type AzureCreateOptions struct {
	SubscriptionID string              `json:"subscriptionId"`
	DefaultRG      string              `json:"defaultResourceGroup"`
	DefaultLoc     string              `json:"defaultLocation"`
	Locations      []string            `json:"locations"`
	ResourceGroups []string            `json:"resourceGroups"`
	VMSizes        []AzureVMSizeOption `json:"vmSizes"`
	Subnets        []AzureSubnetOption `json:"subnets"`
	NSGs           []AzureNSG          `json:"nsgs"`
	Images         []AzureImagePreset  `json:"images"`
}

type AzureDefaultNetworkRequest struct {
	ResourceGroup string `json:"resourceGroup"`
	Location      string `json:"location"`
	VNetName      string `json:"vnetName,omitempty"`
	EnableIPv6    bool   `json:"enableIpv6"`
}

type AzureLaunchInstanceRequest struct {
	Name            string `json:"name"`
	ResourceGroup   string `json:"resourceGroup"`
	Location        string `json:"location"`
	Zone            string `json:"zone,omitempty"`
	VMSize          string `json:"vmSize"`
	ImageID         string `json:"imageId"`
	Publisher       string `json:"publisher,omitempty"`
	Offer           string `json:"offer,omitempty"`
	SKU             string `json:"sku,omitempty"`
	Version         string `json:"version,omitempty"`
	OSDiskType      string `json:"osDiskType"`
	OSDiskSizeGB    int    `json:"osDiskSizeGb"`
	SubnetID        string `json:"subnetId"`
	NSGID           string `json:"nsgId,omitempty"`
	AssignPublicV4  bool   `json:"assignPublicIpv4"`
	AssignIPv6      bool   `json:"assignIpv6"`
	Spot            bool   `json:"spot"`
	AdminUsername   string `json:"adminUsername"`
	AuthType        string `json:"authType"` // "ssh" | "password"
	SSHPublicKey    string `json:"sshPublicKey,omitempty"`
	AdminPassword   string `json:"adminPassword,omitempty"`
	CustomData      string `json:"customData,omitempty"`
}

func defaultAzureLocations() []string {
	return []string{
		"japaneast", "japanwest", "koreacentral", "eastasia", "southeastasia",
		"eastus", "eastus2", "westus2", "westus3", "centralus",
		"canadacentral", "uksouth", "westeurope", "northeurope", "germanywestcentral",
		"francecentral", "swedencentral", "australiaeast", "centralindia",
	}
}

func defaultAzureImages() []AzureImagePreset {
	return []AzureImagePreset{
		{ID: "ubuntu-2404", Label: "Ubuntu 24.04 LTS (Noble)", Publisher: "Canonical", Offer: "ubuntu-24_04-lts", SKU: "server", Version: "latest"},
		{ID: "ubuntu-2204", Label: "Ubuntu 22.04 LTS (Jammy)", Publisher: "Canonical", Offer: "0001-com-ubuntu-server-jammy", SKU: "22_04-lts-gen2", Version: "latest"},
		{ID: "debian-12", Label: "Debian 12 (Bookworm)", Publisher: "Debian", Offer: "debian-12", SKU: "12-gen2", Version: "latest"},
		{ID: "debian-11", Label: "Debian 11 (Bullseye)", Publisher: "Debian", Offer: "debian-11", SKU: "11-gen2", Version: "latest"},
		{ID: "rocky-9", Label: "Rocky Linux 9", Publisher: "resf", Offer: "rockylinux-x86_64", SKU: "9-base", Version: "latest"},
		{ID: "almalinux-9", Label: "AlmaLinux 9", Publisher: "almalinux", Offer: "almalinux-x86_64", SKU: "9-gen2", Version: "latest"},
	}
}

func (az *AzureService) GetCreateOptions(location string) (*AzureCreateOptions, error) {
	loc := strings.TrimSpace(location)
	if loc == "" {
		loc = valueOrDefault(az.Account.Location, "japaneast")
	}

	locations := defaultAzureLocations()
	hasLoc := false
	for _, l := range locations {
		if strings.EqualFold(l, loc) {
			hasLoc = true
			break
		}
	}
	if !hasLoc && loc != "" {
		locations = append([]string{loc}, locations...)
	}

	rgs := az.listResourceGroupNames()
	if az.Account.ResourceGroup != "" {
		foundRG := false
		for _, rg := range rgs {
			if strings.EqualFold(rg, az.Account.ResourceGroup) {
				foundRG = true
				break
			}
		}
		if !foundRG {
			rgs = append([]string{az.Account.ResourceGroup}, rgs...)
		}
	}

	sizes := az.listLocationVMSizes(loc)
	subnets := az.listAllSubnets()
	nsgs, _ := az.ListNSGs()

	return &AzureCreateOptions{
		SubscriptionID: az.Account.SubId,
		DefaultRG:      az.Account.ResourceGroup,
		DefaultLoc:     loc,
		Locations:      locations,
		ResourceGroups: rgs,
		VMSizes:        sizes,
		Subnets:        subnets,
		NSGs:           nsgs,
		Images:         defaultAzureImages(),
	}, nil
}

func (az *AzureService) listResourceGroupNames() []string {
	reqURL := fmt.Sprintf("https://management.azure.com/subscriptions/%s/resourcegroups?api-version=2021-04-01", url.PathEscape(az.Account.SubId))
	items, err := az.fetchPagedARMList(reqURL)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if n := stringValFromMap(item, "name"); n != "" && !strings.EqualFold(n, "NetworkWatcherRG") {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func (az *AzureService) listLocationVMSizes(location string) []AzureVMSizeOption {
	preferred := map[string]int{
		"Standard_B1s":      1,
		"Standard_B1ms":     2,
		"Standard_B2s":      3,
		"Standard_B2ats_v2": 4,
		"Standard_B2pts_v2": 5,
		"Standard_D2s_v3":   6,
		"Standard_D2s_v5":   7,
	}

	reqURL := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/providers/Microsoft.Compute/locations/%s/vmSizes?api-version=2023-03-01",
		url.PathEscape(az.Account.SubId),
		url.PathEscape(location),
	)
	resp, err := az.request("GET", reqURL, nil)
	if err != nil {
		return fallbackAzureVMSizes()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fallbackAzureVMSizes()
	}
	var payload struct {
		Value []struct {
			Name          string `json:"name"`
			NumberOfCores int    `json:"numberOfCores"`
			MemoryInMB    int    `json:"memoryInMB"`
		} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil || len(payload.Value) == 0 {
		return fallbackAzureVMSizes()
	}

	out := make([]AzureVMSizeOption, 0, len(payload.Value))
	for _, item := range payload.Value {
		if item.Name == "" {
			continue
		}
		out = append(out, AzureVMSizeOption{
			Name:          item.Name,
			NumberOfCores: item.NumberOfCores,
			MemoryInMB:    item.MemoryInMB,
			Label:         fmt.Sprintf("%s (%d vCPU, %.1f GB)", item.Name, item.NumberOfCores, float64(item.MemoryInMB)/1024.0),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		pi, oki := preferred[out[i].Name]
		pj, okj := preferred[out[j].Name]
		if oki && okj {
			return pi < pj
		}
		if oki != okj {
			return oki
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > 100 {
		out = out[:100]
	}
	return out
}

func fallbackAzureVMSizes() []AzureVMSizeOption {
	return []AzureVMSizeOption{
		{Name: "Standard_B1s", NumberOfCores: 1, MemoryInMB: 1024, Label: "Standard_B1s (1 vCPU, 1.0 GB · 学生免费适用)"},
		{Name: "Standard_B1ms", NumberOfCores: 1, MemoryInMB: 2048, Label: "Standard_B1ms (1 vCPU, 2.0 GB)"},
		{Name: "Standard_B2s", NumberOfCores: 2, MemoryInMB: 4096, Label: "Standard_B2s (2 vCPU, 4.0 GB)"},
		{Name: "Standard_B2ats_v2", NumberOfCores: 2, MemoryInMB: 1024, Label: "Standard_B2ats_v2 (2 vCPU, 1.0 GB · AMD)"},
		{Name: "Standard_B2pts_v2", NumberOfCores: 2, MemoryInMB: 1024, Label: "Standard_B2pts_v2 (2 vCPU, 1.0 GB · ARM64)"},
		{Name: "Standard_D2s_v3", NumberOfCores: 2, MemoryInMB: 8192, Label: "Standard_D2s_v3 (2 vCPU, 8.0 GB)"},
	}
}

func (az *AzureService) listAllSubnets() []AzureSubnetOption {
	vnets, err := az.listArmResources("Microsoft.Network/virtualNetworks", "2023-04-01", "")
	if err != nil {
		return []AzureSubnetOption{}
	}
	out := make([]AzureSubnetOption, 0)
	for _, vnet := range vnets {
		vnetID := stringValFromMap(vnet, "id")
		vnetName := stringValFromMap(vnet, "name")
		loc := stringValFromMap(vnet, "location")
		rg := extractResourceGroupFromID(vnetID)
		props, _ := vnet["properties"].(map[string]interface{})
		subnets, _ := props["subnets"].([]interface{})
		for _, rawSub := range subnets {
			sub, ok := rawSub.(map[string]interface{})
			if !ok {
				continue
			}
			subName := stringValFromMap(sub, "name")
			if strings.EqualFold(subName, "GatewaySubnet") || strings.EqualFold(subName, "AzureBastionSubnet") {
				continue
			}
			prefixes := extractAddressPrefixes(sub, false)
			var ipv4 string
			var v6s []string
			for _, p := range prefixes {
				if strings.Contains(p, ":") {
					v6s = append(v6s, p)
				} else if ipv4 == "" {
					ipv4 = p
				}
			}
			out = append(out, AzureSubnetOption{
				ID:        stringValFromMap(sub, "id"),
				Name:      subName,
				VNetName:  vnetName,
				VNetID:    vnetID,
				Location:  loc,
				RG:        rg,
				IPv4CIDR:  ipv4,
				IPv6CIDRs: v6s,
				HasIPv6:   len(v6s) > 0,
			})
		}
	}
	return out
}

func (az *AzureService) ensureResourceGroup(rg, location string) error {
	if rg == "" || location == "" {
		return fmt.Errorf("resourceGroup and location are required")
	}
	reqURL := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourcegroups/%s?api-version=2021-04-01",
		url.PathEscape(az.Account.SubId),
		url.PathEscape(rg),
	)
	resp, err := az.request("GET", reqURL, nil)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return nil
		}
	}

	raw, _ := json.Marshal(map[string]interface{}{"location": location})
	putResp, err := az.request("PUT", reqURL, bytes.NewBuffer(raw))
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(putResp.Body)
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK && putResp.StatusCode != http.StatusCreated {
		return fmt.Errorf("创建资源组 %s 失败 (%d): %s", rg, putResp.StatusCode, truncateErrBody(body))
	}
	return nil
}

func (az *AzureService) CreateDefaultNetwork(req AzureDefaultNetworkRequest) (map[string]interface{}, []string, error) {
	rg := valueOrDefault(strings.TrimSpace(req.ResourceGroup), az.Account.ResourceGroup)
	loc := valueOrDefault(strings.TrimSpace(req.Location), valueOrDefault(az.Account.Location, "japaneast"))
	if rg == "" {
		return nil, nil, fmt.Errorf("请指定资源组名称")
	}

	logs := make([]string, 0, 4)
	if err := az.ensureResourceGroup(rg, loc); err != nil {
		return nil, logs, err
	}
	logs = append(logs, fmt.Sprintf("已确认资源组 %s (%s)", rg, loc))

	suffix := fmt.Sprintf("%d", time.Now().Unix()%10000)
	nsgName := "default-nsg-" + suffix
	nsgID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkSecurityGroups/%s", url.PathEscape(az.Account.SubId), url.PathEscape(rg), url.PathEscape(nsgName))
	nsgBody := map[string]interface{}{
		"location": loc,
		"properties": map[string]interface{}{
			"securityRules": []map[string]interface{}{
				{
					"name": "Allow-SSH",
					"properties": map[string]interface{}{
						"priority":                 1000,
						"direction":                "Inbound",
						"access":                   "Allow",
						"protocol":                 "Tcp",
						"sourceAddressPrefix":      "*",
						"sourcePortRange":          "*",
						"destinationAddressPrefix": "*",
						"destinationPortRange":     "22",
					},
				},
			},
		},
	}
	if err := az.armPutJSON(nsgID, "2023-05-01", nsgBody, nil); err != nil {
		return nil, logs, fmt.Errorf("创建默认 NSG 失败: %w", err)
	}
	logs = append(logs, fmt.Sprintf("已创建默认安全组 NSG: %s (已放行 SSH 22)", nsgName))

	vnetName := valueOrDefault(strings.TrimSpace(req.VNetName), "default-vnet-"+suffix)
	vnetID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s", url.PathEscape(az.Account.SubId), url.PathEscape(rg), url.PathEscape(vnetName))

	vnetPrefixes := []string{"10.0.0.0/16"}
	subnetPrefixes := []string{"10.0.0.0/24"}
	if req.EnableIPv6 {
		ula, err := generateAzureULA48()
		if err != nil {
			return nil, logs, err
		}
		sub6, err := deriveSubnetIPv6FromParent(ula, "01")
		if err != nil {
			return nil, logs, err
		}
		vnetPrefixes = append(vnetPrefixes, ula)
		subnetPrefixes = append(subnetPrefixes, sub6)
	}

	vnetBody := map[string]interface{}{
		"location": loc,
		"properties": map[string]interface{}{
			"addressSpace": map[string]interface{}{
				"addressPrefixes": vnetPrefixes,
			},
			"subnets": []map[string]interface{}{
				{
					"name": "default",
					"properties": map[string]interface{}{
						"addressPrefixes":      subnetPrefixes,
						"networkSecurityGroup": map[string]interface{}{"id": nsgID},
					},
				},
			},
		},
	}

	var createdVNet map[string]interface{}
	if err := az.armPutJSON(vnetID, "2023-04-01", vnetBody, &createdVNet); err != nil {
		return nil, logs, fmt.Errorf("创建默认 VNet/子网失败: %w", err)
	}
	subnetID := vnetID + "/subnets/default"
	logs = append(logs, fmt.Sprintf("已创建默认 VNet %s / 子网 default (%s)", vnetName, strings.Join(subnetPrefixes, ", ")))

	return map[string]interface{}{
		"vnetId":   vnetID,
		"vnetName": vnetName,
		"subnetId": subnetID,
		"nsgId":    nsgID,
		"nsgName":  nsgName,
	}, logs, nil
}

func (az *AzureService) LaunchInstance(req AzureLaunchInstanceRequest) (map[string]interface{}, []string, error) {
	name := strings.TrimSpace(req.Name)
	rg := valueOrDefault(strings.TrimSpace(req.ResourceGroup), az.Account.ResourceGroup)
	loc := valueOrDefault(strings.TrimSpace(req.Location), valueOrDefault(az.Account.Location, "japaneast"))
	vmSize := valueOrDefault(strings.TrimSpace(req.VMSize), "Standard_B1s")
	adminUser := valueOrDefault(strings.TrimSpace(req.AdminUsername), "azureuser")

	if name == "" {
		return nil, nil, fmt.Errorf("实例名称不能为空")
	}
	if rg == "" {
		return nil, nil, fmt.Errorf("资源组不能为空")
	}

	authType := strings.ToLower(strings.TrimSpace(req.AuthType))
	if authType == "" {
		authType = "ssh"
	}
	if authType == "ssh" && strings.TrimSpace(req.SSHPublicKey) == "" {
		return nil, nil, fmt.Errorf("SSH 登录方式需要填写 SSH 公钥")
	}
	if authType == "password" && strings.TrimSpace(req.AdminPassword) == "" {
		return nil, nil, fmt.Errorf("密码登录方式需要填写管理员密码")
	}

	publisher, offer, sku, version := req.Publisher, req.Offer, req.SKU, req.Version
	if publisher == "" || offer == "" || sku == "" {
		for _, preset := range defaultAzureImages() {
			if preset.ID == req.ImageID {
				publisher, offer, sku, version = preset.Publisher, preset.Offer, preset.SKU, preset.Version
				break
			}
		}
	}
	if publisher == "" || offer == "" || sku == "" {
		preset := defaultAzureImages()[0]
		publisher, offer, sku, version = preset.Publisher, preset.Offer, preset.SKU, preset.Version
	}
	if version == "" {
		version = "latest"
	}

	logs := []string{fmt.Sprintf("[1/4] 准备在 %s (%s) 创建 Azure VM %s (%s)...", rg, loc, name, vmSize)}
	if err := az.ensureResourceGroup(rg, loc); err != nil {
		return nil, logs, err
	}

	subnetID := strings.TrimSpace(req.SubnetID)
	nsgID := strings.TrimSpace(req.NSGID)
	if subnetID == "" {
		netRes, netLogs, err := az.CreateDefaultNetwork(AzureDefaultNetworkRequest{
			ResourceGroup: rg,
			Location:      loc,
			EnableIPv6:    req.AssignIPv6,
		})
		if err != nil {
			return nil, logs, err
		}
		logs = append(logs, netLogs...)
		subnetID, _ = netRes["subnetId"].(string)
		if nsgID == "" {
			nsgID, _ = netRes["nsgId"].(string)
		}
	} else if req.AssignIPv6 {
		if err := az.ensureSubnetDualStackForLaunch(subnetID, &logs); err != nil {
			return nil, logs, err
		}
	}

	var zones []string
	if z := strings.TrimSpace(req.Zone); z != "" {
		zones = []string{z}
	}

	var pubV4ID, pubV6ID string
	cleanupCreated := func() {
		if pubV4ID != "" {
			_ = az.armDelete(pubV4ID, "2023-04-01")
		}
		if pubV6ID != "" {
			_ = az.armDelete(pubV6ID, "2023-04-01")
		}
	}

	ipConfigs := make([]map[string]interface{}, 0, 2)
	v4CfgProps := map[string]interface{}{
		"primary":                   true,
		"privateIPAddressVersion":   "IPv4",
		"privateIPAllocationMethod": "Dynamic",
		"subnet":                    map[string]interface{}{"id": subnetID},
	}
	if req.AssignPublicV4 {
		pubName := fmt.Sprintf("%s-ip", name)
		id, _, err := az.createPublicIPResource(rg, pubName, loc, "IPv4", "Standard", zones)
		if err != nil {
			return nil, logs, fmt.Errorf("创建公网 IPv4 失败: %w", err)
		}
		pubV4ID = id
		v4CfgProps["publicIPAddress"] = map[string]interface{}{"id": pubV4ID}
		logs = append(logs, fmt.Sprintf("[2/4] 已创建公网 IPv4: %s", pubName))
	}
	ipConfigs = append(ipConfigs, map[string]interface{}{
		"name":       "ipconfig-v4",
		"properties": v4CfgProps,
	})

	if req.AssignIPv6 {
		pub6Name := fmt.Sprintf("%s-ipv6", name)
		id, _, err := az.createPublicIPResource(rg, pub6Name, loc, "IPv6", "Standard", zones)
		if err != nil {
			cleanupCreated()
			return nil, logs, fmt.Errorf("创建公网 IPv6 失败: %w", err)
		}
		pubV6ID = id
		ipConfigs = append(ipConfigs, map[string]interface{}{
			"name": "ipconfig-v6",
			"properties": map[string]interface{}{
				"primary":                   false,
				"privateIPAddressVersion":   "IPv6",
				"privateIPAllocationMethod": "Dynamic",
				"subnet":                    map[string]interface{}{"id": subnetID},
				"publicIPAddress":           map[string]interface{}{"id": pubV6ID},
			},
		})
		logs = append(logs, fmt.Sprintf("[2/4] 已创建公网 IPv6: %s", pub6Name))
	}

	nicName := fmt.Sprintf("%s-nic", name)
	nicID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkInterfaces/%s", url.PathEscape(az.Account.SubId), url.PathEscape(rg), url.PathEscape(nicName))
	nicProps := map[string]interface{}{
		"ipConfigurations": ipConfigs,
	}
	if nsgID != "" {
		nicProps["networkSecurityGroup"] = map[string]interface{}{"id": nsgID}
	}
	if err := az.armPutJSON(nicID, "2023-04-01", map[string]interface{}{
		"location":   loc,
		"properties": nicProps,
	}, nil); err != nil {
		cleanupCreated()
		return nil, logs, fmt.Errorf("创建网卡失败: %w", err)
	}
	logs = append(logs, fmt.Sprintf("[3/4] 已创建网卡: %s", nicName))

	diskType := valueOrDefault(strings.TrimSpace(req.OSDiskType), "StandardSSD_LRS")
	osDisk := map[string]interface{}{
		"createOption": "FromImage",
		"deleteOption": "Delete",
		"managedDisk": map[string]interface{}{
			"storageAccountType": diskType,
		},
	}
	if req.OSDiskSizeGB > 0 {
		osDisk["diskSizeGB"] = req.OSDiskSizeGB
	}

	osProfile := map[string]interface{}{
		"computerName":  name,
		"adminUsername": adminUser,
	}
	if authType == "password" {
		osProfile["adminPassword"] = req.AdminPassword
		osProfile["linuxConfiguration"] = map[string]interface{}{
			"disablePasswordAuthentication": false,
		}
	} else {
		osProfile["linuxConfiguration"] = map[string]interface{}{
			"disablePasswordAuthentication": true,
			"ssh": map[string]interface{}{
				"publicKeys": []map[string]interface{}{
					{
						"path":    fmt.Sprintf("/home/%s/.ssh/authorized_keys", adminUser),
						"keyData": strings.TrimSpace(req.SSHPublicKey),
					},
				},
			},
		}
	}
	if cd := strings.TrimSpace(req.CustomData); cd != "" {
		osProfile["customData"] = base64.StdEncoding.EncodeToString([]byte(cd))
	}

	vmProps := map[string]interface{}{
		"hardwareProfile": map[string]interface{}{
			"vmSize": vmSize,
		},
		"storageProfile": map[string]interface{}{
			"imageReference": map[string]interface{}{
				"publisher": publisher,
				"offer":     offer,
				"sku":       sku,
				"version":   version,
			},
			"osDisk": osDisk,
		},
		"osProfile": osProfile,
		"networkProfile": map[string]interface{}{
			"networkInterfaces": []map[string]interface{}{
				{
					"id": nicID,
					"properties": map[string]interface{}{
						"deleteOption": "Delete",
					},
				},
			},
		},
	}
	if req.Spot {
		vmProps["priority"] = "Spot"
		vmProps["evictionPolicy"] = "Deallocate"
		vmProps["billingProfile"] = map[string]interface{}{"maxPrice": -1}
	}

	vmBody := map[string]interface{}{
		"location":   loc,
		"properties": vmProps,
	}
	if len(zones) > 0 {
		vmBody["zones"] = zones
	}

	vmID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s", url.PathEscape(az.Account.SubId), url.PathEscape(rg), url.PathEscape(name))
	logs = append(logs, fmt.Sprintf("[4/4] 正在创建虚拟机 %s...", name))
	if err := az.armPutJSON(vmID, "2023-03-01", vmBody, nil); err != nil {
		_ = az.armDelete(nicID, "2023-04-01")
		cleanupCreated()
		return nil, logs, fmt.Errorf("创建 Azure VM 失败: %w", err)
	}

	logs = append(logs, fmt.Sprintf("[4/4] Azure VM %s 创建成功！", name))
	if strings.EqualFold(rg, az.Account.ResourceGroup) {
		if vmInfo, err := az.GetVM(name); err == nil {
			return vmInfo, logs, nil
		}
	}
	return map[string]interface{}{"id": name, "name": name, "resourceGroup": rg, "location": loc}, logs, nil
}

func (az *AzureService) ensureSubnetDualStackForLaunch(subnetID string, logs *[]string) error {
	var subnet map[string]interface{}
	if err := az.armGetJSON(subnetID, "2023-04-01", &subnet); err != nil {
		return fmt.Errorf("读取子网失败: %w", err)
	}
	subPrefixes := extractAddressPrefixes(subnet, false)
	if len(filterIPv6CIDRs(subPrefixes)) > 0 {
		return nil
	}

	vnetID := extractVNetIDFromSubnetID(subnetID)
	var vnet map[string]interface{}
	if err := az.armGetJSON(vnetID, "2023-04-01", &vnet); err != nil {
		return fmt.Errorf("读取 VNet 失败: %w", err)
	}
	vnetPrefixes := extractAddressPrefixes(vnet, true)
	vnetV6 := filterIPv6CIDRs(vnetPrefixes)
	if len(vnetV6) == 0 {
		ula, err := generateAzureULA48()
		if err != nil {
			return err
		}
		vnetProps, _ := vnet["properties"].(map[string]interface{})
		addrSpace, _ := vnetProps["addressSpace"].(map[string]interface{})
		addrSpace["addressPrefixes"] = append(vnetPrefixes, ula)
		if err := az.armPutJSON(vnetID, "2023-04-01", vnet, nil); err != nil {
			return fmt.Errorf("为所选 VNet 开启 IPv6 空间失败: %w", err)
		}
		vnetV6 = []string{ula}
		*logs = append(*logs, "已自动为 VNet 补充 IPv6 地址空间: "+ula)
	}
	hexByte := pickFreeHex(collectAzureUsedIPv6Hexes(vnet), "01")
	sub6, err := deriveSubnetIPv6FromParent(vnetV6[0], hexByte)
	if err != nil {
		return err
	}
	subProps, _ := subnet["properties"].(map[string]interface{})
	delete(subProps, "addressPrefix")
	subProps["addressPrefixes"] = append(subPrefixes, sub6)
	if err := az.armPutJSON(subnetID, "2023-04-01", subnet, nil); err != nil {
		return fmt.Errorf("为所选子网开启 IPv6 失败: %w", err)
	}
	*logs = append(*logs, "已自动为子网开启 IPv6 前缀: "+sub6)
	return nil
}
