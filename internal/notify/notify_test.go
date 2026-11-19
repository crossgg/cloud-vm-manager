package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

func TestCustomWebhookJSONEscaping(t *testing.T) {
	var receivedBody string
	var receivedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		receivedHeader = r.Header.Get("X-Token")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := &CustomWebhookChannel{}
	err := ch.Send(context.Background(), srv.Client(), map[string]string{
		"webhook_url":          srv.URL,
		"webhook_method":       "POST",
		"webhook_content_type": "application/json",
		"webhook_body":         `{"title":"$title","msg":"$content"}`,
		"webhook_headers":      "X-Token: secret-123",
	}, `Alert "High"`, "line1\nline2\\end")
	if err != nil {
		t.Fatalf("CustomWebhookChannel.Send failed: %v", err)
	}
	if receivedHeader != "secret-123" {
		t.Fatalf("expected X-Token header, got %q", receivedHeader)
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(receivedBody), &parsed); err != nil {
		t.Fatalf("expected valid JSON body, got %q: %v", receivedBody, err)
	}
	if parsed["title"] != `Alert "High"` || parsed["msg"] != "line1\nline2\\end" {
		t.Fatalf("unexpected decoded JSON payload: %+v", parsed)
	}
}

func TestBarkAndFeishuChannels(t *testing.T) {
	var barkHit, feishuHit int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.HasPrefix(r.URL.Path, "/bark") {
			atomic.AddInt32(&barkHit, 1)
			var body map[string]interface{}
			_ = json.Unmarshal(b, &body)
			if body["title"] != "Title" {
				t.Fatalf("unexpected bark title: %v", body["title"])
			}
			_, _ = w.Write([]byte(`{"code":200}`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/feishu") {
			atomic.AddInt32(&feishuHit, 1)
			var body map[string]interface{}
			_ = json.Unmarshal(b, &body)
			if body["sign"] == "" || body["timestamp"] == "" {
				t.Fatalf("expected feishu signature fields, got %+v", body)
			}
			_, _ = w.Write([]byte(`{"code":0}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	bark := &BarkChannel{}
	if err := bark.Send(context.Background(), srv.Client(), map[string]string{
		"bark_push":  srv.URL + "/bark/key",
		"bark_group": "ops",
	}, "Title", "Body"); err != nil {
		t.Fatalf("BarkChannel failed: %v", err)
	}

	feishu := &FeishuChannel{}
	if err := feishu.Send(context.Background(), srv.Client(), map[string]string{
		"fskey":    srv.URL + "/feishu/hook",
		"fssecret": "my-secret",
	}, "Title", "Body"); err != nil {
		t.Fatalf("FeishuChannel failed: %v", err)
	}

	if atomic.LoadInt32(&barkHit) != 1 || atomic.LoadInt32(&feishuHit) != 1 {
		t.Fatalf("expected both hits, got bark=%d feishu=%d", barkHit, feishuHit)
	}
}

func TestDispatcherTriggerFilterCooldownAndBotAutoNotify(t *testing.T) {
	var hookCalls int32
	var tgCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/sendMessage") {
			atomic.AddInt32(&tgCalls, 1)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		atomic.AddInt32(&hookCalls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.NotifyConfig{
		EnabledEvents: []string{config.NotifyEventOCITaskSucceeded, config.NotifyEventOCIDTAlert},
		TelegramBot: config.TelegramBotConfig{
			Enabled:           true,
			BotToken:          "123:ABC",
			APIHost:           srv.URL,
			AllowedUserIDs:    []int64{42},
			AutoNotify:        true,
			NotifyEvents:      []string{config.NotifyEventOCITaskSucceeded},
			NotifyCooldownSec: 60,
		},
		Channels: []config.NotifyChannelConfig{
			{
				ID:          "hook-1",
				Name:        "Hook",
				Type:        "custom",
				Enabled:     true,
				CooldownSec: 60,
				Events:      []string{config.NotifyEventOCIDTAlert},
				Params: map[string]string{
					"webhook_url":    srv.URL + "/hook",
					"webhook_method": "POST",
				},
			},
		},
	}

	d := NewDispatcher(cfg, nil)

	// Disabled globally -> should not send
	d.Emit(Event{Type: config.NotifyEventVMPower, Title: "Power"})
	// OCI task succeeded -> should only send to TG Bot AutoNotify
	d.Emit(Event{Type: config.NotifyEventOCITaskSucceeded, Title: "抢机成功", Provider: "oci", Account: "acc1"})
	// OCI DT alert with dedupe -> should send once to hook-1, second ignored by cooldown
	d.Emit(Event{Type: config.NotifyEventOCIDTAlert, Title: "流量告警", DedupeKey: "oci_dt:acc1"})
	d.Emit(Event{Type: config.NotifyEventOCIDTAlert, Title: "流量告警", DedupeKey: "oci_dt:acc1"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&tgCalls) == 1 && atomic.LoadInt32(&hookCalls) == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Give any unexpected extra goroutine a moment to settle
	time.Sleep(80 * time.Millisecond)

	if atomic.LoadInt32(&tgCalls) != 1 {
		t.Fatalf("expected 1 tg bot auto-notify call, got %d", tgCalls)
	}
	if atomic.LoadInt32(&hookCalls) != 1 {
		t.Fatalf("expected 1 webhook call after cooldown, got %d", hookCalls)
	}
}
