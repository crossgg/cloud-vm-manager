package oci

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

type OCIResourceItem struct {
	ID                 string `json:"id"`       // "<category>|<ocid>"
	OCID               string `json:"ocid"`
	Name               string `json:"name"`
	Category           string `json:"category"` // boot_volume | block_volume | reserved_ip | boot_backup | volume_backup | custom_image
	AvailabilityDomain string `json:"availabilityDomain,omitempty"`
	SizeGB             int64  `json:"sizeGb,omitempty"`
	IPAddress          string `json:"ipAddress,omitempty"`
	Status             string `json:"status,omitempty"`
	AttachedTo         string `json:"attachedTo,omitempty"`
	Idle               bool   `json:"idle"`
	BillingNote        string `json:"billingNote"`
}

type OCIResourceSummary struct {
	TotalCount       int   `json:"totalCount"`
	IdleVolumeCount  int   `json:"idleVolumeCount"`
	IdleVolumeGB     int64 `json:"idleVolumeGb"`
	TotalVolumeGB    int64 `json:"totalVolumeGb"`
	IdleReservedIPs  int   `json:"idleReservedIps"`
	BackupCount      int   `json:"backupCount"`
	CustomImageCount int   `json:"customImageCount"`
	IdleTotalCount   int   `json:"idleTotalCount"`
}

type OCIResourceScanResult struct {
	Account       string             `json:"account"`
	CompartmentID string             `json:"compartmentId"`
	Items         []OCIResourceItem  `json:"items"`
	Summary       OCIResourceSummary `json:"summary"`
}

