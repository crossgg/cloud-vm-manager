package oci

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/provider"
)

var ociIPPollInterval = 1500 * time.Millisecond

type OCIPublicIPInfo = provider.PublicIPInfo
type OCIPrivateIPInfo = provider.PrivateIPInfo
type OCIIPv6Info = provider.IPv6Info
type OCISubnetIPv6Status = provider.SubnetIPv6Status
type OCIEnableIPv6Request = provider.EnableIPv6Request
type OCIVNICIPInfo = provider.VNICIPInfo
type OCIReservedPublicIP = provider.ReservedPublicIP
type OCIDNSBindingSummary = provider.DNSBindingSummary
type OCIIPManagementInfo = provider.IPManagementInfo
type OCIAttachIPv4Request = provider.AttachIPv4Request
type OCIAttachIPv6Request = provider.AttachIPv6Request
type OCIChangeIPv6Result = provider.ChangeIPv6Result

func ociIPCapabilities() provider.IPCapabilities {
	return provider.IPCapabilities{
		CanAddSecondaryPrivateIPv4: true,
		CanAttachReservedIPv4:      true,
		CanCreateReservedIPv4:      true,
		CanDetachPublicIPv4:        true,
		CanAttachIPv6:              true,
		CanChangeIPv6:              true,
		CanDeleteIPv6:              true,
		CanEnableSubnetIPv6:        true,
		IPv6PrefixConfigurable:     true,
		HasIdlePublicIPFeeWarning:  false,
	}
}

func (o *OCIService) GetIPManagement(instanceID string) (*OCIIPManagementInfo, error) {
	rawInstance, err := o.rawInstance(instanceID)
	if err != nil {
		return nil, err
	}

	vnics, err := o.instanceVNICIPInfos(instanceID)
	if err != nil {
		return nil, err
	}

	reservedIPs, err := o.listAvailableReservedPublicIPs()
	if err != nil {
		// Non-fatal: still show VNIC IPs even if listing reserved IPs fails.
		reservedIPs = []OCIReservedPublicIP{}
	}

	for i := range vnics {
		vnics[i].IPv6Setup = o.inspectVNICIPv6Status(&vnics[i])
	}

	return &OCIIPManagementInfo{
		Provider:     "oci",
		InstanceID:   instanceID,
		InstanceName: valueOrDefault(stringValue(rawInstance["displayName"]), instanceID),
		Capabilities: ociIPCapabilities(),
		VNICs:        vnics,
		ReservedIPs:  reservedIPs,
	}, nil
}

func (o *OCIService) inspectVNICIPv6Status(vnic *OCIVNICIPInfo) *OCISubnetIPv6Status {
	status := &OCISubnetIPv6Status{
		SubnetHasIPv6: len(vnic.IPv6CIDRBlocks) > 0,
		VCNIPv6CIDRs:  []string{},
		UsedHexes:     []string{},
		SuggestedHex:  "7E",
		CanEnable:     true,
	}
	if vnic.SubnetID == "" {
		return status
	}

	var subnet map[string]interface{}
	if err := o.requestJSON("GET", "/subnets/"+url.PathEscape(vnic.SubnetID), nil, nil, &subnet, false); err != nil || subnet == nil {
		return status
	}

	vcnID := valueOrDefault(stringValue(subnet["vcnId"]), vnic.VCNID)
	compartmentID := stringValue(subnet["compartmentId"])
	if vcnID != "" {
		var vcn map[string]interface{}
		if err := o.requestJSON("GET", "/vcns/"+url.PathEscape(vcnID), nil, nil, &vcn, false); err == nil && vcn != nil {
			if compartmentID == "" {
				compartmentID = stringValue(vcn["compartmentId"])
			}
			vcnIPv6 := extractSubnetIPv6CIDRs(vcn)
			status.VCNIPv6CIDRs = vcnIPv6
			status.VCNHasIPv6 = len(vcnIPv6) > 0
		}
	}
	if compartmentID == "" {
		compartmentID, _ = o.compartmentIDOrTenancy()
	}

	// Collect used /64 hex bytes across subnets in the same VCN so we can suggest an unused hex.
	usedSet := make(map[string]struct{})
	addUsedHexes := func(cidrs []string) {
		for _, c := range cidrs {
			ip, _, err := net.ParseCIDR(strings.TrimSpace(c))
			if err != nil {
				continue
			}
			ip16 := ip.To16()
			if ip16 == nil {
				continue
			}
			hexStr := fmt.Sprintf("%02X", ip16[7])
			usedSet[hexStr] = struct{}{}
		}
	}
	addUsedHexes(vnic.IPv6CIDRBlocks)
	if compartmentID != "" && vcnID != "" {
		var siblings []map[string]interface{}
		if err := o.requestJSON("GET", "/subnets", url.Values{
			"compartmentId": {compartmentID},
			"vcnId":         {vcnID},
		}, nil, &siblings, false); err == nil {
			for _, sub := range siblings {
				addUsedHexes(extractSubnetIPv6CIDRs(sub))
			}
		}
	}
	for h := range usedSet {
		status.UsedHexes = append(status.UsedHexes, h)
	}
	sort.Strings(status.UsedHexes)

	if _, taken := usedSet["7E"]; taken {
		for n := 0; n <= 255; n++ {
			cand := fmt.Sprintf("%02X", n)
			if _, ok := usedSet[cand]; !ok {
				status.SuggestedHex = cand
				break
			}
		}
	}

	if rtID := stringValue(subnet["routeTableId"]); rtID != "" {
		var rt map[string]interface{}
		if err := o.requestJSON("GET", "/routeTables/"+url.PathEscape(rtID), nil, nil, &rt, false); err == nil && rt != nil {
			if rawRules, ok := rt["routeRules"].([]interface{}); ok {
				for _, r := range rawRules {
					m, ok := r.(map[string]interface{})
					if !ok {
						continue
					}
					dest := stringValue(m["destination"])
					entity := stringValue(m["networkEntityId"])
					if dest == "::/0" && entity != "" {
						status.RouteHasIPv6Default = true
					}
					if strings.Contains(entity, "internetgateway") {
						status.HasInternetGateway = true
					}
				}
			}
		}
	}
	if !status.HasInternetGateway && compartmentID != "" && vcnID != "" {
		if igwID := o.findVCNInternetGatewayID(compartmentID, vcnID); igwID != "" {
			status.HasInternetGateway = true
		}
	}

	for _, slID := range stringSliceValue(subnet["securityListIds"]) {
		rawSL, err := o.rawSecurityList(slID)
		if err != nil || rawSL == nil {
			continue
		}
		if securityListHasIPv6Egress(rawSL) {
			status.SecurityListHasIPv6Egress = true
			break
		}
	}

	return status
}

