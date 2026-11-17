package oci

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

type OCIQuotaItem struct {
	Service            string  `json:"service"`
	Name               string  `json:"name"`
	Label              string  `json:"label"`
	ScopeType          string  `json:"scopeType"`
	AvailabilityDomain string  `json:"availabilityDomain,omitempty"`
	Limit              float64 `json:"limit"`
	Used               float64 `json:"used"`
	Available          float64 `json:"available"`
}

type OCICostServiceItem struct {
	Service string  `json:"service"`
	Amount  float64 `json:"amount"`
}

type OCIOverview struct {
	Account       string               `json:"account"`
	Region        string               `json:"region"`
	HomeRegion    string               `json:"homeRegion"`
	TenancyID     string               `json:"tenancyId"`
	CompartmentID string               `json:"compartmentId"`
	TenancyName   string               `json:"tenancyName,omitempty"`
	CostStatus    string               `json:"costStatus"` // "ok" | "unauthorized" | "error"
	CostMessage   string               `json:"costMessage,omitempty"`
	TotalCost     float64              `json:"totalCost"`
	Currency      string               `json:"currency"`
	PeriodStart   string               `json:"periodStart"`
	PeriodEnd     string               `json:"periodEnd"`
	CostByService []OCICostServiceItem `json:"costByService"`
	LimitsStatus  string               `json:"limitsStatus"` // "ok" | "unauthorized" | "error"
	LimitsMessage string               `json:"limitsMessage,omitempty"`
	Quotas        []OCIQuotaItem       `json:"quotas"`
}

type quotaWatchSpec struct {
	Service string
	Name    string
	Label   string
}

var ociKeyQuotas = []quotaWatchSpec{
	{Service: "compute", Name: "standard-e2-micro-count", Label: "AMD 免费实例 (E2.1.Micro)"},
	{Service: "compute", Name: "standard-a1-core-count", Label: "ARM 核心数 (A1.Flex OCPU)"},
	{Service: "compute", Name: "standard-a1-memory-count", Label: "ARM 内存 (A1.Flex GB)"},
	{Service: "compute", Name: "standard-e4-core-count", Label: "AMD E4 核心数 (OCPU)"},
	{Service: "block-storage", Name: "total-storage-gb", Label: "块存储/引导卷容量 (GB)"},
	{Service: "virtual-network", Name: "vcn-count", Label: "VCN 数量"},
	{Service: "virtual-network", Name: "reserved-public-ip-count", Label: "保留公网 IP 数量"},
}

func (o *OCIService) GetOverview() (*OCIOverview, error) {
	homeRegion := o.HomeRegion()
	compartmentID, _ := o.compartmentIDOrTenancy()

	now := time.Now().UTC()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)

	overview := &OCIOverview{
		Account:       o.Account.Name,
		Region:        o.Account.Region,
		HomeRegion:    homeRegion,
		TenancyID:     o.Account.Tenancy,
		CompartmentID: compartmentID,
		Currency:      "USD",
		PeriodStart:   periodStart.Format("2006-01-02"),
		PeriodEnd:     periodEnd.AddDate(0, 0, -1).Format("2006-01-02"),
		CostByService: []OCICostServiceItem{},
		Quotas:        []OCIQuotaItem{},
	}

	if strings.TrimSpace(o.Account.Tenancy) != "" {
		var tenancy map[string]interface{}
		if err := o.identityRequestJSON(homeRegion, "GET", "/tenancies/"+url.PathEscape(o.Account.Tenancy), nil, nil, &tenancy, true); err == nil && tenancy != nil {
			overview.TenancyName = stringValue(tenancy["name"])
		}
	}

	o.populateCostOverview(overview, homeRegion, periodStart, periodEnd)
	o.populateLimitsOverview(overview)

	return overview, nil
}

