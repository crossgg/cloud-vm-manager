package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/crossgg/cloud-vm-manager/internal/config"
	cryptoutil "github.com/crossgg/cloud-vm-manager/internal/crypto"
	"github.com/gin-gonic/gin"
)

const (
	accountSecretMask     = "********"
	maxKeyFileUploadBytes = 64 << 10
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

	hasClientSecret := false
	switch provider {
	case "azure":
		found := false
		for _, acc := range cfg.AzureAccounts {
			if strings.EqualFold(acc.Name, accountName) {
				fields["tenant_id"] = maskAccountMiddle(acc.TenantId)
				fields["client_id"] = maskAccountMiddle(acc.ClientId)
				if acc.ClientSecret != "" {
					fields["client_secret"] = accountSecretMask
					hasClientSecret = true
				} else {
					fields["client_secret"] = ""
				}
				fields["subscription_id"] = maskAccountMiddle(acc.SubId)
				fields["resource_group"] = acc.ResourceGroup
				fields["location"] = acc.Location
				group = acc.Group
				found = true
				break
			}
		}
		if !found && (accountName == "default" || accountName == "azure") && cfg.Azure.TenantId != "" {
			fields["tenant_id"] = maskAccountMiddle(cfg.Azure.TenantId)
			fields["client_id"] = maskAccountMiddle(cfg.Azure.ClientId)
			if cfg.Azure.ClientSecret != "" {
				fields["client_secret"] = accountSecretMask
				hasClientSecret = true
			} else {
				fields["client_secret"] = ""
			}
			fields["subscription_id"] = maskAccountMiddle(cfg.Azure.SubId)
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
				fields["user"] = maskAccountMiddle(acc.User)
				fields["fingerprint"] = maskAccountMiddle(acc.Fingerprint)
				fields["tenancy"] = maskAccountMiddle(acc.Tenancy)
				fields["compartment_id"] = maskAccountMiddle(acc.CompartmentID)
				fields["region"] = acc.Region
				fields["key_file"] = maskAccountMiddle(acc.KeyFile)
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
				fields["project_id"] = maskAccountMiddle(acc.ProjectID)
				fields["client_email"] = maskAccountMiddle(acc.ClientEmail)
				fields["key_file"] = maskAccountMiddle(acc.KeyFile)
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
		"provider":          provider,
		"account":           accountName,
		"group":             group,
		"fields":            fields,
		"has_client_secret": hasClientSecret,
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
	if config.HasUnsafeConfigValue(account) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account name contains unsupported characters"})
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
	for k, v := range payload.Fields {
		if config.HasUnsafeConfigValue(k) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("field key %q contains unsupported characters", k)})
			return
		}
		if config.HasUnsafeConfigLineValue(v) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("field %q contains unsupported control characters", k)})
			return
		}
	}

	origAccount := strings.TrimSpace(payload.OriginalAccount)
	restoreMaskedAccountFields(provider, origAccount, account, payload.Fields)
	if (provider == "oci" || provider == "gcp") && strings.TrimSpace(payload.Fields["key_file"]) != "" {
		validatedKeyPath, err := validateAccountKeyFilePath(payload.Fields["key_file"])
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		payload.Fields["key_file"] = validatedKeyPath
	}

	preserveMonitorFields(provider, origAccount, account, payload.Fields)
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

// preserveMonitorFields retains existing account-level monitor settings when editing credentials via Settings.
func preserveMonitorFields(provider, origAccount, account string, fields map[string]string) {
	lookupName := origAccount
	if lookupName == "" {
		lookupName = account
	}
	runtimeState.mu.RLock()
	cfg := runtimeState.cfg
	runtimeState.mu.RUnlock()
	if cfg == nil {
		return
	}
	switch provider {
	case "azure":
		for _, acc := range cfg.AzureAccounts {
			if strings.EqualFold(acc.Name, lookupName) {
				for k, v := range config.AzureCreditConfigFields(acc.CreditMonitor) {
					if _, exists := fields[k]; !exists && v != "" {
						fields[k] = v
					}
				}
				break
			}
		}
	case "oci":
		for _, acc := range cfg.OCIAccounts {
			if strings.EqualFold(acc.Name, lookupName) {
				for k, v := range config.DTMonitorConfigFields(acc.DTMonitor) {
					if _, exists := fields[k]; !exists && v != "" {
						fields[k] = v
					}
				}
				break
			}
		}
	}
}

