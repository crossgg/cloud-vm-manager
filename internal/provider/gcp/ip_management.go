package gcp

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/provider"
)

func gcpIPCapabilities() provider.IPCapabilities {
	return provider.IPCapabilities{
		CanAddSecondaryPrivateIPv4: false,
		CanAttachReservedIPv4:      true,
		CanCreateReservedIPv4:      true,
		CanDetachPublicIPv4:        true,
		CanAttachIPv6:              true,
		CanChangeIPv6:              true,
		CanDeleteIPv6:              true,
		CanEnableSubnetIPv6:        true,
		IPv6PrefixConfigurable:     false,
		HasIdlePublicIPFeeWarning:  true,
		Notes: []string{
			"GCP 单张网卡仅支持 1 个 1:1 NAT 外部 IPv4（不支持同网卡新增辅助私网 IPv4）。",
			"GCP 外部 IPv6 按网卡双栈直接分配 /96 前缀；若使用默认 Auto Mode VPC，开启外部 IPv6 需先将 VPC 不可逆转换为 Custom Mode。",
			"开启 IPv6 后，别忘了在「防火墙」中检查是否存在针对 ::/0 的放行规则（GCP 中 0.0.0.0/0 仅作用于 IPv4）。",
		},
	}
}

func (g *GCPService) GetIPManagement(id string) (*provider.IPManagementInfo, error) {
	zone, name, err := parseGCPID(id)
	if err != nil {
		return nil, err
	}
	region := regionFromZone(zone)

	instance, err := g.rawInstance(zone, name)
	if err != nil {
		return nil, err
	}

	addrByIP, unusedReserved := g.listRegionalAddresses(region)
	firewalls, _ := g.ListFirewalls()

	rawNICs, _ := instance["networkInterfaces"].([]interface{})
	vnics := make([]provider.VNICIPInfo, 0, len(rawNICs))

	for idx, raw := range rawNICs {
		nic, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		nicName := valueOrDefault(stringValue(nic["name"]), fmt.Sprintf("nic%d", idx))
		netURL := stringValue(nic["network"])
		subURL := stringValue(nic["subnetwork"])
		privIP := stringValue(nic["networkIP"])

		var subnetName, ipv4CIDR string
		var subnetIPv6CIDRs []string
		var setup *provider.SubnetIPv6Status

		if subURL != "" {
			var subnet map[string]interface{}
			if err := g.requestJSON("GET", g.computeResourceURL(subURL), nil, &subnet); err == nil && subnet != nil {
				subnetName = stringValue(subnet["name"])
				ipv4CIDR = stringValue(subnet["ipCidrRange"])
				if extPrefix := stringValue(subnet["externalIpv6Prefix"]); extPrefix != "" {
					subnetIPv6CIDRs = append(subnetIPv6CIDRs, extPrefix)
				} else if range6 := stringValue(subnet["ipv6CidrRange"]); range6 != "" {
					subnetIPv6CIDRs = append(subnetIPv6CIDRs, range6)
				}
				setup = g.inspectGCPSubnetIPv6Status(netURL, subnet, firewalls)
			}
		}
		if subnetIPv6CIDRs == nil {
			subnetIPv6CIDRs = []string{}
		}

		var pubInfo *provider.PublicIPInfo
		if accessConfigs, ok := nic["accessConfigs"].([]interface{}); ok && len(accessConfigs) > 0 {
			if ac, ok := accessConfigs[0].(map[string]interface{}); ok {
				natIP := stringValue(ac["natIP"])
				acName := valueOrDefault(stringValue(ac["name"]), "External NAT")
				if natIP != "" {
					lifetime := "EPHEMERAL"
					displayName := acName
					pubID := nicName + "|" + acName
					idleWarn := false
					if addrRes, ok := addrByIP[natIP]; ok {
						lifetime = "RESERVED"
						displayName = valueOrDefault(stringValue(addrRes["name"]), acName)
						pubID = stringValue(addrRes["name"])
						idleWarn = true
					}
					pubInfo = &provider.PublicIPInfo{
						ID:             pubID,
						IPAddress:      natIP,
						Lifetime:       lifetime,
						DisplayName:    displayName,
						SKU:            stringValue(ac["networkTier"]),
						IdleFeeWarning: idleWarn,
					}
				}
			}
		}

		privIPs := []provider.PrivateIPInfo{
			{
				ID:        nicName,
				IPAddress: privIP,
				IsPrimary: idx == 0,
				PublicIP:  pubInfo,
			},
		}

		ipv6s := make([]provider.IPv6Info, 0)
		if v6Configs, ok := nic["ipv6AccessConfigs"].([]interface{}); ok {
			for cfgIdx, rawV6 := range v6Configs {
				ac6, ok := rawV6.(map[string]interface{})
				if !ok {
					continue
				}
				extIPv6 := strings.TrimSpace(stringValue(ac6["externalIpv6"]))
				if extIPv6 == "" {
					continue
				}
				prefixLen := 96
				if p, ok := ac6["externalIpv6PrefixLength"].(float64); ok && p > 0 {
					prefixLen = int(p)
				}
				ac6Name := valueOrDefault(stringValue(ac6["name"]), "External IPv6")
				ipv6s = append(ipv6s, provider.IPv6Info{
					ID:               fmt.Sprintf("%s|ipv6|%d", nicName, cfgIdx),
					IPAddress:        extIPv6,
					CIDRPrefixLength: prefixLen,
					SubnetID:         subnetName,
					VNICID:           nicName,
					CanChange:        true,
					DisplayName:      ac6Name,
					Lifetime:         "EPHEMERAL",
					SKU:              stringValue(ac6["networkTier"]),
				})
			}
		}

		vnics = append(vnics, provider.VNICIPInfo{
			ID:             nicName,
			DisplayName:    nicName,
			IsPrimary:      idx == 0,
			LifecycleState: "ATTACHED",
			SubnetID:       subURL,
			SubnetName:     subnetName,
			VCNID:          netURL,
			IPv4CIDR:       ipv4CIDR,
			IPv6CIDRBlocks: subnetIPv6CIDRs,
			IPv6Setup:      setup,
			PrivateIPs:     privIPs,
			IPv6s:          ipv6s,
		})
	}

	return &provider.IPManagementInfo{
		Provider:     "gcp",
		InstanceID:   id,
		InstanceName: valueOrDefault(stringValue(instance["name"]), name),
		Capabilities: gcpIPCapabilities(),
		VNICs:        vnics,
		ReservedIPs:  unusedReserved,
	}, nil
}

