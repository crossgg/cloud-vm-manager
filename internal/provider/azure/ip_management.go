package azure

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/provider"
)

func azureIPCapabilities() provider.IPCapabilities {
	return provider.IPCapabilities{
		CanAddSecondaryPrivateIPv4: true,
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
			"Azure Standard 公网 IPv4 为独立计费资源，仅解绑不删除仍会产生小时费用；公网 IPv6 本身免费。",
			"Azure 单张网卡通常仅支持 1 个 IPv6 配置；开启子网双栈会自动为 VNet/子网补充 ULA IPv6 地址段。",
		},
	}
}

func (az *AzureService) GetIPManagement(vmName string) (*provider.IPManagementInfo, error) {
	rawVM, err := az.getRawVM(vmName)
	if err != nil {
		return nil, err
	}

	vmLocation := stringValFromMap(rawVM, "location")
	if vmLocation == "" {
		vmLocation = az.Account.Location
	}

	vnics, err := az.instanceVNICIPInfos(rawVM)
	if err != nil {
		return nil, err
	}

	reservedIPs, err := az.listUnattachedPublicIPv4s(vmLocation)
	if err != nil {
		reservedIPs = []provider.ReservedPublicIP{}
	}

	return &provider.IPManagementInfo{
		Provider:     "azure",
		InstanceID:   vmName,
		InstanceName: valueOrDefault(stringValFromMap(rawVM, "name"), vmName),
		Capabilities: azureIPCapabilities(),
		VNICs:        vnics,
		ReservedIPs:  reservedIPs,
	}, nil
}

func (az *AzureService) ChangePrivateIPPublicIP(vmName, privateIPID string) (*provider.ChangeIPResult, error) {
	rawVM, err := az.getRawVM(vmName)
	if err != nil {
		return nil, err
	}

	nic, targetCfg, err := az.findTargetIPv4Config(rawVM, privateIPID)
	if err != nil {
		return nil, err
	}

	nicID := stringValFromMap(nic, "id")
	nicName := valueOrDefault(stringValFromMap(nic, "name"), lastPathSegment(nicID))
	cfgProps, _ := targetCfg["properties"].(map[string]interface{})
	privIP := stringValFromMap(cfgProps, "privateIPAddress")

	oldPubRef, _ := cfgProps["publicIPAddress"].(map[string]interface{})
	oldPubID := stringValFromMap(oldPubRef, "id")
	if oldPubID == "" {
		return nil, fmt.Errorf("当前内网 IP (%s) 未绑定公网 IPv4，请使用「附加 IPv4」", valueOrDefault(privIP, stringValFromMap(targetCfg, "name")))
	}

	var oldPub map[string]interface{}
	var oldIP, oldSKU string
	var oldZones []string
	if fetched, err := az.getPublicIP(oldPubID); err == nil && fetched != nil {
		oldPub = fetched
		oldPubProps, _ := fetched["properties"].(map[string]interface{})
		oldIP = stringValFromMap(oldPubProps, "ipAddress")
		if skuMap, ok := fetched["sku"].(map[string]interface{}); ok {
			oldSKU = stringValFromMap(skuMap, "name")
		}
		oldZones = toStringSlice(fetched["zones"])
	}

	rg := extractResourceGroupFromID(nicID)
	if rg == "" {
		rg = extractResourceGroupFromID(oldPubID)
	}
	if rg == "" {
		rg = az.Account.ResourceGroup
	}
	location := valueOrDefault(stringValFromMap(nic, "location"), valueOrDefault(stringValFromMap(rawVM, "location"), az.Account.Location))
	zones := resolveAzurePublicIPZones(oldZones, toStringSlice(rawVM["zones"]))
	skuName := "Standard"
	if strings.EqualFold(oldSKU, "Basic") {
		skuName = "Standard"
	}

	newPubName := fmt.Sprintf("ip-v4-%d", time.Now().UnixNano()/1e6)
	logs := []string{
		fmt.Sprintf("[1/3] 已定位网卡 %s / 内网 IP %s，正在创建新公网 IPv4 (%s)...", nicName, valueOrDefault(privIP, "primary"), newPubName),
	}

	newPubID, createdPub, err := az.createPublicIPResource(rg, newPubName, location, "IPv4", skuName, zones)
	if err != nil {
		return nil, fmt.Errorf("创建新公网 IPv4 失败: %w", err)
	}
	logs = append(logs, fmt.Sprintf("[1/3] 新公网 IPv4 资源已创建: %s", newPubName))

	logs = append(logs, "[2/3] 正在将新公网 IPv4 关联到网卡并等待同步完成...")
	cfgProps["publicIPAddress"] = map[string]interface{}{"id": newPubID}
	if err := az.putNIC(nicID, nic); err != nil {
		_ = az.armDelete(newPubID, "2023-04-01")
		return nil, fmt.Errorf("关联新公网 IPv4 到网卡失败: %w", err)
	}
	logs = append(logs, "[2/3] 网卡已成功关联新公网 IPv4")

	logs = append(logs, fmt.Sprintf("[3/3] 正在删除旧公网 IPv4 (%s) 以避免闲置计费...", valueOrDefault(oldIP, lastPathSegment(oldPubID))))
	if err := az.armDelete(oldPubID, "2023-04-01"); err != nil {
		logs = append(logs, fmt.Sprintf("[3/3] 警告：删除旧公网 IPv4 失败（请前往资源清理检查是否残留计费）: %v", err))
	} else {
		logs = append(logs, fmt.Sprintf("[3/3] 旧公网 IPv4 已删除: %s", valueOrDefault(oldIP, lastPathSegment(oldPubID))))
	}

	newIP := extractPublicIPAddress(createdPub)
	if newIP == "" {
		if refreshed, err := az.getPublicIP(newPubID); err == nil {
			newIP = extractPublicIPAddress(refreshed)
		}
	}
	if oldPub != nil && newIP != "" {
		logs = append(logs, fmt.Sprintf("公网 IPv4 已更换：%s -> %s", valueOrDefault(oldIP, "未知"), newIP))
	}

	return &provider.ChangeIPResult{
		Success:      true,
		Message:      "IP更换成功",
		NewIPAddress: newIP,
		Logs:         logs,
	}, nil
}

