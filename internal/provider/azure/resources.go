package azure

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type AzureResourceItem struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	ResourceGroup    string `json:"resourceGroup"`
	Location         string `json:"location"`
	Category         string `json:"category"` // vm | nic | public_ip | disk | snapshot | vnet | nsg | other
	Type             string `json:"type"`
	SKU              string `json:"sku,omitempty"`
	SizeGB           int    `json:"sizeGB,omitempty"`
	IPAddress        string `json:"ipAddress,omitempty"`
	IPVersion        string `json:"ipVersion,omitempty"`
	AllocationMethod string `json:"allocationMethod,omitempty"`
	AttachedTo       string `json:"attachedTo,omitempty"`
	AttachedIPID     string `json:"attachedIpId,omitempty"`
	Orphan           bool   `json:"orphan"`
	BillingRisk      string `json:"billingRisk"` // free_eligible | free_network | likely_billable | orphan_billable
	BillingNote      string `json:"billingNote"`
}

type AzureResourceSummary struct {
	TotalCount          int `json:"totalCount"`
	VMCount             int `json:"vmCount"`
	OrphanPublicIPCount int `json:"orphanPublicIpCount"`
	OrphanDiskCount     int `json:"orphanDiskCount"`
	OrphanNICCount      int `json:"orphanNicCount"`
	SnapshotCount       int `json:"snapshotCount"`
	OrphanTotalCount    int `json:"orphanTotalCount"`
	BillableRiskCount   int `json:"billableRiskCount"`
}

type AzureResourceScanResult struct {
	Account             string               `json:"account"`
	Scope               string               `json:"scope"` // "rg" | "subscription"
	TargetResourceGroup string               `json:"targetResourceGroup,omitempty"`
	Items               []AzureResourceItem  `json:"items"`
	Summary             AzureResourceSummary `json:"summary"`
	ScannedAt           time.Time            `json:"scannedAt"`
}

type AzureCleanupRequest struct {
	Mode               string   `json:"mode"` // "orphans" | "selected" | "all_billable"
	Scope              string   `json:"scope,omitempty"`
	ResourceIDs        []string `json:"resourceIds,omitempty"`
	IncludeOrphanIPs   bool     `json:"includeOrphanIps"`
	IncludeOrphanDisks bool     `json:"includeOrphanDisks"`
	IncludeOrphanNICs  bool     `json:"includeOrphanNics"`
	IncludeSnapshots   bool     `json:"includeSnapshots"`
	IncludeNetwork     bool     `json:"includeNetwork"` // for all_billable: whether to also delete vnet/nsg
}

type AzureCleanupFailure struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Error string `json:"error"`
}

type AzureCleanupResult struct {
	Success    bool                  `json:"success"`
	Mode       string                `json:"mode"`
	Deleted    []string              `json:"deleted"`
	DeletedIDs []string              `json:"deletedIds"`
	Failed     []AzureCleanupFailure `json:"failed,omitempty"`
	Logs       []string              `json:"logs"`
}

