package gcp

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

type GCPImagePreset struct {
	Label       string `json:"label"`
	SourceImage string `json:"sourceImage"`
	DefaultUser string `json:"defaultUser"`
}

type GCPNetworkOption struct {
	Name                  string `json:"name"`
	SelfLink              string `json:"selfLink"`
	AutoCreateSubnetworks bool   `json:"autoCreateSubnetworks"`
}

type GCPSubnetOption struct {
	Name               string `json:"name"`
	SelfLink           string `json:"selfLink"`
	Network            string `json:"network"`
	Region             string `json:"region"`
	IPv4CIDR           string `json:"ipv4Cidr"`
	StackType          string `json:"stackType"`
	IPv6AccessType     string `json:"ipv6AccessType,omitempty"`
	ExternalIPv6Prefix string `json:"externalIpv6Prefix,omitempty"`
}

type GCPMachineTypeOption struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	GuestCPUs   int    `json:"guestCpus"`
	MemoryMB    int    `json:"memoryMb"`
}

type GCPCreateOptions struct {
	ProjectID    string                 `json:"projectId"`
	SelectedZone string                 `json:"selectedZone"`
	Zones        []string               `json:"zones"`
	MachineTypes []GCPMachineTypeOption `json:"machineTypes"`
	Networks     []GCPNetworkOption     `json:"networks"`
	Subnets      []GCPSubnetOption      `json:"subnets"`
	Images       []GCPImagePreset       `json:"images"`
}

type GCPLaunchInstanceRequest struct {
	Name                string   `json:"name"`
	Zone                string   `json:"zone"`
	MachineType         string   `json:"machineType"`
	SourceImage         string   `json:"sourceImage"`
	DiskType            string   `json:"diskType"`
	DiskSizeGB          int      `json:"diskSizeGb"`
	Network             string   `json:"network"`
	Subnetwork          string   `json:"subnetwork"`
	AssignPublicIPv4    bool     `json:"assignPublicIpv4"`
	AssignIPv6          bool     `json:"assignIpv6"`
	ConfirmIrreversible bool     `json:"confirmIrreversible"`
	NetworkTier         string   `json:"networkTier"`
	Spot                bool     `json:"spot"`
	SSHUser             string   `json:"sshUser"`
	SSHPublicKey        string   `json:"sshPublicKey"`
	StartupScript       string   `json:"startupScript"`
	Tags                []string `json:"tags"`
}

type GCPResourceItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"` // disk | snapshot | address
	Scope       string `json:"scope"`    // zone/region/global
	ScopeType   string `json:"scopeType"`
	SizeGB      int    `json:"sizeGb,omitempty"`
	IPAddress   string `json:"ipAddress,omitempty"`
	Status      string `json:"status,omitempty"`
	AttachedTo  string `json:"attachedTo,omitempty"`
	Orphan      bool   `json:"orphan"`
	BillingNote string `json:"billingNote"`
}

type GCPResourceSummary struct {
	TotalCount       int `json:"totalCount"`
	DiskCount        int `json:"diskCount"`
	OrphanDiskCount  int `json:"orphanDiskCount"`
	SnapshotCount    int `json:"snapshotCount"`
	AddressCount     int `json:"addressCount"`
	UnusedStaticIPs  int `json:"unusedStaticIps"`
	BillableOrphans  int `json:"billableOrphans"`
}

type GCPResourceScanResult struct {
	ProjectID string               `json:"projectId"`
	Items     []GCPResourceItem    `json:"items"`
	Summary   GCPResourceSummary   `json:"summary"`
}