func (o *OCIService) instanceVNICIPInfos(instanceID string) ([]OCIVNICIPInfo, error) {
	compartmentID, err := o.compartmentIDOrTenancy()
	if err != nil {
		return nil, err
	}

	var attachments []map[string]interface{}
	if err := o.requestJSON("GET", "/vnicAttachments", url.Values{
		"compartmentId": {compartmentID},
		"instanceId":    {instanceID},
	}, nil, &attachments, false); err != nil {
		return nil, err
	}

	vnics := make([]OCIVNICIPInfo, 0, len(attachments))
	for _, attachment := range attachments {
		if state := stringValue(attachment["lifecycleState"]); state != "" && state != "ATTACHED" {
			continue
		}
		vnicID := stringValue(attachment["vnicId"])
		if vnicID == "" {
			continue
		}

		var rawVNIC map[string]interface{}
		if err := o.requestJSON("GET", "/vnics/"+url.PathEscape(vnicID), nil, nil, &rawVNIC, false); err != nil {
			return nil, err
		}

		isPrimary, _ := rawVNIC["isPrimary"].(bool)
		subnetID := stringValue(rawVNIC["subnetId"])
		vcnID := stringValue(rawVNIC["vcnId"])
		var subnetName, ipv4CIDR string
		var ipv6CIDRs []string
		var prohibitPublic bool

		if subnetID != "" {
			var subnet map[string]interface{}
			if err := o.requestJSON("GET", "/subnets/"+url.PathEscape(subnetID), nil, nil, &subnet, false); err == nil && subnet != nil {
				subnetName = stringValue(subnet["displayName"])
				if vcnID == "" {
					vcnID = stringValue(subnet["vcnId"])
				}
				ipv4CIDR = stringValue(subnet["cidrBlock"])
				prohibitPublic, _ = subnet["prohibitPublicIpOnVnic"].(bool)
				ipv6CIDRs = extractSubnetIPv6CIDRs(subnet)
			}
		}

		var rawPrivateIPs []map[string]interface{}
		if err := o.requestJSON("GET", "/privateIps", url.Values{"vnicId": {vnicID}}, nil, &rawPrivateIPs, false); err != nil {
			return nil, err
		}

		privateIPs := make([]OCIPrivateIPInfo, 0, len(rawPrivateIPs))
		for _, rawPriv := range rawPrivateIPs {
			privID := stringValue(rawPriv["id"])
			isPrimPriv, _ := rawPriv["isPrimary"].(bool)
			item := OCIPrivateIPInfo{
				ID:            privID,
				IPAddress:     stringValue(rawPriv["ipAddress"]),
				IsPrimary:     isPrimPriv,
				HostnameLabel: stringValue(rawPriv["hostnameLabel"]),
			}
			if privID != "" {
				if pub, err := o.publicIPByPrivateIP(privID); err == nil && pub != nil && stringValue(pub["id"]) != "" {
					item.PublicIP = &OCIPublicIPInfo{
						ID:          stringValue(pub["id"]),
						IPAddress:   stringValue(pub["ipAddress"]),
						Lifetime:    strings.ToUpper(valueOrDefault(stringValue(pub["lifetime"]), "EPHEMERAL")),
						DisplayName: stringValue(pub["displayName"]),
					}
				}
			}
			privateIPs = append(privateIPs, item)
		}
		sort.SliceStable(privateIPs, func(i, j int) bool {
			return privateIPs[i].IsPrimary && !privateIPs[j].IsPrimary
		})

		ipv6s, err := o.listVNICIPv6s(vnicID)
		if err != nil {
			ipv6s = []OCIIPv6Info{}
		}

		vnics = append(vnics, OCIVNICIPInfo{
			ID:                    vnicID,
			DisplayName:           valueOrDefault(stringValue(rawVNIC["displayName"]), vnicID),
			IsPrimary:             isPrimary,
			LifecycleState:        valueOrDefault(stringValue(attachment["lifecycleState"]), "ATTACHED"),
			MACAddress:            stringValue(rawVNIC["macAddress"]),
			SubnetID:              subnetID,
			SubnetName:            subnetName,
			VCNID:                 vcnID,
			IPv4CIDR:              ipv4CIDR,
			IPv6CIDRBlocks:        ipv6CIDRs,
			ProhibitPublicIPOnNIC: prohibitPublic,
			PrivateIPs:            privateIPs,
			IPv6s:                 ipv6s,
		})
	}

	if len(vnics) == 0 {
		return nil, fmt.Errorf("no attached VNIC found for OCI instance %s", instanceID)
	}
	sort.SliceStable(vnics, func(i, j int) bool {
		return vnics[i].IsPrimary && !vnics[j].IsPrimary
	})
	return vnics, nil
}

func (o *OCIService) listVNICIPv6s(vnicID string) ([]OCIIPv6Info, error) {
	if strings.TrimSpace(vnicID) == "" {
		return []OCIIPv6Info{}, nil
	}
	var rawIPv6s []map[string]interface{}
	if err := o.requestJSON("GET", "/ipv6", url.Values{"vnicId": {vnicID}}, nil, &rawIPv6s, false); err != nil {
		return nil, err
	}
	result := make([]OCIIPv6Info, 0, len(rawIPv6s))
	for _, raw := range rawIPv6s {
		result = append(result, normalizeOCIIPv6(raw))
	}
	return result, nil
}

func normalizeOCIIPv6(raw map[string]interface{}) OCIIPv6Info {
	prefixLen := intFromFloat(float64InterfaceValue(raw["cidrPrefixLength"]))
	if prefixLen == 0 {
		prefixLen = 128
	}
	return OCIIPv6Info{
		ID:               stringValue(raw["id"]),
		IPAddress:        stringValue(raw["ipAddress"]),
		CIDRPrefixLength: prefixLen,
		SubnetID:         stringValue(raw["subnetId"]),
		VNICID:           stringValue(raw["vnicId"]),
		CanChange:        prefixLen == 128,
	}
}