func (az *AzureService) ScanResources(scope string) (*AzureResourceScanResult, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope != "subscription" {
		if strings.TrimSpace(az.Account.ResourceGroup) != "" {
			scope = "rg"
		} else {
			scope = "subscription"
		}
	}

	rgFilter := ""
	if scope == "rg" {
		rgFilter = az.Account.ResourceGroup
	}

	// Fetch typed resources + generic resources
	vmsRaw, err := az.listArmResources("Microsoft.Compute/virtualMachines", "2023-03-01", rgFilter)
	if err != nil {
		return nil, fmt.Errorf("查询虚拟机列表失败: %w", err)
	}
	nicsRaw, err := az.listArmResources("Microsoft.Network/networkInterfaces", "2023-09-01", rgFilter)
	if err != nil {
		return nil, fmt.Errorf("查询网卡列表失败: %w", err)
	}
	pipsRaw, err := az.listArmResources("Microsoft.Network/publicIPAddresses", "2023-09-01", rgFilter)
	if err != nil {
		return nil, fmt.Errorf("查询公网 IP 列表失败: %w", err)
	}
	disksRaw, err := az.listArmResources("Microsoft.Compute/disks", "2023-10-02", rgFilter)
	if err != nil {
		return nil, fmt.Errorf("查询托管磁盘列表失败: %w", err)
	}
	snapsRaw, err := az.listArmResources("Microsoft.Compute/snapshots", "2023-10-02", rgFilter)
	if err != nil {
		return nil, fmt.Errorf("查询快照列表失败: %w", err)
	}
	allRaw, err := az.listGenericResources(rgFilter)
	if err != nil {
		return nil, fmt.Errorf("查询订阅资源列表失败: %w", err)
	}

	// Build NIC -> VM & PublicIP -> NIC lookup maps
	nicToVM := map[string]string{}
	pipToNIC := map[string]string{}
	for _, nic := range nicsRaw {
		nicID := strings.ToLower(stringVal(nic["id"]))
		nicName := stringVal(nic["name"])
		props, _ := nic["properties"].(map[string]interface{})
		if props != nil {
			if vmRef, ok := props["virtualMachine"].(map[string]interface{}); ok {
				vmName := lastPathSegment(stringVal(vmRef["id"]))
				if vmName != "" {
					nicToVM[nicID] = vmName
				}
			}
			if ipConfigs, ok := props["ipConfigurations"].([]interface{}); ok {
				for _, ic := range ipConfigs {
					icMap, _ := ic.(map[string]interface{})
					icProps, _ := icMap["properties"].(map[string]interface{})
					if icProps == nil {
						continue
					}
					if pipRef, ok := icProps["publicIPAddress"].(map[string]interface{}); ok {
						pipID := strings.ToLower(stringVal(pipRef["id"]))
						if pipID != "" {
							label := nicName
							if vmName := nicToVM[nicID]; vmName != "" {
								label = fmt.Sprintf("%s (VM: %s)", nicName, vmName)
							}
							pipToNIC[pipID] = label
						}
					}
				}
			}
		}
	}

	seenIDs := map[string]bool{}
	items := make([]AzureResourceItem, 0, len(allRaw)+8)

	// 1. VMs
	for _, vm := range vmsRaw {
		id := stringVal(vm["id"])
		rg := extractResourceGroupFromID(id)
		seenIDs[strings.ToLower(id)] = true
		props, _ := vm["properties"].(map[string]interface{})
		vmSize := ""
		if hp, ok := props["hardwareProfile"].(map[string]interface{}); ok {
			vmSize = stringVal(hp["vmSize"])
		}
		risk, note := classifyVMFreeTier(vmSize)
		items = append(items, AzureResourceItem{
			ID:            id,
			Name:          stringVal(vm["name"]),
			ResourceGroup: rg,
			Location:      stringVal(vm["location"]),
			Category:      "vm",
			Type:          stringVal(vm["type"]),
			SKU:           vmSize,
			BillingRisk:   risk,
			BillingNote:   note,
		})
	}

	// 2. Public IPs
	for _, pip := range pipsRaw {
		id := stringVal(pip["id"])
		rg := extractResourceGroupFromID(id)
		seenIDs[strings.ToLower(id)] = true
		skuName := ""
		if sku, ok := pip["sku"].(map[string]interface{}); ok {
			skuName = stringVal(sku["name"])
		}
		props, _ := pip["properties"].(map[string]interface{})
		ipAddr := stringVal(props["ipAddress"])
		ipVersion := stringVal(props["publicIPAddressVersion"])
		if ipVersion == "" {
			ipVersion = "IPv4"
		}
		allocMethod := stringVal(props["publicIPAllocationMethod"])
		ipCfgID := ""
		if ic, ok := props["ipConfiguration"].(map[string]interface{}); ok {
			ipCfgID = stringVal(ic["id"])
		}
		natGwID := ""
		if ng, ok := props["natGateway"].(map[string]interface{}); ok {
			natGwID = stringVal(ng["id"])
		}

		attachedTo := pipToNIC[strings.ToLower(id)]
		if attachedTo == "" && ipCfgID != "" {
			attachedTo = lastPathSegment(ipCfgID)
		}
		if attachedTo == "" && natGwID != "" {
			attachedTo = "NAT Gateway: " + lastPathSegment(natGwID)
		}
		orphan := ipCfgID == "" && natGwID == "" && attachedTo == ""

		risk, note := classifyPublicIPRisk(skuName, allocMethod, ipVersion, orphan)
		items = append(items, AzureResourceItem{
			ID:               id,
			Name:             stringVal(pip["name"]),
			ResourceGroup:    rg,
			Location:         stringVal(pip["location"]),
			Category:         "public_ip",
			Type:             stringVal(pip["type"]),
			SKU:              skuName,
			IPAddress:        ipAddr,
			IPVersion:        ipVersion,
			AllocationMethod: allocMethod,
			AttachedTo:       attachedTo,
			Orphan:           orphan,
			BillingRisk:      risk,
			BillingNote:      note,
		})
	}

	// 3. Disks
	for _, disk := range disksRaw {
		id := stringVal(disk["id"])
		rg := extractResourceGroupFromID(id)
		seenIDs[strings.ToLower(id)] = true
		skuName := ""
		if sku, ok := disk["sku"].(map[string]interface{}); ok {
			skuName = stringVal(sku["name"])
		}
		props, _ := disk["properties"].(map[string]interface{})
		sizeGB := 0
		if sz, ok := props["diskSizeGB"].(float64); ok {
			sizeGB = int(sz)
		}
		diskState := stringVal(props["diskState"])
		managedBy := stringVal(disk["managedBy"])
		attachedTo := lastPathSegment(managedBy)
		orphan := managedBy == "" || strings.EqualFold(diskState, "Unattached")

		risk, note := classifyDiskRisk(skuName, sizeGB, orphan)
		items = append(items, AzureResourceItem{
			ID:            id,
			Name:          stringVal(disk["name"]),
			ResourceGroup: rg,
			Location:      stringVal(disk["location"]),
			Category:      "disk",
			Type:          stringVal(disk["type"]),
			SKU:           skuName,
			SizeGB:        sizeGB,
			AttachedTo:    attachedTo,
			Orphan:        orphan,
			BillingRisk:   risk,
			BillingNote:   note,
		})
	}

	// 4. Snapshots
	for _, snap := range snapsRaw {
		id := stringVal(snap["id"])
		rg := extractResourceGroupFromID(id)
		seenIDs[strings.ToLower(id)] = true
		skuName := ""
		if sku, ok := snap["sku"].(map[string]interface{}); ok {
			skuName = stringVal(sku["name"])
		}
		props, _ := snap["properties"].(map[string]interface{})
		sizeGB := 0
		if sz, ok := props["diskSizeGB"].(float64); ok {
			sizeGB = int(sz)
		}
		items = append(items, AzureResourceItem{
			ID:            id,
			Name:          stringVal(snap["name"]),
			ResourceGroup: rg,
			Location:      stringVal(snap["location"]),
			Category:      "snapshot",
			Type:          stringVal(snap["type"]),
			SKU:           skuName,
			SizeGB:        sizeGB,
			Orphan:        false,
			BillingRisk:   "likely_billable",
			BillingNote:   "磁盘快照通常直接按容量计费",
		})
	}

	// 5. NICs
	for _, nic := range nicsRaw {
		id := stringVal(nic["id"])
		rg := extractResourceGroupFromID(id)
		seenIDs[strings.ToLower(id)] = true
		props, _ := nic["properties"].(map[string]interface{})
		attachedVM := nicToVM[strings.ToLower(id)]
		attachedIP := ""
		if ipConfigs, ok := props["ipConfigurations"].([]interface{}); ok {
			for _, ic := range ipConfigs {
				icMap, _ := ic.(map[string]interface{})
				icProps, _ := icMap["properties"].(map[string]interface{})
				if icProps == nil {
					continue
				}
				if pipRef, ok := icProps["publicIPAddress"].(map[string]interface{}); ok {
					attachedIP = stringVal(pipRef["id"])
					break
				}
			}
		}
		orphan := attachedVM == ""
		risk := "free_network"
		note := "网卡本身免费"
		if attachedVM != "" {
			attachedVM = "VM: " + attachedVM
		}
		if orphan {
			risk = "orphan_billable"
			note = "未绑定 VM 的孤立网卡"
			if attachedIP != "" {
				note = "未绑定 VM 的孤立网卡（仍关联公网 IP：" + lastPathSegment(attachedIP) + "）"
			}
		}
		items = append(items, AzureResourceItem{
			ID:            id,
			Name:          stringVal(nic["name"]),
			ResourceGroup: rg,
			Location:      stringVal(nic["location"]),
			Category:      "nic",
			Type:          stringVal(nic["type"]),
			AttachedTo:    attachedVM,
			AttachedIPID:  attachedIP,
			Orphan:        orphan,
			BillingRisk:   risk,
			BillingNote:   note,
		})
	}

	// 6. Remaining generic resources (VNet, NSG, Storage, LB, etc.), excluding Azure NetworkWatcherRG
	for _, res := range allRaw {
		id := stringVal(res["id"])
		if seenIDs[strings.ToLower(id)] {
			continue
		}
		rg := extractResourceGroupFromID(id)
		resType := stringVal(res["type"])
		if strings.EqualFold(rg, "NetworkWatcherRG") || strings.EqualFold(resType, "Microsoft.Network/networkWatchers") {
			continue
		}
		skuName := ""
		if sku, ok := res["sku"].(map[string]interface{}); ok {
			skuName = stringVal(sku["name"])
		}
		cat, risk, note := classifyGenericResource(resType, skuName)
		items = append(items, AzureResourceItem{
			ID:            id,
			Name:          stringVal(res["name"]),
			ResourceGroup: rg,
			Location:      stringVal(res["location"]),
			Category:      cat,
			Type:          resType,
			SKU:           skuName,
			BillingRisk:   risk,
			BillingNote:   note,
		})
	}

	// Sort items cleanly: orphan_billable first, then likely_billable, free_eligible, free_network
	sort.Slice(items, func(i, j int) bool {
		ri, rj := billingRiskRank(items[i].BillingRisk), billingRiskRank(items[j].BillingRisk)
		if ri != rj {
			return ri < rj
		}
		ci, cj := deletePriority(items[i].Category), deletePriority(items[j].Category)
		if ci != cj {
			return ci < cj
		}
		return items[i].Name < items[j].Name
	})

	var summary AzureResourceSummary
	summary.TotalCount = len(items)
	for _, item := range items {
		switch item.Category {
		case "vm":
			summary.VMCount++
		case "public_ip":
			if item.Orphan {
				summary.OrphanPublicIPCount++
				summary.OrphanTotalCount++
			}
		case "disk":
			if item.Orphan {
				summary.OrphanDiskCount++
				summary.OrphanTotalCount++
			}
		case "nic":
			if item.Orphan {
				summary.OrphanNICCount++
				summary.OrphanTotalCount++
			}
		case "snapshot":
			summary.SnapshotCount++
		}
		if item.BillingRisk == "likely_billable" || item.BillingRisk == "orphan_billable" {
			summary.BillableRiskCount++
		}
	}

	return &AzureResourceScanResult{
		Account:             az.Account.Name,
		Scope:               scope,
		TargetResourceGroup: rgFilter,
		Items:               items,
		Summary:             summary,
		ScannedAt:           time.Now(),
	}, nil
}

