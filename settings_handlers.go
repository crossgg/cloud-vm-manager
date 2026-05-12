package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

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
	auth := currentAuthConfig()
	c.JSON(http.StatusOK, gin.H{
		"enabled":        auth.Enabled,
		"username":       auth.Username,
		"session_hours":  auth.SessionHours,
		"cookie_secure":  auth.CookieSecure,
		"has_password":   auth.PasswordHash != "",
		"config_path":    currentConfigPath(),
		"session_secret": auth.SessionSecret != "",
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
	if hasUnsafeConfigValue(username) {
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
	if err := SaveAuthConfig(path, next); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := ReloadRuntimeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("saved but reload failed: %v", err)})
		return
	}

	if next.Enabled {
		auth := currentAuthService()
		if auth != nil {
			if err := auth.setSessionCookie(c, next.Username); err != nil {
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

func hasUnsafeConfigValue(value string) bool {
	return strings.ContainsAny(value, "\r\n=[]")
}

func randomSessionSecret() (string, error) {
	data := make([]byte, 36)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}
