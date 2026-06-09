package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

type OCINetworkSecurityGroup struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	IngressRules []OCISecurityRuleInput `json:"ingressRules"`
	EgressRules  []OCISecurityRuleInput `json:"egressRules"`
}

func listOCINetworkSecurityGroups(c *gin.Context) {
	instanceID := c.Param("name")
	oci, ok := getOCIService(c)
	if !ok {
		return
	}

	groups, err := oci.ListNetworkSecurityGroups(instanceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"networkSecurityGroups": groups})
}

func createOCINetworkSecurityGroup(c *gin.Context) {
	instanceID := c.Param("name")
	oci, ok := getOCIService(c)
	if !ok {
		return
	}

	var payload struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	group, err := oci.CreateAndAttachNetworkSecurityGroup(instanceID, payload.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	groups, err := oci.ListNetworkSecurityGroups(instanceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "networkSecurityGroup": group})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "networkSecurityGroup": group, "networkSecurityGroups": groups})
}

func saveOCINetworkSecurityGroupRules(c *gin.Context) {
	instanceID := c.Param("name")
	groupID := c.Param("groupID")
	oci, ok := getOCIService(c)
	if !ok {
		return
	}

	var payload struct {
		IngressRules []OCISecurityRuleInput `json:"ingressRules"`
		EgressRules  []OCISecurityRuleInput `json:"egressRules"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := oci.SaveNetworkSecurityGroupRules(instanceID, groupID, payload.IngressRules, payload.EgressRules); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	groups, err := oci.ListNetworkSecurityGroups(instanceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "networkSecurityGroups": groups})
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
		"compartmentId": o.account.CompartmentID,
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