func (az *AzureService) AttachIPv4(vmName string, req provider.AttachIPv4Request) ([]string, string, error) {
	rawVM, err := az.getRawVM(vmName)
	if err != nil {
		return nil, "", err
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "ephemeral_public"
	}

	if mode == "ephemeral_public" {
		nic, targetCfg, err := az.findTargetIPv4Config(rawVM, req.PrivateIPID)
		if err != nil {
			return nil, "", err
		}
		nicID := stringValFromMap(nic, "id")
		cfgProps, _ := targetCfg["properties"].(map[string]interface{})
		if pubRef, ok := cfgProps["publicIPAddress"].(map[string]interface{}); ok && stringValFromMap(pubRef, "id") != "" {
			return nil, "", fmt.Errorf("该内网 IP (%s) 已绑定公网 IP", stringValFromMap(cfgProps, "privateIPAddress"))
		}

		rg := valueOrDefault(extractResourceGroupFromID(nicID), az.Account.ResourceGroup)
		location := valueOrDefault(stringValFromMap(nic, "location"), valueOrDefault(stringValFromMap(rawVM, "location"), az.Account.Location))
		zones := resolveAzurePublicIPZones(nil, toStringSlice(rawVM["zones"]))
		newPubName := fmt.Sprintf("ip-v4-%d", time.Now().UnixNano()/1e6)

		newPubID, createdPub, err := az.createPublicIPResource(rg, newPubName, location, "IPv4", "Standard", zones)
		if err != nil {
			return nil, "", fmt.Errorf("创建公网 IPv4 失败: %w", err)
		}

		cfgProps["publicIPAddress"] = map[string]interface{}{"id": newPubID}
		if err := az.putNIC(nicID, nic); err != nil {
			_ = az.armDelete(newPubID, "2023-04-01")
			return nil, "", fmt.Errorf("关联公网 IPv4 到网卡失败: %w", err)
		}

		newIP := extractPublicIPAddress(createdPub)
		if newIP == "" {
			if refreshed, err := az.getPublicIP(newPubID); err == nil {
				newIP = extractPublicIPAddress(refreshed)
			}
		}
		return []string{fmt.Sprintf("已为内网 IP %s 分配并绑定公网 IPv4: %s", stringValFromMap(cfgProps, "privateIPAddress"), valueOrDefault(newIP, newPubName))}, newIP, nil
	}

	if mode == "secondary_private" {
		nic, err := az.findTargetNIC(rawVM, req.VNICID)
		if err != nil {
			return nil, "", err
		}
		nicID := stringValFromMap(nic, "id")
		nicProps, _ := nic["properties"].(map[string]interface{})
		ipConfigs, _ := nicProps["ipConfigurations"].([]interface{})
		subnetID := extractSubnetIDFromNIC(nic)
		if subnetID == "" {
			return nil, "", fmt.Errorf("未找到网卡关联的子网 ID")
		}

		cfgName := fmt.Sprintf("ipconfig-v4-%d", time.Now().UnixNano()/1e6)
		newCfgProps := map[string]interface{}{
			"primary":                   false,
			"privateIPAddressVersion":   "IPv4",
			"privateIPAllocationMethod": "Dynamic",
			"subnet":                    map[string]interface{}{"id": subnetID},
		}
		if customIP := strings.TrimSpace(req.IPAddress); customIP != "" {
			parsed := net.ParseIP(customIP)
			if parsed == nil || parsed.To4() == nil {
				return nil, "", fmt.Errorf("无效的内网 IPv4 地址: %s", customIP)
			}
			newCfgProps["privateIPAllocationMethod"] = "Static"
			newCfgProps["privateIPAddress"] = customIP
		}

		logs := make([]string, 0, 2)
		var newPubID string
		if req.AssignPublicIP {
			rg := valueOrDefault(extractResourceGroupFromID(nicID), az.Account.ResourceGroup)
			location := valueOrDefault(stringValFromMap(nic, "location"), valueOrDefault(stringValFromMap(rawVM, "location"), az.Account.Location))
			zones := resolveAzurePublicIPZones(nil, toStringSlice(rawVM["zones"]))
			newPubName := fmt.Sprintf("ip-v4-%d", time.Now().UnixNano()/1e6)
			createdID, _, err := az.createPublicIPResource(rg, newPubName, location, "IPv4", "Standard", zones)
			if err != nil {
				return nil, "", fmt.Errorf("创建辅助公网 IPv4 失败: %w", err)
			}
			newPubID = createdID
			newCfgProps["publicIPAddress"] = map[string]interface{}{"id": newPubID}
		}

		ipConfigs = append(ipConfigs, map[string]interface{}{
			"name":       cfgName,
			"properties": newCfgProps,
		})
		nicProps["ipConfigurations"] = ipConfigs

		if err := az.putNIC(nicID, nic); err != nil {
			if newPubID != "" {
				_ = az.armDelete(newPubID, "2023-04-01")
			}
			return nil, "", fmt.Errorf("新增辅助内网 IPv4 配置失败: %w", err)
		}

		var assignedPriv, assignedPub string
		if refreshedNIC, err := az.getNetworkInterfaceByID(nicID); err == nil {
			if refProps, ok := refreshedNIC["properties"].(map[string]interface{}); ok {
				if cfgs, ok := refProps["ipConfigurations"].([]interface{}); ok {
					for _, raw := range cfgs {
						if m, ok := raw.(map[string]interface{}); ok && stringValFromMap(m, "name") == cfgName {
							if mp, ok := m["properties"].(map[string]interface{}); ok {
								assignedPriv = stringValFromMap(mp, "privateIPAddress")
							}
							break
						}
					}
				}
			}
		}
		logs = append(logs, fmt.Sprintf("已新增辅助内网 IPv4: %s", valueOrDefault(assignedPriv, cfgName)))
		if newPubID != "" {
			if refreshedPub, err := az.getPublicIP(newPubID); err == nil {
				assignedPub = extractPublicIPAddress(refreshedPub)
			}
			logs = append(logs, fmt.Sprintf("已绑定辅助公网 IPv4: %s", valueOrDefault(assignedPub, lastPathSegment(newPubID))))
			return logs, assignedPub, nil
		}
		return logs, assignedPriv, nil
	}

	return nil, "", fmt.Errorf("不支持的附加 IPv4 模式: %s", mode)
}

func (az *AzureService) DeleteIPv4Managed(vmName string, req provider.DeleteIPv4Request) ([]string, error) {
	rawVM, err := az.getRawVM(vmName)
	if err != nil {
		return nil, err
	}
	nic, targetCfg, err := az.findTargetIPv4Config(rawVM, req.PrivateIPID)
	if err != nil {
		return nil, err
	}

	nicID := stringValFromMap(nic, "id")
	nicProps, _ := nic["properties"].(map[string]interface{})
	cfgName := stringValFromMap(targetCfg, "name")
	cfgProps, _ := targetCfg["properties"].(map[string]interface{})
	isPrimary, _ := cfgProps["primary"].(bool)
	privAddr := stringValFromMap(cfgProps, "privateIPAddress")

	pubRef, _ := cfgProps["publicIPAddress"].(map[string]interface{})
	pubID := stringValFromMap(pubRef, "id")
	if pubID == "" {
		pubID = strings.TrimSpace(req.PublicIPID)
	}

	logs := make([]string, 0, 2)
	deletePub := true
	if req.DeletePublicIP != nil {
		deletePub = *req.DeletePublicIP
	}

	if req.DeletePrivateIP {
		if isPrimary {
			return nil, fmt.Errorf("不能删除主内网 IP 配置 (%s)", privAddr)
		}
		ipConfigs, _ := nicProps["ipConfigurations"].([]interface{})
		filtered := make([]interface{}, 0, len(ipConfigs))
		for _, item := range ipConfigs {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if stringValFromMap(m, "name") == cfgName || stringValFromMap(m, "id") == req.PrivateIPID {
				continue
			}
			filtered = append(filtered, item)
		}
		nicProps["ipConfigurations"] = filtered
		if err := az.putNIC(nicID, nic); err != nil {
			return nil, fmt.Errorf("删除辅助内网 IP 配置失败: %w", err)
		}
		logs = append(logs, fmt.Sprintf("已移除辅助内网 IPv4 配置: %s", valueOrDefault(privAddr, cfgName)))
	} else if pubID != "" {
		delete(cfgProps, "publicIPAddress")
		if err := az.putNIC(nicID, nic); err != nil {
			return nil, fmt.Errorf("解绑公网 IPv4 失败: %w", err)
		}
		if deletePub {
			logs = append(logs, "已从网卡解绑公网 IPv4")
		} else {
			logs = append(logs, fmt.Sprintf("已解绑公网 IPv4（保留资源 %s，注意未绑定 IPv4 仍会产生闲置费用）", lastPathSegment(pubID)))
		}
	}

	if pubID != "" && deletePub {
		if err := az.armDelete(pubID, "2023-04-01"); err != nil {
			return logs, fmt.Errorf("已从网卡解绑，但删除公网 IPv4 资源失败: %w", err)
		}
		logs = append(logs, fmt.Sprintf("已彻底删除公网 IPv4 资源: %s", lastPathSegment(pubID)))
	}

	if len(logs) == 0 {
		return nil, fmt.Errorf("未指定需要删除或解绑的 IPv4 资源")
	}
	return logs, nil
}