func (o *OCIService) populateCostOverview(overview *OCIOverview, homeRegion string, start, end time.Time) {
	if strings.TrimSpace(o.Account.Tenancy) == "" {
		overview.CostStatus = "error"
		overview.CostMessage = "缺少 Tenancy OCID，无法查询成本"
		return
	}

	body := map[string]interface{}{
		"tenantId":         o.Account.Tenancy,
		"timeUsageStarted": start.Format("2006-01-02T15:04:05Z"),
		"timeUsageEnded":   end.Format("2006-01-02T15:04:05Z"),
		"granularity":      "MONTHLY",
		"queryType":        "COST",
		"groupBy":          []string{"service"},
	}

	var resp map[string]interface{}
	err := o.usageRequestJSON(homeRegion, "POST", "/usage", nil, body, &resp, false)
	if err != nil {
		msg := err.Error()
		if isOCIUnauthorizedErr(msg) {
			overview.CostStatus = "unauthorized"
			overview.CostMessage = "当前 API Key 无成本读取权限，请在根区间 Policy 添加：allow group <组名> to read usage-reports in tenancy 及 inspect tenancies in tenancy"
		} else {
			overview.CostStatus = "error"
			overview.CostMessage = "成本查询失败: " + msg
		}
		return
	}

	overview.CostStatus = "ok"
	rawItems, _ := resp["items"].([]interface{})
	serviceTotals := make(map[string]float64)
	for _, raw := range rawItems {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		amount := float64InterfaceValue(m["computedAmount"])
		if c := stringValue(m["currency"]); c != "" {
			overview.Currency = c
		}
		svc := valueOrDefault(stringValue(m["service"]), "其他")
		serviceTotals[svc] += amount
		overview.TotalCost += amount
	}

	items := make([]OCICostServiceItem, 0, len(serviceTotals))
	for svc, amt := range serviceTotals {
		items = append(items, OCICostServiceItem{Service: svc, Amount: amt})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Amount != items[j].Amount {
			return items[i].Amount > items[j].Amount
		}
		return items[i].Service < items[j].Service
	})
	overview.CostByService = items
}

func (o *OCIService) populateLimitsOverview(overview *OCIOverview) {
	scopeID := strings.TrimSpace(o.Account.Tenancy)
	if scopeID == "" {
		scopeID = overview.CompartmentID
	}
	if scopeID == "" {
		overview.LimitsStatus = "error"
		overview.LimitsMessage = "缺少 Tenancy OCID，无法查询配额"
		return
	}

	// Fetch ADs so AD-scoped limits can query resourceAvailability cleanly
	var firstAD string
	var rawADs []map[string]interface{}
	if err := o.identityRequestJSON(o.Account.Region, "GET", "/availabilityDomains", url.Values{
		"compartmentId": {scopeID},
	}, nil, &rawADs, true); err == nil && len(rawADs) > 0 {
		firstAD = stringValue(rawADs[0]["name"])
	}

	services := []string{"compute", "block-storage", "virtual-network"}
	limitMap := make(map[string]map[string]interface{})
	var firstErr error
	for _, svcName := range services {
		var values []map[string]interface{}
		err := o.limitsRequestJSON("GET", "/limitValues", url.Values{
			"compartmentId": {scopeID},
			"serviceName":   {svcName},
			"limit":         {"100"},
		}, nil, &values, false)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, v := range values {
			name := stringValue(v["name"])
			if name == "" {
				continue
			}
			key := svcName + ":" + name
			if _, exists := limitMap[key]; !exists {
				limitMap[key] = v
			}
		}
	}

	if len(limitMap) == 0 && firstErr != nil {
		msg := firstErr.Error()
		if isOCIUnauthorizedErr(msg) {
			overview.LimitsStatus = "unauthorized"
			overview.LimitsMessage = "当前 API Key 无配额读取权限，请添加：allow group <组名> to read limits in tenancy"
		} else {
			overview.LimitsStatus = "error"
			overview.LimitsMessage = "配额查询失败: " + msg
		}
		return
	}

	overview.LimitsStatus = "ok"
	quotas := make([]OCIQuotaItem, 0, len(ociKeyQuotas))
	for _, spec := range ociKeyQuotas {
		v, ok := limitMap[spec.Service+":"+spec.Name]
		if !ok {
			continue
		}
		scopeType := strings.ToUpper(stringValue(v["scopeType"]))
		limitVal := float64InterfaceValue(v["value"])

		query := url.Values{"compartmentId": {scopeID}}
		usedAD := ""
		if scopeType == "AD" && firstAD != "" {
			query.Set("availabilityDomain", firstAD)
			usedAD = firstAD
		}

		var availMap map[string]interface{}
		var usedVal, availVal float64
		availPath := fmt.Sprintf("/services/%s/limits/%s/resourceAvailability", url.PathEscape(spec.Service), url.PathEscape(spec.Name))
		if err := o.limitsRequestJSON("GET", availPath, query, nil, &availMap, true); err == nil && availMap != nil {
			usedVal = float64InterfaceValue(availMap["used"])
			availVal = float64InterfaceValue(availMap["available"])
		} else {
			availVal = limitVal
		}

		quotas = append(quotas, OCIQuotaItem{
			Service:            spec.Service,
			Name:               spec.Name,
			Label:              spec.Label,
			ScopeType:          scopeType,
			AvailabilityDomain: usedAD,
			Limit:              limitVal,
			Used:               usedVal,
			Available:          availVal,
		})
	}
	overview.Quotas = quotas
}

func isOCIUnauthorizedErr(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(msg, "401") || strings.Contains(msg, "403") || strings.Contains(msg, "404") ||
		strings.Contains(lower, "notauthorized") || strings.Contains(lower, "unauthorized")
}