func (az *AzureService) CleanupResources(req AzureCleanupRequest) (*AzureCleanupResult, error) {
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "orphans"
	}

	scan, err := az.ScanResources(req.Scope)
	if err != nil {
		return nil, err
	}

	var targets []AzureResourceItem
	switch mode {
	case "selected":
		selectedSet := make(map[string]bool, len(req.ResourceIDs))
		for _, id := range req.ResourceIDs {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				selectedSet[strings.ToLower(trimmed)] = true
			}
		}
		for _, item := range scan.Items {
			if selectedSet[strings.ToLower(item.ID)] {
				targets = append(targets, item)
			}
		}
	case "orphans":
		if !req.IncludeOrphanIPs && !req.IncludeOrphanDisks && !req.IncludeOrphanNICs && !req.IncludeSnapshots {
			req.IncludeOrphanIPs = true
			req.IncludeOrphanDisks = true
			req.IncludeOrphanNICs = true
		}
		for _, item := range scan.Items {
			switch item.Category {
			case "public_ip":
				if req.IncludeOrphanIPs && item.Orphan {
					targets = append(targets, item)
				}
			case "disk":
				if req.IncludeOrphanDisks && item.Orphan {
					targets = append(targets, item)
				}
			case "nic":
				if req.IncludeOrphanNICs && item.Orphan {
					targets = append(targets, item)
				}
			case "snapshot":
				if req.IncludeSnapshots {
					targets = append(targets, item)
				}
			}
		}
	case "all_billable":
		for _, item := range scan.Items {
			if strings.EqualFold(item.ResourceGroup, "NetworkWatcherRG") {
				continue
			}
			if (item.Category == "vnet" || item.Category == "nsg") && !req.IncludeNetwork {
				continue
			}
			targets = append(targets, item)
		}
	default:
		return nil, fmt.Errorf("不支持的清理模式: %s", req.Mode)
	}

	// Sort targets in dependency-safe delete order: VM -> other -> NIC -> PublicIP -> Disk -> Snapshot -> NSG -> VNet
	sort.Slice(targets, func(i, j int) bool {
		pi, pj := deletePriority(targets[i].Category), deletePriority(targets[j].Category)
		if pi != pj {
			return pi < pj
		}
		return targets[i].Name < targets[j].Name
	})

	result := &AzureCleanupResult{
		Success:    true,
		Mode:       mode,
		Deleted:    []string{},
		DeletedIDs: []string{},
		Logs:       []string{},
	}

	if len(targets) == 0 {
		result.Logs = append(result.Logs, "未匹配到需要清理的资源")
		return result, nil
	}

	for _, item := range targets {
		result.Logs = append(result.Logs, fmt.Sprintf("正在删除 [%s] %s (RG: %s)...", item.Category, item.Name, item.ResourceGroup))
		if err := az.DeleteResourceByID(item.ID, item.Type); err != nil {
			result.Success = false
			result.Failed = append(result.Failed, AzureCleanupFailure{
				ID:    item.ID,
				Name:  item.Name,
				Error: err.Error(),
			})
			result.Logs = append(result.Logs, fmt.Sprintf("❌ 删除 %s 失败: %v", item.Name, err))
			continue
		}
		result.Deleted = append(result.Deleted, item.Name)
		result.DeletedIDs = append(result.DeletedIDs, item.ID)
		result.Logs = append(result.Logs, fmt.Sprintf("✅ 已删除 %s", item.Name))
	}

	return result, nil
}

