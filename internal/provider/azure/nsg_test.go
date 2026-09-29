package azure

import (
	"testing"
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
				"destinationPortRange":     "80",
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

	if rules[1].Name != "Web" || rules[1].SourceAddressPrefix != "1.2.3.4,5.6.7.8" {
		t.Errorf("Web rule with multiple prefixes parsed incorrectly: %+v", rules[1])
	}
}

func TestExtractResourceGroupFromID(t *testing.T) {
	id := "/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Network/networkSecurityGroups/my-nsg"
	rg := extractResourceGroupFromID(id)
	if rg != "my-rg" {
		t.Errorf("expected my-rg, got %q", rg)
	}
}
