package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/gin-gonic/gin"
)

func getAccountDetail(c *gin.Context) {
	provider := strings.ToLower(strings.TrimSpace(c.Param("provider")))
	accountName := strings.TrimSpace(c.Param("account"))
	if provider == "" || accountName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider and account are required"})
		return
	}

	runtimeState.mu.RLock()
	cfg := runtimeState.cfg
	runtimeState.mu.RUnlock()

	if cfg == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "configuration not loaded"})
		return
	}

	fields := make(map[string]string)
	group := ""

	switch provider {
	case "azure":
		found := false
		for _, acc := range cfg.AzureAccounts {
			if strings.EqualFold(acc.Name, accountName) {
				fields["tenant_id"] = acc.TenantId
				fields["client_id"] = acc.ClientId
				fields["client_secret"] = acc.ClientSecret
				fields["subscription_id"] = acc.SubId
				fields["resource_group"] = acc.ResourceGroup
				fields["location"] = acc.Location
				group = acc.Group
				found = true
				break
			}
		}
		if !found && (accountName == "default" || accountName == "azure") && cfg.Azure.TenantId != "" {
			fields["tenant_id"] = cfg.Azure.TenantId
			fields["client_id"] = cfg.Azure.ClientId
			fields["client_secret"] = cfg.Azure.ClientSecret
			fields["subscription_id"] = cfg.Azure.SubId
			fields["resource_group"] = cfg.Azure.ResourceGroup
			fields["location"] = cfg.Azure.Location
			group = cfg.Azure.Group
			found = true
		}
		if !found {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("azure account %s not found", accountName)})
			return
		}

	case "oci":
		found := false
		for _, acc := range cfg.OCIAccounts {
			if strings.EqualFold(acc.Name, accountName) {
				fields["user"] = acc.User
				fields["fingerprint"] = acc.Fingerprint
				fields["tenancy"] = acc.Tenancy
				fields["compartment_id"] = acc.CompartmentID
				fields["region"] = acc.Region
				fields["key_file"] = acc.KeyFile
				group = acc.Group
				found = true
				break
			}
		}
		if !found {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("oci account %s not found", accountName)})
			return
		}

	case "gcp":
		found := false
		for _, acc := range cfg.GCPAccounts {
			if strings.EqualFold(acc.Name, accountName) {
				fields["project_id"] = acc.ProjectID
				fields["client_email"] = acc.ClientEmail
				fields["key_file"] = acc.KeyFile
				group = acc.Group
				found = true
				break
			}
		}
		if !found {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("gcp account %s not found", accountName)})
			return
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported provider: " + provider})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"provider": provider,
		"account":  accountName,
		"group":    group,
		"fields":   fields,
	})
}

func saveAccount(c *gin.Context) {
	var payload struct {
		Provider        string            `json:"provider"`
		Account         string            `json:"account"`
		OriginalAccount string            `json:"originalAccount"`
		Group           string            `json:"group"`
		Fields          map[string]string `json:"fields"`
	}

	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}

	provider := strings.ToLower(strings.TrimSpace(payload.Provider))
	account := strings.TrimSpace(payload.Account)
	if provider != "azure" && provider != "oci" && provider != "gcp" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider must be one of: azure, oci, gcp"})
		return
	}
	if account == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account name cannot be empty"})
		return
	}

	configPath := currentConfigPath()
	if configPath == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "config file path not resolved"})
		return
	}

	if payload.Fields == nil {
		payload.Fields = make(map[string]string)
	}
	if strings.TrimSpace(payload.Group) != "" {
		payload.Fields["group"] = strings.TrimSpace(payload.Group)
	}

	origAccount := strings.TrimSpace(payload.OriginalAccount)
	if origAccount != "" && origAccount != account {
		_ = config.DeleteCloudAccount(configPath, provider, origAccount)
	}

	if err := config.SaveCloudAccount(configPath, provider, account, payload.Fields); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save account to config: " + err.Error()})
		return
	}

	if err := ReloadRuntimeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "saved but reload failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("账号 [%s] %s 已成功保存并热重载生效。", strings.ToUpper(provider), account),
	})
}

func deleteAccount(c *gin.Context) {
	provider := strings.ToLower(strings.TrimSpace(c.Param("provider")))
	account := strings.TrimSpace(c.Param("account"))

	if provider != "azure" && provider != "oci" && provider != "gcp" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider must be one of: azure, oci, gcp"})
		return
	}
	if account == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account name cannot be empty"})
		return
	}

	configPath := currentConfigPath()
	if configPath == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "config file path not resolved"})
		return
	}

	if err := config.DeleteCloudAccount(configPath, provider, account); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete account from config: " + err.Error()})
		return
	}

	if err := ReloadRuntimeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "deleted but reload failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("账号 [%s] %s 已成功删除并热重载生效。", strings.ToUpper(provider), account),
	})
}

func uploadKeyFile(c *gin.Context) {
	file, err := c.FormFile("keyFile")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file field 'keyFile' is required"})
		return
	}

	filename := filepath.Base(file.Filename)
	if filename == "" || filename == "." || filename == "/" || filename == "\\" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	// Determine destination directory
	keysDir := "config/keys"
	if fi, err := os.Stat("/app/config/keys"); err == nil && fi.IsDir() {
		keysDir = "/app/config/keys"
	} else {
		_ = os.MkdirAll(keysDir, 0700)
	}

	destPath := filepath.Join(keysDir, filename)
	// Replace Windows backslash with forward slash for cross-platform config compatibility
	cleanPath := filepath.ToSlash(destPath)

	if err := c.SaveUploadedFile(file, destPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save key file: " + err.Error()})
		return
	}
	_ = os.Chmod(destPath, 0600)

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"filename": filename,
		"path":     cleanPath,
	})
}