func (az *AzureService) AttachReservedIPv4Managed(vmName string, req provider.AttachReservedIPv4Request) (string, []string, error) {
	rawVM, err := az.getRawVM(vmName)
	if err != nil {
		return "", nil, err
	}
	nic, targetCfg, err := az.findTargetIPv4Config(rawVM, req.PrivateIPID)
	if err != nil {
		return "", nil, err
	}
	nicID := stringValFromMap(nic, "id")
	cfgProps, _ := targetCfg["properties"].(map[string]interface{})
	privAddr := stringValFromMap(cfgProps, "privateIPAddress")

	if pubRef, ok := cfgProps["publicIPAddress"].(map[string]interface{}); ok && stringValFromMap(pubRef, "id") != "" {
		return "", nil, fmt.Errorf("当前内网 IP (%s) 已绑定公网 IP，请先删除/解绑后再附加保留 IP", privAddr)
	}

	var targetPubID string
	var ipAddr string
	if req.CreateNew {
		name := strings.TrimSpace(req.DisplayName)
		if name == "" {
			name = fmt.Sprintf("reserved-ip-%d", time.Now().UnixNano()/1e6)
		}
		rg := valueOrDefault(extractResourceGroupFromID(nicID), az.Account.ResourceGroup)
		location := valueOrDefault(stringValFromMap(nic, "location"), valueOrDefault(stringValFromMap(rawVM, "location"), az.Account.Location))
		zones := resolveAzurePublicIPZones(nil, toStringSlice(rawVM["zones"]))

		createdID, createdPub, err := az.createPublicIPResource(rg, name, location, "IPv4", "Standard", zones)
		if err != nil {
			return "", nil, fmt.Errorf("创建静态公网 IPv4 失败: %w", err)
		}
		targetPubID = createdID
		ipAddr = extractPublicIPAddress(createdPub)
	} else {
		targetPubID = strings.TrimSpace(req.ReservedPublicIPID)
		if targetPubID == "" {
			return "", nil, fmt.Errorf("请选择要附加的静态公网 IP")
		}
		existingPub, err := az.getPublicIP(targetPubID)
		if err != nil {
			return "", nil, fmt.Errorf("读取目标公网 IP 失败: %w", err)
		}
		ipAddr = extractPublicIPAddress(existingPub)
	}

	cfgProps["publicIPAddress"] = map[string]interface{}{"id": targetPubID}
	if err := az.putNIC(nicID, nic); err != nil {
		if req.CreateNew {
			_ = az.armDelete(targetPubID, "2023-04-01")
		}
		return "", nil, fmt.Errorf("绑定公网 IPv4 到网卡失败: %w", err)
	}

	if ipAddr == "" {
		if refreshed, err := az.getPublicIP(targetPubID); err == nil {
			ipAddr = extractPublicIPAddress(refreshed)
		}
	}
	return ipAddr, []string{fmt.Sprintf("已将公网 IPv4 (%s) 绑定到内网 IP %s", valueOrDefault(ipAddr, lastPathSegment(targetPubID)), privAddr)}, nil
}

func (az *AzureService) EnableSubnetIPv6(vmName string, req provider.EnableIPv6Request) (string, []string, error) {
	rawVM, err := az.getRawVM(vmName)
	if err != nil {
		return "", nil, err
	}
	nic, err := az.findTargetNIC(rawVM, req.VNICID)
	if err != nil {
		return "", nil, err
	}
	if conflict, reason := az.detectNICBasicSKUConflict(nic); conflict {
		return "", nil, fmt.Errorf("%s", reason)
	}

	subnetID := extractSubnetIDFromNIC(nic)
	if subnetID == "" {
		return "", nil, fmt.Errorf("当前网卡未关联子网 ID")
	}
	vnetID := extractVNetIDFromSubnetID(subnetID)
	if vnetID == "" {
		return "", nil, fmt.Errorf("无法解析子网所属虚拟网络 (VNet) ID")
	}

	var vnet map[string]interface{}
	if err := az.armGetJSON(vnetID, "2023-04-01", &vnet); err != nil {
		return "", nil, fmt.Errorf("读取 VNet 失败: %w", err)
	}
	var subnet map[string]interface{}
	if err := az.armGetJSON(subnetID, "2023-04-01", &subnet); err != nil {
		return "", nil, fmt.Errorf("读取子网失败: %w", err)
	}

	logs := make([]string, 0, 3)
	vnetPrefixes := extractAddressPrefixes(vnet, true)
	vnetIPv6s := filterIPv6CIDRs(vnetPrefixes)
	if len(vnetIPv6s) == 0 {
		ulaPrefix, err := generateAzureULA48()
		if err != nil {
			return "", nil, fmt.Errorf("生成 ULA IPv6 前缀失败: %w", err)
		}
		vnetPrefixes = append(vnetPrefixes, ulaPrefix)
		vnetProps, _ := vnet["properties"].(map[string]interface{})
		if vnetProps == nil {
			vnetProps = map[string]interface{}{}
			vnet["properties"] = vnetProps
		}
		addrSpace, _ := vnetProps["addressSpace"].(map[string]interface{})
		if addrSpace == nil {
			addrSpace = map[string]interface{}{}
			vnetProps["addressSpace"] = addrSpace
		}
		addrSpace["addressPrefixes"] = vnetPrefixes
		if err := az.armPutJSON(vnetID, "2023-04-01", vnet, nil); err != nil {
			return "", nil, fmt.Errorf("为 VNet 添加 IPv6 地址空间失败: %w", err)
		}
		vnetIPv6s = []string{ulaPrefix}
		logs = append(logs, "已为 VNet 添加双栈 IPv6 地址空间: "+ulaPrefix)
	} else {
		logs = append(logs, "VNet 已包含 IPv6 地址空间: "+vnetIPv6s[0])
	}

	subnetPrefixes := extractAddressPrefixes(subnet, false)
	subnetIPv6s := filterIPv6CIDRs(subnetPrefixes)
	if len(subnetIPv6s) > 0 {
		logs = append(logs, "子网已开启 IPv6 前缀: "+subnetIPv6s[0])
		return subnetIPv6s[0], logs, nil
	}

	usedHexes := collectAzureUsedIPv6Hexes(vnet)
	hexByte := strings.ToUpper(strings.TrimSpace(req.HexByte))
	if hexByte == "" {
		hexByte = pickFreeHex(usedHexes, "01")
	}
	subnetIPv6, err := deriveSubnetIPv6FromParent(vnetIPv6s[0], hexByte)
	if err != nil {
		return "", nil, err
	}

	subnetProps, _ := subnet["properties"].(map[string]interface{})
	if subnetProps == nil {
		subnetProps = map[string]interface{}{}
		subnet["properties"] = subnetProps
	}
	delete(subnetProps, "addressPrefix")
	subnetProps["addressPrefixes"] = append(subnetPrefixes, subnetIPv6)

	if err := az.armPutJSON(subnetID, "2023-04-01", subnet, nil); err != nil {
		return "", nil, fmt.Errorf("为子网开启 IPv6 (%s) 失败: %w", subnetIPv6, err)
	}
	logs = append(logs, "已为子网开启双栈 IPv6 前缀: "+subnetIPv6)
	logs = append(logs, "提示：请确认 NSG 安全规则放行所需 IPv6 端口（如源地址为 * 或 ::/0）")
	return subnetIPv6, logs, nil
}