func (az *AzureService) DeallocateAllVMs() ([]string, []string, error) {
	vmsRaw, err := az.listArmResources("Microsoft.Compute/virtualMachines", "2023-03-01", "")
	if err != nil {
		return nil, nil, err
	}
	var stopped []string
	var errs []string
	for _, vm := range vmsRaw {
		id := stringVal(vm["id"])
		name := stringVal(vm["name"])
		if id == "" {
			continue
		}
		cleanID, idErr := validateARMResourceID(id)
		if idErr != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, idErr))
			continue
		}
		url := fmt.Sprintf("https://management.azure.com%s/deallocate?api-version=2023-03-01", cleanID)
		resp, err := az.request("POST", url, nil)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
			errs = append(errs, fmt.Sprintf("%s: HTTP %d %s", name, resp.StatusCode, truncateErrBody(body)))
			continue
		}
		stopped = append(stopped, name)
	}
	return stopped, errs, nil
}

func validateARMResourceID(id string) (string, error) {
	trimmed := strings.TrimSpace(id)
	if !strings.HasPrefix(trimmed, "/subscriptions/") {
		return "", fmt.Errorf("invalid ARM resource id %q", id)
	}
	if strings.ContainsAny(trimmed, "?#\r\n\\") {
		return "", fmt.Errorf("invalid characters in ARM resource id %q", id)
	}
	for _, seg := range strings.Split(trimmed, "/") {
		if seg == ".." || seg == "." {
			return "", fmt.Errorf("invalid path segment in ARM resource id %q", id)
		}
	}
	return trimmed, nil
}

