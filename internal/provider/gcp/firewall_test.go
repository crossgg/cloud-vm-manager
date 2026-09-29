package gcp

import (
	"testing"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

func TestGCPFirewallPayload(t *testing.T) {
	service := NewGCPService(config.GCPConfig{
		ProjectID: "my-test-project",
	})

	rule := GCPFirewallRule{
		Name:         "allow-web",
		Priority:     1000,
		Direction:    "INGRESS",
		Action:       "ALLOW",
		IPProtocol:   "tcp",
		Ports:        []string{"80", "443"},
		SourceRanges: []string{"0.0.0.0/0"},
		TargetTags:   []string{"web-server"},
		Disabled:     false,
	}

	payload := service.buildFirewallPayload(rule)

	if payload["name"] != "allow-web" {
		t.Errorf("expected name allow-web, got %v", payload["name"])
	}
	if payload["network"] != "projects/my-test-project/global/networks/default" {
		t.Errorf("expected default network, got %v", payload["network"])
	}
	if payload["priority"] != 1000 {
		t.Errorf("expected priority 1000, got %v", payload["priority"])
	}
	allowed, ok := payload["allowed"].([]map[string]interface{})
	if !ok || len(allowed) != 1 {
		t.Fatalf("expected allowed list with 1 entry, got %v", payload["allowed"])
	}
	if allowed[0]["IPProtocol"] != "tcp" {
		t.Errorf("expected tcp protocol, got %v", allowed[0]["IPProtocol"])
	}

	// Test normalization
	rawItem := map[string]interface{}{
		"id":        "12345",
		"name":      "allow-ssh",
		"network":   "projects/my-test-project/global/networks/default",
		"priority":  float64(2000),
		"direction": "INGRESS",
		"sourceRanges": []interface{}{
			"1.2.3.4/32",
		},
		"targetTags": []interface{}{
			"ssh-node",
		},
		"allowed": []interface{}{
			map[string]interface{}{
				"IPProtocol": "tcp",
				"ports": []interface{}{
					"22",
				},
			},
		},
		"disabled": false,
	}

	norm := service.normalizeFirewallRule(rawItem)
	if norm.Name != "allow-ssh" || norm.Priority != 2000 || norm.Action != "ALLOW" {
		t.Errorf("normalization mismatch: %+v", norm)
	}
	if len(norm.Ports) != 1 || norm.Ports[0] != "22" {
		t.Errorf("expected port 22, got %v", norm.Ports)
	}
	if len(norm.TargetTags) != 1 || norm.TargetTags[0] != "ssh-node" {
		t.Errorf("expected targetTag ssh-node, got %v", norm.TargetTags)
	}
}
