package gcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/crossgg/cloud-vm-manager/internal/provider"
)

func TestGCPIPManagementAutoVPCEnableDualStackAndSwap(t *testing.T) {
	origBase := gcpComputeBaseURL
	origPoll := gcpPollInterval
	gcpPollInterval = 2 * time.Millisecond
	defer func() {
		gcpComputeBaseURL = origBase
		gcpPollInterval = origPoll
	}()

	isAutoMode := true
	subStackType := "IPV4_ONLY"
	subAccessType := ""
	subExtPrefix := ""
	nicStackType := "IPV4_ONLY"
	nicV6Configs := []map[string]interface{}{}
	nicNatIP := "34.1.1.10"
	v6Counter := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path

		switch {
		case r.Method == http.MethodGet && p == "/projects/proj-1/zones/us-central1-a/instances/vm-1":
			nic := map[string]interface{}{
				"name":        "nic0",
				"fingerprint": "fp-nic0",
				"network":     "https://www.googleapis.com/compute/v1/projects/proj-1/global/networks/default",
				"subnetwork":  "https://www.googleapis.com/compute/v1/projects/proj-1/regions/us-central1/subnetworks/default",
				"networkIP":   "10.128.0.2",
				"stackType":   nicStackType,
			}
			if nicNatIP != "" {
				nic["accessConfigs"] = []map[string]interface{}{
					{"name": "External NAT", "natIP": nicNatIP, "networkTier": "PREMIUM"},
				}
			}
			if len(nicV6Configs) > 0 {
				nic["ipv6AccessConfigs"] = nicV6Configs
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":              "vm-1",
				"zone":              "projects/proj-1/zones/us-central1-a",
				"machineType":       "projects/proj-1/zones/us-central1-a/machineTypes/e2-micro",
				"status":            "RUNNING",
				"networkInterfaces": []map[string]interface{}{nic},
			})

		case r.Method == http.MethodGet && p == "/projects/proj-1/regions/us-central1/addresses":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"name":        "reserved-free",
						"address":     "34.1.1.99",
						"status":      "RESERVED",
						"addressType": "EXTERNAL",
						"ipVersion":   "IPV4",
						"networkTier": "PREMIUM",
					},
				},
			})

		case r.Method == http.MethodGet && p == "/projects/proj-1/global/firewalls":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"name":         "default-allow-ssh",
						"network":      "projects/proj-1/global/networks/default",
						"direction":    "INGRESS",
						"sourceRanges": []string{"0.0.0.0/0"},
						"allowed":      []map[string]interface{}{{"IPProtocol": "tcp", "ports": []string{"22"}}},
					},
				},
			})

		case r.Method == http.MethodGet && p == "/projects/proj-1/global/networks/default":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":                  "default",
				"autoCreateSubnetworks": isAutoMode,
			})

		case r.Method == http.MethodPost && p == "/projects/proj-1/global/networks/default/switchToCustomMode":
			isAutoMode = false
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"name": "op-custom", "status": "DONE"})

		case r.Method == http.MethodGet && p == "/projects/proj-1/regions/us-central1/subnetworks/default":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":               "default",
				"fingerprint":        "fp-sub",
				"ipCidrRange":        "10.128.0.0/20",
				"stackType":          subStackType,
				"ipv6AccessType":     subAccessType,
				"externalIpv6Prefix": subExtPrefix,
			})

		case r.Method == http.MethodPatch && p == "/projects/proj-1/regions/us-central1/subnetworks/default":
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			subStackType = stringValue(body["stackType"])
			subAccessType = stringValue(body["ipv6AccessType"])
			subExtPrefix = "2600:1900:4001:abcd::/64"
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"name": "op-sub", "status": "DONE"})

		case r.Method == http.MethodPatch && p == "/projects/proj-1/zones/us-central1-a/instances/vm-1/updateNetworkInterface":
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			nicStackType = stringValue(body["stackType"])
			if nicStackType == "IPV4_IPV6" {
				v6Counter++
				addr := "2600:1900:4001:abcd:0:1::"
				if v6Counter > 1 {
					addr = "2600:1900:4001:abcd:0:2::"
				}
				nicV6Configs = []map[string]interface{}{
					{
						"name":                     "External IPv6",
						"type":                     "DIRECT_IPV6",
						"externalIpv6":             addr,
						"externalIpv6PrefixLength": float64(96),
						"networkTier":              "PREMIUM",
					},
				}
			} else {
				nicV6Configs = []map[string]interface{}{}
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"name": "op-nic", "status": "DONE"})

		default:
			t.Fatalf("unexpected GCP request: %s %s", r.Method, p)
		}
	}))
	defer srv.Close()
	gcpComputeBaseURL = srv.URL

	svc := NewGCPService(config.GCPConfig{Name: "gcp-1", ProjectID: "proj-1"})
	svc.token = "tok"
	svc.tokenExp = time.Now().Add(time.Hour)

	info, err := svc.GetIPManagement("us-central1-a|vm-1")
	if err != nil {
		t.Fatalf("GetIPManagement failed: %v", err)
	}
	if info.Provider != "gcp" || info.Capabilities.CanAddSecondaryPrivateIPv4 || len(info.ReservedIPs) != 1 {
		t.Fatalf("unexpected GCP capabilities or reserved IPs: %+v", info)
	}
	setup := info.VNICs[0].IPv6Setup
	if setup == nil || !setup.IsAutoModeVPC || !setup.RequiresConfirmation || !setup.NeedsFirewallIPv6Rule {
		t.Fatalf("unexpected GCP IPv6Setup: %+v", setup)
	}

	// Must reject EnableSubnetIPv6 without ConfirmIrreversible on Auto Mode VPC
	if _, _, err := svc.EnableSubnetIPv6("us-central1-a|vm-1", provider.EnableIPv6Request{}); err == nil {
		t.Fatalf("expected error requiring confirmation for Auto Mode VPC")
	}

	// Enable with ConfirmIrreversible=true
	prefix, _, err := svc.EnableSubnetIPv6("us-central1-a|vm-1", provider.EnableIPv6Request{ConfirmIrreversible: true})
	if err != nil {
		t.Fatalf("EnableSubnetIPv6 failed: %v", err)
	}
	if prefix != "2600:1900:4001:abcd::/64" || isAutoMode {
		t.Fatalf("unexpected subnet prefix=%s isAutoMode=%v", prefix, isAutoMode)
	}

	// Attach IPv6
	v6, err := svc.AttachIPv6("us-central1-a|vm-1", provider.AttachIPv6Request{VNICID: "nic0"})
	if err != nil {
		t.Fatalf("AttachIPv6 failed: %v", err)
	}
	if v6.IPAddress != "2600:1900:4001:abcd:0:1::" || v6.CIDRPrefixLength != 96 {
		t.Fatalf("unexpected attached IPv6: %+v", v6)
	}

	// GetVM must expose ipv6 & ipv6s
	vmInfo, err := svc.GetVM("us-central1-a|vm-1")
	if err != nil {
		t.Fatalf("GetVM failed: %v", err)
	}
	if vmInfo["ipv6"] != "2600:1900:4001:abcd:0:1::" {
		t.Fatalf("expected GetVM ipv6, got %v", vmInfo["ipv6"])
	}

	// Change IPv6
	swapRes, err := svc.ChangeIPv6("us-central1-a|vm-1", "nic0", v6.ID)
	if err != nil {
		t.Fatalf("ChangeIPv6 failed: %v", err)
	}
	if swapRes.OldIPAddress != "2600:1900:4001:abcd:0:1::" || swapRes.NewIPAddress != "2600:1900:4001:abcd:0:2::" {
		t.Fatalf("unexpected ChangeIPv6 result: %+v", swapRes)
	}

	// Delete IPv6
	oldIP, err := svc.DeleteIPv6("us-central1-a|vm-1", v6.ID)
	if err != nil || oldIP != "2600:1900:4001:abcd:0:2::" || nicStackType != "IPV4_ONLY" {
		t.Fatalf("DeleteIPv6 failed: old=%q stack=%s err=%v", oldIP, nicStackType, err)
	}
}

