package azure

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

type AzureCostItem struct {
	Service     string  `json:"service"`
	CostUSD     float64 `json:"costUSD"`
	CostBilling float64 `json:"costBilling"`
}

type AzureMonthlyCost struct {
	Month       string  `json:"month"`
	CostUSD     float64 `json:"costUSD"`
	CostBilling float64 `json:"costBilling"`
}

type AzureCostOverview struct {
	Account               string             `json:"account"`
	SubscriptionID        string             `json:"subscriptionId"`
	SubscriptionName      string             `json:"subscriptionName,omitempty"`
	SubscriptionState     string             `json:"subscriptionState,omitempty"`
	QuotaID               string             `json:"quotaId,omitempty"`
	SpendingLimit         string             `json:"spendingLimit,omitempty"`
	IsStudentOffer        bool               `json:"isStudentOffer"`
	CostStatus            string             `json:"costStatus"` // "ok" | "error"
	CostMessage           string             `json:"costMessage,omitempty"`
	BillingCurrency       string             `json:"billingCurrency"`
	MonthToDateCostUSD    float64            `json:"monthToDateCostUSD"`
	MonthToDateCostBill   float64            `json:"monthToDateCostBilling"`
	MonthByService        []AzureCostItem    `json:"monthByService"`
	MonthlyHistory        []AzureMonthlyCost `json:"monthlyHistory"`
	HistoryStartDate      string             `json:"historyStartDate"`
	HistoryEndDate        string             `json:"historyEndDate"`
	PeriodTotalCostUSD    float64            `json:"periodTotalCostUSD"`
	PeriodTotalCostBill   float64            `json:"periodTotalCostBilling"`
	RemainingBasis        string             `json:"remainingBasis"` // "since_start_date" | "last_12_months"
	HasCreditConfig       bool               `json:"hasCreditConfig"`
	MissingCreditFields   []string           `json:"missingCreditFields"`
	InitialCredit         float64            `json:"initialCredit"`
	StartDate             string             `json:"startDate,omitempty"`
	ExpireDate            string             `json:"expireDate,omitempty"`
	EstimatedRemainingUSD *float64           `json:"estimatedRemainingUSD,omitempty"`
	DaysUntilExpire       *int               `json:"daysUntilExpire,omitempty"`
	QueriedAt             time.Time          `json:"queriedAt"`
}

type costColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type costQueryResponse struct {
	Properties struct {
		NextLink string          `json:"nextLink"`
		Columns  []costColumn    `json:"columns"`
		Rows     [][]interface{} `json:"rows"`
	} `json:"properties"`
}

func (az *AzureService) GetCostAndCreditOverview(creditCfg config.AzureCreditMonitorConfig) (*AzureCostOverview, error) {
	return az.getCostAndCreditOverviewAt(creditCfg, time.Now())
}

