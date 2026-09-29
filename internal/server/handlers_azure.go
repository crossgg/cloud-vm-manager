package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/crossgg/cloud-vm-manager/internal/provider/azure"
	"github.com/gin-gonic/gin"
)

func getAzureService(c *gin.Context) (*azure.AzureService, bool) {
	account := c.Param("account")
	if account == "" {
		account = c.Query("account")
	}
	service, _, ok := serviceSnapshot("azure", account)
	if !ok || service == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Azure account %q not found", account)})
		return nil, false
	}
	azSvc, ok := service.(*azure.AzureService)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("account %q is not an Azure service", account)})
		return nil, false
	}
	return azSvc, true
}

func listAzureNSGs(c *gin.Context) {
	azSvc, ok := getAzureService(c)
	if !ok {
		return
	}
	nsgs, err := azSvc.ListNSGs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"securityGroups": nsgs})
}

func saveAzureNSGRule(c *gin.Context) {
	azSvc, ok := getAzureService(c)
	if !ok {
		return
	}
	nsgName := c.Param("nsg")
	rg := c.Query("resourceGroup")

	var rule azure.AzureNSGRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(rule.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rule name cannot be empty"})
		return
	}

	if err := azSvc.CreateOrUpdateNSGRule(rg, nsgName, rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func deleteAzureNSGRule(c *gin.Context) {
	azSvc, ok := getAzureService(c)
	if !ok {
		return
	}
	nsgName := c.Param("nsg")
	ruleName := c.Param("rule")
	rg := c.Query("resourceGroup")

	if err := azSvc.DeleteNSGRule(rg, nsgName, ruleName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
