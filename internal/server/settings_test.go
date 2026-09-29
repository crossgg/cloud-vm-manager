package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crossgg/cloud-vm-manager/internal/auth"
	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func TestPasswordChangeRotatesSessionSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	oldSecret := strings.Repeat("a", 32)
	oldHash := hashPassword(t, "old-password")
	cfgContent := strings.Join([]string{
		"auth=begin",
		"[main]",
		"enabled=true",
		"username=admin",
		"password_hash=" + oldHash,
		"session_secret=" + oldSecret,
		"session_hours=12",
		"cookie_secure=false",
		"auth=end",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "config.conf"), []byte(cfgContent), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, path, err := config.LoadConfigWithPath()
	if err != nil {
		t.Fatal(err)
	}
	authSvc, err := auth.NewAuthService(cfg.Auth)
	if err != nil {
		t.Fatal(err)
	}
	if err := InitRuntime(cfg, path, authSvc); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	authSvc.Register(router)
	router.Use(authSvc.Middleware())
	router.POST("/api/settings/auth", updateAuthSettings)

	token, err := authSvc.NewSessionToken("admin")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"enabled":true,"username":"admin","password":"new-password","session_hours":12,"cookie_secure":false}`)
	req := httptest.NewRequest(http.MethodPost, "http://app.example.com/api/settings/auth", bytes.NewReader(body))
	req.Header.Set("Origin", "http://app.example.com")
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	updated, err := os.ReadFile(filepath.Join(dir, "config.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(updated), "session_secret="+oldSecret) {
		t.Fatalf("expected password change to rotate session_secret")
	}
}

func hashPassword(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(hash)
}