func escapeARMProviderType(providerType string) string {
	parts := strings.Split(strings.Trim(providerType, "/"), "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func (az *AzureService) DeleteResourceByID(resourceID, resourceType string) error {
	if strings.TrimSpace(resourceID) == "" {
		return fmt.Errorf("resource id is empty")
	}
	cleanID, err := validateARMResourceID(resourceID)
	if err != nil {
		return err
	}
	apiVersion := apiVersionForType(resourceType)
	deleteURL := fmt.Sprintf("https://management.azure.com%s?api-version=%s", cleanID, url.QueryEscape(apiVersion))
	resp, err := az.request("DELETE", deleteURL, nil)
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
			return az.waitForAzureOperation(pollURL, 180*time.Second)
		}
		time.Sleep(3 * time.Second)
		return nil
	default:
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateErrBody(body))
	}
}

func (az *AzureService) waitForAzureOperation(pollURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		resp, err := az.requestWithRetry("GET", pollURL, nil, 2)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		statusCode := resp.StatusCode
		resp.Body.Close()

		if statusCode == http.StatusNoContent || statusCode == http.StatusNotFound {
			return nil
		}
		if statusCode == http.StatusOK {
			var statusObj map[string]interface{}
			if json.Unmarshal(body, &statusObj) == nil {
				status := strings.ToLower(stringVal(statusObj["status"]))
				switch status {
				case "succeeded", "":
					return nil
				case "failed", "canceled", "cancelled":
					return fmt.Errorf("异步删除失败: %s", truncateErrBody(body))
				}
			} else {
				return nil
			}
		} else if statusCode != http.StatusAccepted {
			return fmt.Errorf("轮询操作失败 HTTP %d: %s", statusCode, truncateErrBody(body))
		}
	}
	return fmt.Errorf("等待资源删除超时")
}

