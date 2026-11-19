package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

type mockBackend struct {
	mu            sync.Mutex
	powerCalls    []string
	changeIPCalls int
	dnsCalls      int
	taskStarted   bool
	taskStopped   bool
}

func (m *mockBackend) AccountsByProvider(provider string) []AccountSummary {
	return []AccountSummary{{Provider: provider, Account: provider + "-acc1", Group: "prod"}}
}

func (m *mockBackend) GetOverview() OverviewSummary {
	return OverviewSummary{ConfigCount: 3, InstanceCount: 2, RunningCount: 1, StoppedCount: 1, CachedAccountCount: 2}
}

func (m *mockBackend) ListVMs(provider, account string) ([]VMSummary, error) {
	return []VMSummary{{
		ID:       "ocid1.instance.oc1.ap-tokyo-1.very-long-identifier-exceeding-sixty-four-bytes-1234567890",
		Name:     "arm-vm-1",
		State:    "RUNNING",
		PublicIP: "1.2.3.4",
	}}, nil
}

func (m *mockBackend) GetVM(provider, account, vmID string) (*VMDetail, error) {
	return &VMDetail{
		ID:            vmID,
		Name:          "arm-vm-1",
		State:         "RUNNING",
		PublicIP:      "1.2.3.4",
		HasDNSBinding: true,
	}, nil
}

func (m *mockBackend) PowerVM(provider, account, vmID, action string) error {
	m.mu.Lock()
	m.powerCalls = append(m.powerCalls, action)
	m.mu.Unlock()
	return nil
}

func (m *mockBackend) ChangeVMIP(provider, account, vmID string) (*ChangeIPOutcome, error) {
	m.mu.Lock()
	m.changeIPCalls++
	m.mu.Unlock()
	return &ChangeIPOutcome{
		Success:       true,
		Message:       "IP changed",
		OldIP:         "1.2.3.4",
		NewIP:         "5.6.7.8",
		Logs:          []string{"detached old ip", "attached 5.6.7.8"},
		HasDNSBinding: true,
	}, nil
}

func (m *mockBackend) UpdateVMDNS(provider, account, vmID string) ([]string, error) {
	m.mu.Lock()
	m.dnsCalls++
	m.mu.Unlock()
	return []string{"updated A record to 5.6.7.8"}, nil
}

func (m *mockBackend) ListOCICreateTasks(account string) ([]OCITaskSummary, error) {
	return []OCITaskSummary{{ID: "task-1", Name: "ARM抢购", Account: account, Status: "stopped", Shape: "VM.Standard.A1.Flex", Attempts: 10}}, nil
}

func (m *mockBackend) GetOCICreateTask(account, taskID string) (*OCITaskDetail, error) {
	return &OCITaskDetail{
		OCITaskSummary: OCITaskSummary{ID: taskID, Name: "ARM抢购", Account: account, Status: "stopped", Shape: "VM.Standard.A1.Flex", Attempts: 10},
		Logs:           []string{"[10:00:00] retry 10"},
	}, nil
}

func (m *mockBackend) StartOCICreateTask(account, taskID string) (*OCITaskDetail, error) {
	m.mu.Lock()
	m.taskStarted = true
	m.mu.Unlock()
	return &OCITaskDetail{OCITaskSummary: OCITaskSummary{ID: taskID, Name: "ARM抢购", Account: account, Status: "running", Shape: "VM.Standard.A1.Flex"}}, nil
}

func (m *mockBackend) StopOCICreateTask(account, taskID string) (*OCITaskDetail, error) {
	m.mu.Lock()
	m.taskStopped = true
	m.mu.Unlock()
	return &OCITaskDetail{OCITaskSummary: OCITaskSummary{ID: taskID, Name: "ARM抢购", Account: account, Status: "stopped", Shape: "VM.Standard.A1.Flex"}}, nil
}

func (m *mockBackend) GetOCIDTMonitor(account string) (*OCIDTStatus, error) {
	return &OCIDTStatus{Account: account, Running: true, Threshold: 9000, UsageGB: 120.5, Percentage: 1.3, QueryTime: "2026-11-18 20:00:00", Logs: []string{"ok"}}, nil
}

func (m *mockBackend) QueryOCIDTMonitorNow(account string) (*OCIDTStatus, error) {
	return &OCIDTStatus{Account: account, Running: true, Threshold: 9000, UsageGB: 125.0, Percentage: 1.4, QueryTime: "2026-11-18 20:05:00", Logs: []string{"queried"}}, nil
}

func (m *mockBackend) GetAzureCreditMonitor(account string) (*AzureMonitorSnapshot, error) {
	return &AzureMonitorSnapshot{Account: account, Running: true, TriggerAction: "alert", MonthCostUSD: 12.34, CostStatus: "ok", QueriedAt: "2026-11-18 20:00:00"}, nil
}

func (m *mockBackend) QueryAzureCreditMonitorNow(account string) (*AzureMonitorSnapshot, error) {
	return &AzureMonitorSnapshot{Account: account, Running: true, TriggerAction: "alert", MonthCostUSD: 12.50, CostStatus: "ok", QueriedAt: "2026-11-18 20:05:00"}, nil
}

