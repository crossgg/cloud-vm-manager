package server

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/auth"
	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/crossgg/cloud-vm-manager/internal/dns"
	"github.com/crossgg/cloud-vm-manager/internal/provider"
	"github.com/crossgg/cloud-vm-manager/internal/provider/azure"
	"github.com/crossgg/cloud-vm-manager/internal/provider/gcp"
	"github.com/crossgg/cloud-vm-manager/internal/provider/oci"
	"github.com/crossgg/cloud-vm-manager/internal/proxy"
	"github.com/gin-gonic/gin"
)

var runtimeState = &RuntimeState{}

type RuntimeState struct {
	mu              sync.RWMutex
	cfg             *config.Config
	configPath      string
	configModTime   time.Time
	azureService    *azure.AzureService
	azureServices   map[string]*azure.AzureService
	cloudServices   map[string]provider.CloudService
	cloudAccounts   []gin.H
	cloudflare      *dns.CloudflareService
	auth            *auth.AuthService
	lastReloadError string
}

func InitRuntime(cfg *config.Config, path string, authSvc *auth.AuthService) error {
	next, err := buildCloudRuntime(cfg)
	if err != nil {
		return err
	}
	runtimeState.mu.Lock()
	defer runtimeState.mu.Unlock()

	runtimeState.cfg = cfg
	runtimeState.configPath = path
	runtimeState.configModTime = configModTime(path)
	runtimeState.azureService = next.azureService
	runtimeState.azureServices = next.azureServices
	runtimeState.cloudServices = next.cloudServices
	runtimeState.cloudAccounts = next.cloudAccounts
	runtimeState.cloudflare = next.cloudflare
	runtimeState.auth = authSvc
	runtimeState.lastReloadError = ""
	return nil
}

func ReloadRuntimeConfig() error {
	cfg, path, err := config.LoadConfigWithPath()
	if err != nil {
		setReloadError(err)
		return err
	}
	next, err := buildCloudRuntime(cfg)
	if err != nil {
		setReloadError(err)
		return err
	}

	runtimeState.mu.RLock()
	authSvc := runtimeState.auth
	runtimeState.mu.RUnlock()
	if authSvc != nil {
		if err := authSvc.UpdateConfig(cfg.Auth); err != nil {
			setReloadError(err)
			return err
		}
	}

	runtimeState.mu.Lock()
	defer runtimeState.mu.Unlock()
	runtimeState.cfg = cfg
	runtimeState.configPath = path
	runtimeState.configModTime = configModTime(path)
	runtimeState.azureService = next.azureService
	runtimeState.azureServices = next.azureServices
	runtimeState.cloudServices = next.cloudServices
	runtimeState.cloudAccounts = next.cloudAccounts
	runtimeState.cloudflare = next.cloudflare
	runtimeState.lastReloadError = ""
	return nil
}

func currentConfigPath() string {
	runtimeState.mu.RLock()
	defer runtimeState.mu.RUnlock()
	return runtimeState.configPath
}

func currentDNSPath() string {
	runtimeState.mu.RLock()
	defer runtimeState.mu.RUnlock()
	if runtimeState.cfg != nil && runtimeState.cfg.DNSPath != "" {
		return runtimeState.cfg.DNSPath
	}
	return config.DNSConfigPath(runtimeState.configPath)
}

func currentAuthConfig() config.AuthConfig {
	runtimeState.mu.RLock()
	defer runtimeState.mu.RUnlock()
	if runtimeState.cfg == nil {
		return config.AuthConfig{}
	}
	return runtimeState.cfg.Auth
}

func currentAuthService() *auth.AuthService {
	runtimeState.mu.RLock()
	defer runtimeState.mu.RUnlock()
	return runtimeState.auth
}

func configStatus() gin.H {
	runtimeState.mu.RLock()
	defer runtimeState.mu.RUnlock()
	return gin.H{
		"path":            runtimeState.configPath,
		"lastReloadError": runtimeState.lastReloadError,
		"loadedAt":        runtimeState.configModTime.Format(time.RFC3339),
	}
}

