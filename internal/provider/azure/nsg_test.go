package azure

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

func TestParseAzureRules(t *testing.T) {
	rawRules := []interface{}{
		map[string]interface{}{
			"name": "SSH",
			"properties": map[string]interface{}{
				"priority":                 float64(300),
				"direction":                "Inbound",
				"access":                   "Allow",
				"protocol":                 "Tcp",
				"sourceAddressPrefix":      "*",
				"sourcePortRange":          "*",
				"destinationAddressPrefix": "*",
				"destinationPortRange":     "22",
				"description":              "Allow SSH",
			},
		},
		map[string]interface{}{
			"name": "Web",
			"properties": map[string]interface{}{
				"priority":                 float64(310),
				"direction":                "Inbound",
				"access":                   "Allow",
				"protocol":                 "Tcp",
				"sourceAddressPrefixes":    []interface{}{"1.2.3.4", "5.6.7.8"},
				"sourcePortRange":          "*",
				"destinationAddressPrefix": "*",
				"destinationPortRanges":    []interface{}{"80", "443"},
			},
		},
	}

	rules := parseAzureRules(rawRules)
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}

	if rules[0].Name != "SSH" || rules[0].Priority != 300 || rules[0].DestinationPortRange != "22" {
		t.Errorf("SSH rule parsed incorrectly: %+v", rules[0])
	}

	if rules[1].Name != "Web" || rules[1].SourceAddressPrefix != "1.2.3.4,5.6.7.8" || rules[1].DestinationPortRange != "80,443" {
		t.Errorf("Web rule with multiple prefixes/ports parsed incorrectly: %+v", rules[1])
	}
}

func TestCreateOrUpdateNSGRulePluralFields(t *testing.T) {
	var sentBody map[string]interface{}
	svc := NewAzureAccountService(&config.Config{}, config.AzureConfig{
		Name:          "az-1",
		SubId:         "sub-1",
		ResourceGroup: "rg-1",
	})
	svc.token = "tok"
	svc.tokenExp = time.Now().Add(time.Hour)
	svc.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodPut {
				_ = json.NewDecoder(req.Body).Decode(&sentBody)
			}
			return jsonResponse(http.StatusOK, map[string]interface{}{"name": "Web"}), nil
		}),
	}

	err := svc.CreateOrUpdateNSGRule("rg-1", "nsg-1", AzureNSGRule{
		Name:                     "Web",
		Priority:                 310,
		Direction:                "Inbound",
		Access:                   "Allow",
		Protocol:                 "Tcp",
		SourceAddressPrefix:      "1.2.3.4, 5.6.7.8",
		SourcePortRange:          "*",
		DestinationAddressPrefix: "*",
		DestinationPortRange:     "80,443",
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateNSGRule failed: %v", err)
	}
	props, _ := sentBody["properties"].(map[string]interface{})
	if _, ok := props["destinationPortRanges"].([]interface{}); !ok {
		t.Fatalf("expected destinationPortRanges array, got %+v", props)
	}
	if _, ok := props["sourceAddressPrefixes"].([]interface{}); !ok {
		t.Fatalf("expected sourceAddressPrefixes array, got %+v", props)
	}
	if props["destinationPortRange"] != nil {
		t.Fatalf("expected destinationPortRange singular to be omitted when plural is used")
	}
}

func TestExtractResourceGroupFromID(t *testing.T) {
	id := "/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Network/networkSecurityGroups/my-nsg"
	rg := extractResourceGroupFromID(id)
	if rg != "my-rg" {
		t.Errorf("expected my-rg, got %q", rg)
	}
}