func (az *AzureService) AttachIPv6(vmName string, req provider.AttachIPv6Request) (*provider.IPv6Info, error) {
	rawVM, err := az.getRawVM(vmName)
	if err != nil {
		return nil, err
	}
	nic, err := az.findTargetNIC(rawVM, req.VNICID)
	if err != nil {
		return nil, err
	}
	if conflict, reason := az.detectNICBasicSKUConflict(nic); conflict {
		return nil, fmt.Errorf("%s", reason)
	}

	nicID := stringValFromMap(nic, "id")
	subnetID := extractSubnetIDFromNIC(nic)
	if subnetID == "" {
		return nil, fmt.Errorf("未找到网卡关联的子网")
	}

	var subnet map[string]interface{}
	if err := az.armGetJSON(subnetID, "2023-04-01", &subnet); err != nil {
		return nil, fmt.Errorf("读取子网失败: %w", err)
	}
	subnetIPv6s := filterIPv6CIDRs(extractAddressPrefixes(subnet, false))
	if len(subnetIPv6s) == 0 {
		return nil, fmt.Errorf("当前子网尚未开启 IPv6，请先点击「开启子网 IPv6」")
	}

	nicProps, _ := nic["properties"].(map[string]interface{})
	ipConfigs, _ := nicProps["ipConfigurations"].([]interface{})
	var unboundV6Cfg map[string]interface{}
	for _, rawCfg := range ipConfigs {
		cfg, ok := rawCfg.(map[string]interface{})
		if !ok {
			continue
		}
		cp, _ := cfg["properties"].(map[string]interface{})
		if cp == nil {
			continue
		}
		if strings.EqualFold(stringValFromMap(cp, "privateIPAddressVersion"), "IPv6") {
			if pubRef, ok := cp["publicIPAddress"].(map[string]interface{}); ok && stringValFromMap(pubRef, "id") != "" {
				return nil, fmt.Errorf("当前网卡已配置公网 IPv6，Azure 单张网卡仅支持 1 个 IPv6 配置；如需更换请点击「换IP」")
			}
			unboundV6Cfg = cfg
		}
	}

	rg := valueOrDefault(extractResourceGroupFromID(nicID), az.Account.ResourceGroup)
	location := valueOrDefault(stringValFromMap(nic, "location"), valueOrDefault(stringValFromMap(rawVM, "location"), az.Account.Location))
	zones := resolveAzurePublicIPZones(nil, toStringSlice(rawVM["zones"]))
	pubName := fmt.Sprintf("ip-v6-%d", time.Now().UnixNano()/1e6)

	pubV6ID, createdPub, err := az.createPublicIPResource(rg, pubName, location, "IPv6", "Standard", zones)
	if err != nil {
		return nil, fmt.Errorf("创建 Standard 公网 IPv6 失败: %w", err)
	}

	var targetCfgName string
	if unboundV6Cfg != nil {
		targetCfgName = stringValFromMap(unboundV6Cfg, "name")
		cp, _ := unboundV6Cfg["properties"].(map[string]interface{})
		cp["publicIPAddress"] = map[string]interface{}{"id": pubV6ID}
	} else {
		targetCfgName = fmt.Sprintf("ipconfig-v6-%d", time.Now().UnixNano()/1e6)
		ipConfigs = append(ipConfigs, map[string]interface{}{
			"name": targetCfgName,
			"properties": map[string]interface{}{
				"primary":                   false,
				"privateIPAddressVersion":   "IPv6",
				"privateIPAllocationMethod": "Dynamic",
				"subnet":                    map[string]interface{}{"id": subnetID},
				"publicIPAddress":           map[string]interface{}{"id": pubV6ID},
			},
		})
		nicProps["ipConfigurations"] = ipConfigs
	}

	if err := az.putNIC(nicID, nic); err != nil {
		_ = az.armDelete(pubV6ID, "2023-04-01")
		return nil, fmt.Errorf("将公网 IPv6 绑定到网卡失败: %w", err)
	}

	ipAddr := extractPublicIPAddress(createdPub)
	if ipAddr == "" {
		if refreshed, err := az.getPublicIP(pubV6ID); err == nil {
			ipAddr = extractPublicIPAddress(refreshed)
		}
	}

	return &provider.IPv6Info{
		ID:               nicID + "|ipcfg|" + targetCfgName,
		IPAddress:        ipAddr,
		CIDRPrefixLength: 128,
		SubnetID:         subnetID,
		VNICID:           nicID,
		CanChange:        true,
		DisplayName:      pubName,
		Lifetime:         "STATIC",
		SKU:              "Standard",
	}, nil
}

func (az *AzureService) ChangeIPv6(vmName, vnicID, ipv6ID string) (*provider.ChangeIPv6Result, error) {
	rawVM, err := az.getRawVM(vmName)
	if err != nil {
		return nil, err
	}
	nic, targetCfg, err := az.findTargetIPv6Config(rawVM, vnicID, ipv6ID)
	if err != nil {
		return nil, err
	}

	nicID := stringValFromMap(nic, "id")
	cfgName := stringValFromMap(targetCfg, "name")
	cfgProps, _ := targetCfg["properties"].(map[string]interface{})
	oldPubRef, _ := cfgProps["publicIPAddress"].(map[string]interface{})
	oldPubID := stringValFromMap(oldPubRef, "id")

	var oldIP string
	var oldZones []string
	if oldPubID != "" {
		if oldPub, err := az.getPublicIP(oldPubID); err == nil && oldPub != nil {
			oldIP = extractPublicIPAddress(oldPub)
			oldZones = toStringSlice(oldPub["zones"])
		}
	}

	rg := valueOrDefault(extractResourceGroupFromID(nicID), az.Account.ResourceGroup)
	location := valueOrDefault(stringValFromMap(nic, "location"), valueOrDefault(stringValFromMap(rawVM, "location"), az.Account.Location))
	zones := resolveAzurePublicIPZones(oldZones, toStringSlice(rawVM["zones"]))
	newPubName := fmt.Sprintf("ip-v6-%d", time.Now().UnixNano()/1e6)

	logs := []string{
		fmt.Sprintf("[1/3] 正在创建新的 Standard 公网 IPv6 (%s)...", newPubName),
	}
	newPubID, createdPub, err := az.createPublicIPResource(rg, newPubName, location, "IPv6", "Standard", zones)
	if err != nil {
		return nil, fmt.Errorf("创建新公网 IPv6 失败: %w", err)
	}
	logs = append(logs, fmt.Sprintf("[1/3] 已创建新公网 IPv6: %s", newPubName))

	logs = append(logs, "[2/3] 正在将新公网 IPv6 绑定到网卡...")
	cfgProps["publicIPAddress"] = map[string]interface{}{"id": newPubID}
	if err := az.putNIC(nicID, nic); err != nil {
		_ = az.armDelete(newPubID, "2023-04-01")
		return nil, fmt.Errorf("绑定新公网 IPv6 失败: %w", err)
	}
	logs = append(logs, "[2/3] 网卡绑定新公网 IPv6 成功")

	if oldPubID != "" {
		logs = append(logs, fmt.Sprintf("[3/3] 正在删除旧公网 IPv6 (%s)...", valueOrDefault(oldIP, lastPathSegment(oldPubID))))
		if err := az.armDelete(oldPubID, "2023-04-01"); err != nil {
			logs = append(logs, fmt.Sprintf("[3/3] 警告：删除旧公网 IPv6 失败: %v", err))
		} else {
			logs = append(logs, fmt.Sprintf("[3/3] 旧公网 IPv6 已删除: %s", valueOrDefault(oldIP, lastPathSegment(oldPubID))))
		}
	}

	newIP := extractPublicIPAddress(createdPub)
	if newIP == "" {
		if refreshed, err := az.getPublicIP(newPubID); err == nil {
			newIP = extractPublicIPAddress(refreshed)
		}
	}
	logs = append(logs, fmt.Sprintf("IPv6 更换完成：%s -> %s", valueOrDefault(oldIP, "未知"), valueOrDefault(newIP, "pending")))

	return &provider.ChangeIPv6Result{
		Success:      true,
		Message:      "IPv6 更换成功",
		OldIPAddress: oldIP,
		NewIPAddress: newIP,
		IPv6: &provider.IPv6Info{
			ID:               nicID + "|ipcfg|" + cfgName,
			IPAddress:        newIP,
			CIDRPrefixLength: 128,
			VNICID:           nicID,
			CanChange:        true,
			DisplayName:      newPubName,
			Lifetime:         "STATIC",
			SKU:              "Standard",
		},
		Logs: logs,
	}, nil
}

