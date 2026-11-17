package oci

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

type OCIBootVolumeInfo struct {
	ID                 string `json:"id"`
	DisplayName        string `json:"displayName"`
	SizeInGBs          int64  `json:"sizeInGBs"`
	VpusPerGB          int64  `json:"vpusPerGB"`
	LifecycleState     string `json:"lifecycleState"`
	AvailabilityDomain string `json:"availabilityDomain"`
	CompartmentID      string `json:"compartmentId"`
	IsAutoTuneEnabled  bool   `json:"isAutoTuneEnabled,omitempty"`
	TimeCreated        string `json:"timeCreated,omitempty"`
	Attached           bool   `json:"attached"`
	AttachmentID       string `json:"attachmentId,omitempty"`
	AttachmentState    string `json:"attachmentState,omitempty"`
	InstanceID         string `json:"instanceId,omitempty"`
	InstanceName       string `json:"instanceName,omitempty"`
	InstanceState      string `json:"instanceState,omitempty"`
}

type OCIBootVolumeAttachableInstance struct {
	ID                 string `json:"id"`
	DisplayName        string `json:"displayName"`
	LifecycleState     string `json:"lifecycleState"`
	AvailabilityDomain string `json:"availabilityDomain"`
	HasBootVolume      bool   `json:"hasBootVolume"`
	CanAttach          bool   `json:"canAttach"`
}

type OCIBootVolumesOverview struct {
	BootVolumes []OCIBootVolumeInfo               `json:"bootVolumes"`
	Instances   []OCIBootVolumeAttachableInstance `json:"instances"`
}

type OCIInstanceBootVolumeResponse struct {
	InstanceID         string                            `json:"instanceId"`
	InstanceName       string                            `json:"instanceName"`
	InstanceState      string                            `json:"instanceState"`
	AvailabilityDomain string                            `json:"availabilityDomain"`
	BootVolume         *OCIBootVolumeInfo                `json:"bootVolume,omitempty"`
	AttachableVolumes  []OCIBootVolumeInfo               `json:"attachableVolumes,omitempty"`
	Instances          []OCIBootVolumeAttachableInstance `json:"instances,omitempty"`
}

func int64Value(v interface{}) int64 {
	return int64(float64InterfaceValue(v))
}

func isActiveBootAttachmentState(state string) bool {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "ATTACHED", "ATTACHING", "DETACHING":
		return true
	default:
		return false
	}
}

func (o *OCIService) listAvailabilityDomainsForCompartment(compartmentID string) ([]string, error) {
	var rawADs []map[string]interface{}
	if err := o.identityRequestJSON(o.Account.Region, "GET", "/availabilityDomains", url.Values{
		"compartmentId": {compartmentID},
	}, nil, &rawADs, false); err != nil {
		return nil, fmt.Errorf("获取可用域失败: %w", err)
	}
	ads := make([]string, 0, len(rawADs))
	for _, ad := range rawADs {
		if name := strings.TrimSpace(stringValue(ad["name"])); name != "" {
			ads = append(ads, name)
		}
	}
	return ads, nil
}

