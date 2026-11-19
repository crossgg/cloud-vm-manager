package notify

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/crossgg/cloud-vm-manager/internal/proxy"
)

const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"

	maxDeliveryLogs = 80
)

type Event struct {
	Type         string            `json:"type"`
	Severity     string            `json:"severity"`
	Title        string            `json:"title"`
	Content      string            `json:"content"`
	Provider     string            `json:"provider,omitempty"`
	Account      string            `json:"account,omitempty"`
	ResourceID   string            `json:"resourceId,omitempty"`
	ResourceName string            `json:"resourceName,omitempty"`
	DedupeKey    string            `json:"dedupeKey,omitempty"`
	Fields       map[string]string `json:"fields,omitempty"`
	Timestamp    time.Time         `json:"timestamp"`
}

type DeliveryLog struct {
	Time        string `json:"time"`
	EventType   string `json:"eventType"`
	Target      string `json:"target"`
	ChannelType string `json:"channelType"`
	Title       string `json:"title"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
}

type Dispatcher struct {
	mu          sync.RWMutex
	cfg         config.NotifyConfig
	proxyRouter *proxy.ProxyRouter
	cooldowns   map[string]time.Time
	logs        []DeliveryLog
	sem         chan struct{}
}

func NewDispatcher(cfg config.NotifyConfig, proxyRouter *proxy.ProxyRouter) *Dispatcher {
	return &Dispatcher{
		cfg:         cfg.Normalize(),
		proxyRouter: proxyRouter,
		cooldowns:   make(map[string]time.Time),
		sem:         make(chan struct{}, 8),
	}
}

func (d *Dispatcher) Update(cfg config.NotifyConfig, proxyRouter *proxy.ProxyRouter) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cfg = cfg.Normalize()
	d.proxyRouter = proxyRouter
}

func (d *Dispatcher) Config() config.NotifyConfig {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg
}

func (d *Dispatcher) RecentLogs() []DeliveryLog {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]DeliveryLog, len(d.logs))
	copy(out, d.logs)
	return out
}

func (d *Dispatcher) Emit(ev Event) {
	if d == nil {
		return
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	if ev.Severity == "" {
		ev.Severity = SeverityInfo
	}

	d.mu.RLock()
	cfg := d.cfg
	router := d.proxyRouter
	d.mu.RUnlock()

	if !containsFold(cfg.EnabledEvents, ev.Type) {
		return
	}

	title, content := formatPlainEvent(ev)

	for _, chCfg := range cfg.Channels {
		if !chCfg.Enabled {
			continue
		}
		if !containsFold(chCfg.Events, ev.Type) {
			continue
		}
		if !matchesAccountFilter(chCfg.Accounts, ev.Provider, ev.Account) {
			continue
		}
		if !d.allowSend("ch:"+chCfg.ID, ev.DedupeKey, chCfg.CooldownSec, ev.Timestamp) {
			continue
		}
		chCopy := chCfg
		d.dispatchAsync(func(ctx context.Context) {
			err := d.sendChannel(ctx, router, chCopy, title, content)
			d.recordLog(DeliveryLog{
				Time:        time.Now().Format("2006-01-02 15:04:05"),
				EventType:   ev.Type,
				Target:      valueOrFallback(chCopy.Name, chCopy.ID),
				ChannelType: chCopy.Type,
				Title:       title,
				Success:     err == nil,
				Error:       errString(err),
			})
		})
	}

	bot := cfg.TelegramBot
	if bot.Enabled && bot.AutoNotify && bot.BotToken != "" && containsFold(bot.NotifyEvents, ev.Type) {
		chatIDs := bot.EffectiveNotifyChatIDs()
		if len(chatIDs) > 0 && d.allowSend("tgbot", ev.DedupeKey, bot.NotifyCooldownSec, ev.Timestamp) {
			botCopy := bot
			htmlText := formatTelegramHTMLEvent(ev)
			d.dispatchAsync(func(ctx context.Context) {
				err := d.sendBotMessage(ctx, router, botCopy, chatIDs, htmlText)
				d.recordLog(DeliveryLog{
					Time:        time.Now().Format("2006-01-02 15:04:05"),
					EventType:   ev.Type,
					Target:      "Telegram Bot (自动通知)",
					ChannelType: "telegram_bot",
					Title:       title,
					Success:     err == nil,
					Error:       errString(err),
				})
			})
		}
	}
}

func (d *Dispatcher) TestChannel(ctx context.Context, chCfg config.NotifyChannelConfig, router *proxy.ProxyRouter) error {
	chImpl, ok := LookupChannel(chCfg.Type)
	if !ok {
		return fmt.Errorf("不支持的通知渠道类型 %q", chCfg.Type)
	}
	if err := chImpl.ValidateParams(chCfg.Params); err != nil {
		return err
	}
	if router == nil {
		d.mu.RLock()
		router = d.proxyRouter
		d.mu.RUnlock()
	}
	client, err := buildHTTPClient(router, chCfg.ProxyID, chCfg.FallbackProxyID, 20*time.Second)
	if err != nil {
		return err
	}
	title := "cloud-vm-manager 测试通知"
	content := fmt.Sprintf("渠道: %s (%s)\n时间: %s\n状态: 配置正常，通知可达。",
		valueOrFallback(chCfg.Name, chCfg.ID), chCfg.Type, time.Now().Format("2006-01-02 15:04:05"))
	err = chImpl.Send(ctx, client, chCfg.Params, title, content)
	d.recordLog(DeliveryLog{
		Time:        time.Now().Format("2006-01-02 15:04:05"),
		EventType:   "test",
		Target:      valueOrFallback(chCfg.Name, chCfg.ID),
		ChannelType: chCfg.Type,
		Title:       title,
		Success:     err == nil,
		Error:       errString(err),
	})
	return err
}

func (d *Dispatcher) TestTelegramBotNotify(ctx context.Context, bot config.TelegramBotConfig, router *proxy.ProxyRouter) error {
	if strings.TrimSpace(bot.BotToken) == "" {
		return fmt.Errorf("请先填写 Bot Token")
	}
	chatIDs := bot.EffectiveNotifyChatIDs()
	if len(chatIDs) == 0 {
		return fmt.Errorf("未配置接收通知的 Chat ID 或白名单 User ID")
	}
	if router == nil {
		d.mu.RLock()
		router = d.proxyRouter
		d.mu.RUnlock()
	}
	msg := fmt.Sprintf("<b>🤖 cloud-vm-manager Bot 测试通知</b>\n\n时间: <code>%s</code>\n说明: Telegram Bot 推送通道连通正常。",
		html.EscapeString(time.Now().Format("2006-01-02 15:04:05")))
	err := d.sendBotMessage(ctx, router, bot, chatIDs, msg)
	d.recordLog(DeliveryLog{
		Time:        time.Now().Format("2006-01-02 15:04:05"),
		EventType:   "test",
		Target:      "Telegram Bot",
		ChannelType: "telegram_bot",
		Title:       "Bot 测试通知",
		Success:     err == nil,
		Error:       errString(err),
	})
	return err
}

func (d *Dispatcher) sendChannel(ctx context.Context, router *proxy.ProxyRouter, chCfg config.NotifyChannelConfig, title, content string) error {
	chImpl, ok := LookupChannel(chCfg.Type)
	if !ok {
		return fmt.Errorf("unsupported channel type %q", chCfg.Type)
	}
	client, err := buildHTTPClient(router, chCfg.ProxyID, chCfg.FallbackProxyID, 20*time.Second)
	if err != nil {
		return err
	}
	return chImpl.Send(ctx, client, chCfg.Params, title, content)
}

func (d *Dispatcher) sendBotMessage(ctx context.Context, router *proxy.ProxyRouter, bot config.TelegramBotConfig, chatIDs []int64, htmlText string) error {
	client, err := buildHTTPClient(router, bot.ProxyID, bot.FallbackProxyID, 20*time.Second)
	if err != nil {
		return err
	}
	var errs []string
	for _, cid := range chatIDs {
		if err := SendTelegramText(ctx, client, bot.APIHost, bot.BotToken, strconv.FormatInt(cid, 10), htmlText, "HTML"); err != nil {
			errs = append(errs, fmt.Sprintf("chat %d: %v", cid, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func buildHTTPClient(router *proxy.ProxyRouter, proxyID, fallbackID string, timeout time.Duration) (*http.Client, error) {
	if router == nil {
		return &http.Client{Timeout: timeout, Transport: proxy.DirectTransport()}, nil
	}
	return router.HTTPClientByID(proxyID, fallbackID, timeout)
}

func (d *Dispatcher) dispatchAsync(fn func(ctx context.Context)) {
	go func() {
		d.sem <- struct{}{}
		defer func() { <-d.sem }()
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		fn(ctx)
	}()
}

func (d *Dispatcher) allowSend(targetKey, dedupeKey string, cooldownSec int, now time.Time) bool {
	if dedupeKey == "" || cooldownSec <= 0 {
		return true
	}
	fullKey := targetKey + "::" + dedupeKey
	d.mu.Lock()
	defer d.mu.Unlock()
	if last, ok := d.cooldowns[fullKey]; ok {
		if now.Sub(last) < time.Duration(cooldownSec)*time.Second {
			return false
		}
	}
	d.cooldowns[fullKey] = now
	if len(d.cooldowns) > 1000 {
		cutoff := now.Add(-24 * time.Hour)
		for k, ts := range d.cooldowns {
			if ts.Before(cutoff) {
				delete(d.cooldowns, k)
			}
		}
	}
	return true
}

func (d *Dispatcher) recordLog(entry DeliveryLog) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.logs = append([]DeliveryLog{entry}, d.logs...)
	if len(d.logs) > maxDeliveryLogs {
		d.logs = d.logs[:maxDeliveryLogs]
	}
}

func formatPlainEvent(ev Event) (string, string) {
	title := strings.TrimSpace(ev.Title)
	if title == "" {
		title = "cloud-vm-manager 通知"
	}
	var lines []string
	if ev.Provider != "" || ev.Account != "" {
		lines = append(lines, fmt.Sprintf("云平台/账号: %s / %s", strings.ToUpper(ev.Provider), valueOrFallback(ev.Account, "-")))
	}
	if ev.ResourceName != "" || ev.ResourceID != "" {
		res := ev.ResourceName
		if ev.ResourceID != "" && ev.ResourceID != ev.ResourceName {
			if res != "" {
				res += " (" + ev.ResourceID + ")"
			} else {
				res = ev.ResourceID
			}
		}
		lines = append(lines, "实例/资源: "+res)
	}
	if strings.TrimSpace(ev.Content) != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, strings.TrimSpace(ev.Content))
	}
	lines = append(lines, fmt.Sprintf("\n时间: %s", ev.Timestamp.Format("2006-01-02 15:04:05")))
	return title, strings.Join(lines, "\n")
}

func formatTelegramHTMLEvent(ev Event) string {
	icon := "ℹ️"
	switch ev.Severity {
	case SeverityWarning:
		icon = "⚠️"
	case SeverityCritical:
		icon = "🚨"
	default:
		if ev.Type == config.NotifyEventOCITaskSucceeded {
			icon = "🎉"
		}
	}
	title := strings.TrimSpace(ev.Title)
	if title == "" {
		title = "cloud-vm-manager 通知"
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("<b>%s %s</b>", icon, html.EscapeString(title)))
	if ev.Provider != "" || ev.Account != "" {
		lines = append(lines, fmt.Sprintf("云平台/账号: <code>%s / %s</code>",
			html.EscapeString(strings.ToUpper(ev.Provider)),
			html.EscapeString(valueOrFallback(ev.Account, "-"))))
	}
	if ev.ResourceName != "" || ev.ResourceID != "" {
		res := ev.ResourceName
		if ev.ResourceID != "" && ev.ResourceID != ev.ResourceName {
			if res != "" {
				res += " (" + ev.ResourceID + ")"
			} else {
				res = ev.ResourceID
			}
		}
		lines = append(lines, "资源: <code>"+html.EscapeString(res)+"</code>")
	}
	if strings.TrimSpace(ev.Content) != "" {
		lines = append(lines, "")
		lines = append(lines, html.EscapeString(strings.TrimSpace(ev.Content)))
	}
	lines = append(lines, fmt.Sprintf("\n<i>%s</i>", html.EscapeString(ev.Timestamp.Format("2006-01-02 15:04:05"))))
	return strings.Join(lines, "\n")
}

func containsFold(items []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), target) {
			return true
		}
	}
	return false
}

func matchesAccountFilter(accounts []string, provider, account string) bool {
	if len(accounts) == 0 {
		return true
	}
	full := strings.ToLower(strings.TrimSpace(provider)) + "/" + strings.TrimSpace(account)
	for _, item := range accounts {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" || trimmed == "*" {
			return true
		}
		if strings.EqualFold(trimmed, full) || strings.EqualFold(trimmed, account) {
			return true
		}
	}
	return false
}

func valueOrFallback(val, fallback string) string {
	if strings.TrimSpace(val) != "" {
		return val
	}
	return fallback
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
