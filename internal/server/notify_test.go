package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/crossgg/cloud-vm-manager/internal/auth"
	"github.com/crossgg/cloud-vm-manager/internal/config"
)

func TestNotifyConfigAPIAndMaskRestore(t *testing.T) {
	dir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	if err := os.MkdirAll("config", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("config", "config.conf"), []byte("auth=begin\n[main]\nenabled=false\nauth=end\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var testWebhookHits int32
	hookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&testWebhookHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer hookSrv.Close()

	cfg, cfgPath, err := config.LoadConfigWithPath()
	if err != nil {
		t.Fatalf("LoadConfigWithPath failed: %v", err)
	}
	authSvc, err := auth.NewAuthService(cfg.Auth)
	if err != nil {
		t.Fatalf("NewAuthService failed: %v", err)
	}
	if err := InitRuntime(cfg, cfgPath, authSvc); err != nil {
		t.Fatalf("InitRuntime failed: %v", err)
	}
	defer func() {
		if bot := currentTelegramBot(); bot != nil {
			bot.Stop()
		}
	}()

	r := SetupRouter(authSvc)

	// 1. Save notify config with channel + masked bot token off
	payload := config.NotifyConfig{
		EnabledEvents: []string{config.NotifyEventOCITaskSucceeded, config.NotifyEventAuthLogin},
		TelegramBot: config.TelegramBotConfig{
			Enabled:           false,
			BotToken:          "999:SECRET-TOKEN",
			AutoNotify:        true,
			NotifyEvents:      []string{config.NotifyEventOCITaskSucceeded},
			NotifyCooldownSec: 60,
		},
		Channels: []config.NotifyChannelConfig{
			{
				ID:          "bark-1",
				Name:        "Bark 测试",
				Type:        "bark",
				Enabled:     true,
				CooldownSec: 60,
				Events:      []string{config.NotifyEventOCITaskSucceeded},
				Params: map[string]string{
					"bark_push": "my-secret-bark-key",
				},
			},
		},
	}
	bodyBytes, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/notify/config", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/notify/config status=%d body=%s", w.Code, w.Body.String())
	}

	// 2. GET /api/notify/config should mask secrets
	getReq := httptest.NewRequest(http.MethodGet, "/api/notify/config", nil)
	getW := httptest.NewRecorder()
	r.ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("GET /api/notify/config status=%d", getW.Code)
	}
	var getResp struct {
		Config config.NotifyConfig `json:"config"`
	}
	if err := json.Unmarshal(getW.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if getResp.Config.TelegramBot.BotToken != notifySecretMask {
		t.Fatalf("expected masked botToken, got %q", getResp.Config.TelegramBot.BotToken)
	}
	if getResp.Config.Channels[0].Params["bark_push"] != notifySecretMask {
		t.Fatalf("expected masked bark_push, got %q", getResp.Config.Channels[0].Params["bark_push"])
	}

	// 3. Re-saving with masked secrets should preserve underlying real secrets
	put2Bytes, _ := json.Marshal(getResp.Config)
	put2Req := httptest.NewRequest(http.MethodPut, "/api/notify/config", bytes.NewReader(put2Bytes))
	put2Req.Header.Set("Content-Type", "application/json")
	put2W := httptest.NewRecorder()
	r.ServeHTTP(put2W, put2Req)
	if put2W.Code != http.StatusOK {
		t.Fatalf("second PUT failed: %d %s", put2W.Code, put2W.Body.String())
	}
	savedCfg, _, _ := notifyStateSnapshot()
	if savedCfg.TelegramBot.BotToken != "999:SECRET-TOKEN" {
		t.Fatalf("expected preserved botToken, got %q", savedCfg.TelegramBot.BotToken)
	}
	if savedCfg.Channels[0].Params["bark_push"] != "my-secret-bark-key" {
		t.Fatalf("expected preserved bark_push, got %q", savedCfg.Channels[0].Params["bark_push"])
	}

	// 4. Test channel endpoint
	testPayload := config.NotifyChannelConfig{
		ID:   "custom-test",
		Name: "HookTest",
		Type: "custom",
		Params: map[string]string{
			"webhook_url":          hookSrv.URL + "/notify",
			"webhook_method":       "POST",
			"webhook_content_type": "application/json",
			"webhook_body":         `{"title":"$title","body":"$content"}`,
		},
	}
	testBytes, _ := json.Marshal(testPayload)
	testReq := httptest.NewRequest(http.MethodPost, "/api/notify/test-channel", bytes.NewReader(testBytes))
	testReq.Header.Set("Content-Type", "application/json")
	testW := httptest.NewRecorder()
	r.ServeHTTP(testW, testReq)
	if testW.Code != http.StatusOK {
		t.Fatalf("POST /api/notify/test-channel status=%d body=%s", testW.Code, testW.Body.String())
	}
	if atomic.LoadInt32(&testWebhookHits) != 1 {
		t.Fatalf("expected 1 webhook test hit, got %d", testWebhookHits)
	}

	// 5. GET /api/notify/logs should record the test delivery
	logsReq := httptest.NewRequest(http.MethodGet, "/api/notify/logs", nil)
	logsW := httptest.NewRecorder()
	r.ServeHTTP(logsW, logsReq)
	if logsW.Code != http.StatusOK || !strings.Contains(logsW.Body.String(), "HookTest") {
		t.Fatalf("expected HookTest in notify logs, got %d %s", logsW.Code, logsW.Body.String())
	}
}