func extractSubnetIPv6CIDRs(subnet map[string]interface{}) []string {
	seen := make(map[string]struct{})
	cidrs := make([]string, 0)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		key := v
		formatted := v
		if _, ipNet, err := net.ParseCIDR(v); err == nil && ipNet != nil && ipNet.IP.To4() == nil {
			key = ipNet.String()
			formatted = formatExpandedIPv6CIDR(ipNet)
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		cidrs = append(cidrs, formatted)
	}
	for _, s := range stringSliceValue(subnet["ipv6CidrBlocks"]) {
		add(s)
	}
	add(stringValue(subnet["ipv6CidrBlock"]))
	return cidrs
}

func formatExpandedIPv6CIDR(ipNet *net.IPNet) string {
	ip16 := ipNet.IP.To16()
	if ip16 == nil {
		return ipNet.String()
	}
	ones, _ := ipNet.Mask.Size()
	parts := make([]string, 8)
	for i := 0; i < 8; i++ {
		parts[i] = fmt.Sprintf("%02x%02x", ip16[i*2], ip16[i*2+1])
	}
	return fmt.Sprintf("%s/%d", strings.Join(parts, ":"), ones)
}

func canonicalCIDRKey(cidr string) string {
	cidr = strings.TrimSpace(cidr)
	if _, ipNet, err := net.ParseCIDR(cidr); err == nil && ipNet != nil {
		return ipNet.String()
	}
	return cidr
}

func cidrListContains(cidrs []string, target string) bool {
	targetKey := canonicalCIDRKey(target)
	for _, c := range cidrs {
		if canonicalCIDRKey(c) == targetKey {
			return true
		}
	}
	return false
}

func (o *OCIService) listAvailableReservedPublicIPs() ([]OCIReservedPublicIP, error) {
	compartmentID, err := o.compartmentIDOrTenancy()
	if err != nil {
		return nil, err
	}
	var rawPublicIPs []map[string]interface{}
	if err := o.requestJSON("GET", "/publicIps", url.Values{
		"compartmentId": {compartmentID},
		"scope":         {"REGION"},
		"lifetime":      {"RESERVED"},
	}, nil, &rawPublicIPs, false); err != nil {
		return nil, err
	}

	result := make([]OCIReservedPublicIP, 0)
	for _, raw := range rawPublicIPs {
		privID := stringValue(raw["privateIpId"])
		if privID != "" {
			continue
		}
		id := stringValue(raw["id"])
		if id == "" {
			continue
		}
		result = append(result, OCIReservedPublicIP{
			ID:          id,
			IPAddress:   stringValue(raw["ipAddress"]),
			DisplayName: stringValue(raw["displayName"]),
		})
	}
	return result, nil
}

func (o *OCIService) findInstancePrivateIP(instanceID, privateIPID string) (OCIVNICIPInfo, OCIPrivateIPInfo, error) {
	vnics, err := o.instanceVNICIPInfos(instanceID)
	if err != nil {
		return OCIVNICIPInfo{}, OCIPrivateIPInfo{}, err
	}
	for _, vnic := range vnics {
		for _, priv := range vnic.PrivateIPs {
			if privateIPID == "" && vnic.IsPrimary && priv.IsPrimary {
				return vnic, priv, nil
			}
			if priv.ID == privateIPID {
				return vnic, priv, nil
			}
		}
	}
	if privateIPID == "" && len(vnics) > 0 && len(vnics[0].PrivateIPs) > 0 {
		return vnics[0], vnics[0].PrivateIPs[0], nil
	}
	return OCIVNICIPInfo{}, OCIPrivateIPInfo{}, fmt.Errorf("未找到目标私有 IP (%s)", privateIPID)
}

func (o *OCIService) ChangePrivateIPPublicIP(instanceID, privateIPID string) (*provider.ChangeIPResult, error) {
	vnic, priv, err := o.findInstancePrivateIP(instanceID, privateIPID)
	if err != nil {
		return nil, err
	}
	if priv.PublicIP == nil || priv.PublicIP.ID == "" {
		return nil, fmt.Errorf("当前私有 IP (%s) 未绑定公网 IP，已取消换 IP；如需分配请使用「附加 IPv4」", priv.IPAddress)
	}
	if strings.EqualFold(priv.PublicIP.Lifetime, "RESERVED") {
		return nil, fmt.Errorf("当前绑定的是保留 IP (%s)，请先解绑保留 IP 后再分配或更换临时 IP", priv.PublicIP.IPAddress)
	}

	compartmentID, err := o.compartmentIDOrTenancy()
	if err != nil {
		return nil, err
	}

	oldIP := priv.PublicIP.IPAddress
	logs := []string{
		fmt.Sprintf("[1/4] 已定位 VNIC: %s，私有 IP: %s", vnic.DisplayName, priv.IPAddress),
		fmt.Sprintf("[2/4] 正在删除旧临时公网 IP: %s...", oldIP),
	}

	if err := o.deletePublicIP(priv.PublicIP.ID); err != nil {
		return nil, fmt.Errorf("删除旧临时公网 IP 失败: %w", err)
	}
	logs = append(logs, "[2/4] 旧临时公网 IP 已删除")

	if err := o.waitNoPublicIP(priv.ID); err != nil {
		return nil, err
	}
	logs = append(logs, "[3/4] 已确认旧公网 IP 已解除绑定")

	body := map[string]interface{}{
		"compartmentId": compartmentID,
		"lifetime":      "EPHEMERAL",
		"privateIpId":   priv.ID,
		"displayName":   "ephemeral-ip-" + time.Now().Format("20060102150405"),
	}
	var created map[string]interface{}
	logs = append(logs, "[4/4] 正在分配新的临时公网 IP...")
	if err := o.requestJSON("POST", "/publicIps", nil, body, &created, false); err != nil {
		return nil, fmt.Errorf("分配新临时公网 IP 失败: %w", err)
	}

	newIP := stringValue(created["ipAddress"])
	logs = append(logs, fmt.Sprintf("[4/4] 新临时公网 IP 分配成功: %s", valueOrDefault(newIP, "pending")))

	return &provider.ChangeIPResult{
		Success:      true,
		Message:      "IP更换成功",
		NewIPAddress: newIP,
		Logs:         logs,
	}, nil
}

