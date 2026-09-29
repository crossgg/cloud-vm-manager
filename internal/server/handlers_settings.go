package server

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type authSettingsRequest struct {
	Enabled      bool   `json:"enabled"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	SessionHours int    `json:"session_hours"`
	CookieSecure bool   `json:"cookie_secure"`
}

func getConfigStatus(c *gin.Context) {
	c.JSON(http.StatusOK, configStatus())
}

func reloadConfig(c *gin.Context) {
	if err := ReloadRuntimeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "status": configStatus()})
}

func getAuthSettings(c *gin.Context) {
	authCfg := currentAuthConfig()
	c.JSON(http.StatusOK, gin.H{
		"enabled":        authCfg.Enabled,
		"username":       authCfg.Username,
		"session_hours":  authCfg.SessionHours,
		"cookie_secure":  authCfg.CookieSecure,
		"has_password":   authCfg.PasswordHash != "",
		"config_path":    currentConfigPath(),
		"session_secret": authCfg.SessionSecret != "",
	})
}

func updateAuthSettings(c *gin.Context) {
	var req authSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid auth settings request"})
		return
	}

	username := strings.TrimSpace(req.Username)
	if req.Enabled && username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is required when auth is enabled"})
		return
	}
	if config.HasUnsafeConfigValue(username) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username contains unsupported characters"})
		return
	}

	current := currentAuthConfig()
	next := current
	next.Enabled = req.Enabled
	next.Username = username
	next.SessionHours = req.SessionHours
	next.CookieSecure = req.CookieSecure
	if next.SessionHours <= 0 {
		next.SessionHours = 12
	}
	if next.SessionSecret == "" || len(next.SessionSecret) < 32 {
		secret, err := randomSessionSecret()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("generate session secret failed: %v", err)})
			return
		}
		next.SessionSecret = secret
	}

	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("hash password failed: %v", err)})
			return
		}
		next.PasswordHash = string(hash)
		secret, err := randomSessionSecret()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("generate session secret failed: %v", err)})
			return
		}
		next.SessionSecret = secret
	}
	if next.Enabled && next.PasswordHash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password is required when enabling auth for the first time"})
		return
	}

	path := currentConfigPath()
	if err := config.SaveAuthConfig(path, next); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := ReloadRuntimeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("saved but reload failed: %v", err)})
		return
	}

	if next.Enabled {
		authSvc := currentAuthService()
		if authSvc != nil {
			if err := authSvc.SetSessionCookie(c, next.Username); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"auth": gin.H{
			"enabled":       next.Enabled,
			"username":      next.Username,
			"session_hours": next.SessionHours,
			"cookie_secure": next.CookieSecure,
		},
	})
}

func updateUpdateSettings(c *gin.Context) {
	var payload struct {
		DownloadProxy string `json:"downloadProxy"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	configPath := currentConfigPath()
	updateCfg := config.UpdateConfig{
		DownloadProxy: strings.TrimSpace(payload.DownloadProxy),
	}

	if err := config.SaveUpdateConfig(configPath, updateCfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := ReloadRuntimeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("saved but reload failed: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "更新配置已保存，并已自动重载生效。",
	})
}

func randomSessionSecret() (string, error) {
	data := make([]byte, 36)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}
