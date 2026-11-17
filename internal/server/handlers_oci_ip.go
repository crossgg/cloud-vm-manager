package server

import (
	"net/http"
	"strings"

	"github.com/crossgg/cloud-vm-manager/internal/dns"
	"github.com/crossgg/cloud-vm-manager/internal/provider/oci"
	"github.com/gin-gonic/gin"
)

func getOCIServiceWithDNS(c *gin.Context) (*oci.OCIService, *dns.CloudflareService, bool) {
	providerName := strings.ToLower(c.Param("provider"))
	if providerName != "oci" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "this operation is only supported for OCI accounts"})
		return nil, nil, false
	}

	account := c.Param("account")
	service, cf, ok := serviceSnapshot(providerName, account)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider/account not found"})
		return nil, nil, false
	}

	ociSvc, ok := service.(*oci.OCIService)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "selected service is not an OCI account"})
		return nil, nil, false
	}
	return ociSvc, cf, true
}

func getOCIInstanceIPs(c *gin.Context) {
	instanceID := c.Param("name")
	account := c.Param("account")
	ociSvc, cf, ok := getOCIServiceWithDNS(c)
	if !ok {
		return
	}

	info, err := ociSvc.GetIPManagement(instanceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	hasA, hasAAAA := cf.BindingTypes("oci", account, instanceID)
	info.DNSBindings = oci.OCIDNSBindingSummary{
		HasAny:  cf.HasBinding("oci", account, instanceID),
		HasA:    hasA,
		HasAAAA: hasAAAA,
	}
	c.JSON(http.StatusOK, info)
}

func changeOCIIPv4(c *gin.Context) {
	instanceID := c.Param("name")
	account := c.Param("account")
	ociSvc, cf, ok := getOCIServiceWithDNS(c)
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

	result, err := ociSvc.ChangePrivateIPPublicIP(instanceID, payload.PrivateIPID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if payload.UpdateDNS && result.NewIPAddress != "" && cf.HasBinding("oci", account, instanceID) {
		result.Logs = append(result.Logs, cf.UpdateForVMByIPs("oci", account, instanceID, result.NewIPAddress, "")...)
	}
	c.JSON(http.StatusOK, result)
}

func attachOCIIPv4(c *gin.Context) {
	instanceID := c.Param("name")
	account := c.Param("account")
	ociSvc, cf, ok := getOCIServiceWithDNS(c)
	if !ok {
		return
	}

	var payload struct {
		oci.OCIAttachIPv4Request
		UpdateDNS bool `json:"updateDns"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	logs, newIP, err := ociSvc.AttachIPv4(instanceID, payload.OCIAttachIPv4Request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if payload.UpdateDNS && newIP != "" && cf.HasBinding("oci", account, instanceID) {
		logs = append(logs, cf.UpdateForVMByIPs("oci", account, instanceID, newIP, "")...)
	}
	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"newIpAddress": newIP,
		"logs":         logs,
	})
}

func deleteOCIIPv4(c *gin.Context) {
	instanceID := c.Param("name")
	ociSvc, _, ok := getOCIServiceWithDNS(c)
	if !ok {
		return
	}

	var payload struct {
		PrivateIPID     string `json:"privateIpId"`
		PublicIPID      string `json:"publicIpId"`
		DeletePrivateIP bool   `json:"deletePrivateIp"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	logs, err := ociSvc.DeleteIPv4(instanceID, payload.PrivateIPID, payload.PublicIPID, payload.DeletePrivateIP)
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
	ociSvc, cf, ok := getOCIServiceWithDNS(c)
	if !ok {
		return
	}

	var payload struct {
		PrivateIPID        string `json:"privateIpId"`
		ReservedPublicIPID string `json:"reservedPublicIpId"`
		CreateNew          bool   `json:"createNew"`
		DisplayName        string `json:"displayName"`
		UpdateDNS          bool   `json:"updateDns"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ipAddr, logs, err := ociSvc.AttachReservedIPv4(instanceID, payload.PrivateIPID, payload.ReservedPublicIPID, payload.CreateNew, payload.DisplayName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if payload.UpdateDNS && ipAddr != "" && cf.HasBinding("oci", account, instanceID) {
		logs = append(logs, cf.UpdateForVMByIPs("oci", account, instanceID, ipAddr, "")...)
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
	ociSvc, cf, ok := getOCIServiceWithDNS(c)
	if !ok {
		return
	}

	var payload struct {
		oci.OCIAttachIPv6Request
		UpdateDNS bool `json:"updateDns"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := ociSvc.AttachIPv6(instanceID, payload.OCIAttachIPv6Request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	logs := []string{"已分配 IPv6: " + created.IPAddress}
	if payload.UpdateDNS && created.CanChange && created.IPAddress != "" && cf.HasBinding("oci", account, instanceID) {
		logs = append(logs, cf.UpdateForVMByIPs("oci", account, instanceID, "", created.IPAddress)...)
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
	ociSvc, cf, ok := getOCIServiceWithDNS(c)
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

	result, err := ociSvc.ChangeIPv6(instanceID, payload.VNICID, payload.IPv6ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if payload.UpdateDNS && result.NewIPAddress != "" && cf.HasBinding("oci", account, instanceID) {
		result.Logs = append(result.Logs, cf.UpdateForVMByIPs("oci", account, instanceID, "", result.NewIPAddress)...)
	}
	c.JSON(http.StatusOK, result)
}

func deleteOCIIPv6(c *gin.Context) {
	instanceID := c.Param("name")
	ociSvc, _, ok := getOCIServiceWithDNS(c)
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

	oldIP, err := ociSvc.DeleteIPv6(instanceID, payload.IPv6ID)
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
	ociSvc, _, ok := getOCIServiceWithDNS(c)
	if !ok {
		return
	}

	var payload oci.OCIEnableIPv6Request
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	subnetCIDR, logs, err := ociSvc.EnableSubnetIPv6(instanceID, payload)
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