func (g *GCPService) ChangePrivateIPPublicIP(id, nicID string) (*provider.ChangeIPResult, error) {
	zone, name, err := parseGCPID(id)
	if err != nil {
		return nil, err
	}
	instance, err := g.rawInstance(zone, name)
	if err != nil {
		return nil, err
	}

	nic, nicName, err := findGCPNIC(instance, nicID)
	if err != nil {
		return nil, err
	}
	accessConfigName, oldIP := firstAccessConfigOnNIC(nic)
	if accessConfigName == "" {
		accessConfigName = "External NAT"
	}

	region := regionFromZone(zone)
	if oldIP != "" {
		addrByIP, _ := g.listRegionalAddresses(region)
		if addrRes, ok := addrByIP[oldIP]; ok {
			return nil, fmt.Errorf("当前绑定的是静态保留 IP (%s / %s)，请先解绑保留 IP 后再更换临时 IP", oldIP, stringValue(addrRes["name"]))
		}
	}

	oldAddressName := ""
	logs := []string{}
	if oldIP != "" {
		oldAddressName = fmt.Sprintf("old-ip-%s-%d", strings.ToLower(name), time.Now().Unix())
		logs = append(logs, fmt.Sprintf("[1/4] 正在暂存旧临时 IP 为静态地址以强制更换新 IP: %s", oldIP))
		if err := g.reserveStaticAddress(region, oldAddressName, oldIP); err != nil {
			return nil, fmt.Errorf("暂存旧 IP 失败: %w", err)
		}

		logs = append(logs, "[2/4] 正在从网卡解绑旧外部 IPv4...")
		if err := g.accessConfigAction(zone, name, "deleteAccessConfig", url.Values{
			"networkInterface": {nicName},
			"accessConfig":     {accessConfigName},
		}, nil); err != nil {
			_ = g.deleteStaticAddress(region, oldAddressName)
			return nil, fmt.Errorf("解绑旧外部 IPv4 失败: %w", err)
		}
	} else {
		logs = append(logs, "[1/4] 当前无外部 IPv4")
		logs = append(logs, "[2/4] 跳过解绑")
	}

	logs = append(logs, "[3/4] 正在分配新的临时外部 IPv4...")
	if err := g.accessConfigAction(zone, name, "addAccessConfig", url.Values{
		"networkInterface": {nicName},
	}, map[string]interface{}{
		"name":        accessConfigName,
		"type":        "ONE_TO_ONE_NAT",
		"networkTier": "PREMIUM",
	}); err != nil {
		return nil, fmt.Errorf("分配新外部 IPv4 失败: %w", err)
	}

	newIP := ""
	if updated, err := g.rawInstance(zone, name); err == nil {
		if updatedNIC, _, findErr := findGCPNIC(updated, nicName); findErr == nil {
			_, newIP = firstAccessConfigOnNIC(updatedNIC)
		}
	}
	logs = append(logs, fmt.Sprintf("[3/4] 新外部 IPv4: %s", valueOrDefault(newIP, "pending")))

	if oldAddressName != "" {
		logs = append(logs, fmt.Sprintf("[4/4] 正在清理临时暂存的旧地址 (%s)...", oldAddressName))
		if err := g.deleteStaticAddress(region, oldAddressName); err != nil {
			return nil, fmt.Errorf("清理暂存旧地址失败: %w", err)
		}
	} else {
		logs = append(logs, "[4/4] 无需清理旧地址")
	}

	return &provider.ChangeIPResult{
		Success:      true,
		Message:      "IP更换成功",
		NewIPAddress: newIP,
		Logs:         logs,
	}, nil
}