func (az *AzureService) DeleteIPv6(vmName, ipv6ID string) (string, error) {
	rawVM, err := az.getRawVM(vmName)
	if err != nil {
		return "", err
	}
	nic, targetCfg, err := az.findTargetIPv6Config(rawVM, "", ipv6ID)
	if err != nil {
		return "", err
	}

	nicID := stringValFromMap(nic, "id")
	nicProps, _ := nic["properties"].(map[string]interface{})
	cfgName := stringValFromMap(targetCfg, "name")
	cfgProps, _ := targetCfg["properties"].(map[string]interface{})
	pubRef, _ := cfgProps["publicIPAddress"].(map[string]interface{})
	pubID := stringValFromMap(pubRef, "id")

	oldIP := stringValFromMap(cfgProps, "privateIPAddress")
	if pubID != "" {
		if oldPub, err := az.getPublicIP(pubID); err == nil && oldPub != nil {
			if p := extractPublicIPAddress(oldPub); p != "" {
				oldIP = p
			}
		}
	}

	ipConfigs, _ := nicProps["ipConfigurations"].([]interface{})
	filtered := make([]interface{}, 0, len(ipConfigs))
	for _, raw := range ipConfigs {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if stringValFromMap(m, "name") == cfgName || stringValFromMap(m, "id") == stringValFromMap(targetCfg, "id") {
			continue
		}
		filtered = append(filtered, raw)
	}
	nicProps["ipConfigurations"] = filtered

	if err := az.putNIC(nicID, nic); err != nil {
		return "", fmt.Errorf("从网卡移除 IPv6 配置失败: %w", err)
	}
	if pubID != "" {
		if err := az.armDelete(pubID, "2023-04-01"); err != nil {
			return oldIP, fmt.Errorf("已从网卡移除 IPv6 配置，但删除公网 IPv6 资源失败: %w", err)
		}
	}
	return oldIP, nil
}

var _ provider.IPManager = (*AzureService)(nil)

// --- Azure IP Management Internal Helpers ---

