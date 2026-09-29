package server

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/crossgg/cloud-vm-manager/internal/provider/oci"
	"github.com/gin-gonic/gin"
)

func getOCIService(c *gin.Context) (*oci.OCIService, bool) {
	providerName := strings.ToLower(c.Param("provider"))
	if providerName != "oci" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "this operation is only supported for OCI accounts"})
		return nil, false
	}

	account := c.Param("account")
	service, _, ok := serviceSnapshot(providerName, account)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("provider/account %s/%s not found", providerName, account)})
		return nil, false
	}

	ociSvc, ok := service.(*oci.OCIService)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "selected service is not an OCI account"})
		return nil, false
	}
	return ociSvc, true
}

func findOCIConfig(account string) config.OCIConfig {
	runtimeState.mu.RLock()
	defer runtimeState.mu.RUnlock()
	if runtimeState.cfg != nil {
		for _, a := range runtimeState.cfg.OCIAccounts {
			if a.Name == account {
				return a
			}
		}
	}
	return config.OCIConfig{Name: account}
}

// --- Instance Edit Handlers ---

func getOCIInstanceEditOptions(c *gin.Context) {
	instanceID := c.Param("name")
	selectedShape := c.Query("shape")
	ociSvc, ok := getOCIService(c)
	if !ok {
		return
	}

	options, err := ociSvc.InstanceEditOptions(instanceID, selectedShape)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, options)
}

func updateOCIInstance(c *gin.Context) {
	instanceID := c.Param("name")
	ociSvc, ok := getOCIService(c)
	if !ok {
		return
	}

	var payload oci.OCIInstanceEditPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	vm, err := ociSvc.UpdateInstanceConfig(instanceID, payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "OCI 实例编辑请求已提交。",
		"vm":      vm,
	})
}

// --- Security List Handlers ---

func listOCISecurityLists(c *gin.Context) {
	instanceID := c.Param("name")
	ociSvc, ok := getOCIService(c)
	if !ok {
		return
	}

	lists, err := ociSvc.ListSecurityLists(instanceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"securityLists": lists})
}

func saveOCISecurityListRules(c *gin.Context) {
	instanceID := c.Param("name")
	listID := c.Param("listID")
	ociSvc, ok := getOCIService(c)
	if !ok {
		return
	}

	var payload struct {
		IngressRules []oci.OCISecurityRuleInput `json:"ingressRules"`
		EgressRules  []oci.OCISecurityRuleInput `json:"egressRules"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ociSvc.SaveSecurityListRules(instanceID, listID, payload.IngressRules, payload.EgressRules); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	lists, err := ociSvc.ListSecurityLists(instanceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "securityLists": lists})
}

func listAccountOCISecurityLists(c *gin.Context) {
	account := c.Param("account")
	service, _, ok := serviceSnapshot("oci", account)
	if !ok || service == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("OCI account %q not found", account)})
		return
	}
	ociSvc, ok := service.(*oci.OCIService)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("account %q is not an OCI service", account)})
		return
	}

	lists, err := ociSvc.ListAccountSecurityLists()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"securityLists": lists})
}

func saveAccountOCISecurityListRules(c *gin.Context) {
	account := c.Param("account")
	listID := c.Param("listID")
	service, _, ok := serviceSnapshot("oci", account)
	if !ok || service == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("OCI account %q not found", account)})
		return
	}
	ociSvc, ok := service.(*oci.OCIService)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("account %q is not an OCI service", account)})
		return
	}

	var payload struct {
		IngressRules []oci.OCISecurityRuleInput `json:"ingressRules"`
		EgressRules  []oci.OCISecurityRuleInput `json:"egressRules"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ociSvc.SaveAccountSecurityListRules(listID, payload.IngressRules, payload.EgressRules); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	lists, err := ociSvc.ListAccountSecurityLists()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "securityLists": lists})
}

// --- NSG Handlers ---

func listOCINetworkSecurityGroups(c *gin.Context) {
	instanceID := c.Param("name")
	ociSvc, ok := getOCIService(c)
	if !ok {
		return
	}

	groups, err := ociSvc.ListNetworkSecurityGroups(instanceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"networkSecurityGroups": groups})
}

func createOCINetworkSecurityGroup(c *gin.Context) {
	instanceID := c.Param("name")
	ociSvc, ok := getOCIService(c)
	if !ok {
		return
	}

	var payload struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	group, err := ociSvc.CreateAndAttachNetworkSecurityGroup(instanceID, payload.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	groups, err := ociSvc.ListNetworkSecurityGroups(instanceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "networkSecurityGroup": group})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "networkSecurityGroup": group, "networkSecurityGroups": groups})
}