func (az *AzureService) listArmResources(providerType, apiVersion, rg string) ([]map[string]interface{}, error) {
	var base string
	if strings.TrimSpace(rg) != "" {
		base = fmt.Sprintf("https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/%s?api-version=%s",
			url.PathEscape(az.Account.SubId), url.PathEscape(rg), escapeARMProviderType(providerType), url.QueryEscape(apiVersion))
	} else {
		base = fmt.Sprintf("https://management.azure.com/subscriptions/%s/providers/%s?api-version=%s",
			url.PathEscape(az.Account.SubId), escapeARMProviderType(providerType), url.QueryEscape(apiVersion))
	}
	return az.fetchPagedARMList(base)
}

func (az *AzureService) listGenericResources(rg string) ([]map[string]interface{}, error) {
	var base string
	if strings.TrimSpace(rg) != "" {
		base = fmt.Sprintf("https://management.azure.com/subscriptions/%s/resourceGroups/%s/resources?api-version=2021-04-01",
			url.PathEscape(az.Account.SubId), url.PathEscape(rg))
	} else {
		base = fmt.Sprintf("https://management.azure.com/subscriptions/%s/resources?api-version=2021-04-01",
			url.PathEscape(az.Account.SubId))
	}
	return az.fetchPagedARMList(base)
}