func (g *GCPService) AttachIPv4(id string, req provider.AttachIPv4Request) ([]string, string, error) {
	if strings.EqualFold(req.Mode, "secondary_private") {
		return nil, "", fmt.Errorf("GCP 单张网卡不支持新增辅助私网 IPv4 / 多外部 IPv4")
	}
	zone, name, err := parseGCPID(id)
	if err != nil {
		return nil, "", err
	}
	instance, err := g.rawInstance(zone, name)
	if err != nil {
		return nil, "", err
	}
	nicID := valueOrDefault(req.VNICID, req.PrivateIPID)
	nic, nicName, err := findGCPNIC(instance, nicID)
	if err != nil {
		return nil, "", err
	}
	if _, existingIP := firstAccessConfigOnNIC(nic); existingIP != "" {
		return nil, "", fmt.Errorf("网卡 %s 已绑定外部 IPv4 (%s)", nicName, existingIP)
	}

	if err := g.accessConfigAction(zone, name, "addAccessConfig", url.Values{
		"networkInterface": {nicName},
	}, map[string]interface{}{
		"name":        "External NAT",
		"type":        "ONE_TO_ONE_NAT",
		"networkTier": "PREMIUM",
	}); err != nil {
		return nil, "", fmt.Errorf("分配外部 IPv4 失败: %w", err)
	}

	var newIP string
	if updated, err := g.rawInstance(zone, name); err == nil {
		if updatedNIC, _, findErr := findGCPNIC(updated, nicName); findErr == nil {
			_, newIP = firstAccessConfigOnNIC(updatedNIC)
		}
	}
	return []string{fmt.Sprintf("已为网卡 %s 分配临时外部 IPv4: %s", nicName, valueOrDefault(newIP, "pending"))}, newIP, nil
}

func (g *GCPService) DeleteIPv4Managed(id string, req provider.DeleteIPv4Request) ([]string, error) {
	if req.DeletePrivateIP {
		return nil, fmt.Errorf("GCP 不支持删除网卡的主内网 IPv4")
	}
	zone, name, err := parseGCPID(id)
	if err != nil {
		return nil, err
	}
	instance, err := g.rawInstance(zone, name)
	if err != nil {
		return nil, err
	}
	nicID := valueOrDefault(req.VNICID, req.PrivateIPID)
	nic, nicName, err := findGCPNIC(instance, nicID)
	if err != nil {
		return nil, err
	}
	acName, oldIP := firstAccessConfigOnNIC(nic)
	if oldIP == "" {
		return nil, fmt.Errorf("网卡 %s 当前未绑定外部 IPv4", nicName)
	}
	if acName == "" {
		acName = "External NAT"
	}

	region := regionFromZone(zone)
	addrByIP, _ := g.listRegionalAddresses(region)
	addrRes, isReserved := addrByIP[oldIP]

	if err := g.accessConfigAction(zone, name, "deleteAccessConfig", url.Values{
		"networkInterface": {nicName},
		"accessConfig":     {acName},
	}, nil); err != nil {
		return nil, fmt.Errorf("解绑外部 IPv4 失败: %w", err)
	}

	logs := []string{fmt.Sprintf("已从网卡 %s 解绑外部 IPv4: %s", nicName, oldIP)}
	if isReserved {
		addrName := stringValue(addrRes["name"])
		if req.DeletePublicIP != nil && *req.DeletePublicIP && addrName != "" {
			if err := g.deleteStaticAddress(region, addrName); err != nil {
				return logs, fmt.Errorf("已解绑，但删除静态 IP (%s) 失败: %w", addrName, err)
			}
			logs = append(logs, fmt.Sprintf("已同时删除静态外部 IP 资源: %s", addrName))
		} else {
			logs = append(logs, fmt.Sprintf("静态外部 IP 资源 %s 已保留（注意：未使用的静态 IP 仍按小时计费）", addrName))
		}
	}
	return logs, nil
}