func (az *AzureService) getCostAndCreditOverviewAt(creditCfg config.AzureCreditMonitorConfig, now time.Time) (*AzureCostOverview, error) {
	creditCfg.Normalize()

	overview := &AzureCostOverview{
		Account:         az.Account.Name,
		SubscriptionID:  az.Account.SubId,
		BillingCurrency: "USD",
		MonthByService:  []AzureCostItem{},
		MonthlyHistory:  []AzureMonthlyCost{},
		InitialCredit:   creditCfg.InitialCredit,
		StartDate:       creditCfg.StartDate,
		ExpireDate:      creditCfg.ExpireDate,
		QueriedAt:       now,
	}

	var missing []string
	if creditCfg.InitialCredit <= 0 {
		missing = append(missing, "initialCredit")
	}
	if creditCfg.StartDate == "" {
		missing = append(missing, "startDate")
	}
	if creditCfg.ExpireDate == "" {
		missing = append(missing, "expireDate")
	}
	overview.MissingCreditFields = missing
	overview.HasCreditConfig = len(missing) == 0

	if creditCfg.ExpireDate != "" {
		if expTime, err := time.Parse("2006-01-02", creditCfg.ExpireDate); err == nil {
			today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			expDay := time.Date(expTime.Year(), expTime.Month(), expTime.Day(), 0, 0, 0, 0, time.UTC)
			days := int(math.Round(expDay.Sub(today).Hours() / 24))
			overview.DaysUntilExpire = &days
		}
	}

	az.populateSubscriptionMeta(overview)

	// 1. Query Month-To-Date breakdown by ServiceName
	mtdCols, mtdRows, err := az.queryCostManagement(map[string]interface{}{
		"type":      "ActualCost",
		"timeframe": "MonthToDate",
		"dataset": map[string]interface{}{
			"granularity": "None",
			"aggregation": map[string]interface{}{
				"totalCost": map[string]interface{}{
					"name":     "Cost",
					"function": "Sum",
				},
				"totalCostUSD": map[string]interface{}{
					"name":     "CostUSD",
					"function": "Sum",
				},
			},
			"grouping": []map[string]interface{}{
				{"type": "Dimension", "name": "ServiceName"},
			},
		},
	})
	if err != nil {
		overview.CostStatus = "error"
		overview.CostMessage = err.Error()
		return overview, nil
	}

	overview.CostStatus = "ok"
	mtdIdx := buildColumnIndex(mtdCols)
	for _, row := range mtdRows {
		costBill := rowFloat(row, mtdIdx, "Cost")
		costUSD := rowFloat(row, mtdIdx, "CostUSD")
		svc := rowString(row, mtdIdx, "ServiceName")
		if svc == "" {
			svc = "其他"
		}
		if curr := rowString(row, mtdIdx, "Currency"); curr != "" {
			overview.BillingCurrency = curr
		}
		overview.MonthToDateCostUSD += costUSD
		overview.MonthToDateCostBill += costBill
		overview.MonthByService = append(overview.MonthByService, AzureCostItem{
			Service:     svc,
			CostUSD:     costUSD,
			CostBilling: costBill,
		})
	}
	sort.Slice(overview.MonthByService, func(i, j int) bool {
		if overview.MonthByService[i].CostUSD != overview.MonthByService[j].CostUSD {
			return overview.MonthByService[i].CostUSD > overview.MonthByService[j].CostUSD
		}
		return overview.MonthByService[i].Service < overview.MonthByService[j].Service
	})

	// 2. Query last 12 months monthly history
	nowUTC := now.UTC()
	histStart := time.Date(nowUTC.Year(), nowUTC.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -11, 0)
	histEnd := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	overview.HistoryStartDate = histStart.Format("2006-01-02")
	overview.HistoryEndDate = nowUTC.Format("2006-01-02")

	monthlyRows, curr, histErr := az.queryMonthlyRange(histStart, histEnd)
	if histErr == nil {
		if curr != "" && overview.BillingCurrency == "USD" {
			overview.BillingCurrency = curr
		}
		overview.MonthlyHistory = monthlyRows
	}

	// 3. Calculate cumulative cost for credit estimation
	var totalUSD, totalBill float64
	if creditCfg.StartDate != "" {
		if startT, parseErr := time.Parse("2006-01-02", creditCfg.StartDate); parseErr == nil {
			startUTC := time.Date(startT.Year(), startT.Month(), startT.Day(), 0, 0, 0, 0, time.UTC)
			if !startUTC.After(histEnd) {
				if sumUSD, sumBill, sumErr := az.queryTotalRange(startUTC, histEnd); sumErr == nil {
					totalUSD = sumUSD
					totalBill = sumBill
					overview.RemainingBasis = "since_start_date"
				}
			}
		}
	}
	if overview.RemainingBasis == "" {
		for _, m := range overview.MonthlyHistory {
			totalUSD += m.CostUSD
			totalBill += m.CostBilling
		}
		overview.RemainingBasis = "last_12_months"
	}

	overview.PeriodTotalCostUSD = totalUSD
	overview.PeriodTotalCostBill = totalBill

	if creditCfg.InitialCredit > 0 {
		rem := creditCfg.InitialCredit - totalUSD
		if rem < 0 {
			rem = 0
		}
		overview.EstimatedRemainingUSD = &rem
	}

	return overview, nil
}

func (az *AzureService) populateSubscriptionMeta(overview *AzureCostOverview) {
	url := fmt.Sprintf("https://management.azure.com/subscriptions/%s?api-version=2022-12-01", url.PathEscape(az.Account.SubId))
	resp, err := az.requestWithRetry("GET", url, nil, 3)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var sub map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&sub); err != nil {
		return
	}
	if name, ok := sub["displayName"].(string); ok {
		overview.SubscriptionName = name
	}
	if state, ok := sub["state"].(string); ok {
		overview.SubscriptionState = state
	}
	if policies, ok := sub["subscriptionPolicies"].(map[string]interface{}); ok {
		if qid, ok := policies["quotaId"].(string); ok {
			overview.QuotaID = qid
		}
		if sl, ok := policies["spendingLimit"].(string); ok {
			overview.SpendingLimit = sl
		}
	}
	lowerQ := strings.ToLower(overview.QuotaID)
	lowerN := strings.ToLower(overview.SubscriptionName)
	overview.IsStudentOffer = strings.Contains(lowerQ, "student") ||
		strings.Contains(lowerN, "student") ||
		strings.EqualFold(overview.SpendingLimit, "On")
}