func cloudAccountsSnapshot() []gin.H {
	runtimeState.mu.RLock()
	defer runtimeState.mu.RUnlock()
	accounts := make([]gin.H, len(runtimeState.cloudAccounts))
	copy(accounts, runtimeState.cloudAccounts)
	return accounts
}

func serviceSnapshot(providerName, account string) (provider.CloudService, *dns.CloudflareService, bool) {
	runtimeState.mu.RLock()
	defer runtimeState.mu.RUnlock()
	if account == "" {
		return runtimeState.azureService, runtimeState.cloudflare, runtimeState.azureService != nil
	}
	service, ok := runtimeState.cloudServices[serviceKey(providerName, account)]
	return service, runtimeState.cloudflare, ok
}

func setReloadError(err error) {
	runtimeState.mu.Lock()
	defer runtimeState.mu.Unlock()
	runtimeState.lastReloadError = err.Error()
}

func configModTime(path string) time.Time {
	if path == "" {
		return time.Time{}
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func serviceKey(providerName, account string) string {
	return strings.ToLower(providerName) + "/" + account
}

type cloudRuntime struct {
	azureService  *azure.AzureService
	azureServices map[string]*azure.AzureService
	cloudServices map[string]provider.CloudService
	cloudAccounts []gin.H
	cloudflare    *dns.CloudflareService
}

func buildCloudRuntime(cfg *config.Config) (cloudRuntime, error) {
	proxyRouter := proxy.NewProxyRouter(cfg.Proxies, cfg.ProxyBindings)
	next := cloudRuntime{
		azureServices: make(map[string]*azure.AzureService),
		cloudServices: make(map[string]provider.CloudService),
		cloudAccounts: nil,
		cloudflare:    dns.NewCloudflareService(cfg),
	}

	for _, account := range cfg.AzureAccounts {
		service := azure.NewAzureAccountService(cfg, account)
		client, err := proxyRouter.HTTPClient("azure", account.Name, 60*time.Second)
		if err != nil {
			return cloudRuntime{}, fmt.Errorf("configure Azure proxy for %s: %w", account.Name, err)
		}
		service.Client = client
		next.azureServices[account.Name] = service
		next.cloudServices[serviceKey("azure", account.Name)] = service
		next.cloudAccounts = append(next.cloudAccounts, gin.H{
			"provider": "azure",
			"account":  account.Name,
			"group":    account.Group,
		})
		if next.azureService == nil {
			next.azureService = service
		}
	}
	if next.azureService == nil {
		next.azureService = azure.NewAzureService(cfg)
		client, err := proxyRouter.HTTPClient("azure", cfg.Azure.Name, 60*time.Second)
		if err != nil {
			return cloudRuntime{}, fmt.Errorf("configure Azure proxy for %s: %w", cfg.Azure.Name, err)
		}
		next.azureService.Client = client
	}

	for _, account := range cfg.GCPAccounts {
		service := gcp.NewGCPService(account)
		client, err := proxyRouter.HTTPClient("gcp", account.Name, 60*time.Second)
		if err != nil {
			return cloudRuntime{}, fmt.Errorf("configure GCP proxy for %s: %w", account.Name, err)
		}
		service.Client = client
		next.cloudServices[serviceKey("gcp", account.Name)] = service
		next.cloudAccounts = append(next.cloudAccounts, gin.H{
			"provider": "gcp",
			"account":  account.Name,
			"group":    account.Group,
		})
	}
	for _, account := range cfg.OCIAccounts {
		service := oci.NewOCIService(account)
		client, err := proxyRouter.HTTPClient("oci", account.Name, 60*time.Second)
		if err != nil {
			return cloudRuntime{}, fmt.Errorf("configure OCI proxy for %s: %w", account.Name, err)
		}
		service.Client = client
		next.cloudServices[serviceKey("oci", account.Name)] = service
		next.cloudAccounts = append(next.cloudAccounts, gin.H{
			"provider": "oci",
			"account":  account.Name,
			"group":    account.Group,
		})
	}
	return next, nil
}
