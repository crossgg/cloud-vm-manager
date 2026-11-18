package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/crossgg/cloud-vm-manager/internal/dns"
	"github.com/crossgg/cloud-vm-manager/internal/provider"
	"github.com/gin-gonic/gin"
)

func getIPManagerWithDNS(c *gin.Context) (string, provider.IPManager, *dns.CloudflareService, bool) {
	providerName := strings.ToLower(c.Param("provider"))
	account := c.Param("account")
	service, cf, ok := serviceSnapshot(providerName, account)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider/account not found"})
		return "", nil, nil, false
	}

	mgr, ok := service.(provider.IPManager)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("provider %q does not support centralized IP management", providerName)})
		return "", nil, nil, false
	}
	return providerName, mgr, cf, true
}

func getOCIInstanceIPs(c *gin.Context) {
	instanceID := c.Param("name")
	account := c.Param("account")
	providerName, mgr, cf, ok := getIPManagerWithDNS(c)
	if !ok {
		return
	}

	info, err := mgr.GetIPManagement(instanceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	hasA, hasAAAA := cf.BindingTypes(providerName, account, instanceID)
	info.DNSBindings = provider.DNSBindingSummary{
		HasAny:  cf.HasBinding(providerName, account, instanceID),
		HasA:    hasA,
		HasAAAA: hasAAAA,
	}
	c.JSON(http.StatusOK, info)
}

func changeOCIIPv4(c *gin.Context) {
	instanceID := c.Param("name")
	account := c.Param("account")
	providerName, mgr, cf, ok := getIPManagerWithDNS(c)
	if !ok {
		return
	}

	var payload struct {
		PrivateIPID string `json:"privateIpId"`
		UpdateDNS   bool   `json:"updateDns"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := mgr.ChangePrivateIPPublicIP(instanceID, payload.PrivateIPID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if payload.UpdateDNS && result.NewIPAddress != "" && cf.HasBinding(providerName, account, instanceID) {
		result.Logs = append(result.Logs, cf.UpdateForVMByIPs(providerName, account, instanceID, result.NewIPAddress, "")...)
	}
	c.JSON(http.StatusOK, result)
}

func attachOCIIPv4(c *gin.Context) {
	instanceID := c.Param("name")
	account := c.Param("account")
	providerName, mgr, cf, ok := getIPManagerWithDNS(c)
	if !ok {
		return
	}

	var payload struct {
		provider.AttachIPv4Request
		UpdateDNS bool `json:"updateDns"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	logs, newIP, err := mgr.AttachIPv4(instanceID, payload.AttachIPv4Request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if payload.UpdateDNS && newIP != "" && cf.HasBinding(providerName, account, instanceID) {
		logs = append(logs, cf.UpdateForVMByIPs(providerName, account, instanceID, newIP, "")...)
	}
	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"newIpAddress": newIP,
		"logs":         logs,
	})
}

func deleteOCIIPv4(c *gin.Context) {
	instanceID := c.Param("name")
	_, mgr, _, ok := getIPManagerWithDNS(c)
	if !ok {
		return
	}

	var payload provider.DeleteIPv4Request
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	logs, err := mgr.DeleteIPv4Managed(instanceID, payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"logs":    logs,
	})
}

func attachOCIReservedIPv4(c *gin.Context) {
	instanceID := c.Param("name")
	account := c.Param("account")
	providerName, mgr, cf, ok := getIPManagerWithDNS(c)
	if !ok {
		return
	}

	var payload struct {
		provider.AttachReservedIPv4Request
		UpdateDNS bool `json:"updateDns"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ipAddr, logs, err := mgr.AttachReservedIPv4Managed(instanceID, payload.AttachReservedIPv4Request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if payload.UpdateDNS && ipAddr != "" && cf.HasBinding(providerName, account, instanceID) {
		logs = append(logs, cf.UpdateForVMByIPs(providerName, account, instanceID, ipAddr, "")...)
	}
	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"newIpAddress": ipAddr,
		"logs":         logs,
	})
}

func attachOCIIPv6(c *gin.Context) {
	instanceID := c.Param("name")
	account := c.Param("account")
	providerName, mgr, cf, ok := getIPManagerWithDNS(c)
	if !ok {
		return
	}

	var payload struct {
		provider.AttachIPv6Request
		UpdateDNS bool `json:"updateDns"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := mgr.AttachIPv6(instanceID, payload.AttachIPv6Request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	logs := []string{"已分配 IPv6: " + created.IPAddress}
	if payload.UpdateDNS && created.CanChange && created.IPAddress != "" && cf.HasBinding(providerName, account, instanceID) {
		logs = append(logs, cf.UpdateForVMByIPs(providerName, account, instanceID, "", created.IPAddress)...)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"ipv6":    created,
		"logs":    logs,
	})
}

func changeOCIIPv6(c *gin.Context) {
	instanceID := c.Param("name")
	account := c.Param("account")
	providerName, mgr, cf, ok := getIPManagerWithDNS(c)
	if !ok {
		return
	}

	var payload struct {
		VNICID    string `json:"vnicId"`
		IPv6ID    string `json:"ipv6Id"`
		UpdateDNS bool   `json:"updateDns"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := mgr.ChangeIPv6(instanceID, payload.VNICID, payload.IPv6ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if payload.UpdateDNS && result.NewIPAddress != "" && cf.HasBinding(providerName, account, instanceID) {
		result.Logs = append(result.Logs, cf.UpdateForVMByIPs(providerName, account, instanceID, "", result.NewIPAddress)...)
	}
	c.JSON(http.StatusOK, result)
}

func deleteOCIIPv6(c *gin.Context) {
	instanceID := c.Param("name")
	_, mgr, _, ok := getIPManagerWithDNS(c)
	if !ok {
		return
	}

	var payload struct {
		IPv6ID string `json:"ipv6Id"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	oldIP, err := mgr.DeleteIPv6(instanceID, payload.IPv6ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"oldIpAddress": oldIP,
		"logs":         []string{"已删除 IPv6: " + oldIP},
	})
}

func enableOCISubnetIPv6(c *gin.Context) {
	instanceID := c.Param("name")
	_, mgr, _, ok := getIPManagerWithDNS(c)
	if !ok {
		return
	}

	var payload provider.EnableIPv6Request
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	subnetCIDR, logs, err := mgr.EnableSubnetIPv6(instanceID, payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"subnetCidr": subnetCIDR,
		"logs":       logs,
	})
}