func (az *AzureService) queryMonthlyRange(from, to time.Time) ([]AzureMonthlyCost, string, error) {
	cols, rows, err := az.queryCostManagement(map[string]interface{}{
		"type":      "ActualCost",
		"timeframe": "Custom",
		"timePeriod": map[string]interface{}{
			"from": from.Format("2006-01-02T00:00:00+00:00"),
			"to":   to.Format("2006-01-02T00:00:00+00:00"),
		},
		"dataset": map[string]interface{}{
			"granularity": "Monthly",
			"aggregation": map[string]interface{}{
				"totalCost": map[string]interface{}{
					"name":     "Cost",
					"function": "Sum",
				},
				"totalCostUSD": map[string]interface{}{
					"name":     "CostUSD",
					"function": "Sum",
				},
			},
		},
	})
	if err != nil {
		return nil, "", err
	}
	idx := buildColumnIndex(cols)
	var out []AzureMonthlyCost
	currency := ""
	for _, row := range rows {
		monthRaw := rowString(row, idx, "BillingMonth")
		monthLabel := monthRaw
		if len(monthRaw) >= 7 {
			monthLabel = monthRaw[:7]
		}
		if c := rowString(row, idx, "Currency"); c != "" {
			currency = c
		}
		out = append(out, AzureMonthlyCost{
			Month:       monthLabel,
			CostUSD:     rowFloat(row, idx, "CostUSD"),
			CostBilling: rowFloat(row, idx, "Cost"),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Month < out[j].Month
	})
	return out, currency, nil
}

// queryTotalRange sums ActualCost across arbitrary spans (splitting into <=360 day chunks due to Cost Management 1Y query cap).
func (az *AzureService) queryTotalRange(from, to time.Time) (float64, float64, error) {
	var totalUSD, totalBill float64
	cursor := from
	for cursor.Before(to) {
		chunkEnd := cursor.AddDate(0, 0, 360)
		if chunkEnd.After(to) {
			chunkEnd = to
		}
		cols, rows, err := az.queryCostManagement(map[string]interface{}{
			"type":      "ActualCost",
			"timeframe": "Custom",
			"timePeriod": map[string]interface{}{
				"from": cursor.Format("2006-01-02T00:00:00+00:00"),
				"to":   chunkEnd.Format("2006-01-02T00:00:00+00:00"),
			},
			"dataset": map[string]interface{}{
				"granularity": "None",
				"aggregation": map[string]interface{}{
					"totalCost": map[string]interface{}{
						"name":     "Cost",
						"function": "Sum",
					},
					"totalCostUSD": map[string]interface{}{
						"name":     "CostUSD",
						"function": "Sum",
					},
				},
			},
		})
		if err != nil {
			return 0, 0, err
		}
		idx := buildColumnIndex(cols)
		for _, row := range rows {
			totalBill += rowFloat(row, idx, "Cost")
			totalUSD += rowFloat(row, idx, "CostUSD")
		}
		if !chunkEnd.After(cursor) {
			break
		}
		cursor = chunkEnd
	}
	return totalUSD, totalBill, nil
}

func (az *AzureService) queryCostManagement(payload map[string]interface{}) ([]costColumn, [][]interface{}, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	endpoint := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/providers/Microsoft.CostManagement/query?api-version=2023-11-01",
		url.PathEscape(az.Account.SubId),
	)

	var allCols []costColumn
	var allRows [][]interface{}
	nextURL := endpoint

	for page := 0; nextURL != "" && page < 10; page++ {
		resp, err := az.requestWithRetry("POST", nextURL, bodyBytes, 3)
		if err != nil {
			return nil, nil, fmt.Errorf("Cost Management 请求失败: %w", err)
		}
		raw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, nil, fmt.Errorf("读取 Cost Management 响应失败: %w", readErr)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, nil, fmt.Errorf("Cost Management API 返回 HTTP %d: %s", resp.StatusCode, truncateErrBody(raw))
		}
		var parsed costQueryResponse
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, nil, fmt.Errorf("解析 Cost Management 响应失败: %w", err)
		}
		if len(allCols) == 0 {
			allCols = parsed.Properties.Columns
		}
		allRows = append(allRows, parsed.Properties.Rows...)
		nextURL = strings.TrimSpace(parsed.Properties.NextLink)
	}

	return allCols, allRows, nil
}

func buildColumnIndex(cols []costColumn) map[string]int {
	idx := make(map[string]int, len(cols))
	for i, c := range cols {
		idx[c.Name] = i
	}
	return idx
}

func rowFloat(row []interface{}, idx map[string]int, col string) float64 {
	i, ok := idx[col]
	if !ok || i < 0 || i >= len(row) {
		return 0
	}
	v, ok := row[i].(float64)
	if !ok {
		return 0
	}
	return v
}

func rowString(row []interface{}, idx map[string]int, col string) string {
	i, ok := idx[col]
	if !ok || i < 0 || i >= len(row) || row[i] == nil {
		return ""
	}
	if s, ok := row[i].(string); ok {
		return s
	}
	return fmt.Sprintf("%v", row[i])
}

func truncateErrBody(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > 400 {
		return s[:400] + "..."
	}
	return s
}
