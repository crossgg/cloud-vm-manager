package telegram

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

const vmPageSize = 8

type BotService struct {
	mu          sync.RWMutex
	cfg         config.TelegramBotConfig
	proxyRouter *proxy.ProxyRouter
	backend     Backend
	client      *apiClient
	callbacks   *callbackStore

	cancelFunc context.CancelFunc
	doneCh     chan struct{}
	status     BotStatus
}

func NewBotService(cfg config.TelegramBotConfig, router *proxy.ProxyRouter, backend Backend) *BotService {
	return &BotService{
		cfg:         cfg,
		proxyRouter: router,
		backend:     backend,
		callbacks:   newCallbackStore(),
		status: BotStatus{
			Enabled:         cfg.Enabled,
			ProxyID:         cfg.ProxyID,
			FallbackProxyID: cfg.FallbackProxyID,
			AutoNotify:      cfg.AutoNotify,
		},
	}
}

func (b *BotService) Reload(cfg config.TelegramBotConfig, router *proxy.ProxyRouter) error {
	b.Stop()

	b.mu.Lock()
	b.cfg = cfg
	b.proxyRouter = router
	b.status = BotStatus{
		Enabled:         cfg.Enabled,
		ProxyID:         cfg.ProxyID,
		FallbackProxyID: cfg.FallbackProxyID,
		AutoNotify:      cfg.AutoNotify,
	}
	if cfg.Enabled && len(cfg.AllowedUserIDs) == 0 && len(cfg.AllowedChatIDs) == 0 {
		b.status.WhitelistWarning = "未配置允许的 User ID 或 Chat ID，Bot 将拒绝所有交互指令"
	}
	b.mu.Unlock()

	if !cfg.Enabled || strings.TrimSpace(cfg.BotToken) == "" {
		return nil
	}
	return b.Start()
}

func (b *BotService) Start() error {
	b.mu.Lock()
	if b.cancelFunc != nil {
		b.mu.Unlock()
		return nil
	}
	cfg := b.cfg
	router := b.proxyRouter
	if !cfg.Enabled || strings.TrimSpace(cfg.BotToken) == "" {
		b.mu.Unlock()
		return nil
	}
	httpClient, err := buildBotHTTPClient(router, cfg.ProxyID, cfg.FallbackProxyID, 40*time.Second)
	if err != nil {
		b.status.LastError = err.Error()
		b.mu.Unlock()
		return err
	}
	client := newAPIClient(httpClient, cfg.APIHost, cfg.BotToken)
	ctx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan struct{})
	b.client = client
	b.cancelFunc = cancel
	b.doneCh = doneCh
	b.status.Running = true
	b.status.LastError = ""
	b.mu.Unlock()

	go b.pollLoop(ctx, client, doneCh)
	return nil
}

func (b *BotService) Stop() {
	b.mu.Lock()
	cancel := b.cancelFunc
	doneCh := b.doneCh
	b.cancelFunc = nil
	b.doneCh = nil
	b.status.Running = false
	b.status.Connected = false
	b.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if doneCh != nil {
		<-doneCh
	}
}

func (b *BotService) Status() BotStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status
}

func TestBotConnection(ctx context.Context, cfg config.TelegramBotConfig, router *proxy.ProxyRouter) (string, error) {
	if strings.TrimSpace(cfg.BotToken) == "" {
		return "", fmt.Errorf("请先填写 Bot Token")
	}
	httpClient, err := buildBotHTTPClient(router, cfg.ProxyID, cfg.FallbackProxyID, 20*time.Second)
	if err != nil {
		return "", err
	}
	client := newAPIClient(httpClient, cfg.APIHost, cfg.BotToken)
	me, err := client.GetMe(ctx)
	if err != nil {
		return "", err
	}
	return me.Username, nil
}

func buildBotHTTPClient(router *proxy.ProxyRouter, proxyID, fallbackID string, timeout time.Duration) (*http.Client, error) {
	if router == nil {
		return &http.Client{Timeout: timeout, Transport: proxy.DirectTransport()}, nil
	}
	return router.HTTPClientByID(proxyID, fallbackID, timeout)
}

func (b *BotService) pollLoop(ctx context.Context, client *apiClient, doneCh chan struct{}) {
	defer close(doneCh)

	backoff := 2 * time.Second
	commandsRegistered := false
	var offset int64

	for {
		if ctx.Err() != nil {
			return
		}

		if !commandsRegistered {
			meCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			me, err := client.GetMe(meCtx)
			cancel()
			if err != nil {
				b.setPollError(err)
				if !sleepWithContext(ctx, backoff) {
					return
				}
				backoff = nextBackoff(backoff)
				continue
			}
			b.mu.Lock()
			b.status.Connected = true
			b.status.BotUsername = me.Username
			b.status.LastError = ""
			b.mu.Unlock()

			cmdCtx, cmdCancel := context.WithTimeout(ctx, 15*time.Second)
			_ = client.SetMyCommands(cmdCtx, []botCommand{
				{Command: "help", Description: "查看帮助与主菜单"},
				{Command: "oracle", Description: "Oracle 云管理"},
				{Command: "azure", Description: "Azure 云管理"},
				{Command: "gcp", Description: "Google Cloud 管理"},
			})
			cmdCancel()
			commandsRegistered = true
			backoff = 2 * time.Second
		}

		updates, err := client.GetUpdates(ctx, offset, 25)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			b.setPollError(err)
			if !sleepWithContext(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}

		backoff = 2 * time.Second
		b.mu.Lock()
		b.status.Connected = true
		b.status.LastError = ""
		b.status.LastPollAt = time.Now().Format("2006-01-02 15:04:05")
		b.status.UpdatesProcessed += int64(len(updates))
		b.mu.Unlock()

		for _, upd := range updates {
			if upd.UpdateID >= offset {
				offset = upd.UpdateID + 1
			}
			u := upd
			go b.handleUpdate(ctx, client, &u)
		}
	}
}

func (b *BotService) setPollError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status.Connected = false
	b.status.LastError = err.Error()
}