func (g *GCPService) AttachReservedIPv4Managed(id string, req provider.AttachReservedIPv4Request) (string, []string, error) {
	zone, name, err := parseGCPID(id)
	if err != nil {
		return "", nil, err
	}
	region := regionFromZone(zone)
	instance, err := g.rawInstance(zone, name)
	if err != nil {
		return "", nil, err
	}
	nicID := valueOrDefault(req.VNICID, req.PrivateIPID)
	nic, nicName, err := findGCPNIC(instance, nicID)
	if err != nil {
		return "", nil, err
	}
	_, existingIP := firstAccessConfigOnNIC(nic)

	if req.CreateNew {
		addrName := strings.TrimSpace(req.DisplayName)
		if addrName == "" {
			addrName = fmt.Sprintf("static-ip-%s-%d", strings.ToLower(name), time.Now().Unix())
		}
		if existingIP != "" {
			addrByIP, _ := g.listRegionalAddresses(region)
			if _, already := addrByIP[existingIP]; already {
				return "", nil, fmt.Errorf("当前 IP (%s) 已经是静态保留 IP", existingIP)
			}
			if err := g.reserveStaticAddress(region, addrName, existingIP); err != nil {
				return "", nil, fmt.Errorf("将当前临时 IP 升级为静态保留 IP 失败: %w", err)
			}
			return existingIP, []string{fmt.Sprintf("已将当前公网 IP %s 固定为静态保留 IP (%s)", existingIP, addrName)}, nil
		}

		// No external IP yet: create a new static external address and attach it
		if err := g.requestOperation("POST", fmt.Sprintf(
			"%s/projects/%s/regions/%s/addresses",
			gcpComputeBaseURL,
			url.PathEscape(g.Account.ProjectID),
			url.PathEscape(region),
		), map[string]interface{}{
			"name":        addrName,
			"addressType": "EXTERNAL",
			"networkTier": "PREMIUM",
		}); err != nil {
			return "", nil, fmt.Errorf("创建静态外部 IP 失败: %w", err)
		}

		var created map[string]interface{}
		if err := g.requestJSON("GET", fmt.Sprintf(
			"%s/projects/%s/regions/%s/addresses/%s",
			gcpComputeBaseURL,
			url.PathEscape(g.Account.ProjectID),
			url.PathEscape(region),
			url.PathEscape(addrName),
		), nil, &created); err != nil {
			return "", nil, fmt.Errorf("读取新建静态 IP 地址失败: %w", err)
		}
		ipAddr := stringValue(created["address"])
		if err := g.accessConfigAction(zone, name, "addAccessConfig", url.Values{
			"networkInterface": {nicName},
		}, map[string]interface{}{
			"name":        "External NAT",
			"type":        "ONE_TO_ONE_NAT",
			"natIP":       ipAddr,
			"networkTier": "PREMIUM",
		}); err != nil {
			return "", nil, fmt.Errorf("绑定新建静态 IP (%s) 失败: %w", ipAddr, err)
		}
		return ipAddr, []string{fmt.Sprintf("已创建并绑定静态保留 IP %s (%s) 到网卡 %s", ipAddr, addrName, nicName)}, nil
	}

	if existingIP != "" {
		return "", nil, fmt.Errorf("网卡 %s 当前已绑定外部 IP (%s)，请先解绑后再附加保留 IP", nicName, existingIP)
	}
	addrName := strings.TrimSpace(req.ReservedPublicIPID)
	if addrName == "" {
		return "", nil, fmt.Errorf("请选择要绑定的静态保留 IP")
	}
	var addrObj map[string]interface{}
	if err := g.requestJSON("GET", fmt.Sprintf(
		"%s/projects/%s/regions/%s/addresses/%s",
		gcpComputeBaseURL,
		url.PathEscape(g.Account.ProjectID),
		url.PathEscape(region),
		url.PathEscape(addrName),
	), nil, &addrObj); err != nil {
		return "", nil, fmt.Errorf("读取静态保留 IP (%s) 失败: %w", addrName, err)
	}
	ipAddr := stringValue(addrObj["address"])
	if ipAddr == "" {
		return "", nil, fmt.Errorf("静态保留 IP (%s) 暂无可用地址", addrName)
	}
	if err := g.accessConfigAction(zone, name, "addAccessConfig", url.Values{
		"networkInterface": {nicName},
	}, map[string]interface{}{
		"name":        "External NAT",
		"type":        "ONE_TO_ONE_NAT",
		"natIP":       ipAddr,
		"networkTier": "PREMIUM",
	}); err != nil {
		return "", nil, fmt.Errorf("绑定静态保留 IP (%s) 失败: %w", ipAddr, err)
	}
	return ipAddr, []string{fmt.Sprintf("已将静态保留 IP %s (%s) 绑定到网卡 %s", ipAddr, addrName, nicName)}, nil
}