func defaultGCPImagePresets() []GCPImagePreset {
	return []GCPImagePreset{
		{Label: "Debian 12 (Bookworm)", SourceImage: "projects/debian-cloud/global/images/family/debian-12", DefaultUser: "debian"},
		{Label: "Debian 11 (Bullseye)", SourceImage: "projects/debian-cloud/global/images/family/debian-11", DefaultUser: "debian"},
		{Label: "Ubuntu 24.04 LTS (x86_64)", SourceImage: "projects/ubuntu-os-cloud/global/images/family/ubuntu-2404-lts-amd64", DefaultUser: "ubuntu"},
		{Label: "Ubuntu 22.04 LTS (x86_64)", SourceImage: "projects/ubuntu-os-cloud/global/images/family/ubuntu-2204-lts", DefaultUser: "ubuntu"},
		{Label: "Ubuntu 24.04 Minimal (x86_64)", SourceImage: "projects/ubuntu-os-cloud/global/images/family/ubuntu-minimal-2404-lts-amd64", DefaultUser: "ubuntu"},
		{Label: "Rocky Linux 9", SourceImage: "projects/rocky-linux-cloud/global/images/family/rocky-linux-9", DefaultUser: "rocky"},
		{Label: "CentOS Stream 9", SourceImage: "projects/centos-cloud/global/images/family/centos-stream-9", DefaultUser: "centos"},
	}
}

func fallbackGCPZones() []string {
	return []string{
		"us-central1-a", "us-central1-b", "us-west1-a", "us-east1-b", "us-east4-a",
		"asia-east1-a", "asia-east1-b", "asia-northeast1-a", "asia-southeast1-a",
		"europe-west1-b", "europe-west4-a",
	}
}

func (g *GCPService) GetCreateOptions(zone string) (*GCPCreateOptions, error) {
	zones := make([]string, 0)
	var zoneResp struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := g.requestJSON("GET", fmt.Sprintf("%s/projects/%s/zones", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID)), nil, &zoneResp); err == nil {
		for _, z := range zoneResp.Items {
			if strings.EqualFold(stringValue(z["status"]), "UP") {
				if n := stringValue(z["name"]); n != "" {
					zones = append(zones, n)
				}
			}
		}
		sort.Strings(zones)
	}
	if len(zones) == 0 {
		zones = fallbackGCPZones()
	}

	selectedZone := strings.TrimSpace(zone)
	if selectedZone == "" {
		selectedZone = "us-central1-a"
		found := false
		for _, z := range zones {
			if z == selectedZone {
				found = true
				break
			}
		}
		if !found && len(zones) > 0 {
			selectedZone = zones[0]
		}
	}
	region := regionFromZone(selectedZone)

	machineTypes := g.listGCPMachineTypes(selectedZone)
	networks := g.listGCPNetworks()
	subnets := g.listGCPSubnets(region)

	return &GCPCreateOptions{
		ProjectID:    g.Account.ProjectID,
		SelectedZone: selectedZone,
		Zones:        zones,
		MachineTypes: machineTypes,
		Networks:     networks,
		Subnets:      subnets,
		Images:       defaultGCPImagePresets(),
	}, nil
}