func nextBackoff(cur time.Duration) time.Duration {
	next := cur * 2
	if next > 30*time.Second {
		return 30 * time.Second
	}
	return next
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (b *BotService) isAuthorized(userID, chatID int64) bool {
	b.mu.RLock()
	cfg := b.cfg
	b.mu.RUnlock()

	if len(cfg.AllowedUserIDs) == 0 && len(cfg.AllowedChatIDs) == 0 {
		return false
	}
	if len(cfg.AllowedUserIDs) > 0 && !containsInt64(cfg.AllowedUserIDs, userID) {
		return false
	}
	if len(cfg.AllowedChatIDs) > 0 && !containsInt64(cfg.AllowedChatIDs, chatID) {
		return false
	}
	return true
}

func (b *BotService) handleUpdate(ctx context.Context, client *apiClient, upd *tgUpdate) {
	if upd.Message != nil {
		msg := upd.Message
		if msg.From == nil || msg.From.IsBot {
			return
		}
		if !b.isAuthorized(msg.From.ID, msg.Chat.ID) {
			_ = client.SendMessage(ctx, msg.Chat.ID,
				fmt.Sprintf("⛔ 未授权访问\n你的 User ID: <code>%d</code>\n当前 Chat ID: <code>%d</code>\n请在面板「通知与 Bot」白名单中配置后使用。", msg.From.ID, msg.Chat.ID), nil)
			return
		}
		b.handleCommand(ctx, client, msg)
		return
	}
	if upd.CallbackQuery != nil {
		cq := upd.CallbackQuery
		var chatID int64
		if cq.Message != nil {
			chatID = cq.Message.Chat.ID
		}
		if !b.isAuthorized(cq.From.ID, chatID) {
			_ = client.AnswerCallbackQuery(ctx, cq.ID, "⛔ 未授权操作", true)
			return
		}
		b.handleCallback(ctx, client, cq)
	}
}

func (b *BotService) handleCommand(ctx context.Context, client *apiClient, msg *tgMessage) {
	text := strings.TrimSpace(msg.Text)
	if !strings.HasPrefix(text, "/") {
		return
	}
	cmd := strings.Fields(text)[0]
	if idx := strings.Index(cmd, "@"); idx > 0 {
		cmd = cmd[:idx]
	}
	cmd = strings.ToLower(cmd)

	switch cmd {
	case "/start", "/help", "/menu":
		htmlText, markup := b.renderHelpMenu()
		_ = client.SendMessage(ctx, msg.Chat.ID, htmlText, markup)
	case "/oracle":
		htmlText, markup := b.renderProviderAccounts("oci")
		_ = client.SendMessage(ctx, msg.Chat.ID, htmlText, markup)
	case "/azure":
		htmlText, markup := b.renderProviderAccounts("azure")
		_ = client.SendMessage(ctx, msg.Chat.ID, htmlText, markup)
	case "/gcp":
		htmlText, markup := b.renderProviderAccounts("gcp")
		_ = client.SendMessage(ctx, msg.Chat.ID, htmlText, markup)
	default:
		_ = client.SendMessage(ctx, msg.Chat.ID, "未识别的指令，请发送 /help 查看可用命令。", nil)
	}
}

func (b *BotService) handleCallback(ctx context.Context, client *apiClient, cq *tgCallbackQuery) {
	if cq.Message == nil {
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		return
	}
	chatID := cq.Message.Chat.ID
	msgID := cq.Message.MessageID
	parts := strings.Split(cq.Data, ":")
	if len(parts) == 0 {
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		return
	}

	switch parts[0] {
	case "m":
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if len(parts) > 1 && parts[1] == "ov" {
			txt, mk := b.renderOverview()
			_ = client.EditMessageText(ctx, chatID, msgID, txt, mk)
			return
		}
		txt, mk := b.renderHelpMenu()
		_ = client.EditMessageText(ctx, chatID, msgID, txt, mk)

	case "p":
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if len(parts) < 2 {
			return
		}
		txt, mk := b.renderProviderAccounts(parts[1])
		_ = client.EditMessageText(ctx, chatID, msgID, txt, mk)

	case "a":
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if len(parts) < 2 {
			return
		}
		ref, ok := b.callbacks.Get(parts[1])
		if !ok {
			_ = client.EditMessageText(ctx, chatID, msgID, "⚠️ 菜单已过期，请重新发送 /help。", nil)
			return
		}
		txt, mk := b.renderAccountMenu(ref.Provider, ref.Account)
		_ = client.EditMessageText(ctx, chatID, msgID, txt, mk)

	case "vl":
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "⏳ 正在拉取实例列表...", false)
		if len(parts) < 2 {
			return
		}
		ref, ok := b.callbacks.Get(parts[1])
		if !ok {
			_ = client.EditMessageText(ctx, chatID, msgID, "⚠️ 菜单已过期，请重新打开。", nil)
			return
		}
		page := 0
		if len(parts) >= 3 {
			page, _ = strconv.Atoi(parts[2])
		}
		txt, mk := b.renderVMList(ref.Provider, ref.Account, page)
		_ = client.EditMessageText(ctx, chatID, msgID, txt, mk)

	case "vd":
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "⏳ 加载实例详情...", false)
		if len(parts) < 2 {
			return
		}
		ref, ok := b.callbacks.Get(parts[1])
		if !ok {
			_ = client.EditMessageText(ctx, chatID, msgID, "⚠️ 菜单已过期，请重新拉取实例列表。", nil)
			return
		}
		txt, mk := b.renderVMDetail(ref.Provider, ref.Account, ref.TargetID)
		_ = client.EditMessageText(ctx, chatID, msgID, txt, mk)

	case "va":
		// Ask for confirmation on power/changeip actions
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if len(parts) < 3 {
			return
		}
		ref, ok := b.callbacks.Get(parts[1])
		if !ok {
			_ = client.EditMessageText(ctx, chatID, msgID, "⚠️ 操作已过期，请重新打开实例详情。", nil)
			return
		}
		op := parts[2]
		if op == "start" {
			// Start VM directly or with confirmation? Confirming all mutations keeps safety uniform;
			// let's confirm stop/restart/changeip, and start can also confirm cleanly.
		}
		txt, mk := b.renderVMConfirm(parts[1], ref, op)
		_ = client.EditMessageText(ctx, chatID, msgID, txt, mk)

	case "vc":
		if len(parts) < 3 {
			_ = client.AnswerCallbackQuery(ctx, cq.ID, "", false)
			return
		}
		ref, ok := b.callbacks.Get(parts[1])
		if !ok {
			_ = client.AnswerCallbackQuery(ctx, cq.ID, "操作已过期", true)
			return
		}
		op := parts[2]
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "⏳ 正在后台执行，请稍候...", false)
		_ = client.EditMessageText(ctx, chatID, msgID,
			fmt.Sprintf("⏳ 正在对 <code>%s / %s</code> 执行 <b>%s</b>，请稍候...",
				html.EscapeString(ref.Account), html.EscapeString(valueOr(ref.Extra, ref.TargetID)), html.EscapeString(opLabel(op))), nil)
		go b.executeVMAction(ctx, client, chatID, msgID, ref, op)

	case "dns":
		if len(parts) < 2 {
			_ = client.AnswerCallbackQuery(ctx, cq.ID, "", false)
			return
		}
		ref, ok := b.callbacks.Get(parts[1])
		if !ok {
			_ = client.AnswerCallbackQuery(ctx, cq.ID, "操作已过期", true)
			return
		}
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "⏳ 正在同步更新 DNS...", false)
		go b.executeDNSUpdate(ctx, client, chatID, msgID, ref)

	case "ot":
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if len(parts) < 2 {
			return
		}
		ref, ok := b.callbacks.Get(parts[1])
		if !ok {
			_ = client.EditMessageText(ctx, chatID, msgID, "⚠️ 菜单已过期，请重新执行 /oracle。", nil)
			return
		}
		if ref.TargetID == "" {
			txt, mk := b.renderOCITaskList(ref.Account)
			_ = client.EditMessageText(ctx, chatID, msgID, txt, mk)
			return
		}
		txt, mk := b.renderOCITaskDetail(ref.Account, ref.TargetID)
		_ = client.EditMessageText(ctx, chatID, msgID, txt, mk)

	case "ota":
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if len(parts) < 3 {
			return
		}
		ref, ok := b.callbacks.Get(parts[1])
		if !ok {
			_ = client.EditMessageText(ctx, chatID, msgID, "⚠️ 操作已过期。", nil)
			return
		}
		txt, mk := b.renderOCITaskConfirm(parts[1], ref, parts[2])
		_ = client.EditMessageText(ctx, chatID, msgID, txt, mk)

	case "otc":
		if len(parts) < 3 {
			_ = client.AnswerCallbackQuery(ctx, cq.ID, "", false)
			return
		}
		ref, ok := b.callbacks.Get(parts[1])
		if !ok {
			_ = client.AnswerCallbackQuery(ctx, cq.ID, "操作已过期", true)
			return
		}
		op := parts[2]
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "⏳ 正在处理抢购任务...", false)
		var detail *OCITaskDetail
		var err error
		if op == "start" {
			detail, err = b.backend.StartOCICreateTask(ref.Account, ref.TargetID)
		} else {
			detail, err = b.backend.StopOCICreateTask(ref.Account, ref.TargetID)
		}
		if err != nil {
			_ = client.EditMessageText(ctx, chatID, msgID, fmt.Sprintf("❌ 操作失败: <code>%s</code>", html.EscapeString(err.Error())), b.backToOCITasksMarkup(ref.Account))
			return
		}
		txt, mk := b.formatOCITaskDetail(detail)
		_ = client.EditMessageText(ctx, chatID, msgID, "✅ 操作成功\n\n"+txt, mk)

	case "otl":
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if len(parts) < 2 {
			return
		}
		ref, ok := b.callbacks.Get(parts[1])
		if !ok {
			_ = client.EditMessageText(ctx, chatID, msgID, "⚠️ 操作已过期。", nil)
			return
		}
		txt, mk := b.renderOCITaskLogs(ref.Account, ref.TargetID)
		_ = client.EditMessageText(ctx, chatID, msgID, txt, mk)

	case "om":
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if len(parts) < 2 {
			return
		}
		ref, ok := b.callbacks.Get(parts[1])
		if !ok {
			return
		}
		txt, mk := b.renderOCIDTStatus(ref.Account, false)
		_ = client.EditMessageText(ctx, chatID, msgID, txt, mk)

	case "omq":
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "⏳ 正在立即检测 OCI 流量...", false)
		if len(parts) < 2 {
			return
		}
		ref, ok := b.callbacks.Get(parts[1])
		if !ok {
			return
		}
		go func() {
			txt, mk := b.renderOCIDTStatus(ref.Account, true)
			_ = client.EditMessageText(ctx, chatID, msgID, txt, mk)
		}()

	case "oml":
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if len(parts) < 2 {
			return
		}
		ref, ok := b.callbacks.Get(parts[1])
		if !ok {
			return
		}
		txt, mk := b.renderOCIDTLogs(ref.Account)
		_ = client.EditMessageText(ctx, chatID, msgID, txt, mk)

	case "am":
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if len(parts) < 2 {
			return
		}
		ref, ok := b.callbacks.Get(parts[1])
		if !ok {
			return
		}
		txt, mk := b.renderAzureMonitor(ref.Account, false)
		_ = client.EditMessageText(ctx, chatID, msgID, txt, mk)

	case "amq":
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "⏳ 正在立即检测 Azure 赠金/费用...", false)
		if len(parts) < 2 {
			return
		}
		ref, ok := b.callbacks.Get(parts[1])
		if !ok {
			return
		}
		go func() {
			txt, mk := b.renderAzureMonitor(ref.Account, true)
			_ = client.EditMessageText(ctx, chatID, msgID, txt, mk)
		}()

	case "aml":
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "", false)
		if len(parts) < 2 {
			return
		}
		ref, ok := b.callbacks.Get(parts[1])
		if !ok {
			return
		}
		txt, mk := b.renderAzureMonitorLogs(ref.Account)
		_ = client.EditMessageText(ctx, chatID, msgID, txt, mk)

	default:
		_ = client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	}
}