func saveOCINetworkSecurityGroupRules(c *gin.Context) {
	instanceID := c.Param("name")
	groupID := c.Param("groupID")
	ociSvc, ok := getOCIService(c)
	if !ok {
		return
	}

	var payload struct {
		IngressRules []oci.OCISecurityRuleInput `json:"ingressRules"`
		EgressRules  []oci.OCISecurityRuleInput `json:"egressRules"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ociSvc.SaveNetworkSecurityGroupRules(instanceID, groupID, payload.IngressRules, payload.EgressRules); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	groups, err := ociSvc.ListNetworkSecurityGroups(instanceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "networkSecurityGroups": groups})
}

// --- Data Transfer Handlers & Monitors ---

var dtMonitors = struct {
	mu       sync.RWMutex
	monitors map[string]*oci.OCIDataTransferMonitor
}{
	monitors: make(map[string]*oci.OCIDataTransferMonitor),
}

func getOrCreateDTMonitor(account string) *oci.OCIDataTransferMonitor {
	dtMonitors.mu.RLock()
	monitor, ok := dtMonitors.monitors[account]
	dtMonitors.mu.RUnlock()
	if ok {
		return monitor
	}
	return nil
}

func setDTMonitor(account string, monitor *oci.OCIDataTransferMonitor) {
	dtMonitors.mu.Lock()
	defer dtMonitors.mu.Unlock()
	dtMonitors.monitors[account] = monitor
}

func InitDTMonitors() {
	runtimeState.mu.RLock()
	cfg := runtimeState.cfg
	runtimeState.mu.RUnlock()
	if cfg == nil {
		return
	}

	for _, account := range cfg.OCIAccounts {
		service, _, ok := serviceSnapshot("oci", account.Name)
		if !ok {
			continue
		}
		ociSvc, ok := service.(*oci.OCIService)
		if !ok {
			continue
		}
		dtCfg := account.DTMonitor
		monitor := oci.NewOCIDataTransferMonitor(ociSvc, account, dtCfg)
		setDTMonitor(account.Name, monitor)
		if dtCfg.Enabled {
			monitor.Start()
			fmt.Printf("OCI data transfer monitor started for account %s (interval=%ds, threshold=%.0fGB)\n",
				account.Name, dtCfg.Interval, dtCfg.Threshold)
		}
	}
}

func stopAllDTMonitors() {
	dtMonitors.mu.Lock()
	defer dtMonitors.mu.Unlock()
	for _, monitor := range dtMonitors.monitors {
		monitor.Stop()
	}
	dtMonitors.monitors = make(map[string]*oci.OCIDataTransferMonitor)
}

func getOCIDataTransfer(c *gin.Context) {
	account := c.Param("account")
	service, _, ok := serviceSnapshot("oci", account)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("OCI account %s not found", account)})
		return
	}
	ociSvc, ok := service.(*oci.OCIService)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not an OCI service"})
		return
	}

	monitor := getOrCreateDTMonitor(account)
	if monitor == nil {
		ociCfg := findOCIConfig(account)
		monitor = oci.NewOCIDataTransferMonitor(ociSvc, ociCfg, config.DataTransferConfig{
			Threshold: 9000,
		})
		setDTMonitor(account, monitor)
	}

	result := monitor.QueryNow()
	c.JSON(http.StatusOK, result)
}

func getDataTransferConfig(c *gin.Context) {
	account := c.Param("account")
	monitor := getOrCreateDTMonitor(account)
	if monitor == nil {
		c.JSON(http.StatusOK, config.DataTransferConfig{
			Interval:   300,
			Threshold:  9000,
			StopMethod: "soft",
		})
		return
	}
	cfg := monitor.GetConfig()
	c.JSON(http.StatusOK, cfg)
}

func saveDataTransferConfig(c *gin.Context) {
	account := c.Param("account")
	var payload config.DataTransferConfig
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	service, _, ok := serviceSnapshot("oci", account)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("OCI account %s not found", account)})
		return
	}
	ociSvc, ok := service.(*oci.OCIService)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not an OCI service"})
		return
	}

	monitor := getOrCreateDTMonitor(account)
	if monitor == nil {
		ociCfg := findOCIConfig(account)
		monitor = oci.NewOCIDataTransferMonitor(ociSvc, ociCfg, payload)
		setDTMonitor(account, monitor)
	}
	monitor.UpdateConfig(payload)

	runtimeState.mu.Lock()
	if runtimeState.cfg != nil {
		for i, a := range runtimeState.cfg.OCIAccounts {
			if a.Name == account {
				runtimeState.cfg.OCIAccounts[i].DTMonitor = payload
				break
			}
		}
	}
	configPath := runtimeState.configPath
	runtimeState.mu.Unlock()

	if configPath != "" {
		if err := config.SaveDTMonitorConfig(configPath, account, payload); err != nil {
			fmt.Printf("warning: failed to save OCI data transfer config to file: %v\n", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "config": payload})
}

func startDataTransferMonitor(c *gin.Context) {
	account := c.Param("account")
	monitor := getOrCreateDTMonitor(account)
	if monitor == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "monitor not initialized"})
		return
	}
	monitor.Start()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "monitoring started"})
}

func stopDataTransferMonitor(c *gin.Context) {
	account := c.Param("account")
	monitor := getOrCreateDTMonitor(account)
	if monitor == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "monitor not initialized"})
		return
	}
	monitor.Stop()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "monitoring stopped"})
}

func getDataTransferMonitorStatus(c *gin.Context) {
	account := c.Param("account")
	monitor := getOrCreateDTMonitor(account)
	if monitor == nil {
		c.JSON(http.StatusOK, gin.H{
			"running":    false,
			"lastResult": nil,
			"logs":       []string{},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"running":    monitor.IsRunning(),
		"lastResult": monitor.GetLastResult(),
		"logs":       monitor.GetLogs(),
	})
}
