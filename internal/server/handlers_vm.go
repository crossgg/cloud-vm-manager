package server

import (
	"fmt"
	"io"
	"net/http"
	"strings"

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
	providerName := c.Param("provider")
	account := c.Param("account")
	name := c.Param("name")
	service, _, ok := getCloudService(c)
	if !ok {
		return
	}
	if err := service.StartVM(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	emitVMPowerEvent(providerName, account, name, "start", "Web 面板")
	c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("starting VM: %s", name)})
}

func stopVM(c *gin.Context) {
	providerName := c.Param("provider")
	account := c.Param("account")
	name := c.Param("name")
	service, _, ok := getCloudService(c)
	if !ok {
		return
	}
	if err := service.StopVM(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	emitVMPowerEvent(providerName, account, name, "stop", "Web 面板")
	c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("stopping VM: %s", name)})
}

func restartVM(c *gin.Context) {
	providerName := c.Param("provider")
	account := c.Param("account")
	name := c.Param("name")
	service, _, ok := getCloudService(c)
	if !ok {
		return
	}
	if err := service.RestartVM(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	emitVMPowerEvent(providerName, account, name, "restart", "Web 面板")
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
	emitVMChangeIPEvent(providerName, account, name, result.NewIPAddress, result.Message, "Web 面板")
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

	var payload struct {
		IPv4 string `json:"ipv4"`
		IPv6 string `json:"ipv6"`
	}
	if c.Request.Body != nil {
		if err := c.ShouldBindJSON(&payload); err != nil && err != io.EOF {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	ipv4 := strings.TrimSpace(payload.IPv4)
	ipv6 := strings.TrimSpace(payload.IPv6)
	if ipv4 == "" && ipv6 == "" {
		service, _, ok := getCloudService(c)
		if !ok {
			return
		}

		vm, err := service.GetVM(name)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if addr, hasV4 := publicIPAddress(vm); hasV4 {
			ipv4 = addr
		}
		if addr6, hasV6 := primaryIPv6Address(vm); hasV6 {
			ipv6 = addr6
		}
	}

	hasA, hasAAAA := cloudflare.BindingTypes(providerName, account, name)
	if (!hasA || ipv4 == "") && (!hasAAAA || ipv6 == "") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "VM has no matching public IPv4/IPv6 to bind"})
		return
	}

	logs := cloudflare.UpdateForVMByIPs(providerName, account, name, ipv4, ipv6)
	newIP := ipv4
	if newIP == "" {
		newIP = ipv6
	}
	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"newIpAddress": newIP,
		"ipv4":         ipv4,
		"ipv6":         ipv6,
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

func primaryIPv6Address(vm map[string]interface{}) (string, bool) {
	if ipv6s, ok := vm["ipv6s"].([]map[string]interface{}); ok {
		for _, item := range ipv6s {
			ip := strings.TrimSpace(fmt.Sprintf("%v", item["ipAddress"]))
			if canChange, _ := item["canChange"].(bool); !canChange {
				if prefix, ok := item["cidrPrefixLength"].(int); ok && prefix != 0 && prefix != 128 {
					continue
				}
			}
			if ip != "" && ip != "<nil>" && !strings.Contains(ip, "/") {
				return ip, true
			}
		}
	}
	if ipv6, ok := vm["ipv6"].(string); ok {
		ipv6 = strings.TrimSpace(ipv6)
		if ipv6 != "" && ipv6 != "N/A" && ipv6 != "unassigned" && ipv6 != "未分配" && !strings.Contains(ipv6, "/") {
			return ipv6, true
		}
	}
	return "", false
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