func (g *GCPService) listGCPMachineTypes(zone string) []GCPMachineTypeOption {
	preferredOrder := map[string]int{
		"e2-micro":      1,
		"e2-small":      2,
		"e2-medium":     3,
		"e2-standard-2": 4,
		"e2-standard-4": 5,
		"n2-standard-2": 6,
		"f1-micro":      7,
		"g1-small":      8,
	}
	var resp struct {
		Items []map[string]interface{} `json:"items"`
	}
	endpoint := fmt.Sprintf("%s/projects/%s/zones/%s/machineTypes", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID), url.PathEscape(zone))
	if err := g.requestJSON("GET", endpoint, nil, &resp); err != nil || len(resp.Items) == 0 {
		return []GCPMachineTypeOption{
			{Name: "e2-micro", Description: "2 vCPU (shared), 1 GB RAM (免费额度适用)", GuestCPUs: 2, MemoryMB: 1024},
			{Name: "e2-small", Description: "2 vCPU (shared), 2 GB RAM", GuestCPUs: 2, MemoryMB: 2048},
			{Name: "e2-medium", Description: "2 vCPU (shared), 4 GB RAM", GuestCPUs: 2, MemoryMB: 4096},
			{Name: "e2-standard-2", Description: "2 vCPU, 8 GB RAM", GuestCPUs: 2, MemoryMB: 8192},
			{Name: "n2-standard-2", Description: "2 vCPU, 8 GB RAM", GuestCPUs: 2, MemoryMB: 8192},
		}
	}

	out := make([]GCPMachineTypeOption, 0, len(resp.Items))
	for _, item := range resp.Items {
		name := stringValue(item["name"])
		if name == "" {
			continue
		}
		cpus, _ := item["guestCpus"].(float64)
		mem, _ := item["memoryMb"].(float64)
		desc := stringValue(item["description"])
		if cpus > 0 && mem > 0 && desc == "" {
			desc = fmt.Sprintf("%d vCPU, %.1f GB RAM", int(cpus), mem/1024.0)
		}
		out = append(out, GCPMachineTypeOption{
			Name:        name,
			Description: desc,
			GuestCPUs:   int(cpus),
			MemoryMB:    int(mem),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		pi, oki := preferredOrder[out[i].Name]
		pj, okj := preferredOrder[out[j].Name]
		if oki && okj {
			return pi < pj
		}
		if oki != okj {
			return oki
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

func (g *GCPService) listGCPNetworks() []GCPNetworkOption {
	var resp struct {
		Items []map[string]interface{} `json:"items"`
	}
	endpoint := fmt.Sprintf("%s/projects/%s/global/networks", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID))
	if err := g.requestJSON("GET", endpoint, nil, &resp); err != nil {
		return []GCPNetworkOption{{Name: "default", AutoCreateSubnetworks: true}}
	}
	out := make([]GCPNetworkOption, 0, len(resp.Items))
	for _, item := range resp.Items {
		autoMode, _ := item["autoCreateSubnetworks"].(bool)
		out = append(out, GCPNetworkOption{
			Name:                  stringValue(item["name"]),
			SelfLink:              stringValue(item["selfLink"]),
			AutoCreateSubnetworks: autoMode,
		})
	}
	return out
}

func (g *GCPService) listGCPSubnets(region string) []GCPSubnetOption {
	var resp struct {
		Items []map[string]interface{} `json:"items"`
	}
	endpoint := fmt.Sprintf("%s/projects/%s/regions/%s/subnetworks", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID), url.PathEscape(region))
	if err := g.requestJSON("GET", endpoint, nil, &resp); err != nil {
		return []GCPSubnetOption{}
	}
	out := make([]GCPSubnetOption, 0, len(resp.Items))
	for _, item := range resp.Items {
		out = append(out, GCPSubnetOption{
			Name:               stringValue(item["name"]),
			SelfLink:           stringValue(item["selfLink"]),
			Network:            resourceTail(stringValue(item["network"])),
			Region:             resourceTail(stringValue(item["region"])),
			IPv4CIDR:           stringValue(item["ipCidrRange"]),
			StackType:          valueOrDefault(stringValue(item["stackType"]), "IPV4_ONLY"),
			IPv6AccessType:     stringValue(item["ipv6AccessType"]),
			ExternalIPv6Prefix: stringValue(item["externalIpv6Prefix"]),
		})
	}
	return out
}

func (g *GCPService) LaunchInstance(req GCPLaunchInstanceRequest) (map[string]interface{}, []string, error) {
	name := strings.TrimSpace(req.Name)
	zone := strings.TrimSpace(req.Zone)
	machineType := strings.TrimSpace(req.MachineType)
	sourceImage := strings.TrimSpace(req.SourceImage)
	if name == "" {
		return nil, nil, fmt.Errorf("实例名称不能为空")
	}
	if zone == "" {
		return nil, nil, fmt.Errorf("请选择 Zone")
	}
	if machineType == "" {
		machineType = "e2-micro"
	}
	if sourceImage == "" {
		sourceImage = "projects/debian-cloud/global/images/family/debian-12"
	}
	diskType := valueOrDefault(strings.TrimSpace(req.DiskType), "pd-standard")
	diskSize := req.DiskSizeGB
	if diskSize <= 0 {
		diskSize = 10
	}
	networkName := valueOrDefault(strings.TrimSpace(req.Network), "default")
	subnetName := strings.TrimSpace(req.Subnetwork)
	region := regionFromZone(zone)
	tier := strings.ToUpper(valueOrDefault(strings.TrimSpace(req.NetworkTier), "PREMIUM"))

	logs := []string{fmt.Sprintf("[1/3] 准备在 %s 创建 GCP 实例 %s (%s)...", zone, name, machineType)}

	// Ensure subnet dual-stack if IPv6 is requested
	if req.AssignIPv6 {
		if subnetName == "" {
			subnetName = networkName
		}
		if err := g.ensureGCPSubnetExternalIPv6(networkName, region, subnetName, req.ConfirmIrreversible, &logs); err != nil {
			return nil, logs, err
		}
	}

	nic := map[string]interface{}{
		"network": fmt.Sprintf("projects/%s/global/networks/%s", g.Account.ProjectID, networkName),
	}
	if subnetName != "" {
		nic["subnetwork"] = fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", g.Account.ProjectID, region, subnetName)
	}
	if req.AssignPublicIPv4 {
		nic["accessConfigs"] = []map[string]interface{}{
			{
				"name":        "External NAT",
				"type":        "ONE_TO_ONE_NAT",
				"networkTier": tier,
			},
		}
	}
	if req.AssignIPv6 {
		nic["stackType"] = "IPV4_IPV6"
		nic["ipv6AccessConfigs"] = []map[string]interface{}{
			{
				"name":        "External IPv6",
				"type":        "DIRECT_IPV6",
				"networkTier": tier,
			},
		}
	}

	payload := map[string]interface{}{
		"name":        name,
		"machineType": fmt.Sprintf("zones/%s/machineTypes/%s", zone, machineType),
		"disks": []map[string]interface{}{
			{
				"boot":       true,
				"autoDelete": true,
				"initializeParams": map[string]interface{}{
					"sourceImage": sourceImage,
					"diskType":    fmt.Sprintf("zones/%s/diskTypes/%s", zone, diskType),
					"diskSizeGb":  fmt.Sprintf("%d", diskSize),
				},
			},
		},
		"networkInterfaces": []map[string]interface{}{nic},
	}

	if req.Spot {
		payload["scheduling"] = map[string]interface{}{
			"provisioningModel":         "SPOT",
			"instanceTerminationAction": "STOP",
		}
	}
	if len(req.Tags) > 0 {
		cleanTags := make([]string, 0, len(req.Tags))
		for _, t := range req.Tags {
			if st := strings.TrimSpace(t); st != "" {
				cleanTags = append(cleanTags, st)
			}
		}
		if len(cleanTags) > 0 {
			payload["tags"] = map[string]interface{}{"items": cleanTags}
		}
	}

	metaItems := make([]map[string]string, 0, 2)
	if sshKey := strings.TrimSpace(req.SSHPublicKey); sshKey != "" {
		sshUser := valueOrDefault(strings.TrimSpace(req.SSHUser), "root")
		formattedKey := sshKey
		if !strings.Contains(sshKey, ":") {
			formattedKey = fmt.Sprintf("%s:%s", sshUser, sshKey)
		}
		metaItems = append(metaItems, map[string]string{"key": "ssh-keys", "value": formattedKey})
	}
	if script := strings.TrimSpace(req.StartupScript); script != "" {
		// Accept raw script text; base64 not needed for GCP startup-script metadata
		if strings.HasPrefix(script, "base64:") {
			if dec, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(script, "base64:")); err == nil {
				script = string(dec)
			}
		}
		metaItems = append(metaItems, map[string]string{"key": "startup-script", "value": script})
	}
	if len(metaItems) > 0 {
		payload["metadata"] = map[string]interface{}{"items": metaItems}
	}

	logs = append(logs, "[2/3] 正在提交 Compute Engine 创建实例请求...")
	endpoint := fmt.Sprintf("%s/projects/%s/zones/%s/instances", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID), url.PathEscape(zone))
	if err := g.requestOperation("POST", endpoint, payload); err != nil {
		return nil, logs, fmt.Errorf("创建 GCP 实例失败: %w", err)
	}

	vmID := zone + "|" + name
	vm, err := g.GetVM(vmID)
	if err != nil {
		logs = append(logs, fmt.Sprintf("[3/3] 实例 %s 创建操作已完成（读取详情失败: %v）", name, err))
		return map[string]interface{}{"id": vmID, "name": name, "zone": zone}, logs, nil
	}
	logs = append(logs, fmt.Sprintf("[3/3] 实例 %s 创建成功！", name))
	return vm, logs, nil
}

