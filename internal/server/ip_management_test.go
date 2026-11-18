package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/crossgg/cloud-vm-manager/internal/dns"
	"github.com/crossgg/cloud-vm-manager/internal/provider"
	"github.com/gin-gonic/gin"
)

type stubIPManager struct {
	provider string
}

func (s *stubIPManager) ListVMs() ([]map[string]interface{}, error) { return nil, nil }
func (s *stubIPManager) GetVM(id string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"id":       id,
		"publicIP": map[string]interface{}{"ipAddress": "1.2.3.4"},
		"ipv6":     "2600:1900::1",
	}, nil
}
func (s *stubIPManager) StartVM(id string) error   { return nil }
func (s *stubIPManager) StopVM(id string) error    { return nil }
func (s *stubIPManager) RestartVM(id string) error { return nil }
func (s *stubIPManager) ChangeIP(id string) (*provider.ChangeIPResult, error) {
	return &provider.ChangeIPResult{Success: true, NewIPAddress: "5.6.7.8"}, nil
}
func (s *stubIPManager) GetIPManagement(instanceID string) (*provider.IPManagementInfo, error) {
	return &provider.IPManagementInfo{
		Provider:     s.provider,
		InstanceID:   instanceID,
		InstanceName: "test-vm",
		Capabilities: provider.IPCapabilities{CanAttachIPv6: true},
		VNICs:        []provider.VNICIPInfo{{ID: "nic0", DisplayName: "nic0", IsPrimary: true}},
	}, nil
}
func (s *stubIPManager) ChangePrivateIPPublicIP(instanceID, privateIPID string) (*provider.ChangeIPResult, error) {
	return &provider.ChangeIPResult{Success: true, NewIPAddress: "5.6.7.8"}, nil
}
func (s *stubIPManager) AttachIPv4(instanceID string, req provider.AttachIPv4Request) ([]string, string, error) {
	return []string{"ok"}, "5.6.7.9", nil
}
func (s *stubIPManager) DeleteIPv4Managed(instanceID string, req provider.DeleteIPv4Request) ([]string, error) {
	return []string{"deleted"}, nil
}
func (s *stubIPManager) AttachReservedIPv4Managed(instanceID string, req provider.AttachReservedIPv4Request) (string, []string, error) {
	return "5.6.7.10", []string{"reserved"}, nil
}
func (s *stubIPManager) AttachIPv6(instanceID string, req provider.AttachIPv6Request) (*provider.IPv6Info, error) {
	return &provider.IPv6Info{ID: "v6-1", IPAddress: "2600:1900::2", CIDRPrefixLength: 96, CanChange: true}, nil
}
func (s *stubIPManager) ChangeIPv6(instanceID, vnicID, ipv6ID string) (*provider.ChangeIPv6Result, error) {
	return &provider.ChangeIPv6Result{Success: true, OldIPAddress: "2600:1900::1", NewIPAddress: "2600:1900::3"}, nil
}
func (s *stubIPManager) DeleteIPv6(instanceID, ipv6ID string) (string, error) {
	return "2600:1900::1", nil
}
func (s *stubIPManager) EnableSubnetIPv6(instanceID string, req provider.EnableIPv6Request) (string, []string, error) {
	return "2600:1900::/64", []string{"enabled"}, nil
}

func TestUnifiedIPManagementRoutesSupportAzureAndGCP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	runtimeState.mu.Lock()
	origServices := runtimeState.cloudServices
	origCF := runtimeState.cloudflare
	runtimeState.cloudServices = map[string]provider.CloudService{
		serviceKey("azure", "az1"): &stubIPManager{provider: "azure"},
		serviceKey("gcp", "g1"):    &stubIPManager{provider: "gcp"},
	}
	runtimeState.cloudflare = dns.NewCloudflareService(&config.Config{})
	runtimeState.mu.Unlock()
	t.Cleanup(func() {
		runtimeState.mu.Lock()
		runtimeState.cloudServices = origServices
		runtimeState.cloudflare = origCF
		runtimeState.mu.Unlock()
	})

	r := gin.New()
	r.GET("/api/vm/:provider/:account/:name/ips", getOCIInstanceIPs)
	r.POST("/api/vm/:provider/:account/:name/ips/enable-ipv6", enableOCISubnetIPv6)
	r.POST("/api/vm/:provider/:account/:name/ips/change-ipv6", changeOCIIPv6)

	for _, tc := range []struct {
		provider string
		account  string
		vm       string
	}{
		{"azure", "az1", "vm-az"},
		{"gcp", "g1", "us-central1-a|vm-gcp"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/vm/"+tc.provider+"/"+tc.account+"/"+tc.vm+"/ips", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s GET ips returned %d: %s", tc.provider, w.Code, w.Body.String())
		}
		var resp provider.IPManagementInfo
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if resp.Provider != tc.provider || !resp.Capabilities.CanAttachIPv6 {
			t.Fatalf("unexpected IPManagementInfo for %s: %+v", tc.provider, resp)
		}

		body, _ := json.Marshal(provider.EnableIPv6Request{ConfirmIrreversible: true})
		req2 := httptest.NewRequest(http.MethodPost, "/api/vm/"+tc.provider+"/"+tc.account+"/"+tc.vm+"/ips/enable-ipv6", bytes.NewReader(body))
		req2.Header.Set("Content-Type", "application/json")
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)
		if w2.Code != http.StatusOK {
			t.Fatalf("%s POST enable-ipv6 returned %d: %s", tc.provider, w2.Code, w2.Body.String())
		}
	}
}

func TestPrimaryIPv6AddressHandlesGCP96AndAzure128(t *testing.T) {
	gcpVM := map[string]interface{}{
		"ipv6": "2600:1900:4001::",
		"ipv6s": []map[string]interface{}{
			{"id": "nic0|ipv6|0", "ipAddress": "2600:1900:4001::", "cidrPrefixLength": 96, "canChange": true},
		},
	}
	if got, ok := primaryIPv6Address(gcpVM); !ok || got != "2600:1900:4001::" {
		t.Fatalf("expected GCP /96 primary IPv6, got ok=%v ip=%q", ok, got)
	}

	ociPrefixOnlyVM := map[string]interface{}{
		"ipv6s": []map[string]interface{}{
			{"id": "v6-80", "ipAddress": "2603:c020:1:2::", "cidrPrefixLength": 80, "canChange": false},
		},
	}
	if got, ok := primaryIPv6Address(ociPrefixOnlyVM); ok || got != "" {
		t.Fatalf("expected /80 prefix not to be picked as primary IPv6, got ok=%v ip=%q", ok, got)
	}
}