func (b *BotService) renderHelpMenu() (string, *inlineKeyboardMarkup) {
	text := strings.Join([]string{
		"<b>🤖 cloud-vm-manager 控制台</b>",
		"",
		"<b>可用指令：</b>",
		"/help — 查看帮助与主菜单",
		"/oracle — Oracle 云管理（实例 / 抢购任务 / 流量监控日志）",
		"/azure — Azure 云管理（实例 / 赠金费用监控日志）",
		"/gcp — Google Cloud 管理（实例查看与运维）",
		"",
		"<i>权限范围：查看、开关机重启、更换 IP、OCI 抢购启停、日志查看（高危删机/清理功能已隔离）。</i>",
	}, "\n")
	mk := &inlineKeyboardMarkup{
		InlineKeyboard: [][]inlineButton{
			{
				{Text: "☁️ Oracle 云", CallbackData: "p:oci"},
				{Text: "🔷 Azure 云", CallbackData: "p:azure"},
			},
			{
				{Text: "🟡 Google Cloud", CallbackData: "p:gcp"},
				{Text: "📊 全局概览", CallbackData: "m:ov"},
			},
		},
	}
	return text, mk
}

func (b *BotService) renderOverview() (string, *inlineKeyboardMarkup) {
	ov := b.backend.GetOverview()
	text := strings.Join([]string{
		"<b>📊 多云全局概览（缓存汇总）</b>",
		"",
		fmt.Sprintf("已配置云账号数: <code>%d</code>", ov.ConfigCount),
		fmt.Sprintf("已缓存账号数: <code>%d</code>", ov.CachedAccountCount),
		fmt.Sprintf("实例总数: <code>%d</code> (运行中 🟢 %d / 已停止 🔴 %d)", ov.InstanceCount, ov.RunningCount, ov.StoppedCount),
	}, "\n")
	mk := &inlineKeyboardMarkup{
		InlineKeyboard: [][]inlineButton{
			{
				{Text: "☁️ Oracle", CallbackData: "p:oci"},
				{Text: "🔷 Azure", CallbackData: "p:azure"},
				{Text: "🟡 GCP", CallbackData: "p:gcp"},
			},
			{{Text: "⬅ 返回主菜单", CallbackData: "m:menu"}},
		},
	}
	return text, mk
}

