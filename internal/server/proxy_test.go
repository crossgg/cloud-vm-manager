package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/crossgg/cloud-vm-manager/internal/provider"
	"github.com/crossgg/cloud-vm-manager/internal/provider/azure"
	"github.com/crossgg/cloud-vm-manager/internal/provider/gcp"
	"github.com/crossgg/cloud-vm-manager/internal/provider/oci"
)

func TestParseProxyImport(t *testing.T) {
	proxies, importErrors := parseProxyImport("http://127.0.0.1:8080 # first\nsock5://127.0.0.1:1080 # second\n")
	if len(importErrors) != 0 {
		t.Fatalf("parseProxyImport returned errors: %v", importErrors)
	}
	if len(proxies) != 2 {
		t.Fatalf("parseProxyImport returned %d proxies, want 2", len(proxies))
	}
	if proxies[0].Remark != "first" || proxies[1].Remark != "second" {
		t.Fatalf("unexpected remarks: %#v", proxies)
	}
	if proxies[1].URL != "socks5://127.0.0.1:1080" {
		t.Fatalf("sock5 alias was not normalized: %q", proxies[1].URL)
	}
}

func TestProxyPasswordMaskAndRestore(t *testing.T) {
	original := config.ProxyConfig{ID: "proxy", URL: "http://user:secret@127.0.0.1:8080", Remark: "route"}
	masked := maskProxy(original)
	if strings.Contains(masked.URL, "secret") || !strings.Contains(masked.URL, proxyPasswordMask) {
		t.Fatalf("maskProxy(%q) = %q", original.URL, masked.URL)
	}
	restored := restoreMaskedProxyPassword(masked.URL, original.URL)
	if restored != original.URL {
		t.Fatalf("restoreMaskedProxyPassword() = %q, want %q", restored, original.URL)
	}
	changedUser := strings.Replace(masked.URL, "user:", "other:", 1)
	if got := restoreMaskedProxyPassword(changedUser, original.URL); got != "http://other:secret@127.0.0.1:8080" {
		t.Fatalf("restore with changed username = %q", got)
	}
}

func TestBuildCloudRuntimeAppliesProxyToWholeAccount(t *testing.T) {
	cfg := &config.Config{
		AzureAccounts: []config.AzureConfig{{Name: "azure-account"}},
		GCPAccounts:   []config.GCPConfig{{Name: "gcp-account"}},
		OCIAccounts:   []config.OCIConfig{{Name: "oci-account"}},
		Proxies:       []config.ProxyConfig{{ID: "primary", URL: "http://127.0.0.1:8080"}},
		ProxyBindings: []config.ProxyBinding{
			{Provider: "azure", Account: "azure-account", ProxyID: "primary", FallbackProxyID: config.ProxyFallbackDirect},
			{Provider: "gcp", Account: "gcp-account", ProxyID: "primary", FallbackProxyID: config.ProxyFallbackDirect},
			{Provider: "oci", Account: "oci-account", ProxyID: "primary", FallbackProxyID: config.ProxyFallbackDirect},
		},
	}
	runtime, err := buildCloudRuntime(cfg)
	if err != nil {
		t.Fatalf("buildCloudRuntime: %v", err)
	}
	services := []provider.CloudService{
		runtime.cloudServices[serviceKey("azure", "azure-account")],
		runtime.cloudServices[serviceKey("gcp", "gcp-account")],
		runtime.cloudServices[serviceKey("oci", "oci-account")],
	}
	for _, service := range services {
		var client *http.Client
		switch typed := service.(type) {
		case *azure.AzureService:
			client = typed.Client
		case *gcp.GCPService:
			client = typed.Client
		case *oci.OCIService:
			client = typed.Client
		default:
			t.Fatalf("unexpected service type %T", service)
		}
		if client.Transport == nil {
			t.Fatalf("service %T transport is nil, want account proxy transport", service)
		}
	}
}