func (o *OCIService) AttachIPv4(instanceID string, req OCIAttachIPv4Request) ([]string, string, error) {
	compartmentID, err := o.compartmentIDOrTenancy()
	if err != nil {
		return nil, "", err
	}
	vnics, err := o.instanceVNICIPInfos(instanceID)
	if err != nil {
		return nil, "", err
	}

	var targetVNIC *OCIVNICIPInfo
	for i := range vnics {
		if req.VNICID == "" && vnics[i].IsPrimary {
			targetVNIC = &vnics[i]
			break
		}
		if vnics[i].ID == req.VNICID {
			targetVNIC = &vnics[i]
			break
		}
	}
	if targetVNIC == nil {
		targetVNIC = &vnics[0]
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "ephemeral_public"
	}

	if mode == "ephemeral_public" {
		if targetVNIC.ProhibitPublicIPOnNIC {
			return nil, "", fmt.Errorf("当前子网禁止分配公网 IP")
		}
		var targetPriv *OCIPrivateIPInfo
		for i := range targetVNIC.PrivateIPs {
			p := &targetVNIC.PrivateIPs[i]
			if req.PrivateIPID != "" && p.ID == req.PrivateIPID {
				targetPriv = p
				break
			}
			if req.PrivateIPID == "" && p.IsPrimary {
				targetPriv = p
				break
			}
		}
		if targetPriv == nil && len(targetVNIC.PrivateIPs) > 0 {
			targetPriv = &targetVNIC.PrivateIPs[0]
		}
		if targetPriv == nil {
			return nil, "", fmt.Errorf("VNIC 下未找到可用的私有 IP")
		}
		if targetPriv.PublicIP != nil && targetPriv.PublicIP.ID != "" {
			return nil, "", fmt.Errorf("私有 IP %s 已绑定公网 IP (%s)", targetPriv.IPAddress, targetPriv.PublicIP.IPAddress)
		}

		var created map[string]interface{}
		if err := o.requestJSON("POST", "/publicIps", nil, map[string]interface{}{
			"compartmentId": compartmentID,
			"lifetime":      "EPHEMERAL",
			"privateIpId":   targetPriv.ID,
			"displayName":   "ephemeral-ip-" + time.Now().Format("20060102150405"),
		}, &created, false); err != nil {
			return nil, "", fmt.Errorf("分配临时公网 IP 失败: %w", err)
		}
		newIP := stringValue(created["ipAddress"])
		return []string{fmt.Sprintf("已为私有 IP %s 分配临时公网 IP: %s", targetPriv.IPAddress, newIP)}, newIP, nil
	}

	if mode == "secondary_private" {
		body := map[string]interface{}{
			"vnicId": targetVNIC.ID,
		}
		if ip := strings.TrimSpace(req.IPAddress); ip != "" {
			parsed := net.ParseIP(ip)
			if parsed == nil || parsed.To4() == nil {
				return nil, "", fmt.Errorf("无效的私有 IPv4 地址: %s", ip)
			}
			body["ipAddress"] = ip
		}
		if host := strings.TrimSpace(req.HostnameLabel); host != "" {
			body["hostnameLabel"] = host
		}

		var createdPriv map[string]interface{}
		if err := o.requestJSON("POST", "/privateIps", nil, body, &createdPriv, false); err != nil {
			return nil, "", fmt.Errorf("创建辅助私有 IP 失败: %w", err)
		}
		privID := stringValue(createdPriv["id"])
		privAddr := stringValue(createdPriv["ipAddress"])
		logs := []string{fmt.Sprintf("已创建辅助私有 IP: %s", privAddr)}

		if req.AssignPublicIP && privID != "" {
			if targetVNIC.ProhibitPublicIPOnNIC {
				return logs, "", fmt.Errorf("辅助私有 IP %s 已创建，但当前子网禁止分配公网 IP", privAddr)
			}
			var createdPub map[string]interface{}
			if err := o.requestJSON("POST", "/publicIps", nil, map[string]interface{}{
				"compartmentId": compartmentID,
				"lifetime":      "EPHEMERAL",
				"privateIpId":   privID,
				"displayName":   "ephemeral-ip-" + time.Now().Format("20060102150405"),
			}, &createdPub, false); err != nil {
				return logs, "", fmt.Errorf("辅助私有 IP %s 已创建，但分配公网 IP 失败: %w", privAddr, err)
			}
			pubAddr := stringValue(createdPub["ipAddress"])
			logs = append(logs, fmt.Sprintf("已为辅助私有 IP %s 分配临时公网 IP: %s", privAddr, pubAddr))
			return logs, pubAddr, nil
		}
		return logs, privAddr, nil
	}

	return nil, "", fmt.Errorf("不支持的附加 IPv4 模式: %s", mode)
}

func (o *OCIService) DeleteIPv4(instanceID, privateIPID, publicIPID string, deletePrivateIP bool) ([]string, error) {
	_, priv, err := o.findInstancePrivateIP(instanceID, privateIPID)
	if err != nil {
		return nil, err
	}

	logs := make([]string, 0)
	targetPubID := strings.TrimSpace(publicIPID)
	if targetPubID == "" && priv.PublicIP != nil {
		targetPubID = priv.PublicIP.ID
	}

	if targetPubID != "" && priv.PublicIP != nil && priv.PublicIP.ID == targetPubID {
		if strings.EqualFold(priv.PublicIP.Lifetime, "RESERVED") {
			if err := o.unassignReservedPublicIP(targetPubID); err != nil {
				return nil, fmt.Errorf("解绑保留公网 IP 失败: %w", err)
			}
			logs = append(logs, fmt.Sprintf("已解绑保留公网 IP: %s", priv.PublicIP.IPAddress))
		} else {
			if err := o.deletePublicIP(targetPubID); err != nil {
				return nil, fmt.Errorf("删除临时公网 IP 失败: %w", err)
			}
			logs = append(logs, fmt.Sprintf("已删除临时公网 IP: %s", priv.PublicIP.IPAddress))
		}
		if err := o.waitNoPublicIP(priv.ID); err != nil {
			return logs, err
		}
	}

	if deletePrivateIP {
		if priv.IsPrimary {
			return logs, fmt.Errorf("不能删除主私有 IP (%s)", priv.IPAddress)
		}
		if err := o.requestJSON("DELETE", "/privateIps/"+url.PathEscape(priv.ID), nil, nil, nil, false); err != nil {
			return logs, fmt.Errorf("删除辅助私有 IP 失败: %w", err)
		}
		logs = append(logs, fmt.Sprintf("已删除辅助私有 IP: %s", priv.IPAddress))
	}

	if len(logs) == 0 {
		return nil, fmt.Errorf("未指定需要删除或解绑的 IPv4 资源")
	}
	return logs, nil
}