func (g *GCPService) EnableSubnetIPv6(id string, req provider.EnableIPv6Request) (string, []string, error) {
	zone, name, err := parseGCPID(id)
	if err != nil {
		return "", nil, err
	}
	region := regionFromZone(zone)
	instance, err := g.rawInstance(zone, name)
	if err != nil {
		return "", nil, err
	}
	nic, nicName, err := findGCPNIC(instance, req.VNICID)
	if err != nil {
		return "", nil, err
	}
	netURL := stringValue(nic["network"])
	subURL := stringValue(nic["subnetwork"])
	if subURL == "" {
		return "", nil, fmt.Errorf("网卡 %s 缺少子网信息", nicName)
	}

	var network map[string]interface{}
	if netURL != "" {
		if err := g.requestJSON("GET", g.computeResourceURL(netURL), nil, &network); err != nil {
			return "", nil, fmt.Errorf("读取 VPC 网络失败: %w", err)
		}
	}
	var subnet map[string]interface{}
	if err := g.requestJSON("GET", g.computeResourceURL(subURL), nil, &subnet); err != nil {
		return "", nil, fmt.Errorf("读取子网失败: %w", err)
	}

	logs := make([]string, 0, 3)
	isAutoMode, _ := network["autoCreateSubnetworks"].(bool)
	if isAutoMode {
		if !req.ConfirmIrreversible {
			return "", nil, fmt.Errorf("当前 VPC (%s) 为 Auto Mode 默认网络，开启外部 IPv6 前必须将其不可逆转换为 Custom Mode；请确认后重试", resourceTail(netURL))
		}
		netName := valueOrDefault(stringValue(network["name"]), resourceTail(netURL))
		switchURL := fmt.Sprintf("%s/projects/%s/global/networks/%s/switchToCustomMode", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID), url.PathEscape(netName))
		if err := g.requestOperation("POST", switchURL, nil); err != nil {
			return "", nil, fmt.Errorf("将 VPC (%s) 转换为 Custom Mode 失败: %w", netName, err)
		}
		logs = append(logs, fmt.Sprintf("已将 VPC %s 从 Auto Mode 转换为 Custom Mode", netName))
		// Re-read subnet in case fingerprint changed
		if err := g.requestJSON("GET", g.computeResourceURL(subURL), nil, &subnet); err != nil {
			return "", nil, fmt.Errorf("转换后重新读取子网失败: %w", err)
		}
	}

	curStack := strings.ToUpper(stringValue(subnet["stackType"]))
	curAccess := strings.ToUpper(stringValue(subnet["ipv6AccessType"]))
	if curStack == "IPV4_IPV6" && curAccess == "EXTERNAL" {
		prefix := valueOrDefault(stringValue(subnet["externalIpv6Prefix"]), stringValue(subnet["ipv6CidrRange"]))
		logs = append(logs, "子网已开启外部 IPv6 双栈"+prefixParen(prefix))
		return prefix, logs, nil
	}
	if curStack == "IPV4_IPV6" && curAccess == "INTERNAL" {
		return "", nil, fmt.Errorf("当前子网已设置为 INTERNAL IPv6，GCP 不支持将既有 INTERNAL 子网直接改为 EXTERNAL 公网 IPv6")
	}

	subName := valueOrDefault(stringValue(subnet["name"]), resourceTail(subURL))
	patchURL := fmt.Sprintf("%s/projects/%s/regions/%s/subnetworks/%s", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID), url.PathEscape(region), url.PathEscape(subName))
	if err := g.requestOperation("PATCH", patchURL, map[string]interface{}{
		"fingerprint":    stringValue(subnet["fingerprint"]),
		"stackType":      "IPV4_IPV6",
		"ipv6AccessType": "EXTERNAL",
	}); err != nil {
		return "", nil, fmt.Errorf("为子网 %s 开启外部 IPv6 失败: %w", subName, err)
	}

	var updatedSubnet map[string]interface{}
	var prefix string
	if err := g.requestJSON("GET", g.computeResourceURL(subURL), nil, &updatedSubnet); err == nil && updatedSubnet != nil {
		prefix = valueOrDefault(stringValue(updatedSubnet["externalIpv6Prefix"]), stringValue(updatedSubnet["ipv6CidrRange"]))
	}
	logs = append(logs, fmt.Sprintf("已为子网 %s 开启外部 IPv6 双栈%s", subName, prefixParen(prefix)))
	logs = append(logs, "提示：请前往「防火墙管理」确认存在针对 ::/0 的放行规则（GCP 的 0.0.0.0/0 仅对 IPv4 生效）")
	return prefix, logs, nil
}

