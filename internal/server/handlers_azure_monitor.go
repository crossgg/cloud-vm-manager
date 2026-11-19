package server

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/crossgg/cloud-vm-manager/internal/cache"
	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/crossgg/cloud-vm-manager/internal/provider/azure"
	"github.com/gin-gonic/gin"
)

var azureMonitors = struct {
	mu       sync.RWMutex
	monitors map[string]*azure.AzureCreditMonitor
}{
	monitors: make(map[string]*azure.AzureCreditMonitor),
}

func getAzureMonitor(account string) *azure.AzureCreditMonitor {
	azureMonitors.mu.RLock()
	defer azureMonitors.mu.RUnlock()
	return azureMonitors.monitors[account]
}

func setAzureMonitor(account string, m *azure.AzureCreditMonitor) {
	if m != nil {
		m.SetOnAlert(onAzureCreditAlertCallback())
	}
	azureMonitors.mu.Lock()
	defer azureMonitors.mu.Unlock()
	azureMonitors.monitors[account] = m
}

func getOrCreateAzureMonitor(account string, azSvc *azure.AzureService) *azure.AzureCreditMonitor {
	if m := getAzureMonitor(account); m != nil {
		return m
	}
	azCfg := findAzureConfig(account)
	if azCfg.Name == "" && azSvc != nil {
		azCfg = azSvc.Account
	}
	m := azure.NewAzureCreditMonitor(azSvc, azCfg, azCfg.CreditMonitor)
	setAzureMonitor(account, m)
	return m
}

func findAzureConfig(account string) config.AzureConfig {
	runtimeState.mu.RLock()
	cfg := runtimeState.cfg
	runtimeState.mu.RUnlock()
	if cfg == nil {
		return config.AzureConfig{}
	}
	for _, a := range cfg.AzureAccounts {
		if strings.EqualFold(a.Name, account) {
			return a
		}
	}
	if account == "" || strings.EqualFold(account, cfg.Azure.Name) {
		return cfg.Azure
	}
	return config.AzureConfig{}
}

func InitAzureMonitors() {
	runtimeState.mu.RLock()
	cfg := runtimeState.cfg
	runtimeState.mu.RUnlock()
	if cfg == nil {
		return
	}

	for _, account := range cfg.AzureAccounts {
		service, _, ok := serviceSnapshot("azure", account.Name)
		if !ok {
			continue
		}
		azSvc, ok := service.(*azure.AzureService)
		if !ok {
			continue
		}
		creditCfg := account.CreditMonitor
		creditCfg.Normalize()
		monitor := azure.NewAzureCreditMonitor(azSvc, account, creditCfg)
		setAzureMonitor(account.Name, monitor)
		if creditCfg.Enabled {
			monitor.Start()
			fmt.Printf("Azure credit monitor started for account %s (interval=%ds, action=%s)\n",
				account.Name, creditCfg.Interval, creditCfg.TriggerAction)
		}
	}
}

func stopAllAzureMonitors() {
	azureMonitors.mu.Lock()
	defer azureMonitors.mu.Unlock()
	for _, monitor := range azureMonitors.monitors {
		monitor.Stop()
	}
	azureMonitors.monitors = make(map[string]*azure.AzureCreditMonitor)
}

func getAzureCostOverview(c *gin.Context) {
	azSvc, ok := getAzureService(c)
	if !ok {
		return
	}
	account := c.Param("account")
	monitor := getOrCreateAzureMonitor(account, azSvc)
	overview, err := monitor.QueryNow()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"overview": overview,
		"monitor":  monitor.GetStatus(),
	})
}

func scanAzureResources(c *gin.Context) {
	azSvc, ok := getAzureService(c)
	if !ok {
		return
	}
	scope := c.Query("scope")
	res, err := azSvc.ScanResources(scope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func cleanupAzureResources(c *gin.Context) {
	azSvc, ok := getAzureService(c)
	if !ok {
		return
	}
	account := c.Param("account")
	var req azure.AzureCleanupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := azSvc.CleanupResources(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Refresh VM cache after cleanup in case VMs were deleted
	if cStore := cache.Global(); cStore != nil {
		if vms, listErr := azSvc.ListVMs(); listErr == nil {
			cStore.UpdateAccount("azure", account, azSvc.Account.Group, vms)
		}
	}

	c.JSON(http.StatusOK, res)
}

func getAzureMonitorConfig(c *gin.Context) {
	azSvc, ok := getAzureService(c)
	if !ok {
		return
	}
	account := c.Param("account")
	monitor := getOrCreateAzureMonitor(account, azSvc)
	c.JSON(http.StatusOK, monitor.GetConfig())
}

func saveAzureMonitorConfig(c *gin.Context) {
	azSvc, ok := getAzureService(c)
	if !ok {
		return
	}
	account := c.Param("account")

	var payload config.AzureCreditMonitorConfig
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if payload.StartDate != "" && !isValidDateString(payload.StartDate) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "开始日期格式须为 YYYY-MM-DD"})
		return
	}
	if payload.ExpireDate != "" && !isValidDateString(payload.ExpireDate) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "到期日期格式须为 YYYY-MM-DD"})
		return
	}
	if payload.TriggerAction == "delete_all" && payload.Enabled && !payload.ConfirmAutoDeleteAll {
		c.JSON(http.StatusBadRequest, gin.H{"error": "开启自动删除全部计费资源时，必须勾选二次确认开关"})
		return
	}
	payload.Normalize()

	monitor := getOrCreateAzureMonitor(account, azSvc)
	monitor.UpdateConfig(payload)

	runtimeState.mu.Lock()
	if runtimeState.cfg != nil {
		for i, a := range runtimeState.cfg.AzureAccounts {
			if strings.EqualFold(a.Name, account) {
				runtimeState.cfg.AzureAccounts[i].CreditMonitor = payload
				break
			}
		}
		if strings.EqualFold(runtimeState.cfg.Azure.Name, account) {
			runtimeState.cfg.Azure.CreditMonitor = payload
		}
	}
	configPath := runtimeState.configPath
	runtimeState.mu.Unlock()

	if configPath != "" {
		if err := config.SaveAzureCreditConfig(configPath, account, payload); err != nil {
			fmt.Printf("warning: failed to save Azure credit monitor config: %v\n", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"config":  payload,
		"status":  monitor.GetStatus(),
	})
}

func startAzureMonitor(c *gin.Context) {
	azSvc, ok := getAzureService(c)
	if !ok {
		return
	}
	account := c.Param("account")
	monitor := getOrCreateAzureMonitor(account, azSvc)
	monitor.Start()
	c.JSON(http.StatusOK, gin.H{"success": true, "status": monitor.GetStatus()})
}

func stopAzureMonitor(c *gin.Context) {
	account := c.Param("account")
	monitor := getAzureMonitor(account)
	if monitor != nil {
		monitor.Stop()
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func getAzureMonitorStatus(c *gin.Context) {
	azSvc, ok := getAzureService(c)
	if !ok {
		return
	}
	account := c.Param("account")
	monitor := getOrCreateAzureMonitor(account, azSvc)
	c.JSON(http.StatusOK, monitor.GetStatus())
}

func isValidDateString(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) != 10 {
		return false
	}
	for i, ch := range s {
		if i == 4 || i == 7 {
			if ch != '-' {
				return false
			}
		} else if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
