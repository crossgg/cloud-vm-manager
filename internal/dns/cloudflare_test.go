package dns

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestCloudflareUpdateForVMByIPsMatchesRecordType(t *testing.T) {
	var updated []map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"result":  []interface{}{},
			})
			return
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		updated = append(updated, payload)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"result":  map[string]interface{}{"id": "rec-1"},
		})
	}))
	defer srv.Close()

	cf := NewCloudflareService(&config.Config{
		Cloudflare: []config.CloudflareConfig{
			{Name: "cf1", APIToken: "tok", ZoneID: "zone1"},
		},
		DNSBindings: []config.DNSBinding{
			{Name: "b-v4", Cloudflare: "cf1", Provider: "oci", Account: "oci1", VM: "vm-1", Domain: "v4.example.com", Type: "A", TTL: 60},
			{Name: "b-v6", Cloudflare: "cf1", Provider: "oci", Account: "oci1", VM: "vm-1", Domain: "v6.example.com", Type: "AAAA", TTL: 60},
		},
	})
	cf.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			cloned := req.Clone(req.Context())
			cloned.URL.Scheme = "http"
			cloned.URL.Host = strings.TrimPrefix(srv.URL, "http://")
			return srv.Client().Transport.RoundTrip(cloned)
		}),
	}

	hasA, hasAAAA := cf.BindingTypes("oci", "oci1", "vm-1")
	if !hasA || !hasAAAA {
		t.Fatalf("expected both A and AAAA bindings, got hasA=%v hasAAAA=%v", hasA, hasAAAA)
	}

	// Only IPv6 supplied -> only AAAA record should be updated.
	updated = nil
	logs := cf.UpdateForVM("oci", "oci1", "vm-1", "2603:c020::1")
	if len(updated) != 1 || updated[0]["type"] != "AAAA" || updated[0]["content"] != "2603:c020::1" {
		t.Fatalf("expected only AAAA update, got %+v logs=%v", updated, logs)
	}

	// Both IPv4 and IPv6 supplied -> both A and AAAA updated with matching IP.
	updated = nil
	logs = cf.UpdateForVMByIPs("oci", "oci1", "vm-1", "1.2.3.4", "2603:c020::2")
	if len(updated) != 2 {
		t.Fatalf("expected 2 updates, got %+v logs=%v", updated, logs)
	}
	if updated[0]["type"] != "A" || updated[0]["content"] != "1.2.3.4" {
		t.Fatalf("unexpected first update: %+v", updated[0])
	}
	if updated[1]["type"] != "AAAA" || updated[1]["content"] != "2603:c020::2" {
		t.Fatalf("unexpected second update: %+v", updated[1])
	}
}