func (o *OCIService) AttachReservedIPv4(instanceID, privateIPID, reservedPublicIPID string, createNew bool, displayName string) (string, []string, error) {
	compartmentID, err := o.compartmentIDOrTenancy()
	if err != nil {
		return "", nil, err
	}
	vnic, priv, err := o.findInstancePrivateIP(instanceID, privateIPID)
	if err != nil {
		return "", nil, err
	}
	if vnic.ProhibitPublicIPOnNIC {
		return "", nil, fmt.Errorf("当前子网禁止分配公网 IP")
	}
	if priv.PublicIP != nil && priv.PublicIP.ID != "" {
		return "", nil, fmt.Errorf("私有 IP %s 当前已绑定公网 IP (%s)，请先删除/解绑后再附加保留 IP", priv.IPAddress, priv.PublicIP.IPAddress)
	}

	if createNew {
		name := strings.TrimSpace(displayName)
		if name == "" {
			name = "reserved-ip-" + time.Now().Format("20060102150405")
		}
		var created map[string]interface{}
		if err := o.requestJSON("POST", "/publicIps", nil, map[string]interface{}{
			"compartmentId": compartmentID,
			"lifetime":      "RESERVED",
			"privateIpId":   priv.ID,
			"displayName":   name,
		}, &created, false); err != nil {
			return "", nil, fmt.Errorf("创建并绑定保留 IP 失败: %w", err)
		}
		ipAddr := stringValue(created["ipAddress"])
		return ipAddr, []string{fmt.Sprintf("已创建并绑定保留 IP %s 到私有 IP %s", ipAddr, priv.IPAddress)}, nil
	}

	reservedPublicIPID = strings.TrimSpace(reservedPublicIPID)
	if reservedPublicIPID == "" {
		return "", nil, fmt.Errorf("请选择要附加的保留 IP")
	}

	var updated map[string]interface{}
	if err := o.requestJSON("PUT", "/publicIps/"+url.PathEscape(reservedPublicIPID), nil, map[string]interface{}{
		"privateIpId": priv.ID,
	}, &updated, false); err != nil {
		return "", nil, fmt.Errorf("附加保留 IP 失败: %w", err)
	}
	ipAddr := stringValue(updated["ipAddress"])
	return ipAddr, []string{fmt.Sprintf("已将保留 IP %s 绑定到私有 IP %s", valueOrDefault(ipAddr, reservedPublicIPID), priv.IPAddress)}, nil
}

func (o *OCIService) DeleteIPv4Managed(instanceID string, req provider.DeleteIPv4Request) ([]string, error) {
	return o.DeleteIPv4(instanceID, req.PrivateIPID, req.PublicIPID, req.DeletePrivateIP)
}

func (o *OCIService) AttachReservedIPv4Managed(instanceID string, req provider.AttachReservedIPv4Request) (string, []string, error) {
	return o.AttachReservedIPv4(instanceID, req.PrivateIPID, req.ReservedPublicIPID, req.CreateNew, req.DisplayName)
}

var _ provider.IPManager = (*OCIService)(nil)

func (o *OCIService) AttachIPv6(instanceID string, req OCIAttachIPv6Request) (*OCIIPv6Info, error) {
	vnics, err := o.instanceVNICIPInfos(instanceID)
	if err != nil {
		return nil, err
	}
	var targetVNIC *OCIVNICIPInfo
	for i := range vnics {
		if req.VNICID == "" && vnics[i].IsPrimary {
			targetVNIC = &vnics[i]
			break
		}
		if vnics[i].ID == req.VNICID {
			targetVNIC = &vnics[i]
			break
		}
	}
	if targetVNIC == nil {
		return nil, fmt.Errorf("未找到目标 VNIC (%s)", req.VNICID)
	}
	if len(targetVNIC.IPv6CIDRBlocks) == 0 {
		return nil, fmt.Errorf("当前 VNIC 所在子网未启用 IPv6 前缀，无法分配 IPv6")
	}

	prefixLen := req.CIDRPrefixLength
	if prefixLen == 0 {
		prefixLen = 128
	}
	if prefixLen < 80 || prefixLen > 128 || prefixLen%4 != 0 {
		return nil, fmt.Errorf("IPv6 CIDR 前缀长度必须在 80 到 128 之间且为 4 的倍数（如 80、96、112、128）")
	}

	subnetCIDR := strings.TrimSpace(req.IPv6SubnetCIDR)
	if subnetCIDR == "" {
		subnetCIDR = targetVNIC.IPv6CIDRBlocks[0]
	} else if !cidrListContains(targetVNIC.IPv6CIDRBlocks, subnetCIDR) {
		return nil, fmt.Errorf("选择的子网 IPv6 前缀 %s 不属于当前 VNIC 所在子网", subnetCIDR)
	}

	assignMode := strings.ToLower(strings.TrimSpace(req.AssignMode))
	if assignMode == "" {
		assignMode = "auto"
	}

	body := map[string]interface{}{
		"vnicId": targetVNIC.ID,
	}
	if prefixLen != 128 {
		body["cidrPrefixLength"] = prefixLen
	}
	if len(targetVNIC.IPv6CIDRBlocks) > 1 {
		body["ipv6SubnetCidr"] = subnetCIDR
	}

	if assignMode == "manual" {
		ipAddr := strings.TrimSpace(req.IPAddress)
		if ipAddr == "" {
			return nil, fmt.Errorf("手动分配模式下请输入 IPv6 地址")
		}
		parsed := net.ParseIP(ipAddr)
		if parsed == nil || parsed.To4() != nil {
			return nil, fmt.Errorf("无效的 IPv6 地址: %s", ipAddr)
		}
		if _, cidrNet, err := net.ParseCIDR(subnetCIDR); err == nil {
			if !cidrNet.Contains(parsed) {
				return nil, fmt.Errorf("IPv6 地址 %s 不在所选子网前缀 %s 范围内", ipAddr, subnetCIDR)
			}
		}
		body["ipAddress"] = ipAddr
	} else if assignMode != "auto" {
		return nil, fmt.Errorf("不支持的 IPv6 分配模式: %s", assignMode)
	}

	var created map[string]interface{}
	if err := o.requestJSON("POST", "/ipv6", nil, body, &created, false); err != nil {
		return nil, fmt.Errorf("分配 IPv6 失败: %w", err)
	}

	info := normalizeOCIIPv6(created)
	return &info, nil
}