func (o *OCIService) ListAccountBootVolumes() (*OCIBootVolumesOverview, error) {
	compartmentID, err := o.compartmentIDOrTenancy()
	if err != nil {
		return nil, err
	}

	ads, err := o.listAvailabilityDomainsForCompartment(compartmentID)
	if err != nil {
		return nil, err
	}

	var rawInstances []map[string]interface{}
	_ = o.requestJSON("GET", "/instances", url.Values{
		"compartmentId": {compartmentID},
	}, nil, &rawInstances, false)

	instanceByID := make(map[string]map[string]interface{}, len(rawInstances))
	for _, inst := range rawInstances {
		id := stringValue(inst["id"])
		state := strings.ToUpper(stringValue(inst["lifecycleState"]))
		if id == "" || state == "TERMINATED" {
			continue
		}
		instanceByID[id] = inst
	}

	attachedByBV := make(map[string]map[string]interface{})
	attachedByInstance := make(map[string]map[string]interface{})
	rawVolumes := make([]map[string]interface{}, 0)
	seenVolumeIDs := make(map[string]struct{})

	for _, ad := range ads {
		var adAtts []map[string]interface{}
		if err := o.requestJSON("GET", "/bootVolumeAttachments", url.Values{
			"availabilityDomain": {ad},
			"compartmentId":      {compartmentID},
		}, nil, &adAtts, false); err == nil {
			for _, att := range adAtts {
				if !isActiveBootAttachmentState(stringValue(att["lifecycleState"])) {
					continue
				}
				bvID := stringValue(att["bootVolumeId"])
				instID := stringValue(att["instanceId"])
				if bvID != "" {
					attachedByBV[bvID] = att
				}
				if instID != "" {
					attachedByInstance[instID] = att
				}
			}
		}

		var adVols []map[string]interface{}
		if err := o.requestJSON("GET", "/bootVolumes", url.Values{
			"availabilityDomain": {ad},
			"compartmentId":      {compartmentID},
		}, nil, &adVols, false); err != nil {
			return nil, fmt.Errorf("获取可用域 %s 启动盘失败: %w", ad, err)
		}
		for _, vol := range adVols {
			id := stringValue(vol["id"])
			state := strings.ToUpper(stringValue(vol["lifecycleState"]))
			if id == "" || state == "TERMINATED" {
				continue
			}
			if _, exists := seenVolumeIDs[id]; exists {
				continue
			}
			seenVolumeIDs[id] = struct{}{}
			rawVolumes = append(rawVolumes, vol)
		}
	}

	volumes := make([]OCIBootVolumeInfo, 0, len(rawVolumes))
	for _, raw := range rawVolumes {
		volumes = append(volumes, buildBootVolumeInfo(raw, attachedByBV[stringValue(raw["id"])], instanceByID))
	}
	sort.SliceStable(volumes, func(i, j int) bool {
		if volumes[i].Attached != volumes[j].Attached {
			return volumes[i].Attached
		}
		return volumes[i].DisplayName < volumes[j].DisplayName
	})

	instances := make([]OCIBootVolumeAttachableInstance, 0, len(instanceByID))
	for _, inst := range instanceByID {
		id := stringValue(inst["id"])
		state := strings.ToUpper(stringValue(inst["lifecycleState"]))
		_, hasBV := attachedByInstance[id]
		instances = append(instances, OCIBootVolumeAttachableInstance{
			ID:                 id,
			DisplayName:        valueOrDefault(stringValue(inst["displayName"]), id),
			LifecycleState:     state,
			AvailabilityDomain: stringValue(inst["availabilityDomain"]),
			HasBootVolume:      hasBV,
			CanAttach:          state == "STOPPED" && !hasBV,
		})
	}
	sort.SliceStable(instances, func(i, j int) bool {
		return instances[i].DisplayName < instances[j].DisplayName
	})

	return &OCIBootVolumesOverview{
		BootVolumes: volumes,
		Instances:   instances,
	}, nil
}

func buildBootVolumeInfo(raw map[string]interface{}, att map[string]interface{}, instanceByID map[string]map[string]interface{}) OCIBootVolumeInfo {
	id := stringValue(raw["id"])
	vpus := int64Value(raw["vpusPerGB"])
	if vpus == 0 {
		vpus = 10
	}
	autoTune, _ := raw["isAutoTuneEnabled"].(bool)
	info := OCIBootVolumeInfo{
		ID:                 id,
		DisplayName:        valueOrDefault(stringValue(raw["displayName"]), id),
		SizeInGBs:          int64Value(raw["sizeInGBs"]),
		VpusPerGB:          vpus,
		LifecycleState:     strings.ToUpper(stringValue(raw["lifecycleState"])),
		AvailabilityDomain: stringValue(raw["availabilityDomain"]),
		CompartmentID:      stringValue(raw["compartmentId"]),
		IsAutoTuneEnabled:  autoTune,
		TimeCreated:        stringValue(raw["timeCreated"]),
	}
	if att != nil {
		attState := strings.ToUpper(stringValue(att["lifecycleState"]))
		instID := stringValue(att["instanceId"])
		info.Attached = isActiveBootAttachmentState(attState)
		info.AttachmentID = stringValue(att["id"])
		info.AttachmentState = attState
		info.InstanceID = instID
		if inst, ok := instanceByID[instID]; ok {
			info.InstanceName = valueOrDefault(stringValue(inst["displayName"]), instID)
			info.InstanceState = strings.ToUpper(stringValue(inst["lifecycleState"]))
		} else if instID != "" {
			info.InstanceName = instID
		}
	}
	return info
}