func (az *AzureService) getRawVM(vmName string) (map[string]interface{}, error) {
	var reqURL string
	if cleanID, err := validateARMResourceID(vmName); err == nil {
		reqURL = fmt.Sprintf("https://management.azure.com%s?api-version=2023-03-01", cleanID)
	} else {
		reqURL = fmt.Sprintf(
			"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s?api-version=2023-03-01",
			url.PathEscape(az.Account.SubId),
			url.PathEscape(az.Account.ResourceGroup),
			url.PathEscape(vmName),
		)
	}
	var out map[string]interface{}
	if err := az.armGetJSON(reqURL, "", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (az *AzureService) instanceVNICIPInfos(rawVM map[string]interface{}) ([]provider.VNICIPInfo, error) {
	props, _ := rawVM["properties"].(map[string]interface{})
	netProf, _ := props["networkProfile"].(map[string]interface{})
	rawNICs, _ := netProf["networkInterfaces"].([]interface{})
	if len(rawNICs) == 0 {
		return []provider.VNICIPInfo{}, nil
	}

	vnics := make([]provider.VNICIPInfo, 0, len(rawNICs))
	for idx, item := range rawNICs {
		ref, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		nicID := stringValFromMap(ref, "id")
		if nicID == "" {
			continue
		}
		nic, err := az.getNetworkInterfaceByID(nicID)
		if err != nil {
			return nil, err
		}
		var isPrimary bool
		if refProps, ok := ref["properties"].(map[string]interface{}); ok {
			isPrimary, _ = refProps["primary"].(bool)
		}
		if idx == 0 && len(rawNICs) == 1 {
			isPrimary = true
		}

		nicProps, _ := nic["properties"].(map[string]interface{})
		subnetID := extractSubnetIDFromNIC(nic)
		vnetID := extractVNetIDFromSubnetID(subnetID)

		var subnetName, ipv4CIDR string
		var subnetIPv6s []string
		var setup *provider.SubnetIPv6Status
		if subnetID != "" {
			var subnet map[string]interface{}
			if err := az.armGetJSON(subnetID, "2023-04-01", &subnet); err == nil && subnet != nil {
				subnetName = stringValFromMap(subnet, "name")
				allSubPrefixes := extractAddressPrefixes(subnet, false)
				for _, p := range allSubPrefixes {
					if strings.Contains(p, ":") {
						subnetIPv6s = append(subnetIPv6s, p)
					} else if ipv4CIDR == "" {
						ipv4CIDR = p
					}
				}
				setup = az.inspectAzureSubnetIPv6Status(nic, subnet, vnetID, subnetIPv6s)
			}
		}
		if subnetIPv6s == nil {
			subnetIPv6s = []string{}
		}

		privIPs := make([]provider.PrivateIPInfo, 0)
		ipv6s := make([]provider.IPv6Info, 0)
		ipConfigs, _ := nicProps["ipConfigurations"].([]interface{})
		for _, rawCfg := range ipConfigs {
			cfg, ok := rawCfg.(map[string]interface{})
			if !ok {
				continue
			}
			cp, _ := cfg["properties"].(map[string]interface{})
			if cp == nil {
				continue
			}
			cfgName := stringValFromMap(cfg, "name")
			cfgID := valueOrDefault(stringValFromMap(cfg, "id"), nicID+"|ipcfg|"+cfgName)
			privVer := stringValFromMap(cp, "privateIPAddressVersion")
			privAddr := stringValFromMap(cp, "privateIPAddress")
			isCfgPrimary, _ := cp["primary"].(bool)
			isV6 := strings.EqualFold(privVer, "IPv6") || strings.Contains(privAddr, ":")

			var pubInfo *provider.PublicIPInfo
			if pubRef, ok := cp["publicIPAddress"].(map[string]interface{}); ok {
				if pubID := stringValFromMap(pubRef, "id"); pubID != "" {
					if rawPub, err := az.getPublicIP(pubID); err == nil && rawPub != nil {
						pubProps, _ := rawPub["properties"].(map[string]interface{})
						skuName := ""
						if skuMap, ok := rawPub["sku"].(map[string]interface{}); ok {
							skuName = stringValFromMap(skuMap, "name")
						}
						allocMethod := strings.ToUpper(stringValFromMap(pubProps, "publicIPAllocationMethod"))
						if ver := stringValFromMap(pubProps, "publicIPAddressVersion"); strings.EqualFold(ver, "IPv6") {
							isV6 = true
						}
						lifetime := "STATIC"
						if allocMethod == "DYNAMIC" {
							lifetime = "EPHEMERAL"
						}
						pubInfo = &provider.PublicIPInfo{
							ID:             pubID,
							IPAddress:      valueOrDefault(extractPublicIPAddress(rawPub), "分配中"),
							Lifetime:       lifetime,
							DisplayName:    stringValFromMap(rawPub, "name"),
							SKU:            skuName,
							IdleFeeWarning: !isV6,
						}
					}
				}
			}

			if isV6 {
				v6Addr := privAddr
				pubName := cfgName
				lifetime := "EPHEMERAL"
				skuName := ""
				canChange := false
				if pubInfo != nil {
					if pubInfo.IPAddress != "" && pubInfo.IPAddress != "分配中" {
						v6Addr = pubInfo.IPAddress
					}
					pubName = pubInfo.DisplayName
					lifetime = pubInfo.Lifetime
					skuName = pubInfo.SKU
					canChange = true
				}
				ipv6s = append(ipv6s, provider.IPv6Info{
					ID:               cfgID,
					IPAddress:        v6Addr,
					CIDRPrefixLength: 128,
					SubnetID:         subnetID,
					VNICID:           nicID,
					CanChange:        canChange,
					DisplayName:      pubName,
					Lifetime:         lifetime,
					SKU:              skuName,
				})
				continue
			}

			privIPs = append(privIPs, provider.PrivateIPInfo{
				ID:            cfgID,
				IPAddress:     privAddr,
				IsPrimary:     isCfgPrimary,
				HostnameLabel: cfgName,
				PublicIP:      pubInfo,
			})
		}

		vnics = append(vnics, provider.VNICIPInfo{
			ID:             nicID,
			DisplayName:    valueOrDefault(stringValFromMap(nic, "name"), lastPathSegment(nicID)),
			IsPrimary:      isPrimary,
			LifecycleState: "ATTACHED",
			MACAddress:     stringValFromMap(nicProps, "macAddress"),
			SubnetID:       subnetID,
			SubnetName:     subnetName,
			VCNID:          vnetID,
			IPv4CIDR:       ipv4CIDR,
			IPv6CIDRBlocks: subnetIPv6s,
			IPv6Setup:      setup,
			PrivateIPs:     privIPs,
			IPv6s:          ipv6s,
		})
	}
	return vnics, nil
}

func (az *AzureService) inspectAzureSubnetIPv6Status(nic, subnet map[string]interface{}, vnetID string, subnetIPv6s []string) *provider.SubnetIPv6Status {
	status := &provider.SubnetIPv6Status{
		SubnetHasIPv6: len(subnetIPv6s) > 0,
		VCNIPv6CIDRs:  []string{},
		UsedHexes:     []string{},
		SuggestedHex:  "01",
		CanEnable:     true,
		FirewallHint:  "请在「防火墙」中检查 NSG 入站/出站规则是否包含目标端口（源/目的为 * 或 ::/0）",
	}

	if conflict, reason := az.detectNICBasicSKUConflict(nic); conflict {
		status.CanEnable = false
		status.HasBasicSKUConflict = true
		status.BlockingReason = reason
	}

	if vnetID != "" {
		var vnet map[string]interface{}
		if err := az.armGetJSON(vnetID, "2023-04-01", &vnet); err == nil && vnet != nil {
			v6s := filterIPv6CIDRs(extractAddressPrefixes(vnet, true))
			status.VCNIPv6CIDRs = v6s
			status.VCNHasIPv6 = len(v6s) > 0
			used := collectAzureUsedIPv6Hexes(vnet)
			status.UsedHexes = used
			status.SuggestedHex = pickFreeHex(used, "01")
		}
	}

	// Check if subnet or NIC has an NSG; note Azure default AllowInternetOutBound already allows IPv6 egress.
	status.RouteHasIPv6Default = true
	status.HasInternetGateway = true
	status.SecurityListHasIPv6Egress = true
	return status
}

func (az *AzureService) detectNICBasicSKUConflict(nic map[string]interface{}) (bool, string) {
	nicProps, _ := nic["properties"].(map[string]interface{})
	ipConfigs, _ := nicProps["ipConfigurations"].([]interface{})
	for _, rawCfg := range ipConfigs {
		cfg, ok := rawCfg.(map[string]interface{})
		if !ok {
			continue
		}
		cp, _ := cfg["properties"].(map[string]interface{})
		if pubRef, ok := cp["publicIPAddress"].(map[string]interface{}); ok {
			if pubID := stringValFromMap(pubRef, "id"); pubID != "" {
				if rawPub, err := az.getPublicIP(pubID); err == nil && rawPub != nil {
					if skuMap, ok := rawPub["sku"].(map[string]interface{}); ok {
						if strings.EqualFold(stringValFromMap(skuMap, "name"), "Basic") {
							return true, fmt.Sprintf("检测到网卡已绑定 Basic SKU 公网 IP (%s)，Azure 不支持 Basic SKU 与 IPv6 混用；请先点击「换IP」升级为 Standard SKU。", stringValFromMap(rawPub, "name"))
						}
					}
				}
			}
		}
	}
	return false, ""
}

func (az *AzureService) listUnattachedPublicIPv4s(location string) ([]provider.ReservedPublicIP, error) {
	items, err := az.listArmResources("Microsoft.Network/publicIPAddresses", "2023-04-01", "")
	if err != nil {
		return nil, err
	}
	out := make([]provider.ReservedPublicIP, 0)
	for _, item := range items {
		if location != "" && !strings.EqualFold(stringValFromMap(item, "location"), location) {
			continue
		}
		props, _ := item["properties"].(map[string]interface{})
		if props == nil {
			continue
		}
		if ipCfg, _ := props["ipConfiguration"].(map[string]interface{}); ipCfg != nil && stringValFromMap(ipCfg, "id") != "" {
			continue
		}
		ver := stringValFromMap(props, "publicIPAddressVersion")
		if ver != "" && !strings.EqualFold(ver, "IPv4") {
			continue
		}
		skuName := ""
		if skuMap, ok := item["sku"].(map[string]interface{}); ok {
			skuName = stringValFromMap(skuMap, "name")
		}
		if strings.EqualFold(skuName, "Basic") {
			continue
		}
		ipAddr := stringValFromMap(props, "ipAddress")
		if ipAddr == "" {
			continue
		}
		out = append(out, provider.ReservedPublicIP{
			ID:          stringValFromMap(item, "id"),
			IPAddress:   ipAddr,
			DisplayName: stringValFromMap(item, "name"),
			SKU:         skuName,
		})
	}
	return out, nil
}

func (az *AzureService) findTargetNIC(rawVM map[string]interface{}, nicID string) (map[string]interface{}, error) {
	props, _ := rawVM["properties"].(map[string]interface{})
	netProf, _ := props["networkProfile"].(map[string]interface{})
	rawNICs, _ := netProf["networkInterfaces"].([]interface{})
	if len(rawNICs) == 0 {
		return nil, fmt.Errorf("实例未关联任何网卡")
	}

	chosenID := ""
	for _, item := range rawNICs {
		ref, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id := stringValFromMap(ref, "id")
		if nicID != "" && (strings.EqualFold(id, nicID) || strings.EqualFold(lastPathSegment(id), nicID)) {
			chosenID = id
			break
		}
		if nicID == "" {
			if refProps, ok := ref["properties"].(map[string]interface{}); ok {
				if isPrim, _ := refProps["primary"].(bool); isPrim {
					chosenID = id
					break
				}
			}
			if chosenID == "" {
				chosenID = id
			}
		}
	}
	if chosenID == "" {
		return nil, fmt.Errorf("未找到目标网卡 (%s)", nicID)
	}
	return az.getNetworkInterfaceByID(chosenID)
}

func (az *AzureService) findTargetIPv4Config(rawVM map[string]interface{}, privateIPID string) (map[string]interface{}, map[string]interface{}, error) {
	props, _ := rawVM["properties"].(map[string]interface{})
	netProf, _ := props["networkProfile"].(map[string]interface{})
	rawNICs, _ := netProf["networkInterfaces"].([]interface{})

	var fallbackNIC, fallbackCfg map[string]interface{}
	for _, item := range rawNICs {
		ref, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		nicID := stringValFromMap(ref, "id")
		if nicID == "" {
			continue
		}
		nic, err := az.getNetworkInterfaceByID(nicID)
		if err != nil {
			continue
		}
		nicProps, _ := nic["properties"].(map[string]interface{})
		ipConfigs, _ := nicProps["ipConfigurations"].([]interface{})
		for _, rawCfg := range ipConfigs {
			cfg, ok := rawCfg.(map[string]interface{})
			if !ok {
				continue
			}
			cp, _ := cfg["properties"].(map[string]interface{})
			if cp == nil {
				continue
			}
			if strings.EqualFold(stringValFromMap(cp, "privateIPAddressVersion"), "IPv6") || strings.Contains(stringValFromMap(cp, "privateIPAddress"), ":") {
				continue
			}
			cfgID := valueOrDefault(stringValFromMap(cfg, "id"), nicID+"|ipcfg|"+stringValFromMap(cfg, "name"))
			if privateIPID != "" && (cfgID == privateIPID || stringValFromMap(cfg, "id") == privateIPID || stringValFromMap(cfg, "name") == privateIPID) {
				return nic, cfg, nil
			}
			if privateIPID == "" {
				if isPrim, _ := cp["primary"].(bool); isPrim {
					return nic, cfg, nil
				}
				if fallbackCfg == nil {
					fallbackNIC = nic
					fallbackCfg = cfg
				}
			}
		}
	}
	if privateIPID == "" && fallbackNIC != nil && fallbackCfg != nil {
		return fallbackNIC, fallbackCfg, nil
	}
	return nil, nil, fmt.Errorf("未找到目标 IPv4 配置 (%s)", privateIPID)
}

func (az *AzureService) findTargetIPv6Config(rawVM map[string]interface{}, vnicID, ipv6ID string) (map[string]interface{}, map[string]interface{}, error) {
	props, _ := rawVM["properties"].(map[string]interface{})
	netProf, _ := props["networkProfile"].(map[string]interface{})
	rawNICs, _ := netProf["networkInterfaces"].([]interface{})

	for _, item := range rawNICs {
		ref, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		nicID := stringValFromMap(ref, "id")
		if nicID == "" {
			continue
		}
		if vnicID != "" && !strings.EqualFold(nicID, vnicID) && !strings.EqualFold(lastPathSegment(nicID), vnicID) {
			continue
		}
		nic, err := az.getNetworkInterfaceByID(nicID)
		if err != nil {
			continue
		}
		nicProps, _ := nic["properties"].(map[string]interface{})
		ipConfigs, _ := nicProps["ipConfigurations"].([]interface{})
		for _, rawCfg := range ipConfigs {
			cfg, ok := rawCfg.(map[string]interface{})
			if !ok {
				continue
			}
			cp, _ := cfg["properties"].(map[string]interface{})
			if cp == nil {
				continue
			}
			if !strings.EqualFold(stringValFromMap(cp, "privateIPAddressVersion"), "IPv6") && !strings.Contains(stringValFromMap(cp, "privateIPAddress"), ":") {
				continue
			}
			cfgID := valueOrDefault(stringValFromMap(cfg, "id"), nicID+"|ipcfg|"+stringValFromMap(cfg, "name"))
			if ipv6ID == "" || cfgID == ipv6ID || stringValFromMap(cfg, "id") == ipv6ID || stringValFromMap(cfg, "name") == ipv6ID {
				return nic, cfg, nil
			}
		}
	}
	return nil, nil, fmt.Errorf("未找到目标 IPv6 配置 (%s)", ipv6ID)
}

func (az *AzureService) createPublicIPResource(rg, ipName, location, ipVersion, skuName string, zones []string) (string, map[string]interface{}, error) {
	if rg == "" {
		rg = az.Account.ResourceGroup
	}
	if location == "" {
		location = az.Account.Location
	}
	if skuName == "" {
		skuName = "Standard"
	}
	pubID := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/publicIPAddresses/%s",
		url.PathEscape(az.Account.SubId),
		url.PathEscape(rg),
		url.PathEscape(ipName),
	)
	body := map[string]interface{}{
		"location": location,
		"properties": map[string]interface{}{
			"publicIPAllocationMethod": "Static",
			"publicIPAddressVersion":   ipVersion,
		},
		"sku": map[string]interface{}{
			"name": skuName,
			"tier": "Regional",
		},
	}
	if len(zones) > 0 {
		body["zones"] = zones
	}
	var out map[string]interface{}
	if err := az.armPutJSON(pubID, "2023-04-01", body, &out); err != nil {
		return "", nil, err
	}
	if out == nil || extractPublicIPAddress(out) == "" {
		if refreshed, err := az.getPublicIP(pubID); err == nil && refreshed != nil {
			out = refreshed
		}
	}
	return pubID, out, nil
}

func (az *AzureService) putNIC(nicID string, nic map[string]interface{}) error {
	return az.armPutJSON(nicID, "2023-04-01", nic, nil)
}

func (az *AzureService) armURL(armIDOrURL, apiVersion string) (string, error) {
	trimmed := strings.TrimSpace(armIDOrURL)
	if strings.HasPrefix(trimmed, "https://") {
		return trimmed, nil
	}
	cleanID, err := validateARMResourceID(trimmed)
	if err != nil {
		return "", err
	}
	if apiVersion == "" {
		apiVersion = "2023-04-01"
	}
	return fmt.Sprintf("https://management.azure.com%s?api-version=%s", cleanID, url.QueryEscape(apiVersion)), nil
}

func (az *AzureService) armGetJSON(armIDOrURL, apiVersion string, out interface{}) error {
	reqURL, err := az.armURL(armIDOrURL, apiVersion)
	if err != nil {
		return err
	}
	resp, err := az.request("GET", reqURL, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Azure GET %s failed (%d): %s", reqURL, resp.StatusCode, truncateErrBody(body))
	}
	if out != nil && len(body) > 0 {
		return json.Unmarshal(body, out)
	}
	return nil
}

func (az *AzureService) armPutJSON(armIDOrURL, apiVersion string, payload interface{}, out interface{}) error {
	reqURL, err := az.armURL(armIDOrURL, apiVersion)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := az.request("PUT", reqURL, bytes.NewBuffer(raw))
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(resp.Body)
	asyncURL := resp.Header.Get("Azure-AsyncOperation")
	locationURL := resp.Header.Get("Location")
	resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		if asyncURL != "" && isARMProvisioningUpdating(body) {
			if err := az.waitForAzureOperation(asyncURL, 120*time.Second); err != nil {
				return err
			}
			if out != nil {
				return az.armGetJSON(armIDOrURL, apiVersion, out)
			}
			return nil
		}
		if out != nil && len(body) > 0 {
			return json.Unmarshal(body, out)
		}
		return nil
	case http.StatusAccepted:
		pollURL := asyncURL
		if pollURL == "" {
			pollURL = locationURL
		}
		if pollURL != "" {
			if err := az.waitForAzureOperation(pollURL, 120*time.Second); err != nil {
				return err
			}
		} else {
			time.Sleep(azurePollInterval)
		}
		if out != nil {
			return az.armGetJSON(armIDOrURL, apiVersion, out)
		}
		return nil
	default:
		return fmt.Errorf("Azure PUT failed (%d): %s", resp.StatusCode, truncateErrBody(body))
	}
}