func (o *OCIService) ChangeIPv6(instanceID, vnicID, ipv6ID string) (*OCIChangeIPv6Result, error) {
	ipv6ID = strings.TrimSpace(ipv6ID)
	if ipv6ID == "" {
		return nil, fmt.Errorf("缺少 IPv6 ID")
	}

	vnics, err := o.instanceVNICIPInfos(instanceID)
	if err != nil {
		return nil, err
	}

	var targetVNIC *OCIVNICIPInfo
	var targetIPv6 *OCIIPv6Info
	for i := range vnics {
		if vnicID != "" && vnics[i].ID != vnicID {
			continue
		}
		for j := range vnics[i].IPv6s {
			if vnics[i].IPv6s[j].ID == ipv6ID {
				targetVNIC = &vnics[i]
				targetIPv6 = &vnics[i].IPv6s[j]
				break
			}
		}
		if targetIPv6 != nil {
			break
		}
	}
	if targetVNIC == nil || targetIPv6 == nil {
		return nil, fmt.Errorf("在实例 %s 中未找到目标 IPv6 (%s)", instanceID, ipv6ID)
	}
	if !targetIPv6.CanChange {
		return nil, fmt.Errorf("仅支持更换 128 位 (/128) 的 IPv6 地址，当前为 /%d", targetIPv6.CIDRPrefixLength)
	}

	matchedPrefix := matchIPv6SubnetCIDR(targetIPv6.IPAddress, targetVNIC.IPv6CIDRBlocks)
	oldIP := targetIPv6.IPAddress
	logs := []string{
		fmt.Sprintf("[1/3] 已定位 VNIC: %s，旧 IPv6: %s/128", targetVNIC.DisplayName, oldIP),
		fmt.Sprintf("[2/3] 正在删除旧 IPv6 (%s)...", oldIP),
	}

	if err := o.requestJSON("DELETE", "/ipv6/"+url.PathEscape(ipv6ID), nil, nil, nil, false); err != nil {
		return nil, fmt.Errorf("删除旧 IPv6 (%s) 失败: %w", oldIP, err)
	}
	if err := o.waitIPv6Deleted(ipv6ID); err != nil {
		return nil, fmt.Errorf("旧 IPv6 (%s) 删除等待超时: %w", oldIP, err)
	}
	logs = append(logs, fmt.Sprintf("[2/3] 旧 IPv6 (%s) 已删除", oldIP))

	createBody := map[string]interface{}{
		"vnicId": targetVNIC.ID,
	}
	if len(targetVNIC.IPv6CIDRBlocks) > 1 && matchedPrefix != "" {
		createBody["ipv6SubnetCidr"] = matchedPrefix
	}
	logs = append(logs, "[3/3] 正在从子网前缀自动分配新 IPv6...")
	var created map[string]interface{}
	if err := o.requestJSON("POST", "/ipv6", nil, createBody, &created, false); err != nil {
		return nil, fmt.Errorf("旧 IPv6 (%s) 已删除，但分配新 IPv6 失败: %w", oldIP, err)
	}

	info := normalizeOCIIPv6(created)
	logs = append(logs, fmt.Sprintf("[3/3] 新 IPv6 分配成功: %s", info.IPAddress))
	return &OCIChangeIPv6Result{
		Success:      true,
		Message:      "IPv6 更换成功",
		OldIPAddress: oldIP,
		NewIPAddress: info.IPAddress,
		IPv6:         &info,
		Logs:         logs,
	}, nil
}

func (o *OCIService) DeleteIPv6(instanceID, ipv6ID string) (string, error) {
	ipv6ID = strings.TrimSpace(ipv6ID)
	if ipv6ID == "" {
		return "", fmt.Errorf("缺少 IPv6 ID")
	}
	vnics, err := o.instanceVNICIPInfos(instanceID)
	if err != nil {
		return "", err
	}
	var oldIP string
	found := false
	for _, vnic := range vnics {
		for _, ip6 := range vnic.IPv6s {
			if ip6.ID == ipv6ID {
				oldIP = ip6.IPAddress
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return "", fmt.Errorf("在实例 %s 中未找到目标 IPv6 (%s)", instanceID, ipv6ID)
	}
	if err := o.requestJSON("DELETE", "/ipv6/"+url.PathEscape(ipv6ID), nil, nil, nil, false); err != nil {
		return "", fmt.Errorf("删除 IPv6 失败: %w", err)
	}
	return oldIP, nil
}

func (o *OCIService) waitIPv6Deleted(ipv6ID string) error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var raw map[string]interface{}
		err := o.requestJSON("GET", "/ipv6/"+url.PathEscape(ipv6ID), nil, nil, &raw, true)
		if err != nil {
			return err
		}
		if raw == nil || stringValue(raw["id"]) == "" {
			return nil
		}
		state := strings.ToUpper(stringValue(raw["lifecycleState"]))
		if state == "TERMINATED" || state == "DELETED" {
			return nil
		}
		time.Sleep(ociIPPollInterval)
	}
	return fmt.Errorf("等待 OCI 删除 IPv6 (%s) 超时", ipv6ID)
}

