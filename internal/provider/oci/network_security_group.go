package oci

import (
	"fmt"
	"net/url"
	"strings"
)

type OCINetworkSecurityGroup struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	VCNID        string                 `json:"vcnId,omitempty"`
	IngressRules []OCISecurityRuleInput `json:"ingressRules"`
	EgressRules  []OCISecurityRuleInput `json:"egressRules"`
}

type OCINetworkSecurityGroupOption struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	VCNID    string `json:"vcnId,omitempty"`
	Attached bool   `json:"attached"`
}

func (o *OCIService) compartmentIDOrTenancy() (string, error) {
	compartmentID := strings.TrimSpace(o.Account.CompartmentID)
	if compartmentID == "" {
		compartmentID = strings.TrimSpace(o.Account.Tenancy)
	}
	if compartmentID == "" {
		return "", fmt.Errorf("missing OCI compartmentId / tenancy")
	}
	return compartmentID, nil
}

func (o *OCIService) resolveVNICVCNID(vnic map[string]interface{}) (string, error) {
	if vcnID := stringValue(vnic["vcnId"]); vcnID != "" {
		return vcnID, nil
	}
	subnetID := stringValue(vnic["subnetId"])
	if subnetID == "" {
		return "", fmt.Errorf("OCI VNIC has no subnet id")
	}
	var subnet map[string]interface{}
	if err := o.requestJSON("GET", "/subnets/"+url.PathEscape(subnetID), nil, nil, &subnet, false); err != nil {
		return "", err
	}
	vcnID := stringValue(subnet["vcnId"])
	if vcnID == "" {
		return "", fmt.Errorf("OCI subnet has no VCN id")
	}
	return vcnID, nil
}

func (o *OCIService) ListInstanceNetworkSecurityGroups(instanceID string) ([]OCINetworkSecurityGroup, []OCINetworkSecurityGroupOption, string, error) {
	_, vnic, err := o.primaryPrivateIP(instanceID)
	if err != nil {
		return nil, nil, "", err
	}

	vcnID, err := o.resolveVNICVCNID(vnic)
	if err != nil {
		return nil, nil, "", err
	}

	nsgIDs := stringSliceValue(vnic["nsgIds"])
	attached := make([]OCINetworkSecurityGroup, 0, len(nsgIDs))
	for _, nsgID := range nsgIDs {
		group, err := o.rawNetworkSecurityGroup(nsgID)
		if err != nil {
			return nil, nil, "", err
		}
		ingressRules, err := o.networkSecurityGroupRules(nsgID, "INGRESS")
		if err != nil {
			return nil, nil, "", err
		}
		egressRules, err := o.networkSecurityGroupRules(nsgID, "EGRESS")
		if err != nil {
			return nil, nil, "", err
		}
		attached = append(attached, OCINetworkSecurityGroup{
			ID:           nsgID,
			Name:         valueOrDefault(stringValue(group["displayName"]), nsgID),
			VCNID:        valueOrDefault(stringValue(group["vcnId"]), vcnID),
			IngressRules: ingressRules,
			EgressRules:  egressRules,
		})
	}

	compartmentID, err := o.compartmentIDOrTenancy()
	if err != nil {
		return nil, nil, "", err
	}
	var rawGroups []map[string]interface{}
	if err := o.requestJSON("GET", "/networkSecurityGroups", url.Values{
		"compartmentId": {compartmentID},
		"vcnId":         {vcnID},
	}, nil, &rawGroups, false); err != nil {
		return nil, nil, "", err
	}

	options := make([]OCINetworkSecurityGroupOption, 0, len(rawGroups))
	for _, raw := range rawGroups {
		state := strings.ToUpper(stringValue(raw["lifecycleState"]))
		if state == "TERMINATING" || state == "TERMINATED" {
			continue
		}
		id := stringValue(raw["id"])
		if id == "" {
			continue
		}
		options = append(options, OCINetworkSecurityGroupOption{
			ID:       id,
			Name:     valueOrDefault(stringValue(raw["displayName"]), id),
			VCNID:    valueOrDefault(stringValue(raw["vcnId"]), vcnID),
			Attached: containsString(nsgIDs, id),
		})
	}

	return attached, options, vcnID, nil
}

