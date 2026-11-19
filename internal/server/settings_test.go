package server

import (
	"bytes"
	"encoding/json"
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
	body := []byte(`{"enabled":true,"username":"admin","current_password":"old-password","password":"new-password","session_hours":12,"cookie_secure":false}`)
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

func TestAuthSettingsRequireCurrentPassword(t *testing.T) {
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

	oldHash := hashPassword(t, "old-password")
	cfgContent := strings.Join([]string{
		"auth=begin",
		"[main]",
		"enabled=true",
		"username=admin",
		"password_hash=" + oldHash,
		"session_secret=" + strings.Repeat("a", 32),
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

	router := SetupRouter(authSvc)
	token, err := authSvc.NewSessionToken("admin")
	if err != nil {
		t.Fatal(err)
	}

	// Attempt to disable auth without current_password -> 403
	req := httptest.NewRequest(http.MethodPost, "http://app.example.com/api/settings/auth", bytes.NewBufferString(`{"enabled":false,"username":"admin","session_hours":12}`))
	req.Header.Set("Origin", "http://app.example.com")
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when disabling auth without current password, got %d: %s", w.Code, w.Body.String())
	}

	// Attempt to disable auth with correct current_password -> 200
	req2 := httptest.NewRequest(http.MethodPost, "http://app.example.com/api/settings/auth", bytes.NewBufferString(`{"enabled":false,"username":"admin","current_password":"old-password","session_hours":12}`))
	req2.Header.Set("Origin", "http://app.example.com")
	req2.Header.Set("Content-Type", "application/json")
	req2.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 when disabling auth with valid current password, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestAccountDetailMasksSecretAndValidatesKeyPath(t *testing.T) {
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

	if err := os.MkdirAll(filepath.Join(dir, "config", "keys"), 0700); err != nil {
		t.Fatal(err)
	}
	cfgContent := strings.Join([]string{
		"azure=begin",
		"[az1]",
		"subscription_id=sub-1",
		"tenant_id=ten-1",
		"client_id=cid-1",
		"client_secret=super=secret=value",
		"resource_group=rg-1",
		"location=eastasia",
		"azure=end",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "config", "config.conf"), []byte(cfgContent), 0600); err != nil {
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
	router := SetupRouter(authSvc)

	// 1. Verify getAccountDetail masks client_secret
	req := httptest.NewRequest(http.MethodGet, "http://app.example.com/api/accounts/azure/az1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing X-Content-Type-Options: nosniff header")
	}
	var detail struct {
		HasClientSecret bool              `json:"has_client_secret"`
		Fields          map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if !detail.HasClientSecret || detail.Fields["client_secret"] != accountSecretMask {
		t.Fatalf("expected masked client_secret, got %+v", detail)
	}

	// 2. Saving with masked secret preserves existing secret (even with '=')
	saveBody := `{"provider":"azure","account":"az1","originalAccount":"az1","fields":{"subscription_id":"sub-1","tenant_id":"ten-1","client_id":"cid-1","client_secret":"********","resource_group":"rg-2","location":"eastasia"}}`
	reqSave := httptest.NewRequest(http.MethodPost, "http://app.example.com/api/accounts", bytes.NewBufferString(saveBody))
	reqSave.Header.Set("Content-Type", "application/json")
	wSave := httptest.NewRecorder()
	router.ServeHTTP(wSave, reqSave)
	if wSave.Code != http.StatusOK {
		t.Fatalf("expected 200 on saveAccount, got %d: %s", wSave.Code, wSave.Body.String())
	}
	savedConf, _ := os.ReadFile(filepath.Join(dir, "config", "config.conf"))
	if !strings.Contains(string(savedConf), "client_secret=super=secret=value") {
		t.Fatalf("expected original client_secret preserved, got:\n%s", string(savedConf))
	}

	// 3. Reject newline injection in account field
	injBody := `{"provider":"azure","account":"az1","fields":{"location":"eastasia\n[hacked]"}}`
	reqInj := httptest.NewRequest(http.MethodPost, "http://app.example.com/api/accounts", bytes.NewBufferString(injBody))
	reqInj.Header.Set("Content-Type", "application/json")
	wInj := httptest.NewRecorder()
	router.ServeHTTP(wInj, reqInj)
	if wInj.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for newline config injection, got %d", wInj.Code)
	}

	// 4. Reject key_file outside config/keys
	keyOutBody := `{"provider":"gcp","account":"gcp1","fields":{"project_id":"p1","client_email":"a@b.c","key_file":"/etc/passwd"}}`
	reqKeyOut := httptest.NewRequest(http.MethodPost, "http://app.example.com/api/accounts", bytes.NewBufferString(keyOutBody))
	reqKeyOut.Header.Set("Content-Type", "application/json")
	wKeyOut := httptest.NewRecorder()
	router.ServeHTTP(wKeyOut, reqKeyOut)
	if wKeyOut.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for key_file outside config/keys, got %d: %s", wKeyOut.Code, wKeyOut.Body.String())
	}

	// 5. Accept key_file inside config/keys
	keyInBody := `{"provider":"gcp","account":"gcp1","fields":{"project_id":"p1","client_email":"a@b.c","key_file":"config/keys/gcp1.pem"}}`
	reqKeyIn := httptest.NewRequest(http.MethodPost, "http://app.example.com/api/accounts", bytes.NewBufferString(keyInBody))
	reqKeyIn.Header.Set("Content-Type", "application/json")
	wKeyIn := httptest.NewRecorder()
	router.ServeHTTP(wKeyIn, reqKeyIn)
	if wKeyIn.Code != http.StatusOK {
		t.Fatalf("expected 200 for key_file inside config/keys, got %d: %s", wKeyIn.Code, wKeyIn.Body.String())
	}

	// 6. Verify OCI / GCP detail endpoints mask middle of sensitive fields and preserve on edit
	ociBody := `{"provider":"oci","account":"oci1","fields":{"user":"ocid1.user.oc1..aaaa11112222zzzz","fingerprint":"aa:bb:cc:dd:ee:ff:00:11","tenancy":"ocid1.tenancy.oc1..bbbb33334444yyyy","compartment_id":"ocid1.compartment.oc1..cccc5555xxxx","region":"ap-tokyo-1","key_file":"config/keys/oci1.pem"}}`
	reqOci := httptest.NewRequest(http.MethodPost, "http://app.example.com/api/accounts", bytes.NewBufferString(ociBody))
	reqOci.Header.Set("Content-Type", "application/json")
	wOci := httptest.NewRecorder()
	router.ServeHTTP(wOci, reqOci)
	if wOci.Code != http.StatusOK {
		t.Fatalf("expected 200 saving oci1, got %d: %s", wOci.Code, wOci.Body.String())
	}

	reqOciGet := httptest.NewRequest(http.MethodGet, "http://app.example.com/api/accounts/oci/oci1", nil)
	wOciGet := httptest.NewRecorder()
	router.ServeHTTP(wOciGet, reqOciGet)
	if wOciGet.Code != http.StatusOK {
		t.Fatalf("expected 200 getting oci1, got %d", wOciGet.Code)
	}
	var ociDetail struct {
		Fields map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(wOciGet.Body.Bytes(), &ociDetail); err != nil {
		t.Fatal(err)
	}
	if ociDetail.Fields["region"] != "ap-tokyo-1" {
		t.Fatalf("expected plaintext region, got %q", ociDetail.Fields["region"])
	}
	for _, k := range []string{"user", "fingerprint", "tenancy", "compartment_id", "key_file"} {
		if !strings.Contains(ociDetail.Fields[k], "****") {
			t.Fatalf("expected masked field %s, got %q", k, ociDetail.Fields[k])
		}
	}

	// Edit only region while submitting masked values back -> originals must be preserved
	ociEditPayload, _ := json.Marshal(map[string]interface{}{
		"provider":        "oci",
		"account":         "oci1",
		"originalAccount": "oci1",
		"fields": map[string]string{
			"user":           ociDetail.Fields["user"],
			"fingerprint":    ociDetail.Fields["fingerprint"],
			"tenancy":        ociDetail.Fields["tenancy"],
			"compartment_id": ociDetail.Fields["compartment_id"],
			"region":         "ap-osaka-1",
			"key_file":       ociDetail.Fields["key_file"],
		},
	})
	reqOciEdit := httptest.NewRequest(http.MethodPost, "http://app.example.com/api/accounts", bytes.NewReader(ociEditPayload))
	reqOciEdit.Header.Set("Content-Type", "application/json")
	wOciEdit := httptest.NewRecorder()
	router.ServeHTTP(wOciEdit, reqOciEdit)
	if wOciEdit.Code != http.StatusOK {
		t.Fatalf("expected 200 editing oci1 with masked values, got %d: %s", wOciEdit.Code, wOciEdit.Body.String())
	}
	updatedConf, _ := os.ReadFile(filepath.Join(dir, "config", "config.conf"))
	if !strings.Contains(string(updatedConf), "user=ocid1.user.oc1..aaaa11112222zzzz") ||
		!strings.Contains(string(updatedConf), "key_file=config/keys/oci1.pem") ||
		!strings.Contains(string(updatedConf), "region=ap-osaka-1") {
		t.Fatalf("expected original OCI credentials preserved and region updated, got:\n%s", string(updatedConf))
	}

	// 7. Reject backslash traversal on /public/
	reqTrav := httptest.NewRequest(http.MethodGet, `http://app.example.com/public/..\config\config.conf`, nil)
	wTrav := httptest.NewRecorder()
	router.ServeHTTP(wTrav, reqTrav)
	if wTrav.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for backslash traversal, got %d", wTrav.Code)
	}
}