func (g *GCPService) AttachIPv6(id string, req provider.AttachIPv6Request) (*provider.IPv6Info, error) {
	zone, name, err := parseGCPID(id)
	if err != nil {
		return nil, err
	}
	instance, err := g.rawInstance(zone, name)
	if err != nil {
		return nil, err
	}
	nic, nicName, err := findGCPNIC(instance, req.VNICID)
	if err != nil {
		return nil, err
	}
	subURL := stringValue(nic["subnetwork"])
	if subURL != "" {
		var subnet map[string]interface{}
		if err := g.requestJSON("GET", g.computeResourceURL(subURL), nil, &subnet); err == nil && subnet != nil {
			if !strings.EqualFold(stringValue(subnet["stackType"]), "IPV4_IPV6") || !strings.EqualFold(stringValue(subnet["ipv6AccessType"]), "EXTERNAL") {
				return nil, fmt.Errorf("当前子网尚未开启外部 IPv6 双栈，请先点击「开启子网 IPv6」")
			}
		}
	}

	if v6Configs, ok := nic["ipv6AccessConfigs"].([]interface{}); ok && len(v6Configs) > 0 {
		return nil, fmt.Errorf("网卡 %s 已分配外部 IPv6，如需重新分配请点击「换IP」", nicName)
	}

	endpoint := fmt.Sprintf(
		"%s/projects/%s/zones/%s/instances/%s/updateNetworkInterface?networkInterface=%s",
		gcpComputeBaseURL,
		url.PathEscape(g.Account.ProjectID),
		url.PathEscape(zone),
		url.PathEscape(name),
		url.QueryEscape(nicName),
	)
	if err := g.requestOperation("PATCH", endpoint, map[string]interface{}{
		"fingerprint": stringValue(nic["fingerprint"]),
		"stackType":   "IPV4_IPV6",
		"ipv6AccessConfigs": []map[string]interface{}{
			{
				"name":        "External IPv6",
				"type":        "DIRECT_IPV6",
				"networkTier": "PREMIUM",
			},
		},
	}); err != nil {
		return nil, fmt.Errorf("为网卡 %s 启用外部 IPv6 失败: %w", nicName, err)
	}

	updated, err := g.rawInstance(zone, name)
	if err != nil {
		return nil, err
	}
	updatedNIC, _, err := findGCPNIC(updated, nicName)
	if err != nil {
		return nil, err
	}
	extIPv6, prefixLen, acName := firstIPv6AccessConfigOnNIC(updatedNIC)
	if extIPv6 == "" {
		return nil, fmt.Errorf("已开启网卡双栈，但暂未读取到分配的外部 IPv6 地址，请刷新查看")
	}

	return &provider.IPv6Info{
		ID:               nicName + "|ipv6|0",
		IPAddress:        extIPv6,
		CIDRPrefixLength: prefixLen,
		SubnetID:         resourceTail(subURL),
		VNICID:           nicName,
		CanChange:        true,
		DisplayName:      valueOrDefault(acName, "External IPv6"),
		Lifetime:         "EPHEMERAL",
		SKU:              "PREMIUM",
	}, nil
}

func (g *GCPService) ChangeIPv6(id, vnicID, ipv6ID string) (*provider.ChangeIPv6Result, error) {
	zone, name, err := parseGCPID(id)
	if err != nil {
		return nil, err
	}
	nicName := strings.TrimSpace(vnicID)
	if nicName == "" && strings.Contains(ipv6ID, "|") {
		nicName = strings.SplitN(ipv6ID, "|", 2)[0]
	}

	instance, err := g.rawInstance(zone, name)
	if err != nil {
		return nil, err
	}
	nic, resolvedNIC, err := findGCPNIC(instance, nicName)
	if err != nil {
		return nil, err
	}
	oldIP, _, _ := firstIPv6AccessConfigOnNIC(nic)
	logs := []string{
		fmt.Sprintf("[1/2] 正在临时关闭网卡 %s 双栈以释放旧 IPv6 (%s)...", resolvedNIC, valueOrDefault(oldIP, "未知")),
	}

	if _, err := g.DeleteIPv6(id, resolvedNIC+"|ipv6|0"); err != nil {
		return nil, fmt.Errorf("释放旧 IPv6 失败: %w", err)
	}
	logs = append(logs, "[1/2] 已释放旧 IPv6 配置")

	logs = append(logs, "[2/2] 正在重新开启网卡双栈以分配外部 IPv6 /96...")
	created, err := g.AttachIPv6(id, provider.AttachIPv6Request{VNICID: resolvedNIC})
	if err != nil {
		return nil, fmt.Errorf("重新分配 IPv6 失败: %w", err)
	}
	logs = append(logs, fmt.Sprintf("[2/2] 已重新分配外部 IPv6: %s/%d", created.IPAddress, created.CIDRPrefixLength))
	if oldIP != "" && oldIP == created.IPAddress {
		logs = append(logs, "提示：GCP 池短暂回收后重新分配了相同 /96 前缀；你可以在系统内从该 /96 前缀中任选主机地址")
	}

	return &provider.ChangeIPv6Result{
		Success:      true,
		Message:      "IPv6 更换操作完成",
		OldIPAddress: oldIP,
		NewIPAddress: created.IPAddress,
		IPv6:         created,
		Logs:         logs,
	}, nil
}

