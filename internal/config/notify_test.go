package config

import (
	"path/filepath"
	"testing"
)

func TestNotifyConfigSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notify.conf")

	original := NotifyConfig{
		EnabledEvents: []string{
			NotifyEventOCITaskSucceeded,
			NotifyEventAzureCreditAlert,
			NotifyEventAuthLogin,
		},
		TelegramBot: TelegramBotConfig{
			Enabled:           true,
			BotToken:          "123456:ABC-DEF",
			APIHost:           "https://api.telegram.org",
			ProxyID:           "proxy-1",
			FallbackProxyID:   ProxyFallbackDirect,
			AllowedUserIDs:    []int64{10001, 10002},
			AllowedChatIDs:    []int64{-100999},
			AutoNotify:        true,
			NotifyChatIDs:     []int64{10001},
			NotifyEvents:      []string{NotifyEventOCITaskSucceeded, NotifyEventAuthLogin},
			NotifyCooldownSec: 120,
		},
		Channels: []NotifyChannelConfig{
			{
				ID:              "bark-1",
				Name:            "我的 Bark",
				Type:            "bark",
				Enabled:         true,
				ProxyID:         ProxyFallbackNone,
				FallbackProxyID: ProxyFallbackNone,
				CooldownSec:     60,
				Events:          []string{NotifyEventOCITaskSucceeded},
				Accounts:        []string{"oci/shuai-jp"},
				Params: map[string]string{
					"bark_push":  "https://api.day.app/abc",
					"bark_group": "vm",
				},
			},
			{
				ID:              "custom-1",
				Name:            "自定义 Hook",
				Type:            "custom",
				Enabled:         true,
				ProxyID:         "proxy-1",
				FallbackProxyID: ProxyFallbackNone,
				CooldownSec:     0,
				Events:          []string{NotifyEventVMChangeIP},
				Params: map[string]string{
					"webhook_url":          "https://example.com/hook?t=$title",
					"webhook_method":       "POST",
					"webhook_content_type": "application/json",
					"webhook_body":         "{\"title\":\"$title\",\"content\":\"$content\"}",
					"webhook_headers":      "Authorization: Bearer abc\nX-Custom: 1\\2",
				},
			},
		},
	}

	if err := SaveNotifyConfig(path, original); err != nil {
		t.Fatalf("SaveNotifyConfig failed: %v", err)
	}

	var loadedCfg Config
	if err := LoadNotifyConfig(path, &loadedCfg); err != nil {
		t.Fatalf("LoadNotifyConfig failed: %v", err)
	}

	got := loadedCfg.Notify
	if !got.TelegramBot.Enabled || got.TelegramBot.BotToken != "123456:ABC-DEF" {
		t.Fatalf("unexpected telegram bot config: %+v", got.TelegramBot)
	}
	if len(got.TelegramBot.AllowedUserIDs) != 2 || got.TelegramBot.AllowedUserIDs[0] != 10001 {
		t.Fatalf("unexpected allowedUserIds: %+v", got.TelegramBot.AllowedUserIDs)
	}
	if len(got.Channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(got.Channels))
	}
	if got.Channels[1].Params["webhook_headers"] != "Authorization: Bearer abc\nX-Custom: 1\\2" {
		t.Fatalf("multiline header roundtrip mismatch: %q", got.Channels[1].Params["webhook_headers"])
	}
	if got.Channels[1].Params["webhook_body"] != "{\"title\":\"$title\",\"content\":\"$content\"}" {
		t.Fatalf("webhook_body roundtrip mismatch: %q", got.Channels[1].Params["webhook_body"])
	}
}

func TestValidateNotifyProxies(t *testing.T) {
	cfg := DefaultNotifyConfig()
	cfg.TelegramBot.ProxyID = "missing-proxy"
	err := ValidateNotifyProxies(cfg, []ProxyConfig{{ID: "proxy-1", URL: "socks5://127.0.0.1:1080"}})
	if err == nil {
		t.Fatal("expected error for missing proxy")
	}
}
