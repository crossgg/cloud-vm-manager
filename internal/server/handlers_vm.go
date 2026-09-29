package server

import (
	"fmt"
	"net/http"

	"github.com/crossgg/cloud-vm-manager/internal/cache"
	"github.com/crossgg/cloud-vm-manager/internal/dns"
	"github.com/crossgg/cloud-vm-manager/internal/provider"
	"github.com/gin-gonic/gin"
)

func listAccounts(c *gin.Context) {
	c.JSON(http.StatusOK, cloudAccountsSnapshot())
}

func listVMs(c *gin.Context) {
	providerName := c.Query("provider")
	account := c.Query("account")
	if providerName == "" || account == "" {
		c.JSON(http.StatusOK, []map[string]interface{}{})
		return
	}

	service, cloudflare, ok := serviceSnapshot(providerName, account)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("provider/account %s/%s not found", providerName, account)})
		return
	}

	vms, err := service.ListVMs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, vm := range vms {
		if id, ok := vm["id"].(string); ok {
			vm["dnsEnabled"] = cloudflare.HasBinding(providerName, account, id)
		}
	}
	cStore := cache.Global()
	if cStore != nil {
		group := ""
		for _, acc := range cloudAccountsSnapshot() {
			if acc["provider"] == providerName && acc["account"] == account {
				if g, ok := acc["group"].(string); ok {
					group = g
				}
				break
			}
		}
		cStore.UpdateAccount(providerName, account, group, vms)
	}
	c.JSON(http.StatusOK, vms)
}

func getVM(c *gin.Context) {
	name := c.Param("name")
	service, _, ok := getCloudService(c)
	if !ok {
		return
	}
	vm, err := service.GetVM(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, vm)
}

func startVM(c *gin.Context) {
	name := c.Param("name")
	service, _, ok := getCloudService(c)
	if !ok {
		return
	}
	if err := service.StartVM(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("starting VM: %s", name)})
}

func stopVM(c *gin.Context) {
	name := c.Param("name")
	service, _, ok := getCloudService(c)
	if !ok {
		return
	}
	if err := service.StopVM(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("stopping VM: %s", name)})
}

func restartVM(c *gin.Context) {
	name := c.Param("name")
	service, _, ok := getCloudService(c)
	if !ok {
		return
	}
	if err := service.RestartVM(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("restarting VM: %s", name)})
}

func changeIP(c *gin.Context) {
	providerName := c.Param("provider")
	account := c.Param("account")
	name := c.Param("name")
	updateDNSAfterChange := c.DefaultQuery("update_dns", "false") == "true"
	service, cloudflare, ok := getCloudService(c)
	if !ok {
		return
	}

	result, err := service.ChangeIP(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if updateDNSAfterChange && result.NewIPAddress != "" && cloudflare.HasBinding(providerName, account, name) {
		result.Logs = append(result.Logs, cloudflare.UpdateForVM(providerName, account, name, result.NewIPAddress)...)
	}
	c.JSON(http.StatusOK, result)
}

func updateDNS(c *gin.Context) {
	providerName := c.Param("provider")
	account := c.Param("account")
	name := c.Param("name")

	_, cloudflare, ok := serviceSnapshot(providerName, account)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("provider/account %s/%s not found", providerName, account)})
		return
	}
	if !cloudflare.HasBinding(providerName, account, name) {
		c.JSON(http.StatusNotFound, gin.H{"error": "no DNS binding configured for this VM"})
		return
	}

	service, _, ok := getCloudService(c)
	if !ok {
		return
	}

	vm, err := service.GetVM(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ip, ok := publicIPAddress(vm)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "VM has no public IP to bind"})
		return
	}

	logs := cloudflare.UpdateForVM(providerName, account, name, ip)
	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"newIpAddress": ip,
		"logs":         logs,
	})
}

func refreshVM(c *gin.Context) {
	name := c.Param("name")
	service, cloudflare, ok := getCloudService(c)
	if !ok {
		return
	}
	vm, err := service.GetVM(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	vm["dnsEnabled"] = cloudflare.HasBinding(c.Param("provider"), c.Param("account"), name)
	c.JSON(http.StatusOK, vm)
}

func publicIPAddress(vm map[string]interface{}) (string, bool) {
	publicIP, ok := vm["publicIP"].(map[string]interface{})
	if !ok {
		return "", false
	}
	ip, ok := publicIP["ipAddress"].(string)
	if !ok || ip == "" || ip == "N/A" || ip == "unassigned" || ip == "未分配" {
		return "", false
	}
	return ip, true
}

func getCloudService(c *gin.Context) (provider.CloudService, *dns.CloudflareService, bool) {
	providerName := c.Param("provider")
	if providerName == "" {
		providerName = "azure"
	}
	account := c.Param("account")
	if account == "" {
		account = c.Query("account")
	}
	if account == "" {
		service, cloudflare, ok := serviceSnapshot(providerName, "")
		return service, cloudflare, ok
	}

	service, cloudflare, ok := serviceSnapshot(providerName, account)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("provider/account %s/%s not found", providerName, account)})
		return nil, nil, false
	}
	return service, cloudflare, true
}