func (o *OCIService) GetInstanceBootVolume(instanceID string) (*OCIInstanceBootVolumeResponse, error) {
	rawInst, err := o.rawInstance(instanceID)
	if err != nil {
		return nil, err
	}
	instState := strings.ToUpper(stringValue(rawInst["lifecycleState"]))
	instName := valueOrDefault(stringValue(rawInst["displayName"]), instanceID)
	ad := stringValue(rawInst["availabilityDomain"])

	overview, err := o.ListAccountBootVolumes()
	if err != nil {
		return nil, err
	}

	resp := &OCIInstanceBootVolumeResponse{
		InstanceID:         instanceID,
		InstanceName:       instName,
		InstanceState:      instState,
		AvailabilityDomain: ad,
		Instances:          make([]OCIBootVolumeAttachableInstance, 0),
		AttachableVolumes:  make([]OCIBootVolumeInfo, 0),
	}

	for i := range overview.BootVolumes {
		bv := overview.BootVolumes[i]
		if bv.Attached && bv.InstanceID == instanceID {
			bvCopy := bv
			resp.BootVolume = &bvCopy
		} else if !bv.Attached && bv.LifecycleState == "AVAILABLE" && (ad == "" || bv.AvailabilityDomain == ad) {
			resp.AttachableVolumes = append(resp.AttachableVolumes, bv)
		}
	}
	for _, inst := range overview.Instances {
		if ad == "" || inst.AvailabilityDomain == ad {
			resp.Instances = append(resp.Instances, inst)
		}
	}
	return resp, nil
}

func (o *OCIService) UpdateBootVolume(bootVolumeID string, sizeInGBs, vpusPerGB *int64) (*OCIBootVolumeInfo, error) {
	bootVolumeID = strings.TrimSpace(bootVolumeID)
	if bootVolumeID == "" {
		return nil, fmt.Errorf("missing OCI boot volume id")
	}
	if sizeInGBs == nil && vpusPerGB == nil {
		return nil, fmt.Errorf("请至少指定新的容量 (SizeInGBs) 或性能单元 (VPU)")
	}

	var current map[string]interface{}
	if err := o.requestJSON("GET", "/bootVolumes/"+url.PathEscape(bootVolumeID), nil, nil, &current, false); err != nil {
		return nil, err
	}
	currentSize := int64Value(current["sizeInGBs"])

	body := map[string]interface{}{
		"definedTags":  valueOrEmptyMap(current["definedTags"]),
		"freeformTags": valueOrEmptyMap(current["freeformTags"]),
	}
	if name := stringValue(current["displayName"]); name != "" {
		body["displayName"] = name
	}

	if sizeInGBs != nil {
		if *sizeInGBs <= currentSize {
			return nil, fmt.Errorf("OCI 启动盘仅支持扩容：新大小 (%d GB) 必须大于当前大小 (%d GB)", *sizeInGBs, currentSize)
		}
		body["sizeInGBs"] = *sizeInGBs
	}

	if vpusPerGB != nil {
		vpu := *vpusPerGB
		if vpu < 10 || vpu > 120 || vpu%10 != 0 {
			return nil, fmt.Errorf("VPU 值无效 (%d)，OCI 支持 10、20 或 30-120 之间 10 的倍数", vpu)
		}
		body["vpusPerGB"] = vpu
	}

	var updated map[string]interface{}
	if err := o.requestJSON("PUT", "/bootVolumes/"+url.PathEscape(bootVolumeID), nil, body, &updated, false); err != nil {
		return nil, fmt.Errorf("更新启动盘失败: %w", err)
	}
	info := buildBootVolumeInfo(updated, nil, nil)
	return &info, nil
}

func (o *OCIService) DetachBootVolume(attachmentID string) error {
	attachmentID = strings.TrimSpace(attachmentID)
	if attachmentID == "" {
		return fmt.Errorf("missing boot volume attachment id")
	}

	var att map[string]interface{}
	if err := o.requestJSON("GET", "/bootVolumeAttachments/"+url.PathEscape(attachmentID), nil, nil, &att, false); err != nil {
		return err
	}
	attState := strings.ToUpper(stringValue(att["lifecycleState"]))
	if attState == "DETACHED" {
		return nil
	}
	if instID := stringValue(att["instanceId"]); instID != "" {
		if inst, err := o.rawInstance(instID); err == nil {
			instState := strings.ToUpper(stringValue(inst["lifecycleState"]))
			if instState != "STOPPED" && instState != "TERMINATED" {
				return fmt.Errorf("卸载启动盘前，实例必须处于“已停止 (STOPPED)”状态，当前状态为 %s", instState)
			}
		}
	}

	if err := o.requestJSON("DELETE", "/bootVolumeAttachments/"+url.PathEscape(attachmentID), nil, nil, nil, false); err != nil {
		return fmt.Errorf("卸载启动盘失败: %w", err)
	}

	for attempt := 0; attempt < 15; attempt++ {
		var cur map[string]interface{}
		err := o.requestJSON("GET", "/bootVolumeAttachments/"+url.PathEscape(attachmentID), nil, nil, &cur, true)
		if err != nil {
			return err
		}
		if cur == nil || stringValue(cur["id"]) == "" || strings.ToUpper(stringValue(cur["lifecycleState"])) == "DETACHED" {
			return nil
		}
		time.Sleep(ociIPPollInterval)
	}
	return nil
}