func (o *OCIService) ScanIdleResources() (*OCIResourceScanResult, error) {
	compartmentID, err := o.compartmentIDOrTenancy()
	if err != nil {
		return nil, err
	}

	items := make([]OCIResourceItem, 0)
	var summary OCIResourceSummary

	// 1. Boot Volumes (reuse existing ListAccountBootVolumes logic)
	if bvOverview, err := o.ListAccountBootVolumes(); err == nil && bvOverview != nil {
		for _, bv := range bvOverview.BootVolumes {
			state := strings.ToUpper(strings.TrimSpace(bv.LifecycleState))
			if state == "TERMINATED" || state == "TERMINATING" {
				continue
			}
			idle := !bv.Attached && state == "AVAILABLE"
			summary.TotalVolumeGB += bv.SizeInGBs
			note := "已挂载使用中"
			if bv.Attached {
				note = "已挂载到实例（使用中，不可直接删除）"
			} else if idle {
				note = fmt.Sprintf("未挂载孤立启动盘（占用 %d GB / 200 GB 免费块存储配额）", bv.SizeInGBs)
				summary.IdleVolumeCount++
				summary.IdleVolumeGB += bv.SizeInGBs
				summary.IdleTotalCount++
			} else {
				note = "当前状态不可删除: " + bv.LifecycleState
			}
			items = append(items, OCIResourceItem{
				ID:                 "boot_volume|" + bv.ID,
				OCID:               bv.ID,
				Name:               valueOrDefault(bv.DisplayName, bv.ID),
				Category:           "boot_volume",
				AvailabilityDomain: bv.AvailabilityDomain,
				SizeGB:             bv.SizeInGBs,
				Status:             bv.LifecycleState,
				AttachedTo:         bv.InstanceName,
				Idle:               idle,
				BillingNote:        note,
			})
		}
	}

	// 2. Block Volumes (数据卷)
	if ads, err := o.listAvailabilityDomainsForCompartment(compartmentID); err == nil {
		var rawInstances []map[string]interface{}
		_ = o.requestJSON("GET", "/instances", url.Values{"compartmentId": {compartmentID}}, nil, &rawInstances, false)
		instNames := make(map[string]string, len(rawInstances))
		for _, inst := range rawInstances {
			instNames[stringValue(inst["id"])] = valueOrDefault(stringValue(inst["displayName"]), stringValue(inst["id"]))
		}

		attachedVolToInst := make(map[string]string)
		seenVols := make(map[string]struct{})
		for _, ad := range ads {
			var atts []map[string]interface{}
			if err := o.requestJSON("GET", "/volumeAttachments", url.Values{
				"availabilityDomain": {ad},
				"compartmentId":      {compartmentID},
			}, nil, &atts, false); err == nil {
				for _, att := range atts {
					if !isActiveBootAttachmentState(stringValue(att["lifecycleState"])) {
						continue
					}
					volID := stringValue(att["volumeId"])
					instID := stringValue(att["instanceId"])
					if volID != "" {
						attachedVolToInst[volID] = valueOrDefault(instNames[instID], instID)
					}
				}
			}

			var vols []map[string]interface{}
			if err := o.requestJSON("GET", "/volumes", url.Values{
				"availabilityDomain": {ad},
				"compartmentId":      {compartmentID},
			}, nil, &vols, false); err == nil {
				for _, vol := range vols {
					volID := stringValue(vol["id"])
					if volID == "" {
						continue
					}
					if _, dup := seenVols[volID]; dup {
						continue
					}
					seenVols[volID] = struct{}{}
					state := strings.ToUpper(stringValue(vol["lifecycleState"]))
					if state == "TERMINATED" || state == "TERMINATING" {
						continue
					}
					sizeGB := int64Value(vol["sizeInGBs"])
					summary.TotalVolumeGB += sizeGB
					attachedTo := attachedVolToInst[volID]
					attached := attachedTo != ""
					idle := !attached && state == "AVAILABLE"
					note := "数据卷挂载使用中"
					if idle {
						note = fmt.Sprintf("未挂载孤立数据卷（占用 %d GB / 200 GB 免费块存储配额）", sizeGB)
						summary.IdleVolumeCount++
						summary.IdleVolumeGB += sizeGB
						summary.IdleTotalCount++
					}
					items = append(items, OCIResourceItem{
						ID:                 "block_volume|" + volID,
						OCID:               volID,
						Name:               valueOrDefault(stringValue(vol["displayName"]), volID),
						Category:           "block_volume",
						AvailabilityDomain: valueOrDefault(stringValue(vol["availabilityDomain"]), ad),
						SizeGB:             sizeGB,
						Status:             state,
						AttachedTo:         attachedTo,
						Idle:               idle,
						BillingNote:        note,
					})
				}
			}
		}
	}

	// 3. Reserved Public IPs
	var rawPubIPs []map[string]interface{}
	if err := o.requestJSON("GET", "/publicIps", url.Values{
		"scope":         {"REGION"},
		"compartmentId": {compartmentID},
		"lifetime":      {"RESERVED"},
	}, nil, &rawPubIPs, false); err == nil {
		for _, p := range rawPubIPs {
			pubID := stringValue(p["id"])
			if pubID == "" {
				continue
			}
			state := strings.ToUpper(stringValue(p["lifecycleState"]))
			if state == "TERMINATED" || state == "TERMINATING" {
				continue
			}
			privID := strings.TrimSpace(stringValue(p["privateIpId"]))
			idle := privID == ""
			note := "已绑定私网 IP（使用中）"
			attached := ""
			if privID != "" {
				attached = "PrivateIP: " + privID
			}
			if idle {
				note = "未绑定闲置保留公网 IP（占用保留 IP 配额，可释放）"
				summary.IdleReservedIPs++
				summary.IdleTotalCount++
			}
			items = append(items, OCIResourceItem{
				ID:          "reserved_ip|" + pubID,
				OCID:        pubID,
				Name:        valueOrDefault(stringValue(p["displayName"]), stringValue(p["ipAddress"])),
				Category:    "reserved_ip",
				IPAddress:   stringValue(p["ipAddress"]),
				Status:      valueOrDefault(state, "AVAILABLE"),
				AttachedTo:  attached,
				Idle:        idle,
				BillingNote: note,
			})
		}
	}

	// 4. Boot Volume Backups & Volume Backups
	var bvBackups []map[string]interface{}
	if err := o.requestJSON("GET", "/bootVolumeBackups", url.Values{
		"compartmentId": {compartmentID},
	}, nil, &bvBackups, false); err == nil {
		for _, b := range bvBackups {
			id := stringValue(b["id"])
			state := strings.ToUpper(stringValue(b["lifecycleState"]))
			if id == "" || state == "TERMINATED" || state == "TERMINATING" {
				continue
			}
			sizeGB := int64Value(b["uniqueSizeInGBs"])
			if sizeGB == 0 {
				sizeGB = int64Value(b["sizeInGBs"])
			}
			idle := state == "AVAILABLE"
			summary.BackupCount++
			if idle {
				summary.IdleTotalCount++
			}
			items = append(items, OCIResourceItem{
				ID:          "boot_backup|" + id,
				OCID:        id,
				Name:        valueOrDefault(stringValue(b["displayName"]), id),
				Category:    "boot_backup",
				SizeGB:      sizeGB,
				Status:      state,
				Idle:        idle,
				BillingNote: "引导卷备份（占用存储空间，确认无需恢复后可清理）",
			})
		}
	}

	var volBackups []map[string]interface{}
	if err := o.requestJSON("GET", "/volumeBackups", url.Values{
		"compartmentId": {compartmentID},
	}, nil, &volBackups, false); err == nil {
		for _, b := range volBackups {
			id := stringValue(b["id"])
			state := strings.ToUpper(stringValue(b["lifecycleState"]))
			if id == "" || state == "TERMINATED" || state == "TERMINATING" {
				continue
			}
			sizeGB := int64Value(b["uniqueSizeInGBs"])
			if sizeGB == 0 {
				sizeGB = int64Value(b["sizeInGBs"])
			}
			idle := state == "AVAILABLE"
			summary.BackupCount++
			if idle {
				summary.IdleTotalCount++
			}
			items = append(items, OCIResourceItem{
				ID:          "volume_backup|" + id,
				OCID:        id,
				Name:        valueOrDefault(stringValue(b["displayName"]), id),
				Category:    "volume_backup",
				SizeGB:      sizeGB,
				Status:      state,
				Idle:        idle,
				BillingNote: "数据卷备份（占用存储空间，确认无需恢复后可清理）",
			})
		}
	}

	// 5. Custom Images (only images owned by this compartment, excluding Oracle official platform images)
	var rawImages []map[string]interface{}
	if err := o.requestJSON("GET", "/images", url.Values{
		"compartmentId": {compartmentID},
	}, nil, &rawImages, false); err == nil {
		for _, img := range rawImages {
			id := stringValue(img["id"])
			ownerComp := strings.TrimSpace(stringValue(img["compartmentId"]))
			state := strings.ToUpper(stringValue(img["lifecycleState"]))
			if id == "" || ownerComp == "" || !strings.EqualFold(ownerComp, compartmentID) {
				continue
			}
			if state == "DELETED" || state == "DISABLED" {
				continue
			}
			sizeMB := int64Value(img["sizeInMBs"])
			sizeGB := sizeMB / 1024
			if sizeGB == 0 && sizeMB > 0 {
				sizeGB = 1
			}
			idle := state == "AVAILABLE"
			summary.CustomImageCount++
			if idle {
				summary.IdleTotalCount++
			}
			items = append(items, OCIResourceItem{
				ID:          "custom_image|" + id,
				OCID:        id,
				Name:        valueOrDefault(stringValue(img["displayName"]), id),
				Category:    "custom_image",
				SizeGB:      sizeGB,
				Status:      state,
				Idle:        idle,
				BillingNote: "自定义镜像（占用自定义镜像配额与空间）",
			})
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Idle != items[j].Idle {
			return items[i].Idle
		}
		if items[i].Category != items[j].Category {
			return items[i].Category < items[j].Category
		}
		return items[i].Name < items[j].Name
	})

	summary.TotalCount = len(items)
	return &OCIResourceScanResult{
		Account:       o.Account.Name,
		CompartmentID: compartmentID,
		Items:         items,
		Summary:       summary,
	}, nil
}