type recordedTGCall struct {
	Method string
	Body   map[string]interface{}
}

func TestBotAuthorizationAndChangeIPWithDNSOptionB(t *testing.T) {
	var mu sync.Mutex
	var calls []recordedTGCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]interface{}
		_ = json.Unmarshal(b, &body)
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		method := parts[len(parts)-1]
		mu.Lock()
		calls = append(calls, recordedTGCall{Method: method, Body: body})
		mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer srv.Close()

	backend := &mockBackend{}
	cfg := config.TelegramBotConfig{
		Enabled:        true,
		BotToken:       "123:TEST",
		APIHost:        srv.URL,
		AllowedUserIDs: []int64{777},
	}
	bot := NewBotService(cfg, nil, backend)
	client := newAPIClient(srv.Client(), srv.URL, cfg.BotToken)

	// 1. Unauthorized user
	bot.handleUpdate(context.Background(), client, &tgUpdate{
		Message: &tgMessage{MessageID: 1, From: &tgUser{ID: 999}, Chat: tgChat{ID: 999}, Text: "/oracle"},
	})
	mu.Lock()
	if len(calls) != 1 || !strings.Contains(calls[0].Body["text"].(string), "未授权") {
		t.Fatalf("expected unauthorized rejection, got %+v", calls)
	}
	calls = nil
	mu.Unlock()

	// 2. Authorized /help and /oracle
	bot.handleUpdate(context.Background(), client, &tgUpdate{
		Message: &tgMessage{MessageID: 2, From: &tgUser{ID: 777}, Chat: tgChat{ID: 777}, Text: "/help"},
	})
	bot.handleUpdate(context.Background(), client, &tgUpdate{
		Message: &tgMessage{MessageID: 3, From: &tgUser{ID: 777}, Chat: tgChat{ID: 777}, Text: "/oracle"},
	})

	// 3. Ensure long OCID VM list keeps callback_data <= 64 bytes
	_, mk := bot.renderVMList("oci", "oci-acc1", 0)
	for _, row := range mk.InlineKeyboard {
		for _, btn := range row {
			if len(btn.CallbackData) > 64 {
				t.Fatalf("callback_data exceeds 64 bytes: %d (%q)", len(btn.CallbackData), btn.CallbackData)
			}
		}
	}

	// 4. Execute ChangeIP via callback and verify Option B DNS sync button appears
	ref := callbackRef{
		Provider: "oci",
		Account:  "oci-acc1",
		TargetID: "ocid1.instance.oc1.ap-tokyo-1.very-long-identifier-exceeding-sixty-four-bytes-1234567890",
		Extra:    "arm-vm-1",
	}
	tok := bot.callbacks.Put(ref)
	bot.handleCallback(context.Background(), client, &tgCallbackQuery{
		ID:      "cq-1",
		From:    tgUser{ID: 777},
		Message: &tgMessage{MessageID: 10, Chat: tgChat{ID: 777}},
		Data:    "vc:" + tok + ":changeip",
	})

	deadline := time.Now().Add(2 * time.Second)
	var editBody map[string]interface{}
	for time.Now().Before(deadline) {
		mu.Lock()
		for _, c := range calls {
			if c.Method == "editMessageText" {
				if txt, ok := c.Body["text"].(string); ok && strings.Contains(txt, "5.6.7.8") {
					editBody = c.Body
				}
			}
		}
		mu.Unlock()
		if editBody != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if editBody == nil {
		t.Fatal("expected editMessageText with new IP 5.6.7.8")
	}

	// Check Option B button
	markupJSON, _ := json.Marshal(editBody["reply_markup"])
	if !strings.Contains(string(markupJSON), "同步更新 DNS") {
		t.Fatalf("expected Option B Sync DNS button in reply_markup, got %s", string(markupJSON))
	}

	// Trigger DNS update button
	var dnsCallbackData string
	var parsedMarkup inlineKeyboardMarkup
	_ = json.Unmarshal(markupJSON, &parsedMarkup)
	for _, row := range parsedMarkup.InlineKeyboard {
		for _, btn := range row {
			if strings.HasPrefix(btn.CallbackData, "dns:") {
				dnsCallbackData = btn.CallbackData
			}
		}
	}
	if dnsCallbackData == "" {
		t.Fatal("missing dns: callback button")
	}

	bot.handleCallback(context.Background(), client, &tgCallbackQuery{
		ID:      "cq-2",
		From:    tgUser{ID: 777},
		Message: &tgMessage{MessageID: 10, Chat: tgChat{ID: 777}},
		Data:    dnsCallbackData,
	})

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		backend.mu.Lock()
		dc := backend.dnsCalls
		backend.mu.Unlock()
		if dc == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	backend.mu.Lock()
	if backend.dnsCalls != 1 {
		t.Fatalf("expected UpdateVMDNS to be called once, got %d", backend.dnsCalls)
	}
	backend.mu.Unlock()
}
