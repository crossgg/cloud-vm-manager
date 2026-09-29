package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/crossgg/cloud-vm-manager/internal/provider/gcp"
	"github.com/gin-gonic/gin"
)

func getGCPService(c *gin.Context) (*gcp.GCPService, bool) {
	account := c.Param("account")
	if account == "" {
		account = c.Query("account")
	}
	service, _, ok := serviceSnapshot("gcp", account)
	if !ok || service == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("GCP account %q not found", account)})
		return nil, false
	}
	gcpSvc, ok := service.(*gcp.GCPService)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("account %q is not a GCP service", account)})
		return nil, false
	}
	return gcpSvc, true
}

func listGCPFirewalls(c *gin.Context) {
	gcpSvc, ok := getGCPService(c)
	if !ok {
		return
	}
	rules, err := gcpSvc.ListFirewalls()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"firewalls": rules})
}

func createGCPFirewall(c *gin.Context) {
	gcpSvc, ok := getGCPService(c)
	if !ok {
		return
	}
	var rule gcp.GCPFirewallRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(rule.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "firewall rule name cannot be empty"})
		return
	}
	if err := gcpSvc.CreateFirewall(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func updateGCPFirewall(c *gin.Context) {
	gcpSvc, ok := getGCPService(c)
	if !ok {
		return
	}
	name := c.Param("name")
	var rule gcp.GCPFirewallRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rule.Name = name
	if err := gcpSvc.UpdateFirewall(name, rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func toggleGCPFirewall(c *gin.Context) {
	gcpSvc, ok := getGCPService(c)
	if !ok {
		return
	}
	name := c.Param("name")
	var payload struct {
		Disabled bool `json:"disabled"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := gcpSvc.ToggleFirewall(name, payload.Disabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func deleteGCPFirewall(c *gin.Context) {
	gcpSvc, ok := getGCPService(c)
	if !ok {
		return
	}
	name := c.Param("name")
	if err := gcpSvc.DeleteFirewall(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