func (o *OCIService) AttachNetworkSecurityGroup(instanceID, nsgID string) error {
	nsgID = strings.TrimSpace(nsgID)
	if nsgID == "" {
		return fmt.Errorf("missing OCI network security group id")
	}
	_, vnic, err := o.primaryPrivateIP(instanceID)
	if err != nil {
		return err
	}
	vcnID, err := o.resolveVNICVCNID(vnic)
	if err != nil {
		return err
	}

	group, err := o.rawNetworkSecurityGroup(nsgID)
	if err != nil {
		return err
	}
	if groupVCN := stringValue(group["vcnId"]); groupVCN != "" && vcnID != "" && groupVCN != vcnID {
		return fmt.Errorf("网络安全组与实例主 VNIC 不在同一个 VCN 下，无法绑定")
	}

	current := stringSliceValue(vnic["nsgIds"])
	if containsString(current, nsgID) {
		return nil
	}
	if len(current) >= 5 {
		return fmt.Errorf("OCI 单个 VNIC 最多只能绑定 5 个网络安全组")
	}
	next := appendUniqueString(current, nsgID)
	return o.updateVNICNSGs(stringValue(vnic["id"]), next)
}

func (o *OCIService) DetachNetworkSecurityGroup(instanceID, nsgID string) error {
	nsgID = strings.TrimSpace(nsgID)
	if nsgID == "" {
		return fmt.Errorf("missing OCI network security group id")
	}
	_, vnic, err := o.primaryPrivateIP(instanceID)
	if err != nil {
		return err
	}
	current := stringSliceValue(vnic["nsgIds"])
	if !containsString(current, nsgID) {
		return nil
	}
	next := removeStringValue(current, nsgID)
	return o.updateVNICNSGs(stringValue(vnic["id"]), next)
}

func (o *OCIService) ListAccountNetworkSecurityGroups() ([]OCINetworkSecurityGroup, error) {
	compartmentID, err := o.compartmentIDOrTenancy()
	if err != nil {
		return nil, err
	}
	var rawGroups []map[string]interface{}
	if err := o.requestJSON("GET", "/networkSecurityGroups", url.Values{
		"compartmentId": {compartmentID},
	}, nil, &rawGroups, false); err != nil {
		return nil, err
	}

	groups := make([]OCINetworkSecurityGroup, 0, len(rawGroups))
	for _, raw := range rawGroups {
		state := strings.ToUpper(stringValue(raw["lifecycleState"]))
		if state == "TERMINATING" || state == "TERMINATED" {
			continue
		}
		nsgID := stringValue(raw["id"])
		if nsgID == "" {
			continue
		}
		ingressRules, err := o.networkSecurityGroupRules(nsgID, "INGRESS")
		if err != nil {
			return nil, err
		}
		egressRules, err := o.networkSecurityGroupRules(nsgID, "EGRESS")
		if err != nil {
			return nil, err
		}
		groups = append(groups, OCINetworkSecurityGroup{
			ID:           nsgID,
			Name:         valueOrDefault(stringValue(raw["displayName"]), nsgID),
			VCNID:        stringValue(raw["vcnId"]),
			IngressRules: ingressRules,
			EgressRules:  egressRules,
		})
	}
	return groups, nil
}

func (o *OCIService) SaveAccountNetworkSecurityGroupRules(nsgID string, ingressRules, egressRules []OCISecurityRuleInput) error {
	nsgID = strings.TrimSpace(nsgID)
	if nsgID == "" {
		return fmt.Errorf("missing OCI network security group id")
	}
	if err := o.saveNetworkSecurityGroupRulesByDirection(nsgID, "INGRESS", ingressRules); err != nil {
		return err
	}
	return o.saveNetworkSecurityGroupRulesByDirection(nsgID, "EGRESS", egressRules)
}