func (o *OCIService) AttachBootVolume(bootVolumeID, instanceID string) (map[string]interface{}, error) {
	bootVolumeID = strings.TrimSpace(bootVolumeID)
	instanceID = strings.TrimSpace(instanceID)
	if bootVolumeID == "" || instanceID == "" {
		return nil, fmt.Errorf("请指定启动盘 ID 与目标实例 ID")
	}

	var bv map[string]interface{}
	if err := o.requestJSON("GET", "/bootVolumes/"+url.PathEscape(bootVolumeID), nil, nil, &bv, false); err != nil {
		return nil, err
	}
	bvState := strings.ToUpper(stringValue(bv["lifecycleState"]))
	if bvState != "AVAILABLE" {
		return nil, fmt.Errorf("当前启动盘状态为 %s，仅 AVAILABLE 状态支持附加", bvState)
	}

	inst, err := o.rawInstance(instanceID)
	if err != nil {
		return nil, err
	}
	instState := strings.ToUpper(stringValue(inst["lifecycleState"]))
	if instState != "STOPPED" {
		return nil, fmt.Errorf("附加启动盘前，目标实例必须处于“已停止 (STOPPED)”状态，当前状态为 %s", instState)
	}

	bvAD := stringValue(bv["availabilityDomain"])
	instAD := stringValue(inst["availabilityDomain"])
	if bvAD != "" && instAD != "" && bvAD != instAD {
		return nil, fmt.Errorf("启动盘可用域 (%s) 与目标实例可用域 (%s) 不一致，无法跨可用域附加", bvAD, instAD)
	}

	var created map[string]interface{}
	if err := o.requestJSON("POST", "/bootVolumeAttachments", nil, map[string]interface{}{
		"bootVolumeId": bootVolumeID,
		"instanceId":   instanceID,
	}, &created, false); err != nil {
		return nil, fmt.Errorf("附加启动盘失败: %w", err)
	}

	attID := stringValue(created["id"])
	for attempt := 0; attempt < 10 && attID != ""; attempt++ {
		if strings.ToUpper(stringValue(created["lifecycleState"])) == "ATTACHED" {
			break
		}
		time.Sleep(ociIPPollInterval)
		var cur map[string]interface{}
		if err := o.requestJSON("GET", "/bootVolumeAttachments/"+url.PathEscape(attID), nil, nil, &cur, false); err == nil && cur != nil {
			created = cur
		}
	}
	return created, nil
}

func (o *OCIService) DeleteBootVolume(bootVolumeID string) error {
	bootVolumeID = strings.TrimSpace(bootVolumeID)
	if bootVolumeID == "" {
		return fmt.Errorf("missing OCI boot volume id")
	}

	var bv map[string]interface{}
	if err := o.requestJSON("GET", "/bootVolumes/"+url.PathEscape(bootVolumeID), nil, nil, &bv, false); err != nil {
		return err
	}

	ad := stringValue(bv["availabilityDomain"])
	compartmentID := stringValue(bv["compartmentId"])
	if compartmentID == "" {
		compartmentID, _ = o.compartmentIDOrTenancy()
	}
	if ad != "" && compartmentID != "" {
		var atts []map[string]interface{}
		if err := o.requestJSON("GET", "/bootVolumeAttachments", url.Values{
			"availabilityDomain": {ad},
			"compartmentId":      {compartmentID},
			"bootVolumeId":       {bootVolumeID},
		}, nil, &atts, false); err == nil {
			for _, att := range atts {
				if isActiveBootAttachmentState(stringValue(att["lifecycleState"])) {
					return fmt.Errorf("该启动盘仍处于已附加状态，请先停止实例并卸载后再删除")
				}
			}
		}
	}

	if err := o.requestJSON("DELETE", "/bootVolumes/"+url.PathEscape(bootVolumeID), nil, nil, nil, false); err != nil {
		return fmt.Errorf("删除启动盘失败: %w", err)
	}
	return nil
}
