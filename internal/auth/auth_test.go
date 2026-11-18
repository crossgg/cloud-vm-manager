package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
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

func TestValidSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth := testAuthService(t)
	token, err := auth.NewSessionToken("admin")
	if err != nil {
		t.Fatalf("failed to create session token: %v", err)
	}

	router := gin.New()
	auth.Register(router)
	router.Use(auth.Middleware())
	router.GET("/api/protected", func(c *gin.Context) {
		user, ok := auth.ValidSession(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"user": user})
	})

	req := httptest.NewRequest(http.MethodGet, "http://app.example.com/api/protected", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestUntrustedForwardedHeadersIgnored(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth := testAuthService(t)
	token, err := auth.NewSessionToken("admin")
	if err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	auth.Register(router)
	router.Use(auth.Middleware())
	router.POST("/api/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// Public peer attempting to spoof X-Forwarded-Host must be rejected.
	req := httptest.NewRequest(http.MethodPost, "http://app.example.com/api/protected", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("X-Forwarded-Host", "evil.example.com")
	req.Header.Set("Origin", "http://evil.example.com")
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for untrusted X-Forwarded-Host, got %d", w.Code)
	}

	// Trusted loopback/LAN proxy forwarding X-Forwarded-Host + X-Forwarded-Proto should succeed.
	req2 := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:3000/api/protected", nil)
	req2.RemoteAddr = "127.0.0.1:54321"
	req2.Header.Set("X-Forwarded-Host", "panel.example.com")
	req2.Header.Set("X-Forwarded-Proto", "https")
	req2.Header.Set("Origin", "https://panel.example.com")
	req2.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 for trusted proxy headers, got %d", w2.Code)
	}
}

func TestResolveTrustedProxiesFromEnv(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "none")
	got, err := ResolveTrustedProxiesFromEnv()
	if err != nil || got != nil {
		t.Fatalf("expected nil, nil for TRUSTED_PROXIES=none, got %v, %v", got, err)
	}

	t.Setenv("TRUSTED_PROXIES", "127.0.0.1, 10.10.0.0/16")
	got, err = ResolveTrustedProxiesFromEnv()
	if err != nil || len(got) != 2 {
		t.Fatalf("expected 2 trusted proxies, got %v, %v", got, err)
	}
}

func testAuthService(t *testing.T) *AuthService {
	t.Helper()
	auth, err := NewAuthService(config.AuthConfig{
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