func (b *BotService) renderProviderAccounts(provider string) (string, *inlineKeyboardMarkup) {
	accounts := b.backend.AccountsByProvider(provider)
	title := providerDisplayName(provider)
	if len(accounts) == 0 {
		return fmt.Sprintf("<b>%s</b>\n\n暂无已配置的账号。", html.EscapeString(title)), &inlineKeyboardMarkup{
			InlineKeyboard: [][]inlineButton{{{Text: "⬅ 返回主菜单", CallbackData: "m:menu"}}},
		}
	}
	var rows [][]inlineButton
	for _, acc := range accounts {
		tok := b.callbacks.Put(callbackRef{Provider: provider, Account: acc.Account})
		label := acc.Account
		if acc.Group != "" {
			label = fmt.Sprintf("%s (%s)", acc.Account, acc.Group)
		}
		rows = append(rows, []inlineButton{{Text: "📁 " + label, CallbackData: "a:" + tok}})
	}
	rows = append(rows, []inlineButton{{Text: "⬅ 返回主菜单", CallbackData: "m:menu"}})
	return fmt.Sprintf("<b>%s</b>\n\n请选择要管理的账号配置：", html.EscapeString(title)), &inlineKeyboardMarkup{InlineKeyboard: rows}
}

func (b *BotService) renderAccountMenu(provider, account string) (string, *inlineKeyboardMarkup) {
	tok := b.callbacks.Put(callbackRef{Provider: provider, Account: account})
	var rows [][]inlineButton
	rows = append(rows, []inlineButton{{Text: "🖥 实例列表与运维", CallbackData: "vl:" + tok + ":0"}})
	switch provider {
	case "oci":
		rows = append(rows, []inlineButton{
			{Text: "🎯 抢购任务管理", CallbackData: "ot:" + tok},
			{Text: "📈 流量监控与日志", CallbackData: "om:" + tok},
		})
	case "azure":
		rows = append(rows, []inlineButton{
			{Text: "💰 赠金监控与日志", CallbackData: "am:" + tok},
		})
	}
	rows = append(rows, []inlineButton{{Text: "⬅ 返回账号列表", CallbackData: "p:" + provider}})

	text := fmt.Sprintf("<b>%s — %s</b>\n\n请选择操作模块：",
		html.EscapeString(providerDisplayName(provider)), html.EscapeString(account))
	return text, &inlineKeyboardMarkup{InlineKeyboard: rows}
}

