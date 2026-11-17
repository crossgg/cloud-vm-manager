package oci

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

type OCIRouteTargetEntity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type OCIRouteRule struct {
	Destination       string `json:"destination"`
	DestinationType   string `json:"destinationType"`
	NetworkEntityID   string `json:"networkEntityId"`
	NetworkEntityName string `json:"networkEntityName,omitempty"`
	Description       string `json:"description,omitempty"`
	RouteType         string `json:"routeType,omitempty"`
}

type OCIRouteTable struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	VCNID             string                 `json:"vcnId"`
	VCNName           string                 `json:"vcnName,omitempty"`
	SubnetNames       []string               `json:"subnetNames"`
	RouteRules        []OCIRouteRule         `json:"routeRules"`
	AvailableEntities []OCIRouteTargetEntity `json:"availableEntities"`
}

func (o *OCIService) ListAccountRouteTables() ([]OCIRouteTable, error) {
	compartmentID, err := o.compartmentIDOrTenancy()
	if err != nil {
		return nil, err
	}

	var rawTables []map[string]interface{}
	if err := o.requestJSON("GET", "/routeTables", url.Values{
		"compartmentId": {compartmentID},
	}, nil, &rawTables, false); err != nil {
		return nil, err
	}

	vcnNames := map[string]string{}
	var rawVCNs []map[string]interface{}
	if err := o.requestJSON("GET", "/vcns", url.Values{"compartmentId": {compartmentID}}, nil, &rawVCNs, false); err == nil {
		for _, vcn := range rawVCNs {
			if id := stringValue(vcn["id"]); id != "" {
				vcnNames[id] = valueOrDefault(stringValue(vcn["displayName"]), id)
			}
		}
	}

	rtSubnets := map[string][]string{}
	var rawSubnets []map[string]interface{}
	if err := o.requestJSON("GET", "/subnets", url.Values{"compartmentId": {compartmentID}}, nil, &rawSubnets, false); err == nil {
		for _, sub := range rawSubnets {
			rtID := stringValue(sub["routeTableId"])
			if rtID == "" {
				continue
			}
			subName := valueOrDefault(stringValue(sub["displayName"]), stringValue(sub["id"]))
			if subName != "" {
				rtSubnets[rtID] = append(rtSubnets[rtID], subName)
			}
		}
	}

	entitiesByVCN, globalEntities, entityLabels := o.collectRouteTargetEntities(compartmentID)

	tables := make([]OCIRouteTable, 0, len(rawTables))
	for _, raw := range rawTables {
		rtID := stringValue(raw["id"])
		vcnID := stringValue(raw["vcnId"])

		rawRules, _ := raw["routeRules"].([]interface{})
		rules := make([]OCIRouteRule, 0, len(rawRules))
		for _, item := range rawRules {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			entityID := stringValue(m["networkEntityId"])
			rules = append(rules, OCIRouteRule{
				Destination:       valueOrDefault(stringValue(m["destination"]), stringValue(m["cidrBlock"])),
				DestinationType:   valueOrDefault(stringValue(m["destinationType"]), "CIDR_BLOCK"),
				NetworkEntityID:   entityID,
				NetworkEntityName: entityLabels[entityID],
				Description:       stringValue(m["description"]),
				RouteType:         stringValue(m["routeType"]),
			})
		}

		avail := make([]OCIRouteTargetEntity, 0, len(entitiesByVCN[vcnID])+len(globalEntities))
		avail = append(avail, entitiesByVCN[vcnID]...)
		avail = append(avail, globalEntities...)

		subNames := rtSubnets[rtID]
		if subNames == nil {
			subNames = []string{}
		} else {
			sort.Strings(subNames)
		}

		tables = append(tables, OCIRouteTable{
			ID:                rtID,
			Name:              valueOrDefault(stringValue(raw["displayName"]), rtID),
			VCNID:             vcnID,
			VCNName:           vcnNames[vcnID],
			SubnetNames:       subNames,
			RouteRules:        rules,
			AvailableEntities: avail,
		})
	}

	return tables, nil
}

func (o *OCIService) collectRouteTargetEntities(compartmentID string) (map[string][]OCIRouteTargetEntity, []OCIRouteTargetEntity, map[string]string) {
	byVCN := make(map[string][]OCIRouteTargetEntity)
	global := make([]OCIRouteTargetEntity, 0)
	labels := make(map[string]string)

	fetchGatewayType := func(path, gwType, idField string, isGlobal bool) {
		var items []map[string]interface{}
		if err := o.requestJSON("GET", path, url.Values{"compartmentId": {compartmentID}}, nil, &items, false); err != nil {
			return
		}
		for _, item := range items {
			id := stringValue(item[idField])
			if id == "" {
				continue
			}
			name := valueOrDefault(stringValue(item["displayName"]), id)
			entity := OCIRouteTargetEntity{
				ID:   id,
				Name: name,
				Type: gwType,
			}
			labels[id] = fmt.Sprintf("[%s] %s", gwType, name)
			if isGlobal {
				if _, exists := byVCN["__global_"+id]; !exists {
					byVCN["__global_"+id] = nil
					global = append(global, entity)
				}
				continue
			}
			if vcnID := stringValue(item["vcnId"]); vcnID != "" {
				byVCN[vcnID] = append(byVCN[vcnID], entity)
			}
		}
	}

	fetchGatewayType("/internetGateways", "IGW", "id", false)
	fetchGatewayType("/natGateways", "NAT", "id", false)
	fetchGatewayType("/serviceGateways", "SGW", "id", false)
	fetchGatewayType("/localPeeringGateways", "LPG", "id", false)
	fetchGatewayType("/drgAttachments", "DRG", "drgId", true)

	return byVCN, global, labels
}

func (o *OCIService) SaveAccountRouteTableRules(routeTableID string, rules []OCIRouteRule) error {
	routeTableID = strings.TrimSpace(routeTableID)
	if routeTableID == "" {
		return fmt.Errorf("missing OCI route table id")
	}

	var rawTable map[string]interface{}
	if err := o.requestJSON("GET", "/routeTables/"+url.PathEscape(routeTableID), nil, nil, &rawTable, false); err != nil {
		return err
	}

	payloadRules := make([]map[string]interface{}, 0, len(rules))
	for i, r := range rules {
		dest := strings.TrimSpace(r.Destination)
		if dest == "" {
			return fmt.Errorf("第 %d 条路由规则缺少目标 CIDR / Service CIDR", i+1)
		}
		entityID := strings.TrimSpace(r.NetworkEntityID)
		if entityID == "" {
			return fmt.Errorf("第 %d 条路由规则缺少目标网关/实体 (networkEntityId)", i+1)
		}
		destType := strings.TrimSpace(r.DestinationType)
		if destType == "" {
			destType = "CIDR_BLOCK"
		}
		item := map[string]interface{}{
			"destination":     dest,
			"destinationType": destType,
			"networkEntityId": entityID,
		}
		if desc := strings.TrimSpace(r.Description); desc != "" {
			item["description"] = desc
		}
		if rt := strings.TrimSpace(r.RouteType); rt != "" {
			item["routeType"] = rt
		}
		payloadRules = append(payloadRules, item)
	}

	body := map[string]interface{}{
		"definedTags":  valueOrEmptyMap(rawTable["definedTags"]),
		"freeformTags": valueOrEmptyMap(rawTable["freeformTags"]),
		"routeRules":   payloadRules,
	}
	if displayName := stringValue(rawTable["displayName"]); displayName != "" {
		body["displayName"] = displayName
	}

	return o.requestJSON("PUT", "/routeTables/"+url.PathEscape(routeTableID), nil, body, nil, false)
}
