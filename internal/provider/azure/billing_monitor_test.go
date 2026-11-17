package azure

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body interface{}) *http.Response {
	b, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(b)),
	}
}

func TestEvaluateCreditTriggers(t *testing.T) {
	rem := 8.5
	days := 5
	ov := &AzureCostOverview{
		CostStatus:            "ok",
		MonthToDateCostUSD:    12.0,
		EstimatedRemainingUSD: &rem,
		DaysUntilExpire:       &days,
	}

	cfg := config.AzureCreditMonitorConfig{
		MinRemainingCredit: 10,
		DaysBeforeExpire:   7,
		MaxMonthlyCost:     10,
	}
	triggered, reasons := EvaluateCreditTriggers(cfg, ov)
	if !triggered || len(reasons) != 3 {
		t.Fatalf("expected all 3 triggers, got triggered=%v reasons=%v", triggered, reasons)
	}

	// When cost/date fields are unset, should not trigger
	ovMissing := &AzureCostOverview{CostStatus: "error"}
	triggered, reasons = EvaluateCreditTriggers(cfg, ovMissing)
	if triggered || len(reasons) != 0 {
		t.Fatalf("expected no triggers when cost error and dates unset, got %v %v", triggered, reasons)
	}
}

func TestGetCostAndCreditOverviewMock(t *testing.T) {
	az := NewAzureAccountService(&config.Config{}, config.AzureConfig{
		Name:          "az001",
		TenantId:      "tenant-1",
		ClientId:      "client-1",
		ClientSecret:  "secret-1",
		SubId:         "sub-1",
		ResourceGroup: "stu11",
	})
	az.token = "mock-token"
	az.tokenExp = time.Now().Add(time.Hour)

	az.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			u := req.URL.String()
			if strings.Contains(u, "/subscriptions/sub-1?") {
				return jsonResponse(200, map[string]interface{}{
					"displayName": "Azure for Students",
					"state":       "Enabled",
					"subscriptionPolicies": map[string]interface{}{
						"quotaId":       "AzureForStudents_2018-01-01",
						"spendingLimit": "On",
					},
				}), nil
			}
			if strings.Contains(u, "Microsoft.CostManagement/query") {
				raw, _ := io.ReadAll(req.Body)
				bodyStr := string(raw)
				if strings.Contains(bodyStr, `"MonthToDate"`) {
					return jsonResponse(200, map[string]interface{}{
						"properties": map[string]interface{}{
							"columns": []map[string]string{
								{"name": "Cost", "type": "Number"},
								{"name": "CostUSD", "type": "Number"},
								{"name": "ServiceName", "type": "String"},
								{"name": "Currency", "type": "String"},
							},
							"rows": [][]interface{}{
								{12.80, 1.92, "Virtual Network", "CNY"},
								{0.0, 0.0, "Virtual Machines", "CNY"},
							},
						},
					}), nil
				}
				if strings.Contains(bodyStr, `"Monthly"`) {
					return jsonResponse(200, map[string]interface{}{
						"properties": map[string]interface{}{
							"columns": []map[string]string{
								{"name": "Cost", "type": "Number"},
								{"name": "CostUSD", "type": "Number"},
								{"name": "BillingMonth", "type": "Datetime"},
								{"name": "Currency", "type": "String"},
							},
							"rows": [][]interface{}{
								{24.0, 3.60, "2026-10-01T00:00:00", "CNY"},
								{12.80, 1.92, "2026-11-01T00:00:00", "CNY"},
							},
						},
					}), nil
				}
				// StartDate custom range total
				return jsonResponse(200, map[string]interface{}{
					"properties": map[string]interface{}{
						"columns": []map[string]string{
							{"name": "Cost", "type": "Number"},
							{"name": "CostUSD", "type": "Number"},
							{"name": "Currency", "type": "String"},
						},
						"rows": [][]interface{}{
							{176.6, 26.49, "CNY"},
						},
					},
				}), nil
			}
			return jsonResponse(404, map[string]string{"error": "not mocked"}), nil
		}),
	}

	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

	// 1. With credit config
	ov, err := az.getCostAndCreditOverviewAt(config.AzureCreditMonitorConfig{
		InitialCredit: 100,
		StartDate:     "2026-04-01",
		ExpireDate:    "2027-04-01",
	}, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ov.IsStudentOffer || ov.CostStatus != "ok" || !ov.HasCreditConfig {
		t.Fatalf("unexpected overview flags: %+v", ov)
	}
	if ov.EstimatedRemainingUSD == nil || *ov.EstimatedRemainingUSD != 73.51 {
		t.Fatalf("expected remaining 73.51, got %v", ov.EstimatedRemainingUSD)
	}
	if ov.DaysUntilExpire == nil || *ov.DaysUntilExpire != 135 {
		t.Fatalf("expected 135 days until expire, got %v", ov.DaysUntilExpire)
	}

	// 2. Without credit config
	ovNoCfg, err := az.getCostAndCreditOverviewAt(config.AzureCreditMonitorConfig{}, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ovNoCfg.HasCreditConfig || len(ovNoCfg.MissingCreditFields) != 3 {
		t.Fatalf("expected 3 missing fields, got %v", ovNoCfg.MissingCreditFields)
	}
	if ovNoCfg.EstimatedRemainingUSD != nil || ovNoCfg.DaysUntilExpire != nil {
		t.Fatalf("expected nil remaining/days when unset")
	}
}

func TestScanAndCleanupResourcesMock(t *testing.T) {
	az := NewAzureAccountService(&config.Config{}, config.AzureConfig{
		Name:          "az001",
		SubId:         "sub-1",
		ResourceGroup: "stu11",
	})
	az.token = "mock-token"
	az.tokenExp = time.Now().Add(time.Hour)

	var deletedPaths []string
	az.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			u := req.URL.String()
			if req.Method == "DELETE" {
				deletedPaths = append(deletedPaths, req.URL.Path)
				return jsonResponse(200, map[string]interface{}{}), nil
			}
			switch {
			case strings.Contains(u, "Microsoft.Compute/virtualMachines"):
				return jsonResponse(200, map[string]interface{}{
					"value": []map[string]interface{}{
						{
							"id":       "/subscriptions/sub-1/resourceGroups/stu11/providers/Microsoft.Compute/virtualMachines/krkr",
							"name":     "krkr",
							"type":     "Microsoft.Compute/virtualMachines",
							"location": "koreacentral",
							"properties": map[string]interface{}{
								"hardwareProfile": map[string]interface{}{"vmSize": "Standard_B2ats_v2"},
							},
						},
					},
				}), nil
			case strings.Contains(u, "Microsoft.Network/networkInterfaces"):
				return jsonResponse(200, map[string]interface{}{
					"value": []map[string]interface{}{
						{
							"id":       "/subscriptions/sub-1/resourceGroups/stu11/providers/Microsoft.Network/networkInterfaces/krkr27",
							"name":     "krkr27",
							"type":     "Microsoft.Network/networkInterfaces",
							"location": "koreacentral",
							"properties": map[string]interface{}{
								"virtualMachine": map[string]interface{}{
									"id": "/subscriptions/sub-1/resourceGroups/stu11/providers/Microsoft.Compute/virtualMachines/krkr",
								},
								"ipConfigurations": []map[string]interface{}{
									{
										"properties": map[string]interface{}{
											"publicIPAddress": map[string]interface{}{
												"id": "/subscriptions/sub-1/resourceGroups/stu11/providers/Microsoft.Network/publicIPAddresses/pip-active",
											},
										},
									},
								},
							},
						},
					},
				}), nil
			case strings.Contains(u, "Microsoft.Network/publicIPAddresses"):
				return jsonResponse(200, map[string]interface{}{
					"value": []map[string]interface{}{
						{
							"id":       "/subscriptions/sub-1/resourceGroups/stu11/providers/Microsoft.Network/publicIPAddresses/pip-active",
							"name":     "pip-active",
							"type":     "Microsoft.Network/publicIPAddresses",
							"location": "koreacentral",
							"sku":      map[string]interface{}{"name": "Standard"},
							"properties": map[string]interface{}{
								"ipAddress":                "1.2.3.4",
								"publicIPAllocationMethod": "Static",
								"ipConfiguration": map[string]interface{}{
									"id": "/subscriptions/sub-1/resourceGroups/stu11/providers/Microsoft.Network/networkInterfaces/krkr27/ipConfigurations/ipconfig1",
								},
							},
						},
						{
							"id":       "/subscriptions/sub-1/resourceGroups/stu11/providers/Microsoft.Network/publicIPAddresses/pip-orphan",
							"name":     "pip-orphan",
							"type":     "Microsoft.Network/publicIPAddresses",
							"location": "koreacentral",
							"sku":      map[string]interface{}{"name": "Standard"},
							"properties": map[string]interface{}{
								"ipAddress":                "5.6.7.8",
								"publicIPAllocationMethod": "Static",
							},
						},
					},
				}), nil
			case strings.Contains(u, "Microsoft.Compute/disks"):
				return jsonResponse(200, map[string]interface{}{
					"value": []map[string]interface{}{
						{
							"id":        "/subscriptions/sub-1/resourceGroups/stu11/providers/Microsoft.Compute/disks/osdisk",
							"name":      "osdisk",
							"type":      "Microsoft.Compute/disks",
							"location":  "koreacentral",
							"managedBy": "/subscriptions/sub-1/resourceGroups/stu11/providers/Microsoft.Compute/virtualMachines/krkr",
							"sku":       map[string]interface{}{"name": "Premium_LRS"},
							"properties": map[string]interface{}{
								"diskSizeGB": 64,
								"diskState":  "Attached",
							},
						},
						{
							"id":       "/subscriptions/sub-1/resourceGroups/stu11/providers/Microsoft.Compute/disks/orphan-disk",
							"name":     "orphan-disk",
							"type":     "Microsoft.Compute/disks",
							"location": "koreacentral",
							"sku":      map[string]interface{}{"name": "Premium_LRS"},
							"properties": map[string]interface{}{
								"diskSizeGB": 128,
								"diskState":  "Unattached",
							},
						},
					},
				}), nil
			case strings.Contains(u, "Microsoft.Compute/snapshots"):
				return jsonResponse(200, map[string]interface{}{"value": []interface{}{}}), nil
			case strings.Contains(u, "/resources?"):
				return jsonResponse(200, map[string]interface{}{
					"value": []map[string]interface{}{
						{
							"id":       "/subscriptions/sub-1/resourceGroups/NetworkWatcherRG/providers/Microsoft.Network/networkWatchers/nw",
							"name":     "nw",
							"type":     "Microsoft.Network/networkWatchers",
							"location": "koreacentral",
						},
						{
							"id":       "/subscriptions/sub-1/resourceGroups/stu11/providers/Microsoft.Network/virtualNetworks/krkr-vnet",
							"name":     "krkr-vnet",
							"type":     "Microsoft.Network/virtualNetworks",
							"location": "koreacentral",
						},
					},
				}), nil
			}
			return jsonResponse(404, map[string]string{"error": "unexpected path: " + u}), nil
		}),
	}

	scan, err := az.ScanResources("rg")
	if err != nil {
		t.Fatalf("ScanResources failed: %v", err)
	}
	if scan.Summary.OrphanPublicIPCount != 1 || scan.Summary.OrphanDiskCount != 1 {
		t.Fatalf("unexpected orphan counts: %+v", scan.Summary)
	}

	// Cleanup only orphan IPs and orphan disks (must leave VM and active IP untouched)
	res, err := az.CleanupResources(AzureCleanupRequest{
		Mode:               "orphans",
		Scope:              "rg",
		IncludeOrphanIPs:   true,
		IncludeOrphanDisks: true,
	})
	if err != nil {
		t.Fatalf("CleanupResources failed: %v", err)
	}
	if len(res.Deleted) != 2 {
		t.Fatalf("expected 2 deleted resources, got %v (logs: %v)", res.Deleted, res.Logs)
	}
	if !strings.HasSuffix(deletedPaths[0], "/pip-orphan") || !strings.HasSuffix(deletedPaths[1], "/orphan-disk") {
		t.Fatalf("unexpected delete order or targets: %v", deletedPaths)
	}
}