func (g *GCPService) DeleteIPv6(id, ipv6ID string) (string, error) {
	zone, name, err := parseGCPID(id)
	if err != nil {
		return "", err
	}
	nicName := ""
	if strings.Contains(ipv6ID, "|") {
		nicName = strings.SplitN(ipv6ID, "|", 2)[0]
	} else {
		nicName = strings.TrimSpace(ipv6ID)
	}

	instance, err := g.rawInstance(zone, name)
	if err != nil {
		return "", err
	}
	nic, resolvedNIC, err := findGCPNIC(instance, nicName)
	if err != nil {
		return "", err
	}
	oldIP, _, _ := firstIPv6AccessConfigOnNIC(nic)

	endpoint := fmt.Sprintf(
		"%s/projects/%s/zones/%s/instances/%s/updateNetworkInterface?networkInterface=%s",
		gcpComputeBaseURL,
		url.PathEscape(g.Account.ProjectID),
		url.PathEscape(zone),
		url.PathEscape(name),
		url.QueryEscape(resolvedNIC),
	)
	if err := g.requestOperation("PATCH", endpoint, map[string]interface{}{
		"fingerprint":       stringValue(nic["fingerprint"]),
		"stackType":         "IPV4_ONLY",
		"ipv6AccessConfigs": []interface{}{},
	}); err != nil {
		return "", fmt.Errorf("移除网卡 %s 外部 IPv6 失败: %w", resolvedNIC, err)
	}
	return oldIP, nil
}

var _ provider.IPManager = (*GCPService)(nil)

// --- GCP IP Management Internal Helpers ---

func (g *GCPService) computeResourceURL(selfLinkOrPath string) string {
	trimmed := strings.TrimSpace(selfLinkOrPath)
	if idx := strings.Index(trimmed, "/projects/"); idx >= 0 {
		return gcpComputeBaseURL + trimmed[idx:]
	}
	return trimmed
}

func (g *GCPService) listRegionalAddresses(region string) (map[string]map[string]interface{}, []provider.ReservedPublicIP) {
	byIP := make(map[string]map[string]interface{})
	unused := make([]provider.ReservedPublicIP, 0)
	if region == "" {
		return byIP, unused
	}
	var result struct {
		Items []map[string]interface{} `json:"items"`
	}
	endpoint := fmt.Sprintf("%s/projects/%s/regions/%s/addresses", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID), url.PathEscape(region))
	if err := g.requestJSON("GET", endpoint, nil, &result); err != nil {
		return byIP, unused
	}
	for _, item := range result.Items {
		addrType := strings.ToUpper(stringValue(item["addressType"]))
		if addrType != "" && addrType != "EXTERNAL" {
			continue
		}
		ipVer := strings.ToUpper(stringValue(item["ipVersion"]))
		if ipVer != "" && ipVer != "IPV4" {
			continue
		}
		ip := strings.TrimSpace(stringValue(item["address"]))
		if ip == "" {
			continue
		}
		status := strings.ToUpper(stringValue(item["status"]))
		if status == "IN_USE" {
			byIP[ip] = item
		} else if status == "RESERVED" {
			unused = append(unused, provider.ReservedPublicIP{
				ID:          stringValue(item["name"]),
				IPAddress:   ip,
				DisplayName: stringValue(item["name"]),
				SKU:         stringValue(item["networkTier"]),
			})
		}
	}
	return byIP, unused
}