func (az *AzureService) armDelete(armIDOrURL, apiVersion string) error {
	reqURL, err := az.armURL(armIDOrURL, apiVersion)
	if err != nil {
		return err
	}
	resp, err := az.request("DELETE", reqURL, nil)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(resp.Body)
	asyncURL := resp.Header.Get("Azure-AsyncOperation")
	locationURL := resp.Header.Get("Location")
	resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
		return nil
	case http.StatusAccepted:
		pollURL := asyncURL
		if pollURL == "" {
			pollURL = locationURL
		}
		if pollURL != "" {
			return az.waitForAzureOperation(pollURL, 120*time.Second)
		}
		time.Sleep(azurePollInterval)
		return nil
	default:
		return fmt.Errorf("Azure DELETE failed (%d): %s", resp.StatusCode, truncateErrBody(body))
	}
}

func isARMProvisioningUpdating(body []byte) bool {
	var obj map[string]interface{}
	if json.Unmarshal(body, &obj) != nil {
		return false
	}
	props, _ := obj["properties"].(map[string]interface{})
	if props == nil {
		return false
	}
	state := strings.ToLower(stringValFromMap(props, "provisioningState"))
	return state == "updating" || state == "creating"
}

func extractPublicIPAddress(pub map[string]interface{}) string {
	if pub == nil {
		return ""
	}
	props, _ := pub["properties"].(map[string]interface{})
	return strings.TrimSpace(stringValFromMap(props, "ipAddress"))
}

