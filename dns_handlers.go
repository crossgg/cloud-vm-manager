package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// --- Cloudflare account management ---

func listCloudflareAccounts(c *gin.Context) {
	runtimeState.mu.RLock()
	cfg := runtimeState.cfg
	runtimeState.mu.RUnlock()

	if cfg == nil {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	accounts := make([]gin.H, 0, len(cfg.Cloudflare))
	for _, cf := range cfg.Cloudflare {
		accounts = append(accounts, gin.H{
			"name":      cf.Name,
			"remark":    cf.Remark,
			"api_token": maskToken(cf.APIToken),
			"zone_id":   maskToken(cf.ZoneID),
		})
	}
	c.JSON(http.StatusOK, accounts)
}

func saveCloudflareAccounts(c *gin.Context) {
	var req struct {
		Accounts []struct {
			Name     string `json:"name"`
			Remark   string `json:"remark"`
			APIToken string `json:"api_token"`
			ZoneID   string `json:"zone_id"`
		} `json:"accounts"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	runtimeState.mu.RLock()
	cfg := runtimeState.cfg
	runtimeState.mu.RUnlock()
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "config not loaded"})
		return
	}

	// Build existing token and zone_id maps for unmasking
	existingTokens := make(map[string]string)
	existingZoneIDs := make(map[string]string)
	for _, cf := range cfg.Cloudflare {
		existingTokens[cf.Name] = cf.APIToken
		existingZoneIDs[cf.Name] = cf.ZoneID
	}

	var accounts []CloudflareConfig
	for _, a := range req.Accounts {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			continue
		}
		if hasUnsafeConfigValue(name) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("名称 %q 包含不支持的字符", name)})
			return
		}
		token := strings.TrimSpace(a.APIToken)
		// If the token looks masked, restore the original
		if strings.Contains(token, "***") {
			if orig, ok := existingTokens[name]; ok {
				token = orig
			}
		}
		zoneID := strings.TrimSpace(a.ZoneID)
		// If the zone_id looks masked, restore the original
		if strings.Contains(zoneID, "***") {
			if orig, ok := existingZoneIDs[name]; ok {
				zoneID = orig
			}
		}
		accounts = append(accounts, CloudflareConfig{
			Name:     name,
			Remark:   strings.TrimSpace(a.Remark),
			APIToken: token,
			ZoneID:   zoneID,
		})
	}

	dnsPath := currentDNSPath()
	if err := SaveDNSConfig(dnsPath, accounts, cfg.DNSBindings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := ReloadRuntimeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("saved but reload failed: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// --- DNS binding management ---

func listDNSBindings(c *gin.Context) {
	runtimeState.mu.RLock()
	cfg := runtimeState.cfg
	runtimeState.mu.RUnlock()

	if cfg == nil {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	bindings := make([]gin.H, 0, len(cfg.DNSBindings))
	for _, b := range cfg.DNSBindings {
		bindings = append(bindings, gin.H{
			"name":       b.Name,
			"cloudflare": b.Cloudflare,
			"provider":   b.Provider,
			"account":    b.Account,
			"vm":         b.VM,
			"domain":     b.Domain,
			"type":       b.Type,
			"ttl":        b.TTL,
			"proxied":    b.Proxied,
		})
	}
	c.JSON(http.StatusOK, bindings)
}

func saveDNSBindings(c *gin.Context) {
	var req struct {
		Bindings []struct {
			Name       string `json:"name"`
			Cloudflare string `json:"cloudflare"`
			Provider   string `json:"provider"`
			Account    string `json:"account"`
			VM         string `json:"vm"`
			Domain     string `json:"domain"`
			Type       string `json:"type"`
			TTL        int    `json:"ttl"`
			Proxied    bool   `json:"proxied"`
		} `json:"bindings"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	runtimeState.mu.RLock()
	cfg := runtimeState.cfg
	runtimeState.mu.RUnlock()
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "config not loaded"})
		return
	}

	var bindings []DNSBinding
	for _, b := range req.Bindings {
		name := strings.TrimSpace(b.Name)
		if name == "" {
			continue
		}
		if hasUnsafeConfigValue(name) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("名称 %q 包含不支持的字符", name)})
			return
		}
		ttl := b.TTL
		if ttl <= 0 {
			ttl = 1
		}
		bindings = append(bindings, DNSBinding{
			Name:       name,
			Cloudflare: strings.TrimSpace(b.Cloudflare),
			Provider:   strings.ToLower(strings.TrimSpace(b.Provider)),
			Account:    strings.TrimSpace(b.Account),
			VM:         strings.TrimSpace(b.VM),
			Domain:     strings.TrimSpace(b.Domain),
			Type:       valueOrDefault(strings.ToUpper(strings.TrimSpace(b.Type)), "A"),
			TTL:        ttl,
			Proxied:    b.Proxied,
		})
	}

	dnsPath := currentDNSPath()
	if err := SaveDNSConfig(dnsPath, cfg.Cloudflare, bindings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := ReloadRuntimeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("saved but reload failed: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}


// --- Get DNS bindings for a specific VM ---

func getVMDNSBindings(c *gin.Context) {
	provider := c.Param("provider")
	account := c.Param("account")
	vmID := c.Param("name")

	runtimeState.mu.RLock()
	cfg := runtimeState.cfg
	runtimeState.mu.RUnlock()

	if cfg == nil {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	var cfNames []gin.H
	for _, cf := range cfg.Cloudflare {
		cfNames = append(cfNames, gin.H{
			"name":    cf.Name,
			"zone_id": cf.ZoneID,
		})
	}

	var vmBindings []gin.H
	for _, b := range cfg.DNSBindings {
		if b.matches(provider, account, vmID) {
			vmBindings = append(vmBindings, gin.H{
				"name":       b.Name,
				"cloudflare": b.Cloudflare,
				"provider":   b.Provider,
				"account":    b.Account,
				"vm":         b.VM,
				"domain":     b.Domain,
				"type":       b.Type,
				"ttl":        b.TTL,
				"proxied":    b.Proxied,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"cloudflare_accounts": cfNames,
		"bindings":            vmBindings,
	})
}

// --- Save DNS bindings for a specific VM (add/update/delete) ---

func saveVMDNSBindings(c *gin.Context) {
	provider := c.Param("provider")
	account := c.Param("account")
	vmID := c.Param("name")

	var req struct {
		Bindings []struct {
			Name       string `json:"name"`
			Cloudflare string `json:"cloudflare"`
			Domain     string `json:"domain"`
			Type       string `json:"type"`
			TTL        int    `json:"ttl"`
			Proxied    bool   `json:"proxied"`
		} `json:"bindings"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	runtimeState.mu.RLock()
	cfg := runtimeState.cfg
	runtimeState.mu.RUnlock()
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "config not loaded"})
		return
	}

	// Keep bindings that do NOT match this VM
	var kept []DNSBinding
	for _, b := range cfg.DNSBindings {
		if !b.matches(provider, account, vmID) {
			kept = append(kept, b)
		}
	}

	// Add new bindings for this VM
	for _, b := range req.Bindings {
		name := strings.TrimSpace(b.Name)
		if name == "" {
			name = fmt.Sprintf("%s-%s-%s", provider, account, vmID)
		}
		if hasUnsafeConfigValue(name) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("名称 %q 包含不支持的字符", name)})
			return
		}
		ttl := b.TTL
		if ttl <= 0 {
			ttl = 1
		}
		kept = append(kept, DNSBinding{
			Name:       name,
			Cloudflare: strings.TrimSpace(b.Cloudflare),
			Provider:   strings.ToLower(provider),
			Account:    account,
			VM:         vmID,
			Domain:     strings.TrimSpace(b.Domain),
			Type:       valueOrDefault(strings.ToUpper(strings.TrimSpace(b.Type)), "A"),
			TTL:        ttl,
			Proxied:    b.Proxied,
		})
	}

	dnsPath := currentDNSPath()
	if err := SaveDNSConfig(dnsPath, cfg.Cloudflare, kept); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := ReloadRuntimeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("saved but reload failed: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// --- Get raw dns.conf content for display ---

func getDNSConfigRaw(c *gin.Context) {
	dnsPath := currentDNSPath()
	data, err := readFileString(dnsPath)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"content": "", "path": dnsPath})
		return
	}
	// Mask sensitive api_token values
	masked := maskSensitiveLines(data)
	c.JSON(http.StatusOK, gin.H{"content": masked, "path": dnsPath})
}

func deleteDNSBinding(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	} 
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	runtimeState.mu.RLock()
	cfg := runtimeState.cfg
	runtimeState.mu.RUnlock()
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "config not loaded"})
		return
	}

	var kept []DNSBinding
	for _, b := range cfg.DNSBindings {
		if b.Name != name {
			kept = append(kept, b)
		}
	}

	dnsPath := currentDNSPath()
	if err := SaveDNSConfig(dnsPath, cfg.Cloudflare, kept); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := ReloadRuntimeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("saved but reload failed: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// --- helpers ---

func maskToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "***" + token[len(token)-4:]
}

func maskDomain(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) <= 2 {
		return "***"
	}
	return "***." + strings.Join(parts[len(parts)-2:], ".")
}

func maskSensitiveLines(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if key, val, ok := strings.Cut(trimmed, "="); ok {
			k := strings.TrimSpace(key)
			v := strings.TrimSpace(val)
			if k == "api_token" || k == "client_secret" || k == "password" || k == "zone_id" {
				lines[i] = k + "=" + maskToken(v)
			} else if k == "domain" {
				lines[i] = k + "=" + maskDomain(v)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func readFileString(path string) (string, error) {
	data, err := readFileBytes(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// intFromString is a helper for parsing int query params.
func intFromString(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}