func (g *GCPService) ensureGCPSubnetExternalIPv6(networkName, region, subnetName string, confirmIrreversible bool, logs *[]string) error {
	var subnet map[string]interface{}
	subURL := fmt.Sprintf("%s/projects/%s/regions/%s/subnetworks/%s", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID), url.PathEscape(region), url.PathEscape(subnetName))
	if err := g.requestJSON("GET", subURL, nil, &subnet); err != nil {
		return fmt.Errorf("读取子网 %s 失败: %w", subnetName, err)
	}
	if strings.EqualFold(stringValue(subnet["stackType"]), "IPV4_IPV6") && strings.EqualFold(stringValue(subnet["ipv6AccessType"]), "EXTERNAL") {
		return nil
	}
	if strings.EqualFold(stringValue(subnet["stackType"]), "IPV4_IPV6") && strings.EqualFold(stringValue(subnet["ipv6AccessType"]), "INTERNAL") {
		return fmt.Errorf("子网 %s 已是 INTERNAL IPv6，无法直接改为 EXTERNAL IPv6", subnetName)
	}

	var netObj map[string]interface{}
	netURL := fmt.Sprintf("%s/projects/%s/global/networks/%s", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID), url.PathEscape(networkName))
	if err := g.requestJSON("GET", netURL, nil, &netObj); err == nil && netObj != nil {
		if isAuto, _ := netObj["autoCreateSubnetworks"].(bool); isAuto {
			if !confirmIrreversible {
				return fmt.Errorf("所选 VPC (%s) 为默认 Auto Mode，创建带 IPv6 的实例需先将其不可逆转换为 Custom Mode 并开启子网双栈；请勾选确认框后再试", networkName)
			}
			if err := g.requestOperation("POST", netURL+"/switchToCustomMode", nil); err != nil {
				return fmt.Errorf("转换 VPC (%s) 为 Custom Mode 失败: %w", networkName, err)
			}
			*logs = append(*logs, fmt.Sprintf("已将 VPC %s 转换为 Custom Mode", networkName))
			_ = g.requestJSON("GET", subURL, nil, &subnet)
		}
	}

	if err := g.requestOperation("PATCH", subURL, map[string]interface{}{
		"fingerprint":    stringValue(subnet["fingerprint"]),
		"stackType":      "IPV4_IPV6",
		"ipv6AccessType": "EXTERNAL",
	}); err != nil {
		return fmt.Errorf("为子网 %s 开启外部 IPv6 失败: %w", subnetName, err)
	}
	*logs = append(*logs, fmt.Sprintf("已为子网 %s 开启外部 IPv6 双栈", subnetName))
	return nil
}