func (b *BotService) renderVMList(provider, account string, page int) (string, *inlineKeyboardMarkup) {
	vms, err := b.backend.ListVMs(provider, account)
	accTok := b.callbacks.Put(callbackRef{Provider: provider, Account: account})
	if err != nil {
		return fmt.Sprintf("❌ 获取 <code>%s</code> 实例列表失败：\n<code>%s</code>",
				html.EscapeString(account), html.EscapeString(err.Error())),
			&inlineKeyboardMarkup{InlineKeyboard: [][]inlineButton{
				{{Text: "🔄 重试", CallbackData: fmt.Sprintf("vl:%s:%d", accTok, page)}},
				{{Text: "⬅ 返回", CallbackData: "a:" + accTok}},
			}}
	}
	if len(vms) == 0 {
		return fmt.Sprintf("<b>%s — %s</b>\n\n当前账号下暂无实例。",
				html.EscapeString(providerDisplayName(provider)), html.EscapeString(account)),
			&inlineKeyboardMarkup{InlineKeyboard: [][]inlineButton{{{Text: "⬅ 返回", CallbackData: "a:" + accTok}}}}
	}

	if page < 0 {
		page = 0
	}
	totalPages := (len(vms) + vmPageSize - 1) / vmPageSize
	if page >= totalPages {
		page = totalPages - 1
	}
	start := page * vmPageSize
	end := start + vmPageSize
	if end > len(vms) {
		end = len(vms)
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("<b>🖥 %s / %s 实例列表</b> (共 %d 台，第 %d/%d 页)",
		html.EscapeString(strings.ToUpper(provider)), html.EscapeString(account), len(vms), page+1, totalPages))
	lines = append(lines, "")

	var rows [][]inlineButton
	for _, vm := range vms[start:end] {
		icon := stateEmoji(vm.State)
		ipText := valueOr(vm.PublicIP, "无公网IP")
		lines = append(lines, fmt.Sprintf("%s <b>%s</b> | <code>%s</code> | %s",
			icon, html.EscapeString(valueOr(vm.Name, vm.ID)), html.EscapeString(ipText), html.EscapeString(vm.State)))
		vmTok := b.callbacks.Put(callbackRef{Provider: provider, Account: account, TargetID: vm.ID, Extra: vm.Name})
		rows = append(rows, []inlineButton{{
			Text:         fmt.Sprintf("%s %s (%s)", icon, truncateLabel(valueOr(vm.Name, vm.ID), 22), ipText),
			CallbackData: "vd:" + vmTok,
		}})
	}

	var nav []inlineButton
	if page > 0 {
		nav = append(nav, inlineButton{Text: "◀ 上一页", CallbackData: fmt.Sprintf("vl:%s:%d", accTok, page-1)})
	}
	if page+1 < totalPages {
		nav = append(nav, inlineButton{Text: "下一页 ▶", CallbackData: fmt.Sprintf("vl:%s:%d", accTok, page+1)})
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	rows = append(rows, []inlineButton{
		{Text: "🔄 刷新列表", CallbackData: fmt.Sprintf("vl:%s:%d", accTok, page)},
		{Text: "⬅ 返回", CallbackData: "a:" + accTok},
	})
	return strings.Join(lines, "\n"), &inlineKeyboardMarkup{InlineKeyboard: rows}
}

func (b *BotService) renderVMDetail(provider, account, vmID string) (string, *inlineKeyboardMarkup) {
	accTok := b.callbacks.Put(callbackRef{Provider: provider, Account: account})
	vm, err := b.backend.GetVM(provider, account, vmID)
	if err != nil {
		return fmt.Sprintf("❌ 获取实例详情失败：\n<code>%s</code>", html.EscapeString(err.Error())),
			&inlineKeyboardMarkup{InlineKeyboard: [][]inlineButton{{{Text: "⬅ 返回列表", CallbackData: "vl:" + accTok + ":0"}}}}
	}

	vmTok := b.callbacks.Put(callbackRef{Provider: provider, Account: account, TargetID: vm.ID, Extra: vm.Name})
	var lines []string
	lines = append(lines, fmt.Sprintf("<b>%s %s</b>", stateEmoji(vm.State), html.EscapeString(valueOr(vm.Name, vm.ID))))
	lines = append(lines, fmt.Sprintf("云平台/账号: <code>%s / %s</code>", html.EscapeString(strings.ToUpper(provider)), html.EscapeString(account)))
	lines = append(lines, fmt.Sprintf("状态: <code>%s</code>", html.EscapeString(vm.State)))
	lines = append(lines, fmt.Sprintf("公网 IPv4: <code>%s</code>", html.EscapeString(valueOr(vm.PublicIP, "-"))))
	if vm.IPv6 != "" {
		lines = append(lines, fmt.Sprintf("IPv6: <code>%s</code>", html.EscapeString(vm.IPv6)))
	}
	if vm.PrivateIP != "" {
		lines = append(lines, fmt.Sprintf("内网 IP: <code>%s</code>", html.EscapeString(vm.PrivateIP)))
	}
	if vm.Shape != "" {
		lines = append(lines, fmt.Sprintf("规格: <code>%s</code>", html.EscapeString(vm.Shape)))
	}
	if vm.Region != "" {
		lines = append(lines, fmt.Sprintf("区域: <code>%s</code>", html.EscapeString(vm.Region)))
	}
	dnsText := "未绑定"
	if vm.HasDNSBinding {
		dnsText = "已绑定 Cloudflare DNS"
	}
	lines = append(lines, fmt.Sprintf("DNS: <code>%s</code>", dnsText))

	rows := [][]inlineButton{
		{
			{Text: "▶ 开机", CallbackData: "va:" + vmTok + ":start"},
			{Text: "⏹ 关机", CallbackData: "va:" + vmTok + ":stop"},
			{Text: "🔄 重启", CallbackData: "va:" + vmTok + ":restart"},
		},
		{
			{Text: "🔀 更换公网 IP", CallbackData: "va:" + vmTok + ":changeip"},
			{Text: "🔍 刷新状态", CallbackData: "vd:" + vmTok},
		},
		{
			{Text: "⬅ 返回实例列表", CallbackData: "vl:" + accTok + ":0"},
		},
	}
	return strings.Join(lines, "\n"), &inlineKeyboardMarkup{InlineKeyboard: rows}
}

func (b *BotService) renderVMConfirm(vmTok string, ref callbackRef, op string) (string, *inlineKeyboardMarkup) {
	name := valueOr(ref.Extra, ref.TargetID)
	text := fmt.Sprintf("⚠️ <b>请确认操作</b>\n\n账号: <code>%s / %s</code>\n实例: <code>%s</code>\n操作: <b>%s</b>",
		html.EscapeString(strings.ToUpper(ref.Provider)), html.EscapeString(ref.Account), html.EscapeString(name), html.EscapeString(opLabel(op)))
	mk := &inlineKeyboardMarkup{
		InlineKeyboard: [][]inlineButton{
			{
				{Text: "✅ 确认执行", CallbackData: fmt.Sprintf("vc:%s:%s", vmTok, op)},
				{Text: "❌ 取消", CallbackData: "vd:" + vmTok},
			},
		},
	}
	return text, mk
}

func (b *BotService) executeVMAction(ctx context.Context, client *apiClient, chatID, msgID int64, ref callbackRef, op string) {
	vmTok := b.callbacks.Put(ref)
	name := valueOr(ref.Extra, ref.TargetID)

	if op == "changeip" {
		outcome, err := b.backend.ChangeVMIP(ref.Provider, ref.Account, ref.TargetID)
		if err != nil {
			_ = client.EditMessageText(ctx, chatID, msgID,
				fmt.Sprintf("❌ <b>更换 IP 失败</b>\n实例: <code>%s</code>\n错误: <code>%s</code>", html.EscapeString(name), html.EscapeString(err.Error())),
				&inlineKeyboardMarkup{InlineKeyboard: [][]inlineButton{{{Text: "⬅ 返回实例详情", CallbackData: "vd:" + vmTok}}}})
			return
		}
		var lines []string
		lines = append(lines, "✅ <b>更换公网 IP 完成</b>")
		lines = append(lines, fmt.Sprintf("账号/实例: <code>%s / %s</code>", html.EscapeString(ref.Account), html.EscapeString(name)))
		if outcome.NewIP != "" {
			lines = append(lines, fmt.Sprintf("新公网 IP: <code>%s</code>", html.EscapeString(outcome.NewIP)))
		}
		if outcome.Message != "" {
			lines = append(lines, html.EscapeString(outcome.Message))
		}
		if len(outcome.Logs) > 0 {
			lines = append(lines, "\n<b>执行日志：</b>\n<pre>"+html.EscapeString(strings.Join(tailLines(outcome.Logs, 12), "\n"))+"</pre>")
		}
		var rows [][]inlineButton
		// Option B: offer an explicit "Sync DNS" button if Cloudflare DNS is bound
		if outcome.HasDNSBinding {
			rows = append(rows, []inlineButton{{Text: "🌐 同步更新 DNS", CallbackData: "dns:" + vmTok}})
		}
		rows = append(rows, []inlineButton{{Text: "⬅ 返回实例详情", CallbackData: "vd:" + vmTok}})
		_ = client.EditMessageText(ctx, chatID, msgID, strings.Join(lines, "\n"), &inlineKeyboardMarkup{InlineKeyboard: rows})
		return
	}

	if err := b.backend.PowerVM(ref.Provider, ref.Account, ref.TargetID, op); err != nil {
		_ = client.EditMessageText(ctx, chatID, msgID,
			fmt.Sprintf("❌ <b>%s 失败</b>\n实例: <code>%s</code>\n错误: <code>%s</code>",
				html.EscapeString(opLabel(op)), html.EscapeString(name), html.EscapeString(err.Error())),
			&inlineKeyboardMarkup{InlineKeyboard: [][]inlineButton{{{Text: "⬅ 返回实例详情", CallbackData: "vd:" + vmTok}}}})
		return
	}

	_ = client.EditMessageText(ctx, chatID, msgID,
		fmt.Sprintf("✅ <b>%s 指令已提交</b>\n账号: <code>%s</code>\n实例: <code>%s</code>",
			html.EscapeString(opLabel(op)), html.EscapeString(ref.Account), html.EscapeString(name)),
		&inlineKeyboardMarkup{InlineKeyboard: [][]inlineButton{
			{{Text: "🔍 查看最新状态", CallbackData: "vd:" + vmTok}},
		}})
}

func (b *BotService) executeDNSUpdate(ctx context.Context, client *apiClient, chatID, msgID int64, ref callbackRef) {
	vmTok := b.callbacks.Put(ref)
	logs, err := b.backend.UpdateVMDNS(ref.Provider, ref.Account, ref.TargetID)
	backMk := &inlineKeyboardMarkup{InlineKeyboard: [][]inlineButton{{{Text: "⬅ 返回实例详情", CallbackData: "vd:" + vmTok}}}}
	if err != nil {
		_ = client.EditMessageText(ctx, chatID, msgID,
			fmt.Sprintf("❌ <b>同步更新 DNS 失败</b>\n错误: <code>%s</code>", html.EscapeString(err.Error())), backMk)
		return
	}
	_ = client.EditMessageText(ctx, chatID, msgID,
		fmt.Sprintf("✅ <b>DNS 同步完成</b>\n\n<pre>%s</pre>", html.EscapeString(strings.Join(logs, "\n"))), backMk)
}

func (b *BotService) renderOCITaskList(account string) (string, *inlineKeyboardMarkup) {
	accTok := b.callbacks.Put(callbackRef{Provider: "oci", Account: account})
	tasks, err := b.backend.ListOCICreateTasks(account)
	if err != nil {
		return fmt.Sprintf("❌ 获取 OCI 抢购任务失败: <code>%s</code>", html.EscapeString(err.Error())), b.backToAccountMarkup("oci", account)
	}
	if len(tasks) == 0 {
		return fmt.Sprintf("<b>🎯 OCI 抢购任务 — %s</b>\n\n暂无抢购任务（请先在 Web 面板创建任务模板）。", html.EscapeString(account)),
			b.backToAccountMarkup("oci", account)
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("<b>🎯 OCI 抢购任务 — %s</b> (共 %d 个)", html.EscapeString(account), len(tasks)))
	lines = append(lines, "")
	var rows [][]inlineButton
	for _, t := range tasks {
		icon := taskEmoji(t.Status)
		lines = append(lines, fmt.Sprintf("%s <b>%s</b> [%s] | %s | 已尝试 %d 次",
			icon, html.EscapeString(t.Name), html.EscapeString(t.Status), html.EscapeString(t.Shape), t.Attempts))
		tok := b.callbacks.Put(callbackRef{Provider: "oci", Account: account, TargetID: t.ID, Extra: t.Name})
		rows = append(rows, []inlineButton{{
			Text:         fmt.Sprintf("%s %s (%s)", icon, truncateLabel(t.Name, 22), t.Status),
			CallbackData: "ot:" + tok,
		}})
	}
	rows = append(rows, []inlineButton{
		{Text: "🔄 刷新", CallbackData: "ot:" + accTok},
		{Text: "⬅ 返回", CallbackData: "a:" + accTok},
	})
	return strings.Join(lines, "\n"), &inlineKeyboardMarkup{InlineKeyboard: rows}
}

func (b *BotService) renderOCITaskDetail(account, taskID string) (string, *inlineKeyboardMarkup) {
	detail, err := b.backend.GetOCICreateTask(account, taskID)
	if err != nil {
		return fmt.Sprintf("❌ 获取任务详情失败: <code>%s</code>", html.EscapeString(err.Error())), b.backToOCITasksMarkup(account)
	}
	return b.formatOCITaskDetail(detail)
}

func (b *BotService) formatOCITaskDetail(detail *OCITaskDetail) (string, *inlineKeyboardMarkup) {
	tok := b.callbacks.Put(callbackRef{Provider: "oci", Account: detail.Account, TargetID: detail.ID, Extra: detail.Name})
	accTok := b.callbacks.Put(callbackRef{Provider: "oci", Account: detail.Account})
	var lines []string
	lines = append(lines, fmt.Sprintf("<b>%s %s</b>", taskEmoji(detail.Status), html.EscapeString(detail.Name)))
	lines = append(lines, fmt.Sprintf("账号: <code>%s</code>", html.EscapeString(detail.Account)))
	lines = append(lines, fmt.Sprintf("状态: <code>%s</code> | 尝试次数: <code>%d</code>", html.EscapeString(detail.Status), detail.Attempts))
	lines = append(lines, fmt.Sprintf("规格: <code>%s (%.0f OCPU / %.0f GB)</code>", html.EscapeString(detail.Shape), detail.OCPUs, detail.MemoryInGBs))
	if detail.AvailabilityDomain != "" {
		lines = append(lines, fmt.Sprintf("可用域: <code>%s</code>", html.EscapeString(detail.AvailabilityDomain)))
	}
	if detail.LastAttemptAt != "" {
		lines = append(lines, fmt.Sprintf("最后尝试: <code>%s</code>", html.EscapeString(detail.LastAttemptAt)))
	}
	if detail.LastMessage != "" {
		lines = append(lines, fmt.Sprintf("状态说明: %s", html.EscapeString(detail.LastMessage)))
	}
	if detail.CreatedVMID != "" {
		lines = append(lines, fmt.Sprintf("已创建实例 ID: <code>%s</code>", html.EscapeString(detail.CreatedVMID)))
	}

	var actionRow []inlineButton
	if detail.Status == "running" {
		actionRow = append(actionRow, inlineButton{Text: "⏹ 停止抢购", CallbackData: "ota:" + tok + ":stop"})
	} else if detail.Status != "succeeded" {
		actionRow = append(actionRow, inlineButton{Text: "▶ 开启抢购", CallbackData: "ota:" + tok + ":start"})
	}
	actionRow = append(actionRow, inlineButton{Text: "📜 查看日志", CallbackData: "otl:" + tok})

	rows := [][]inlineButton{
		actionRow,
		{
			{Text: "🔄 刷新", CallbackData: "ot:" + tok},
			{Text: "⬅ 返回任务列表", CallbackData: "ot:" + accTok},
		},
	}
	return strings.Join(lines, "\n"), &inlineKeyboardMarkup{InlineKeyboard: rows}
}

func (b *BotService) renderOCITaskConfirm(tok string, ref callbackRef, op string) (string, *inlineKeyboardMarkup) {
	label := "开启抢购任务"
	if op == "stop" {
		label = "停止抢购任务"
	}
	text := fmt.Sprintf("⚠️ <b>请确认操作</b>\n\n账号: <code>%s</code>\n任务: <code>%s</code>\n操作: <b>%s</b>",
		html.EscapeString(ref.Account), html.EscapeString(valueOr(ref.Extra, ref.TargetID)), label)
	mk := &inlineKeyboardMarkup{
		InlineKeyboard: [][]inlineButton{
			{
				{Text: "✅ 确认", CallbackData: fmt.Sprintf("otc:%s:%s", tok, op)},
				{Text: "❌ 取消", CallbackData: "ot:" + tok},
			},
		},
	}
	return text, mk
}

func (b *BotService) renderOCITaskLogs(account, taskID string) (string, *inlineKeyboardMarkup) {
	tok := b.callbacks.Put(callbackRef{Provider: "oci", Account: account, TargetID: taskID})
	detail, err := b.backend.GetOCICreateTask(account, taskID)
	mk := &inlineKeyboardMarkup{InlineKeyboard: [][]inlineButton{{{Text: "⬅ 返回任务详情", CallbackData: "ot:" + tok}}}}
	if err != nil {
		return fmt.Sprintf("❌ 获取日志失败: <code>%s</code>", html.EscapeString(err.Error())), mk
	}
	logLines := tailLines(detail.Logs, 25)
	if len(logLines) == 0 {
		return fmt.Sprintf("<b>📜 抢购日志 — %s</b>\n\n暂无日志记录。", html.EscapeString(detail.Name)), mk
	}
	return fmt.Sprintf("<b>📜 抢购日志 — %s</b> (最近 %d 条)\n\n<pre>%s</pre>",
		html.EscapeString(detail.Name), len(logLines), html.EscapeString(strings.Join(logLines, "\n"))), mk
}

func (b *BotService) renderOCIDTStatus(account string, queryNow bool) (string, *inlineKeyboardMarkup) {
	tok := b.callbacks.Put(callbackRef{Provider: "oci", Account: account})
	accTok := tok
	var st *OCIDTStatus
	var err error
	if queryNow {
		st, err = b.backend.QueryOCIDTMonitorNow(account)
	} else {
		st, err = b.backend.GetOCIDTMonitor(account)
	}
	if err != nil {
		return fmt.Sprintf("❌ 查询 OCI 流量监控失败: <code>%s</code>", html.EscapeString(err.Error())), b.backToAccountMarkup("oci", account)
	}

	runText := "已停止"
	if st.Running {
		runText = "🟢 运行中"
	}
	autoStopText := "关闭"
	if st.AutoStop {
		autoStopText = fmt.Sprintf("开启 (%s)", st.StopMethod)
	}
	lines := []string{
		fmt.Sprintf("<b>📈 OCI 流量监控 — %s</b>", html.EscapeString(account)),
		"",
		fmt.Sprintf("监控状态: <code>%s</code> (周期 %ds)", runText, st.Interval),
		fmt.Sprintf("告警阈值: <code>%.0f GB</code> | 自动停机: <code>%s</code>", st.Threshold, html.EscapeString(autoStopText)),
	}
	if st.QueryTime != "" {
		if st.QueryError != "" {
			lines = append(lines, fmt.Sprintf("最近检测 (%s): ❌ <code>%s</code>", html.EscapeString(st.QueryTime), html.EscapeString(st.QueryError)))
		} else {
			lines = append(lines, fmt.Sprintf("当月已用流量: <code>%.2f GB / %.0f GB (%.1f%%)</code>", st.UsageGB, st.Threshold, st.Percentage))
			lines = append(lines, fmt.Sprintf("检测时间: <code>%s</code>", html.EscapeString(st.QueryTime)))
		}
	} else {
		lines = append(lines, "当月已用流量: <i>暂无缓存结果，可点击下方「立即检测」</i>")
	}

	mk := &inlineKeyboardMarkup{
		InlineKeyboard: [][]inlineButton{
			{
				{Text: "🔍 立即检测", CallbackData: "omq:" + tok},
				{Text: "📜 查看日志", CallbackData: "oml:" + tok},
			},
			{{Text: "⬅ 返回账号菜单", CallbackData: "a:" + accTok}},
		},
	}
	return strings.Join(lines, "\n"), mk
}

func (b *BotService) renderOCIDTLogs(account string) (string, *inlineKeyboardMarkup) {
	tok := b.callbacks.Put(callbackRef{Provider: "oci", Account: account})
	st, err := b.backend.GetOCIDTMonitor(account)
	mk := &inlineKeyboardMarkup{InlineKeyboard: [][]inlineButton{{{Text: "⬅ 返回流量监控", CallbackData: "om:" + tok}}}}
	if err != nil {
		return fmt.Sprintf("❌ 获取日志失败: <code>%s</code>", html.EscapeString(err.Error())), mk
	}
	logs := tailLines(st.Logs, 25)
	if len(logs) == 0 {
		return fmt.Sprintf("<b>📜 OCI 流量监控日志 — %s</b>\n\n暂无日志。", html.EscapeString(account)), mk
	}
	return fmt.Sprintf("<b>📜 OCI 流量监控日志 — %s</b> (最近 %d 条)\n\n<pre>%s</pre>",
		html.EscapeString(account), len(logs), html.EscapeString(strings.Join(logs, "\n"))), mk
}

func (b *BotService) renderAzureMonitor(account string, queryNow bool) (string, *inlineKeyboardMarkup) {
	tok := b.callbacks.Put(callbackRef{Provider: "azure", Account: account})
	var snap *AzureMonitorSnapshot
	var err error
	if queryNow {
		snap, err = b.backend.QueryAzureCreditMonitorNow(account)
	} else {
		snap, err = b.backend.GetAzureCreditMonitor(account)
	}
	if err != nil {
		return fmt.Sprintf("❌ 查询 Azure 赠金监控失败: <code>%s</code>", html.EscapeString(err.Error())), b.backToAccountMarkup("azure", account)
	}

	runText := "已停止"
	if snap.Running {
		runText = "🟢 运行中"
	}
	lines := []string{
		fmt.Sprintf("<b>💰 Azure 赠金/费用监控 — %s</b>", html.EscapeString(account)),
		"",
		fmt.Sprintf("监控状态: <code>%s</code> (周期 %ds / 触发动作 %s)", runText, snap.Interval, html.EscapeString(snap.TriggerAction)),
	}
	if snap.QueriedAt != "" {
		if snap.CostStatus == "ok" {
			lines = append(lines, fmt.Sprintf("本月消费: <code>$%.2f USD</code> | 周期累计: <code>$%.2f USD</code>", snap.MonthCostUSD, snap.PeriodCostUSD))
		} else if snap.CostMessage != "" {
			lines = append(lines, fmt.Sprintf("费用查询状态: <code>%s</code>", html.EscapeString(snap.CostMessage)))
		}
		if snap.RemainingUSD != nil {
			lines = append(lines, fmt.Sprintf("估算剩余赠金: <code>$%.2f USD</code>", *snap.RemainingUSD))
		}
		if snap.DaysUntilExpire != nil {
			lines = append(lines, fmt.Sprintf("距到期天数: <code>%d 天</code>", *snap.DaysUntilExpire))
		}
		if snap.Triggered && len(snap.TriggerReasons) > 0 {
			lines = append(lines, fmt.Sprintf("⚠️ 触发告警: <code>%s</code>", html.EscapeString(strings.Join(snap.TriggerReasons, "；"))))
		}
		lines = append(lines, fmt.Sprintf("查询时间: <code>%s</code>", html.EscapeString(snap.QueriedAt)))
	} else {
		lines = append(lines, "费用概览: <i>暂无缓存结果，可点击下方「立即检测」</i>")
	}

	mk := &inlineKeyboardMarkup{
		InlineKeyboard: [][]inlineButton{
			{
				{Text: "🔍 立即检测", CallbackData: "amq:" + tok},
				{Text: "📜 查看日志", CallbackData: "aml:" + tok},
			},
			{{Text: "⬅ 返回账号菜单", CallbackData: "a:" + tok}},
		},
	}
	return strings.Join(lines, "\n"), mk
}

func (b *BotService) renderAzureMonitorLogs(account string) (string, *inlineKeyboardMarkup) {
	tok := b.callbacks.Put(callbackRef{Provider: "azure", Account: account})
	snap, err := b.backend.GetAzureCreditMonitor(account)
	mk := &inlineKeyboardMarkup{InlineKeyboard: [][]inlineButton{{{Text: "⬅ 返回赠金监控", CallbackData: "am:" + tok}}}}
	if err != nil {
		return fmt.Sprintf("❌ 获取日志失败: <code>%s</code>", html.EscapeString(err.Error())), mk
	}
	logs := tailLines(snap.Logs, 25)
	if len(logs) == 0 {
		return fmt.Sprintf("<b>📜 Azure 监控日志 — %s</b>\n\n暂无日志。", html.EscapeString(account)), mk
	}
	return fmt.Sprintf("<b>📜 Azure 监控日志 — %s</b> (最近 %d 条)\n\n<pre>%s</pre>",
		html.EscapeString(account), len(logs), html.EscapeString(strings.Join(logs, "\n"))), mk
}

func (b *BotService) backToAccountMarkup(provider, account string) *inlineKeyboardMarkup {
	tok := b.callbacks.Put(callbackRef{Provider: provider, Account: account})
	return &inlineKeyboardMarkup{InlineKeyboard: [][]inlineButton{{{Text: "⬅ 返回账号菜单", CallbackData: "a:" + tok}}}}
}

func (b *BotService) backToOCITasksMarkup(account string) *inlineKeyboardMarkup {
	tok := b.callbacks.Put(callbackRef{Provider: "oci", Account: account})
	return &inlineKeyboardMarkup{InlineKeyboard: [][]inlineButton{{{Text: "⬅ 返回抢购任务", CallbackData: "ot:" + tok}}}}
}

func providerDisplayName(provider string) string {
	switch strings.ToLower(provider) {
	case "oci":
		return "☁️ Oracle Cloud (OCI)"
	case "azure":
		return "🔷 Microsoft Azure"
	case "gcp":
		return "🟡 Google Cloud (GCP)"
	default:
		return strings.ToUpper(provider)
	}
}

func opLabel(op string) string {
	switch op {
	case "start":
		return "开机"
	case "stop":
		return "关机"
	case "restart":
		return "重启"
	case "changeip":
		return "更换公网 IP"
	default:
		return op
	}
}

func stateEmoji(state string) string {
	s := strings.ToLower(state)
	switch {
	case strings.Contains(s, "run"):
		return "🟢"
	case strings.Contains(s, "stop") || strings.Contains(s, "dealloc") || strings.Contains(s, "term"):
		return "🔴"
	default:
		return "🟡"
	}
}

func taskEmoji(status string) string {
	switch status {
	case "running":
		return "🟢"
	case "succeeded":
		return "🎉"
	case "stopped":
		return "⏹"
	default:
		return "⚪"
	}
}

func tailLines(lines []string, limit int) []string {
	if limit <= 0 || len(lines) <= limit {
		return lines
	}
	return lines[len(lines)-limit:]
}

func truncateLabel(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes-1]) + "…"
}

func valueOr(val, fallback string) string {
	if strings.TrimSpace(val) != "" {
		return val
	}
	return fallback
}

func containsInt64(items []int64, target int64) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
