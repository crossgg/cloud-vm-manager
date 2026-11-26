package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/crossgg/cloud-vm-manager/internal/dns"
	"github.com/gin-gonic/gin"
)

func TestDNSAccountsAndVMScopedBindingsAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmpDir := t.TempDir()
	mainConf := filepath.Join(tmpDir, "config.conf")
	dnsConf := filepath.Join(tmpDir, "dns.conf")

	cfg := &config.Config{
		DNSPath: dnsConf,
		DNSAccounts: []config.DNSAccountConfig{
			{
				Name:            "ali-prod",
				Provider:        "alidns",
				AccessKeyID:     "ak-1234",
				AccessKeySecret: "sk-secret-value-9999",
				Domains:         []string{"example.com", "example.org"},
			},
			{
				Name:     "cf-prod",
				Provider: "cloudflare",
				APIToken: "cf-tok-secret-8888",
				ZoneID:   "zone-abc",
				Domains:  []string{"cf.example.net"},
			},
		},
		DNSBindings: []config.DNSBinding{
			{
				Name:       "bind-1",
				DNSAccount: "ali-prod",
				Provider:   "oci",
				Account:    "oci1",
				VM:         "vm-a",
				Domain:     "a.example.com",
				Type:       "A",
				TTL:        600,
			},
			{
				Name:       "bind-2",
				DNSAccount: "cf-prod",
				Provider:   "azure",
				Account:    "az1",
				VM:         "vm-b",
				Domain:     "b.cf.example.net",
				Type:       "AAAA",
				TTL:        1,
				Proxied:    true,
			},
		},
	}

	runtimeState.mu.Lock()
	runtimeState.cfg = cfg
	runtimeState.configPath = mainConf
	runtimeState.cloudflare = dns.NewCloudflareService(cfg)
	runtimeState.mu.Unlock()

	r := SetupRouter(nil)

	// 1. Verify GET /api/dns/accounts masks secrets
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/dns/accounts", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/dns/accounts returned %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "sk-secret-value-9999") || strings.Contains(w.Body.String(), "cf-tok-secret-8888") {
		t.Fatalf("expected secrets to be masked in /api/dns/accounts response: %s", w.Body.String())
	}

	var accResp struct {
		Accounts []dnsAccountView `json:"accounts"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &accResp); err != nil {
		t.Fatalf("decode accounts resp: %v", err)
	}
	if len(accResp.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accResp.Accounts))
	}

	// 2. Verify domain boundary check rejects domain not configured on account
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/api/dns/console/records?account=ali-prod&domain=unconfigured.com", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when accessing unconfigured domain, got %d: %s", w2.Code, w2.Body.String())
	}

	// 3. Verify GET /api/vm/:provider/:account/:name/dns returns all dns_accounts and strict matched bindings
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodGet, "/api/vm/oci/oci1/vm-a/dns", nil)
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("GET vm dns returned %d: %s", w3.Code, w3.Body.String())
	}
	var vmResp struct {
		Bindings    []map[string]interface{} `json:"bindings"`
		DNSAccounts []map[string]interface{} `json:"dns_accounts"`
	}
	if err := json.Unmarshal(w3.Body.Bytes(), &vmResp); err != nil {
		t.Fatalf("decode vm dns resp: %v", err)
	}
	if len(vmResp.Bindings) != 1 || vmResp.Bindings[0]["domain"] != "a.example.com" {
		t.Fatalf("expected only vm-a binding, got %+v", vmResp.Bindings)
	}
	if len(vmResp.DNSAccounts) != 2 {
		t.Fatalf("expected 2 dns_accounts for VM modal, got %d", len(vmResp.DNSAccounts))
	}

	// 4. Verify POST /api/dns/bindings rejects incomplete wildcard bindings
	badPayload, _ := json.Marshal(map[string]interface{}{
		"bindings": []map[string]interface{}{
			{"dns_account": "ali-prod", "provider": "oci", "account": "oci1", "vm": "", "domain": "wild.example.com", "type": "A"},
		},
	})
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest(http.MethodPost, "/api/dns/bindings", bytes.NewReader(badPayload))
	req4.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w4, req4)
	if w4.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 rejecting wildcard binding, got %d: %s", w4.Code, w4.Body.String())
	}
}