func (g *GCPService) ScanResources() (*GCPResourceScanResult, error) {
	items := make([]GCPResourceItem, 0)
	var summary GCPResourceSummary

	// 1. Disks
	var diskResp struct {
		Items map[string]struct {
			Disks []map[string]interface{} `json:"disks"`
		} `json:"items"`
	}
	if err := g.requestJSON("GET", fmt.Sprintf("%s/projects/%s/aggregated/disks", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID)), nil, &diskResp); err != nil {
		return nil, fmt.Errorf("查询 GCP 磁盘列表失败: %w", err)
	}
	for _, scoped := range diskResp.Items {
		for _, d := range scoped.Disks {
			name := stringValue(d["name"])
			zone := resourceTail(stringValue(d["zone"]))
			users := toStringSlice(d["users"])
			orphan := len(users) == 0
			sizeGB := 0
			if rawSize := stringValue(d["sizeGb"]); rawSize != "" {
				_, _ = fmt.Sscanf(rawSize, "%d", &sizeGB)
			}
			attached := ""
			if len(users) > 0 {
				names := make([]string, 0, len(users))
				for _, u := range users {
					names = append(names, resourceTail(u))
				}
				attached = strings.Join(names, ", ")
			}
			note := "挂载使用中"
			if orphan {
				note = "未挂载孤立磁盘（仍按容量持续计费，可安全清理）"
				summary.OrphanDiskCount++
				summary.BillableOrphans++
			}
			summary.DiskCount++
			items = append(items, GCPResourceItem{
				ID:          "disk|" + zone + "|" + name,
				Name:        name,
				Category:    "disk",
				Scope:       zone,
				ScopeType:   "zone",
				SizeGB:      sizeGB,
				Status:      stringValue(d["status"]),
				AttachedTo:  attached,
				Orphan:      orphan,
				BillingNote: note,
			})
		}
	}

	// 2. Snapshots
	var snapResp struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := g.requestJSON("GET", fmt.Sprintf("%s/projects/%s/global/snapshots", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID)), nil, &snapResp); err == nil {
		for _, s := range snapResp.Items {
			name := stringValue(s["name"])
			sizeGB := 0
			if rawSize := stringValue(s["diskSizeGb"]); rawSize != "" {
				_, _ = fmt.Sscanf(rawSize, "%d", &sizeGB)
			}
			summary.SnapshotCount++
			summary.BillableOrphans++
			items = append(items, GCPResourceItem{
				ID:          "snapshot|global|" + name,
				Name:        name,
				Category:    "snapshot",
				Scope:       "global",
				ScopeType:   "global",
				SizeGB:      sizeGB,
				Status:      stringValue(s["status"]),
				Orphan:      true,
				BillingNote: "全局快照（按存储容量持续计费，确认无需恢复后可清理）",
			})
		}
	}

	// 3. Regional Static Addresses
	var addrResp struct {
		Items map[string]struct {
			Addresses []map[string]interface{} `json:"addresses"`
		} `json:"items"`
	}
	if err := g.requestJSON("GET", fmt.Sprintf("%s/projects/%s/aggregated/addresses", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID)), nil, &addrResp); err == nil {
		for _, scoped := range addrResp.Items {
			for _, a := range scoped.Addresses {
				name := stringValue(a["name"])
				region := resourceTail(stringValue(a["region"]))
				status := strings.ToUpper(stringValue(a["status"]))
				users := toStringSlice(a["users"])
				orphan := status == "RESERVED" && len(users) == 0
				attached := ""
				if len(users) > 0 {
					attached = resourceTail(users[0])
				}
				note := "静态保留 IP（已绑定使用中）"
				if orphan {
					note = "未绑定闲置静态 IP（按小时计费，强烈建议清理或绑定）"
					summary.UnusedStaticIPs++
					summary.BillableOrphans++
				}
				summary.AddressCount++
				items = append(items, GCPResourceItem{
					ID:          "address|" + region + "|" + name,
					Name:        name,
					Category:    "address",
					Scope:       region,
					ScopeType:   "region",
					IPAddress:   stringValue(a["address"]),
					Status:      status,
					AttachedTo:  attached,
					Orphan:      orphan,
					BillingNote: note,
				})
			}
		}
	}

	summary.TotalCount = len(items)
	return &GCPResourceScanResult{
		ProjectID: g.Account.ProjectID,
		Items:     items,
		Summary:   summary,
	}, nil
}