func uploadKeyFile(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxKeyFileUploadBytes+4096)
	file, err := c.FormFile("keyFile")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file field 'keyFile' is required or exceeds 64KB limit"})
		return
	}
	if file.Size > maxKeyFileUploadBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key file exceeds maximum allowed size of 64KB"})
		return
	}

	rawName := strings.ReplaceAll(file.Filename, "\\", "/")
	filename := filepath.Base(rawName)
	if filename == "" || filename == "." || filename == ".." || filename == "/" || strings.HasPrefix(filename, ".") || config.HasUnsafeConfigValue(filename) || strings.Contains(filename, "\x00") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read uploaded file: " + err.Error()})
		return
	}
	defer src.Close()

	data, err := io.ReadAll(io.LimitReader(src, maxKeyFileUploadBytes+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read key file: " + err.Error()})
		return
	}
	if len(data) > maxKeyFileUploadBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key file exceeds maximum allowed size of 64KB"})
		return
	}
	if _, err := cryptoutil.ParseRSAPrivateKeyPEM(data, filename); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid RSA PEM private key: " + err.Error()})
		return
	}

	keysDir := resolveKeysDir()
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare keys directory: " + err.Error()})
		return
	}

	destPath := filepath.Join(keysDir, filename)
	// Replace Windows backslash with forward slash for cross-platform config compatibility
	cleanPath := filepath.ToSlash(destPath)

	if err := os.WriteFile(destPath, data, 0600); err != nil {
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

func resolveKeysDir() string {
	if fi, err := os.Stat("/app/config/keys"); err == nil && fi.IsDir() {
		return "/app/config/keys"
	}
	if cfgPath := currentConfigPath(); cfgPath != "" {
		dir := filepath.Dir(cfgPath)
		if dir != "" && dir != "." {
			return filepath.Join(dir, "keys")
		}
	}
	return filepath.Join("config", "keys")
}

func validateAccountKeyFilePath(rawPath string) (string, error) {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return "", nil
	}
	if config.HasUnsafeConfigLineValue(trimmed) {
		return "", fmt.Errorf("key_file contains unsupported control characters")
	}

	keysDir := resolveKeysDir()
	absKeysDir, err := filepath.Abs(keysDir)
	if err != nil {
		return "", fmt.Errorf("resolve keys directory failed: %w", err)
	}
	if resolvedDir, err := filepath.EvalSymlinks(absKeysDir); err == nil {
		absKeysDir = resolvedDir
	}

	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.FromSlash(candidate)
	}
	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("invalid key_file path: %w", err)
	}
	if resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(absCandidate)); err == nil {
		absCandidate = filepath.Join(resolvedParent, filepath.Base(absCandidate))
	}

	rel, err := filepath.Rel(absKeysDir, absCandidate)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("key_file must be inside %s", filepath.ToSlash(keysDir))
	}
	return filepath.ToSlash(trimmed), nil
}

func maskAccountMiddle(raw string) string {
	val := strings.TrimSpace(raw)
	if val == "" {
		return ""
	}
	runes := []rune(val)
	n := len(runes)
	if n <= 4 {
		return "****"
	}
	keep := 2
	switch {
	case n > 16:
		keep = 6
	case n > 8:
		keep = 4
	}
	return string(runes[:keep]) + "****" + string(runes[n-keep:])
}

func restoreMaskedAccountFields(provider, origAccount, account string, fields map[string]string) {
	existing := findExistingAccountFields(provider, origAccount, account)
	if len(existing) == 0 {
		if provider == "azure" && strings.TrimSpace(fields["client_secret"]) == accountSecretMask {
			fields["client_secret"] = ""
		}
		return
	}
	if provider == "azure" {
		secret := strings.TrimSpace(fields["client_secret"])
		if secret == "" || secret == accountSecretMask || strings.Contains(secret, "***") {
			if existingSecret := existing["client_secret"]; existingSecret != "" {
				fields["client_secret"] = existingSecret
			} else if secret == accountSecretMask {
				fields["client_secret"] = ""
			}
		}
	}
	var maskedKeys []string
	switch provider {
	case "azure":
		maskedKeys = []string{"tenant_id", "client_id", "subscription_id"}
	case "oci":
		maskedKeys = []string{"user", "fingerprint", "tenancy", "compartment_id", "key_file"}
	case "gcp":
		maskedKeys = []string{"project_id", "client_email", "key_file"}
	}
	for _, k := range maskedKeys {
		origVal := existing[k]
		if origVal == "" {
			continue
		}
		if strings.TrimSpace(fields[k]) == maskAccountMiddle(origVal) {
			fields[k] = origVal
		}
	}
}

func findExistingAccountFields(provider, origAccount, account string) map[string]string {
	lookupName := origAccount
	if lookupName == "" {
		lookupName = account
	}
	if lookupName == "" {
		return nil
	}
	runtimeState.mu.RLock()
	cfg := runtimeState.cfg
	runtimeState.mu.RUnlock()
	if cfg == nil {
		return nil
	}
	switch provider {
	case "azure":
		for _, acc := range cfg.AzureAccounts {
			if strings.EqualFold(acc.Name, lookupName) {
				return map[string]string{
					"tenant_id":       acc.TenantId,
					"client_id":       acc.ClientId,
					"client_secret":   acc.ClientSecret,
					"subscription_id": acc.SubId,
				}
			}
		}
		if (lookupName == "default" || lookupName == "azure") && cfg.Azure.TenantId != "" {
			return map[string]string{
				"tenant_id":       cfg.Azure.TenantId,
				"client_id":       cfg.Azure.ClientId,
				"client_secret":   cfg.Azure.ClientSecret,
				"subscription_id": cfg.Azure.SubId,
			}
		}
	case "oci":
		for _, acc := range cfg.OCIAccounts {
			if strings.EqualFold(acc.Name, lookupName) {
				return map[string]string{
					"user":           acc.User,
					"fingerprint":    acc.Fingerprint,
					"tenancy":        acc.Tenancy,
					"compartment_id": acc.CompartmentID,
					"key_file":       acc.KeyFile,
				}
			}
		}
	case "gcp":
		for _, acc := range cfg.GCPAccounts {
			if strings.EqualFold(acc.Name, lookupName) {
				return map[string]string{
					"project_id":   acc.ProjectID,
					"client_email": acc.ClientEmail,
					"key_file":     acc.KeyFile,
				}
			}
		}
	}
	return nil
}