func matchIPv6SubnetCIDR(ipAddr string, cidrs []string) string {
	if len(cidrs) == 0 {
		return ""
	}
	parsed := net.ParseIP(strings.TrimSpace(ipAddr))
	if parsed != nil {
		for _, cidr := range cidrs {
			if _, ipNet, err := net.ParseCIDR(cidr); err == nil && ipNet.Contains(parsed) {
				return cidr
			}
		}
	}
	return cidrs[0]
}

func deriveSubnetIPv6CIDR(vcnIPv6CIDR, hexByte string) (string, error) {
	hexByte = strings.TrimSpace(hexByte)
	if len(hexByte) != 2 {
		return "", fmt.Errorf("请输入 2 位十六进制字符（00 - FF），例如 7E")
	}
	val, err := strconv.ParseUint(hexByte, 16, 8)
	if err != nil {
		return "", fmt.Errorf("无效的十六进制数 %q，请输入 00 - FF", hexByte)
	}
	_, ipNet, err := net.ParseCIDR(strings.TrimSpace(vcnIPv6CIDR))
	if err != nil || ipNet == nil {
		return "", fmt.Errorf("无效的 VCN IPv6 前缀: %s", vcnIPv6CIDR)
	}
	ip16 := ipNet.IP.To16()
	if ip16 == nil {
		return "", fmt.Errorf("无法解析 IPv6 前缀: %s", vcnIPv6CIDR)
	}
	out := make(net.IP, 16)
	copy(out, ip16)
	out[7] = byte(val)
	for i := 8; i < 16; i++ {
		out[i] = 0
	}
	return fmt.Sprintf("%s/64", out.String()), nil
}

func (o *OCIService) findVCNInternetGatewayID(compartmentID, vcnID string) string {
	if compartmentID == "" || vcnID == "" {
		return ""
	}
	var igws []map[string]interface{}
	if err := o.requestJSON("GET", "/internetGateways", url.Values{
		"compartmentId": {compartmentID},
		"vcnId":         {vcnID},
	}, nil, &igws, false); err != nil {
		return ""
	}
	fallback := ""
	for _, igw := range igws {
		id := stringValue(igw["id"])
		if id == "" {
			continue
		}
		if enabled, ok := igw["isEnabled"].(bool); ok && enabled {
			return id
		}
		if fallback == "" {
			fallback = id
		}
	}
	return fallback
}

func securityListHasIPv6Egress(rawSL map[string]interface{}) bool {
	rawEgress, ok := rawSL["egressSecurityRules"].([]interface{})
	if !ok {
		return false
	}
	for _, item := range rawEgress {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		dest := strings.TrimSpace(stringValue(m["destination"]))
		proto := strings.ToLower(strings.TrimSpace(stringValue(m["protocol"])))
		if dest == "::/0" && (proto == "all" || proto == "") {
			return true
		}
	}
	return false
}

func (o *OCIService) ensureSecurityListIPv6Egress(slID string) (bool, error) {
	if slID == "" {
		return false, fmt.Errorf("missing OCI security list id")
	}
	rawList, err := o.rawSecurityList(slID)
	if err != nil {
		return false, err
	}
	if securityListHasIPv6Egress(rawList) {
		return false, nil
	}

	var egressRules []interface{}
	if existing, ok := rawList["egressSecurityRules"].([]interface{}); ok {
		egressRules = append(egressRules, existing...)
	}
	egressRules = append(egressRules, map[string]interface{}{
		"destination":     "::/0",
		"destinationType": "CIDR_BLOCK",
		"protocol":        "all",
		"isStateless":     false,
	})

	var ingressRules interface{} = []interface{}{}
	if existing, ok := rawList["ingressSecurityRules"].([]interface{}); ok {
		ingressRules = existing
	}

	body := map[string]interface{}{
		"definedTags":          valueOrEmptyMap(rawList["definedTags"]),
		"freeformTags":         valueOrEmptyMap(rawList["freeformTags"]),
		"ingressSecurityRules": ingressRules,
		"egressSecurityRules":  egressRules,
	}
	if displayName := stringValue(rawList["displayName"]); displayName != "" {
		body["displayName"] = displayName
	}

	if err := o.requestJSON("PUT", "/securityLists/"+url.PathEscape(slID), nil, body, nil, false); err != nil {
		return false, err
	}
	return true, nil
}