func (g *GCPService) inspectGCPSubnetIPv6Status(netURL string, subnet map[string]interface{}, firewalls []GCPFirewallRule) *provider.SubnetIPv6Status {
	curStack := strings.ToUpper(stringValue(subnet["stackType"]))
	curAccess := strings.ToUpper(stringValue(subnet["ipv6AccessType"]))
	subnetHasV6 := curStack == "IPV4_IPV6" && curAccess == "EXTERNAL"

	status := &provider.SubnetIPv6Status{
		SubnetHasIPv6:             subnetHasV6,
		VCNIPv6CIDRs:              []string{},
		UsedHexes:                 []string{},
		RouteHasIPv6Default:       true,
		HasInternetGateway:        true,
		SecurityListHasIPv6Egress: true,
		CanEnable:                 true,
	}

	if prefix := valueOrDefault(stringValue(subnet["externalIpv6Prefix"]), stringValue(subnet["ipv6CidrRange"])); prefix != "" {
		status.VCNIPv6CIDRs = []string{prefix}
	}

	if curStack == "IPV4_IPV6" && curAccess == "INTERNAL" {
		status.CanEnable = false
		status.BlockingReason = "当前子网已配置为 INTERNAL IPv6，GCP 不支持直接修改为 EXTERNAL 公网 IPv6"
	}

	if netURL != "" {
		var network map[string]interface{}
		if err := g.requestJSON("GET", g.computeResourceURL(netURL), nil, &network); err == nil && network != nil {
			isAuto, _ := network["autoCreateSubnetworks"].(bool)
			status.IsAutoModeVPC = isAuto
			status.VCNHasIPv6 = !isAuto || subnetHasV6
			if isAuto && !subnetHasV6 {
				status.RequiresConfirmation = true
				status.ConfirmationMessage = fmt.Sprintf(
					"当前 VPC (%s) 为默认 Auto Mode，开启外部 IPv6 需要先将此 VPC 不可逆转换为 Custom Mode，再为子网开启 IPV4_IPV6 (EXTERNAL) 双栈。已有实例与子网不受影响，但今后新建区域不会自动生成默认子网。",
					resourceTail(netURL),
				)
			}
		}
	}

	netName := resourceTail(netURL)
	hasV6Ingress := false
	for _, fw := range firewalls {
		if fw.Disabled {
			continue
		}
		if netName != "" && fw.Network != "" && !strings.EqualFold(fw.Network, netName) {
			continue
		}
		if strings.EqualFold(fw.Direction, "INGRESS") && strings.EqualFold(fw.Action, "ALLOW") {
			for _, sr := range fw.SourceRanges {
				if strings.Contains(sr, ":") {
					hasV6Ingress = true
					break
				}
			}
		}
		if hasV6Ingress {
			break
		}
	}
	status.NeedsFirewallIPv6Rule = !hasV6Ingress
	if !hasV6Ingress {
		status.FirewallHint = "未检测到包含 ::/0 的入站放行规则；GCP 中 0.0.0.0/0 仅放行 IPv4，如需 IPv6 入站访问请前往「防火墙管理」添加 ::/0 规则"
	}
	return status
}

func findGCPNIC(instance map[string]interface{}, nicID string) (map[string]interface{}, string, error) {
	rawNICs, _ := instance["networkInterfaces"].([]interface{})
	if len(rawNICs) == 0 {
		return nil, "", fmt.Errorf("GCP 实例没有任何网卡")
	}
	for idx, raw := range rawNICs {
		nic, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name := valueOrDefault(stringValue(nic["name"]), fmt.Sprintf("nic%d", idx))
		if nicID == "" || strings.EqualFold(name, nicID) {
			return nic, name, nil
		}
	}
	return nil, "", fmt.Errorf("未找到网卡 %s", nicID)
}

func firstAccessConfigOnNIC(nic map[string]interface{}) (string, string) {
	accessConfigs, ok := nic["accessConfigs"].([]interface{})
	if !ok || len(accessConfigs) == 0 {
		return "", ""
	}
	ac, ok := accessConfigs[0].(map[string]interface{})
	if !ok {
		return "", ""
	}
	return stringValue(ac["name"]), stringValue(ac["natIP"])
}

func firstIPv6AccessConfigOnNIC(nic map[string]interface{}) (string, int, string) {
	v6Configs, ok := nic["ipv6AccessConfigs"].([]interface{})
	if !ok || len(v6Configs) == 0 {
		return "", 0, ""
	}
	ac, ok := v6Configs[0].(map[string]interface{})
	if !ok {
		return "", 0, ""
	}
	prefixLen := 96
	if p, ok := ac["externalIpv6PrefixLength"].(float64); ok && p > 0 {
		prefixLen = int(p)
	}
	return strings.TrimSpace(stringValue(ac["externalIpv6"])), prefixLen, stringValue(ac["name"])
}

func prefixParen(prefix string) string {
	if strings.TrimSpace(prefix) == "" {
		return ""
	}
	return " (" + prefix + ")"
}