func (o *OCIService) RenameNetworkSecurityGroup(nsgID, name string) (OCINetworkSecurityGroup, error) {
	nsgID = strings.TrimSpace(nsgID)
	if nsgID == "" {
		return OCINetworkSecurityGroup{}, fmt.Errorf("missing OCI network security group id")
	}
	displayName := strings.TrimSpace(name)
	if displayName == "" {
		return OCINetworkSecurityGroup{}, fmt.Errorf("网络安全组名称不能为空")
	}

	var updated map[string]interface{}
	if err := o.requestJSON("PUT", "/networkSecurityGroups/"+url.PathEscape(nsgID), nil, map[string]interface{}{
		"displayName": displayName,
	}, &updated, false); err != nil {
		return OCINetworkSecurityGroup{}, err
	}
	return OCINetworkSecurityGroup{
		ID:    nsgID,
		Name:  valueOrDefault(stringValue(updated["displayName"]), displayName),
		VCNID: stringValue(updated["vcnId"]),
	}, nil
}

func (o *OCIService) DeleteNetworkSecurityGroup(nsgID string) error {
	nsgID = strings.TrimSpace(nsgID)
	if nsgID == "" {
		return fmt.Errorf("missing OCI network security group id")
	}

	var attachedVNICs []map[string]interface{}
	if err := o.requestJSON("GET", "/networkSecurityGroups/"+url.PathEscape(nsgID)+"/vnics", nil, nil, &attachedVNICs, true); err == nil {
		if len(attachedVNICs) > 0 {
			return fmt.Errorf("该网络安全组仍绑定在 %d 个 VNIC（实例网卡）上，请先在对应实例的「安全规则 → 网络安全组」中解绑后再删除", len(attachedVNICs))
		}
	}

	if err := o.requestJSON("DELETE", "/networkSecurityGroups/"+url.PathEscape(nsgID), nil, nil, nil, false); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "409") || strings.Contains(strings.ToLower(msg), "conflict") || strings.Contains(strings.ToLower(msg), "in use") || strings.Contains(strings.ToLower(msg), "referenced") {
			return fmt.Errorf("该网络安全组仍被实例 VNIC 绑定或被安全规则引用，请先解绑或移除引用后再删除（原始错误：%w）", err)
		}
		return err
	}
	return nil
}