func (o *OCIService) EnableSubnetIPv6(instanceID string, req OCIEnableIPv6Request) (string, []string, error) {
	vnics, err := o.instanceVNICIPInfos(instanceID)
	if err != nil {
		return "", nil, err
	}
	var targetVNIC *OCIVNICIPInfo
	for i := range vnics {
		if req.VNICID == "" && vnics[i].IsPrimary {
			targetVNIC = &vnics[i]
			break
		}
		if vnics[i].ID == req.VNICID {
			targetVNIC = &vnics[i]
			break
		}
	}
	if targetVNIC == nil {
		targetVNIC = &vnics[0]
	}
	if targetVNIC.SubnetID == "" {
		return "", nil, fmt.Errorf("VNIC 没有关联的子网 ID")
	}

	var subnet map[string]interface{}
	if err := o.requestJSON("GET", "/subnets/"+url.PathEscape(targetVNIC.SubnetID), nil, nil, &subnet, false); err != nil {
		return "", nil, err
	}
	existingSubnetIPv6 := extractSubnetIPv6CIDRs(subnet)
	hexByte := strings.TrimSpace(req.HexByte)
	if len(existingSubnetIPv6) == 0 {
		if len(hexByte) != 2 {
			return "", nil, fmt.Errorf("子网 IPv6 Hex 必须是 2 位十六进制字符 (00-FF)，当前为 %q", hexByte)
		}
		if _, err := strconv.ParseUint(hexByte, 16, 8); err != nil {
			return "", nil, fmt.Errorf("子网 IPv6 Hex 无效 (%q): %w", hexByte, err)
		}
	}

	vcnID := valueOrDefault(stringValue(subnet["vcnId"]), targetVNIC.VCNID)
	if vcnID == "" {
		return "", nil, fmt.Errorf("未能定位子网所属的 VCN")
	}
	compartmentID := stringValue(subnet["compartmentId"])

	var vcn map[string]interface{}
	if err := o.requestJSON("GET", "/vcns/"+url.PathEscape(vcnID), nil, nil, &vcn, false); err != nil {
		return "", nil, err
	}
	if compartmentID == "" {
		compartmentID = stringValue(vcn["compartmentId"])
	}
	if compartmentID == "" {
		compartmentID, _ = o.compartmentIDOrTenancy()
	}

	logs := make([]string, 0)
	vcnIPv6CIDRs := extractSubnetIPv6CIDRs(vcn)
	if len(vcnIPv6CIDRs) == 0 {
		if err := o.requestJSON("POST", "/vcns/"+url.PathEscape(vcnID)+"/actions/addIpv6Cidr", nil, map[string]interface{}{
			"isOracleGuaAllocationEnabled": true,
		}, nil, false); err != nil {
			return "", nil, fmt.Errorf("为 VCN 启用 IPv6 失败: %w", err)
		}
		for attempt := 0; attempt < 12; attempt++ {
			if err := o.requestJSON("GET", "/vcns/"+url.PathEscape(vcnID), nil, nil, &vcn, false); err != nil {
				return "", nil, err
			}
			vcnIPv6CIDRs = extractSubnetIPv6CIDRs(vcn)
			if len(vcnIPv6CIDRs) > 0 {
				break
			}
			time.Sleep(ociIPPollInterval)
		}
		if len(vcnIPv6CIDRs) == 0 {
			return "", nil, fmt.Errorf("VCN 启用 IPv6 后暂未获取到 /56 前缀，请稍后重试")
		}
		logs = append(logs, "已为 VCN 分配 /56 IPv6 前缀: "+vcnIPv6CIDRs[0])
	}

	var subnetCIDR string
	if len(existingSubnetIPv6) > 0 {
		subnetCIDR = existingSubnetIPv6[0]
		logs = append(logs, "当前子网已存在 /64 IPv6 前缀: "+subnetCIDR)
	} else {
		subnetCIDR, err = deriveSubnetIPv6CIDR(vcnIPv6CIDRs[0], hexByte)
		if err != nil {
			return "", nil, err
		}
		if err := o.requestJSON("POST", "/subnets/"+url.PathEscape(targetVNIC.SubnetID)+"/actions/addIpv6Cidr", nil, map[string]interface{}{
			"ipv6CidrBlock": subnetCIDR,
		}, nil, false); err != nil {
			return "", nil, fmt.Errorf("为子网分配 IPv6 前缀 (%s) 失败: %w", subnetCIDR, err)
		}
		logs = append(logs, "已为子网分配 /64 IPv6 前缀: "+subnetCIDR)
	}

	if req.AddDefaultRoute {
		rtID := stringValue(subnet["routeTableId"])
		if rtID == "" {
			logs = append(logs, "警告：子网未关联路由表，无法自动添加 ::/0 默认路由，请前往 OCI 控制台检查路由表")
		} else {
			var rt map[string]interface{}
			if err := o.requestJSON("GET", "/routeTables/"+url.PathEscape(rtID), nil, nil, &rt, false); err != nil || rt == nil {
				logs = append(logs, fmt.Sprintf("警告：读取子网路由表失败，未自动补充 ::/0 路由: %v", err))
			} else {
				rawRules, _ := rt["routeRules"].([]interface{})
				hasV6Default := false
				igwEntity := ""
				rules := make([]map[string]interface{}, 0, len(rawRules)+1)
				for _, r := range rawRules {
					m, ok := r.(map[string]interface{})
					if !ok {
						continue
					}
					rules = append(rules, m)
					dest := stringValue(m["destination"])
					entity := stringValue(m["networkEntityId"])
					if dest == "::/0" && entity != "" {
						hasV6Default = true
					}
					if strings.Contains(entity, "internetgateway") && igwEntity == "" {
						igwEntity = entity
					}
				}
				if hasV6Default {
					logs = append(logs, "路由表已包含 ::/0 默认 IPv6 路由")
				} else {
					if igwEntity == "" {
						igwEntity = o.findVCNInternetGatewayID(compartmentID, vcnID)
					}
					if igwEntity == "" {
						logs = append(logs, "警告：未找到可用的互联网网关 (IGW)，未自动添加 ::/0 默认路由，请检查 VCN 网关配置")
					} else {
						rules = append(rules, map[string]interface{}{
							"destination":     "::/0",
							"destinationType": "CIDR_BLOCK",
							"networkEntityId": igwEntity,
						})
						if err := o.requestJSON("PUT", "/routeTables/"+url.PathEscape(rtID), nil, map[string]interface{}{
							"routeRules": rules,
						}, nil, false); err != nil {
							logs = append(logs, fmt.Sprintf("警告：路由表添加 ::/0 -> IGW 失败（子网前缀已分配）: %v", err))
						} else {
							logs = append(logs, "已为路由表添加 ::/0 -> IGW 默认 IPv6 路由")
						}
					}
				}
			}
		}
	}

	if req.AddSecurityListEgress {
		slIDs := stringSliceValue(subnet["securityListIds"])
		if len(slIDs) == 0 {
			logs = append(logs, "警告：当前子网未关联安全列表，请前往「防火墙管理」手动添加 IPv6 出站 ::/0 规则")
		} else {
			alreadyHas := false
			for _, slID := range slIDs {
				if rawSL, err := o.rawSecurityList(slID); err == nil && rawSL != nil && securityListHasIPv6Egress(rawSL) {
					alreadyHas = true
					break
				}
			}
			if alreadyHas {
				logs = append(logs, "子网安全列表已包含 IPv6 出站 ::/0 规则")
			} else {
				added, err := o.ensureSecurityListIPv6Egress(slIDs[0])
				if err != nil {
					logs = append(logs, fmt.Sprintf("警告：自动添加安全列表 ::/0 出站规则失败，请前往「防火墙管理」手动配置: %v", err))
				} else if added {
					logs = append(logs, "已在子网默认安全列表添加 IPv6 出站 ::/0 (所有协议) 规则；如需入站端口放行，请前往「防火墙管理」配置")
				}
			}
		}
	} else {
		logs = append(logs, "提示：未自动修改安全列表规则，如需允许 IPv6 出站或入站访问，请前往「防火墙管理」添加 ::/0 出站及所需入站规则")
	}

	return subnetCIDR, logs, nil
}
