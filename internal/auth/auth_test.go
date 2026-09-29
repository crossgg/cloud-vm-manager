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
