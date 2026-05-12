package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func TestCrossOriginPostRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth := testAuthService(t)
	router := gin.New()
	auth.Register(router)
	router.Use(auth.Middleware())
	router.POST("/api/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "http://app.example.com/api/protected", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-origin POST, got %d", w.Code)
	}
}

func TestLoginRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldLockDuration := loginLockDuration
	oldFailureWindow := loginFailureWindow
	loginLockDuration = time.Minute
	loginFailureWindow = time.Minute
	t.Cleanup(func() {
		loginLockDuration = oldLockDuration
		loginFailureWindow = oldFailureWindow
	})

	auth := testAuthService(t)
	router := gin.New()
	auth.Register(router)
	router.Use(auth.Middleware())

	for i := 0; i < maxLoginFailures; i++ {
		w := postLogin(router, "wrong-password")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, w.Code)
		}
	}

	w := postLogin(router, "correct-password")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after too many failures, got %d", w.Code)
	}
}

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
	config := strings.Join([]string{
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
	if err := os.WriteFile(filepath.Join(dir, "config.conf"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, path, err := LoadConfigWithPath()
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewAuthService(cfg.Auth)
	if err != nil {
		t.Fatal(err)
	}
	if err := InitRuntime(cfg, path, auth); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	auth.Register(router)
	router.Use(auth.Middleware())
	router.POST("/api/settings/auth", updateAuthSettings)

	token, err := auth.newSessionToken("admin")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"enabled":true,"username":"admin","password":"new-password","session_hours":12,"cookie_secure":false}`)
	req := httptest.NewRequest(http.MethodPost, "http://app.example.com/api/settings/auth", bytes.NewReader(body))
	req.Header.Set("Origin", "http://app.example.com")
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
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

func testAuthService(t *testing.T) *AuthService {
	t.Helper()
	auth, err := NewAuthService(AuthConfig{
		Enabled:       true,
		Username:      "admin",
		PasswordHash:  hashPassword(t, "correct-password"),
		SessionSecret: strings.Repeat("s", 32),
		SessionHours:  12,
	})
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func hashPassword(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(hash)
}

func postLogin(router http.Handler, password string) *httptest.ResponseRecorder {
	body := []byte(`{"username":"admin","password":"` + password + `"}`)
	req := httptest.NewRequest(http.MethodPost, "http://app.example.com/api/login", bytes.NewReader(body))
	req.Header.Set("Origin", "http://app.example.com")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