func (g *GCPService) CleanupResources(ids []string) ([]string, []string) {
	deleted := make([]string, 0)
	errs := make([]string, 0)
	for _, id := range ids {
		parts := strings.SplitN(id, "|", 3)
		if len(parts) != 3 {
			errs = append(errs, fmt.Sprintf("无效的资源标识: %s", id))
			continue
		}
		cat, scope, name := parts[0], parts[1], parts[2]
		var endpoint string
		switch cat {
		case "disk":
			endpoint = fmt.Sprintf("%s/projects/%s/zones/%s/disks/%s", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID), url.PathEscape(scope), url.PathEscape(name))
		case "snapshot":
			endpoint = fmt.Sprintf("%s/projects/%s/global/snapshots/%s", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID), url.PathEscape(name))
		case "address":
			endpoint = fmt.Sprintf("%s/projects/%s/regions/%s/addresses/%s", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID), url.PathEscape(scope), url.PathEscape(name))
		default:
			errs = append(errs, fmt.Sprintf("不支持清理的资源类型: %s", cat))
			continue
		}
		if err := g.requestOperation("DELETE", endpoint, nil); err != nil {
			errs = append(errs, fmt.Sprintf("%s (%s): %v", name, cat, err))
			continue
		}
		deleted = append(deleted, id)
	}
	return deleted, errs
}