func extractSubnetIDFromNIC(nic map[string]interface{}) string {
	props, _ := nic["properties"].(map[string]interface{})
	cfgs, _ := props["ipConfigurations"].([]interface{})
	for _, raw := range cfgs {
		cfg, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		cp, _ := cfg["properties"].(map[string]interface{})
		if sub, ok := cp["subnet"].(map[string]interface{}); ok {
			if id := stringValFromMap(sub, "id"); id != "" {
				return id
			}
		}
	}
	return ""
}

func extractVNetIDFromSubnetID(subnetID string) string {
	idx := strings.Index(strings.ToLower(subnetID), "/subnets/")
	if idx <= 0 {
		return ""
	}
	return subnetID[:idx]
}

func resolveAzurePublicIPZones(oldZones, vmZones []string) []string {
	if len(oldZones) > 0 {
		return oldZones
	}
	if len(vmZones) > 0 {
		return vmZones
	}
	return nil
}

func toStringSlice(raw interface{}) []string {
	arr, ok := raw.([]interface{})
	if !ok {
		if direct, ok := raw.([]string); ok {
			return direct
		}
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func extractAddressPrefixes(resource map[string]interface{}, isVNet bool) []string {
	if resource == nil {
		return nil
	}
	props, _ := resource["properties"].(map[string]interface{})
	if props == nil {
		return nil
	}
	var rawPrefixes []string
	if isVNet {
		if addrSpace, ok := props["addressSpace"].(map[string]interface{}); ok {
			rawPrefixes = append(rawPrefixes, toStringSlice(addrSpace["addressPrefixes"])...)
		}
	} else {
		rawPrefixes = append(rawPrefixes, toStringSlice(props["addressPrefixes"])...)
		if p := stringValFromMap(props, "addressPrefix"); p != "" {
			found := false
			for _, existing := range rawPrefixes {
				if strings.EqualFold(existing, p) {
					found = true
					break
				}
			}
			if !found {
				rawPrefixes = append(rawPrefixes, p)
			}
		}
	}
	return rawPrefixes
}

func filterIPv6CIDRs(cidrs []string) []string {
	out := make([]string, 0)
	for _, c := range cidrs {
		if strings.Contains(c, ":") {
			out = append(out, c)
		}
	}
	return out
}

func generateAzureULA48() (string, error) {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("fd%02x:%02x%02x:%02x%02x::/48", b[0], b[1], b[2], b[3], b[4]), nil
}

func collectAzureUsedIPv6Hexes(vnet map[string]interface{}) []string {
	props, _ := vnet["properties"].(map[string]interface{})
	subnets, _ := props["subnets"].([]interface{})
	usedMap := make(map[string]struct{})
	for _, rawSub := range subnets {
		sub, ok := rawSub.(map[string]interface{})
		if !ok {
			continue
		}
		for _, cidr := range filterIPv6CIDRs(extractAddressPrefixes(sub, false)) {
			ip, _, err := net.ParseCIDR(strings.TrimSpace(cidr))
			if err != nil {
				continue
			}
			ip16 := ip.To16()
			if ip16 == nil {
				continue
			}
			usedMap[fmt.Sprintf("%02X", ip16[7])] = struct{}{}
		}
	}
	out := make([]string, 0, len(usedMap))
	for k := range usedMap {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func pickFreeHex(used []string, preferred string) string {
	set := make(map[string]struct{}, len(used))
	for _, u := range used {
		set[strings.ToUpper(u)] = struct{}{}
	}
	if preferred != "" {
		if _, taken := set[strings.ToUpper(preferred)]; !taken {
			return strings.ToUpper(preferred)
		}
	}
	for n := 0; n <= 255; n++ {
		cand := fmt.Sprintf("%02X", n)
		if _, taken := set[cand]; !taken {
			return cand
		}
	}
	return "01"
}

func deriveSubnetIPv6FromParent(vnetCIDR, hexByte string) (string, error) {
	val, err := strconv.ParseUint(strings.TrimSpace(hexByte), 16, 8)
	if err != nil {
		return "", fmt.Errorf("无效的子网十六进制索引 %q: %w", hexByte, err)
	}
	_, ipNet, err := net.ParseCIDR(strings.TrimSpace(vnetCIDR))
	if err != nil || ipNet == nil {
		return "", fmt.Errorf("无效的 VNet IPv6 地址空间: %s", vnetCIDR)
	}
	ip16 := ipNet.IP.To16()
	if ip16 == nil {
		return "", fmt.Errorf("无法解析 VNet IPv6 前缀: %s", vnetCIDR)
	}
	out := make(net.IP, 16)
	copy(out, ip16)
	out[7] = byte(val)
	for i := 8; i < 16; i++ {
		out[i] = 0
	}
	return fmt.Sprintf("%s/64", out.String()), nil
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