func (o *OCIService) CleanupIdleResources(resourceIDs []string) ([]string, []string) {
	deleted := make([]string, 0)
	errs := make([]string, 0)

	for _, rawID := range resourceIDs {
		cat, ocid, ok := strings.Cut(strings.TrimSpace(rawID), "|")
		if !ok || cat == "" || ocid == "" {
			errs = append(errs, fmt.Sprintf("无效的 OCI 资源标识: %s", rawID))
			continue
		}

		var err error
		switch cat {
		case "boot_volume":
			err = o.DeleteBootVolume(ocid)
		case "block_volume":
			err = o.deleteBlockVolumeSafely(ocid)
		case "reserved_ip":
			err = o.deleteReservedIPSafely(ocid)
		case "boot_backup":
			err = o.requestJSON("DELETE", "/bootVolumeBackups/"+url.PathEscape(ocid), nil, nil, nil, false)
		case "volume_backup":
			err = o.requestJSON("DELETE", "/volumeBackups/"+url.PathEscape(ocid), nil, nil, nil, false)
		case "custom_image":
			err = o.requestJSON("DELETE", "/images/"+url.PathEscape(ocid), nil, nil, nil, false)
		default:
			err = fmt.Errorf("不支持的 OCI 资源类型: %s", cat)
		}

		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", rawID, err))
			continue
		}
		deleted = append(deleted, rawID)
	}
	return deleted, errs
}

func (o *OCIService) deleteBlockVolumeSafely(volID string) error {
	var vol map[string]interface{}
	if err := o.requestJSON("GET", "/volumes/"+url.PathEscape(volID), nil, nil, &vol, false); err != nil {
		return err
	}
	compID := valueOrDefault(stringValue(vol["compartmentId"]), o.Account.CompartmentID)
	ad := stringValue(vol["availabilityDomain"])
	if compID != "" && ad != "" {
		var atts []map[string]interface{}
		if err := o.requestJSON("GET", "/volumeAttachments", url.Values{
			"availabilityDomain": {ad},
			"compartmentId":      {compID},
			"volumeId":           {volID},
		}, nil, &atts, false); err == nil {
			for _, att := range atts {
				if isActiveBootAttachmentState(stringValue(att["lifecycleState"])) {
					return fmt.Errorf("数据卷仍挂载在实例 (%s) 上，禁止删除", stringValue(att["instanceId"]))
				}
			}
		}
	}
	return o.requestJSON("DELETE", "/volumes/"+url.PathEscape(volID), nil, nil, nil, false)
}

func (o *OCIService) deleteReservedIPSafely(pubID string) error {
	var pub map[string]interface{}
	if err := o.requestJSON("GET", "/publicIps/"+url.PathEscape(pubID), nil, nil, &pub, false); err != nil {
		return err
	}
	if privID := strings.TrimSpace(stringValue(pub["privateIpId"])); privID != "" {
		return fmt.Errorf("保留公网 IP (%s) 正在绑定使用中，请先在「IP 管理」中解绑", stringValue(pub["ipAddress"]))
	}
	return o.deletePublicIP(pubID)
}