func (az *AzureService) fetchPagedARMList(nextURL string) ([]map[string]interface{}, error) {
	var all []map[string]interface{}
	for page := 0; nextURL != "" && page < 15; page++ {
		resp, err := az.requestWithRetry("GET", nextURL, nil, 3)
		if err != nil {
			return nil, err
		}
		raw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateErrBody(raw))
		}
		var parsed struct {
			Value    []map[string]interface{} `json:"value"`
			NextLink string                   `json:"nextLink"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, err
		}
		all = append(all, parsed.Value...)
		nextURL = strings.TrimSpace(parsed.NextLink)
	}
	return all, nil
}

func classifyVMFreeTier(vmSize string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(vmSize)) {
	case "standard_b1s", "standard_b2ats_v2", "standard_b2pts_v2":
		return "free_eligible", fmt.Sprintf("%s (750小时/月免费额度机型)", vmSize)
	case "":
		return "unknown", "未识别机型"
	default:
		return "likely_billable", fmt.Sprintf("%s (非免费规格，按小时计费)", vmSize)
	}
}

func classifyPublicIPRisk(sku, allocMethod, ipVersion string, orphan bool) (string, string) {
	if strings.EqualFold(ipVersion, "IPv6") {
		if orphan {
			return "free_network", "未绑定公网 IPv6（Azure 公网 IPv6 本身免费，可按需清理）"
		}
		return "free_network", "公网 IPv6 地址免费（仅出站流量按规则计费）"
	}
	if orphan {
		if strings.EqualFold(sku, "Standard") {
			return "orphan_billable", "未绑定公网 IPv4 (Standard SKU)，持续按小时扣费，建议清理"
		}
		return "orphan_billable", "未绑定公网 IPv4，建议清理"
	}
	if strings.EqualFold(sku, "Standard") {
		return "likely_billable", "Standard 静态公网 IPv4（学生订阅通常约 $3.6/月）"
	}
	if strings.EqualFold(allocMethod, "Static") {
		return "likely_billable", "静态公网 IPv4（请核对是否计费）"
	}
	return "free_eligible", "Basic 动态公网 IPv4（通常在免费额度内）"
}

func classifyDiskRisk(sku string, sizeGB int, orphan bool) (string, string) {
	if orphan {
		return "orphan_billable", fmt.Sprintf("未挂载孤立磁盘 (%dGB %s)，仍会产生存储费", sizeGB, sku)
	}
	if sizeGB > 0 && sizeGB <= 64 {
		return "free_eligible", fmt.Sprintf("%dGB %s（≤64GB P6/标准额度内通常免费）", sizeGB, sku)
	}
	return "likely_billable", fmt.Sprintf("%dGB %s（大于 64GB，按容量持续扣费）", sizeGB, sku)
}

func classifyGenericResource(resType, sku string) (string, string, string) {
	lower := strings.ToLower(resType)
	switch {
	case strings.Contains(lower, "virtualnetworks"):
		return "vnet", "free_network", "虚拟网络框架免费"
	case strings.Contains(lower, "networksecuritygroups"):
		return "nsg", "free_network", "网络安全组免费"
	case strings.Contains(lower, "loadbalancers"):
		if strings.EqualFold(sku, "Basic") {
			return "other", "free_eligible", "Basic 负载均衡器"
		}
		return "other", "likely_billable", "负载均衡器（可能产生小时及规则费用）"
	case strings.Contains(lower, "storageaccounts"):
		return "other", "likely_billable", "存储账户（按实际容量与请求计费）"
	case strings.Contains(lower, "natgateways") || strings.Contains(lower, "bastionhosts") || strings.Contains(lower, "publicipprefixes"):
		return "other", "likely_billable", "高计费风险网络资源"
	default:
		return "other", "likely_billable", "其他资源，建议核对账单明细"
	}
}

func apiVersionForType(resType string) string {
	switch strings.ToLower(strings.TrimSpace(resType)) {
	case "microsoft.compute/virtualmachines":
		return "2023-03-01"
	case "microsoft.compute/disks", "microsoft.compute/snapshots":
		return "2023-10-02"
	case "microsoft.network/networkinterfaces",
		"microsoft.network/publicipaddresses",
		"microsoft.network/virtualnetworks",
		"microsoft.network/networksecuritygroups",
		"microsoft.network/loadbalancers",
		"microsoft.network/natgateways":
		return "2023-09-01"
	case "microsoft.storage/storageaccounts":
		return "2023-01-01"
	default:
		return "2021-04-01"
	}
}

func deletePriority(cat string) int {
	switch cat {
	case "vm":
		return 1
	case "other":
		return 2
	case "nic":
		return 3
	case "public_ip":
		return 4
	case "disk":
		return 5
	case "snapshot":
		return 6
	case "nsg":
		return 7
	case "vnet":
		return 8
	default:
		return 9
	}
}

func billingRiskRank(risk string) int {
	switch risk {
	case "orphan_billable":
		return 1
	case "likely_billable":
		return 2
	case "free_eligible":
		return 3
	case "free_network":
		return 4
	default:
		return 5
	}
}

func stringVal(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func lastPathSegment(id string) string {
	parts := splitPath(id)
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}