func TestGCPRejectsChangeIPWhenReservedAndSecondaryPrivateIP(t *testing.T) {
	origBase := gcpComputeBaseURL
	defer func() { gcpComputeBaseURL = origBase }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/projects/proj-1/zones/us-central1-a/instances/vm-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "vm-1",
				"networkInterfaces": []map[string]interface{}{{
					"name":          "nic0",
					"networkIP":     "10.128.0.2",
					"accessConfigs": []map[string]interface{}{{"name": "External NAT", "natIP": "34.1.1.50"}},
				}},
			})
		case "/projects/proj-1/regions/us-central1/addresses":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]interface{}{{
					"name":        "my-static",
					"address":     "34.1.1.50",
					"status":      "IN_USE",
					"addressType": "EXTERNAL",
					"ipVersion":   "IPV4",
				}},
			})
		}
	}))
	defer srv.Close()
	gcpComputeBaseURL = srv.URL

	svc := NewGCPService(config.GCPConfig{Name: "gcp-1", ProjectID: "proj-1"})
	svc.token = "tok"
	svc.tokenExp = time.Now().Add(time.Hour)

	if _, err := svc.ChangePrivateIPPublicIP("us-central1-a|vm-1", "nic0"); err == nil || !strings.Contains(err.Error(), "静态保留 IP") {
		t.Fatalf("expected error refusing swap of static reserved IP, got: %v", err)
	}
	if _, _, err := svc.AttachIPv4("us-central1-a|vm-1", provider.AttachIPv4Request{Mode: "secondary_private"}); err == nil {
		t.Fatalf("expected error rejecting secondary_private on GCP")
	}
}