func removeStringValue(values []string, target string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && value != target {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func (o *OCIService) ListNetworkSecurityGroups(instanceID string) ([]OCINetworkSecurityGroup, error) {
	_, vnic, err := o.primaryPrivateIP(instanceID)
	if err != nil {
		return nil, err
	}

	nsgIDs := stringSliceValue(vnic["nsgIds"])
	if len(nsgIDs) == 0 {
		return []OCINetworkSecurityGroup{}, nil
	}

	groups := make([]OCINetworkSecurityGroup, 0, len(nsgIDs))
	for _, nsgID := range nsgIDs {
		group, err := o.rawNetworkSecurityGroup(nsgID)
		if err != nil {
			return nil, err
		}
		ingressRules, err := o.networkSecurityGroupRules(nsgID, "INGRESS")
		if err != nil {
			return nil, err
		}
		egressRules, err := o.networkSecurityGroupRules(nsgID, "EGRESS")
		if err != nil {
			return nil, err
		}

		groups = append(groups, OCINetworkSecurityGroup{
			ID:           nsgID,
			Name:         valueOrDefault(stringValue(group["displayName"]), nsgID),
			IngressRules: ingressRules,
			EgressRules:  egressRules,
		})
	}
	return groups, nil
}

func (o *OCIService) CreateAndAttachNetworkSecurityGroup(instanceID, name string) (OCINetworkSecurityGroup, error) {
	_, vnic, err := o.primaryPrivateIP(instanceID)
	if err != nil {
		return OCINetworkSecurityGroup{}, err
	}
	subnetID := stringValue(vnic["subnetId"])
	if subnetID == "" {
		return OCINetworkSecurityGroup{}, fmt.Errorf("OCI VNIC has no subnet id")
	}
	displayName := strings.TrimSpace(name)
	if displayName == "" {
		displayName = "vm-manager-nsg"
	}

	body := map[string]interface{}{
		"compartmentId": o.Account.CompartmentID,
		"displayName":   displayName,
		"vcnId":         stringValue(vnic["vcnId"]),
	}
	if body["vcnId"] == "" {
		var subnet map[string]interface{}
		if err := o.requestJSON("GET", "/subnets/"+url.PathEscape(subnetID), nil, nil, &subnet, false); err != nil {
			return OCINetworkSecurityGroup{}, err
		}
		body["vcnId"] = stringValue(subnet["vcnId"])
	}
	if body["vcnId"] == "" {
		return OCINetworkSecurityGroup{}, fmt.Errorf("OCI subnet has no VCN id")
	}

	var created map[string]interface{}
	if err := o.requestJSON("POST", "/networkSecurityGroups", nil, body, &created, false); err != nil {
		return OCINetworkSecurityGroup{}, err
	}
	nsgID := stringValue(created["id"])
	if nsgID == "" {
		return OCINetworkSecurityGroup{}, fmt.Errorf("OCI did not return a network security group id")
	}

	nsgIDs := appendUniqueString(stringSliceValue(vnic["nsgIds"]), nsgID)
	if err := o.updateVNICNSGs(stringValue(vnic["id"]), nsgIDs); err != nil {
		return OCINetworkSecurityGroup{}, err
	}

	return OCINetworkSecurityGroup{
		ID:           nsgID,
		Name:         valueOrDefault(stringValue(created["displayName"]), displayName),
		IngressRules: []OCISecurityRuleInput{},
		EgressRules:  []OCISecurityRuleInput{},
	}, nil
}

func (o *OCIService) SaveNetworkSecurityGroupRules(instanceID, nsgID string, ingressRules, egressRules []OCISecurityRuleInput) error {
	if nsgID == "" {
		return fmt.Errorf("missing OCI network security group id")
	}
	_, vnic, err := o.primaryPrivateIP(instanceID)
	if err != nil {
		return err
	}
	if !containsString(stringSliceValue(vnic["nsgIds"]), nsgID) {
		return fmt.Errorf("network security group %s is not attached to instance %s", nsgID, instanceID)
	}
	if err := o.saveNetworkSecurityGroupRulesByDirection(nsgID, "INGRESS", ingressRules); err != nil {
		return err
	}
	return o.saveNetworkSecurityGroupRulesByDirection(nsgID, "EGRESS", egressRules)
}

func (o *OCIService) saveNetworkSecurityGroupRulesByDirection(nsgID, direction string, rules []OCISecurityRuleInput) error {
	existingRules, err := o.rawNetworkSecurityGroupRules(nsgID, direction)
	if err != nil {
		return err
	}

	existingIDs := make(map[string]struct{}, len(existingRules))
	for _, rule := range existingRules {
		if id := stringValue(rule["id"]); id != "" {
			existingIDs[id] = struct{}{}
		}
	}

	incomingIDs := make(map[string]struct{}, len(rules))
	addRules := make([]map[string]interface{}, 0)
	updateRules := make([]map[string]interface{}, 0)
	isEgress := direction == "EGRESS"

	for _, rule := range rules {
		rule.IsDestination = isEgress
		normalized, err := securityRulePayload(rule)
		if err != nil {
			return err
		}
		normalized["direction"] = direction

		if rule.ID == "" {
			addRules = append(addRules, normalized)
			continue
		}
		if _, ok := existingIDs[rule.ID]; !ok {
			return fmt.Errorf("security rule %s is not present in network security group %s", rule.ID, nsgID)
		}
		if _, ok := incomingIDs[rule.ID]; ok {
			return fmt.Errorf("duplicate security rule id %s", rule.ID)
		}
		incomingIDs[rule.ID] = struct{}{}
		normalized["id"] = rule.ID
		updateRules = append(updateRules, normalized)
	}

	removeIDs := make([]string, 0)
	for id := range existingIDs {
		if _, ok := incomingIDs[id]; !ok {
			removeIDs = append(removeIDs, id)
		}
	}

	if len(removeIDs) > 0 {
		for _, batch := range chunkStrings(removeIDs, 25) {
			if err := o.requestJSON("POST", "/networkSecurityGroups/"+url.PathEscape(nsgID)+"/actions/removeSecurityRules", nil, map[string]interface{}{
				"securityRuleIds": batch,
			}, nil, false); err != nil {
				return err
			}
		}
	}
	if len(updateRules) > 0 {
		for _, batch := range chunkRulePayloads(updateRules, 25) {
			if err := o.requestJSON("POST", "/networkSecurityGroups/"+url.PathEscape(nsgID)+"/actions/updateSecurityRules", nil, map[string]interface{}{
				"securityRules": batch,
			}, nil, false); err != nil {
				return err
			}
		}
	}
	if len(addRules) > 0 {
		for _, batch := range chunkRulePayloads(addRules, 25) {
			if err := o.requestJSON("POST", "/networkSecurityGroups/"+url.PathEscape(nsgID)+"/actions/addSecurityRules", nil, map[string]interface{}{
				"securityRules": batch,
			}, nil, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func (o *OCIService) rawNetworkSecurityGroup(nsgID string) (map[string]interface{}, error) {
	var group map[string]interface{}
	err := o.requestJSON("GET", "/networkSecurityGroups/"+url.PathEscape(nsgID), nil, nil, &group, false)
	return group, err
}

func (o *OCIService) networkSecurityGroupRules(nsgID, direction string) ([]OCISecurityRuleInput, error) {
	rawRules, err := o.rawNetworkSecurityGroupRules(nsgID, direction)
	if err != nil {
		return nil, err
	}
	rules := make([]OCISecurityRuleInput, 0, len(rawRules))
	for _, rule := range rawRules {
		rules = append(rules, normalizeOCISecurityRule(rule, direction == "EGRESS"))
	}
	return rules, nil
}

func (o *OCIService) rawNetworkSecurityGroupRules(nsgID, direction string) ([]map[string]interface{}, error) {
	var rawRules []map[string]interface{}
	err := o.requestJSON("GET", "/networkSecurityGroups/"+url.PathEscape(nsgID)+"/securityRules", url.Values{
		"direction": {direction},
	}, nil, &rawRules, false)
	return rawRules, err
}

func (o *OCIService) updateVNICNSGs(vnicID string, nsgIDs []string) error {
	if vnicID == "" {
		return fmt.Errorf("missing OCI VNIC id")
	}
	return o.requestJSON("PUT", "/vnics/"+url.PathEscape(vnicID), nil, map[string]interface{}{
		"nsgIds": nsgIDs,
	}, nil, false)
}

func appendUniqueString(values []string, value string) []string {
	if value == "" || containsString(values, value) {
		return values
	}
	return append(values, value)
}

func chunkStrings(values []string, size int) [][]string {
	if size <= 0 || len(values) == 0 {
		return nil
	}
	chunks := make([][]string, 0, (len(values)+size-1)/size)
	for start := 0; start < len(values); start += size {
		end := start + size
		if end > len(values) {
			end = len(values)
		}
		chunks = append(chunks, values[start:end])
	}
	return chunks
}

func chunkRulePayloads(values []map[string]interface{}, size int) [][]map[string]interface{} {
	if size <= 0 || len(values) == 0 {
		return nil
	}
	chunks := make([][]map[string]interface{}, 0, (len(values)+size-1)/size)
	for start := 0; start < len(values); start += size {
		end := start + size
		if end > len(values) {
			end = len(values)
		}
		chunks = append(chunks, values[start:end])
	}
	return chunks
}
