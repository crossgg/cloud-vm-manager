package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/bot/telegram"
	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/crossgg/cloud-vm-manager/internal/notify"
	"github.com/crossgg/cloud-vm-manager/internal/proxy"
	"github.com/gin-gonic/gin"
)

const notifySecretMask = "********"

var sensitiveNotifyParamKeys = map[string]bool{
	"bark_push":       true,
	"qywx_key":        true,
	"qywx_am":         true,
	"fskey":           true,
	"fssecret":        true,
	"tg_bot_token":    true,
	"push_plus_token": true,
	"webhook_headers": true,
}

func getNotifyConfig(c *gin.Context) {
	cfg, proxies, notifyPath := notifyStateSnapshot()
	var botStatus telegram.BotStatus
	if bot := currentTelegramBot(); bot != nil {
		botStatus = bot.Status()
	}
	var logs []notify.DeliveryLog
	if d := currentNotifyDispatcher(); d != nil {
		logs = d.RecentLogs()
	}

	c.JSON(http.StatusOK, gin.H{
		"config":       maskNotifyConfig(cfg),
		"configPath":   notifyPath,
		"eventDefs":    config.AllNotifyEventDefinitions(),
		"channelTypes": config.SupportedNotifyChannelTypes(),
		"proxies":      maskedProxies(proxies),
		"botStatus":    botStatus,
		"logs":         logs,
	})
}

func saveNotifyConfigHandler(c *gin.Context) {
	var input config.NotifyConfig
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "通知配置格式无效: " + err.Error()})
		return
	}

	currentCfg, proxies, notifyPath := notifyStateSnapshot()
	if strings.TrimSpace(notifyPath) == "" {
		notifyPath = config.NotifyConfigPath(currentConfigPath())
	}

	restored := restoreMaskedNotifyConfig(input, currentCfg)
	for i := range restored.Channels {
		if strings.TrimSpace(restored.Channels[i].ID) == "" {
			restored.Channels[i].ID = newNotifyChannelID(restored.Channels[i].Type)
		}
	}
	restored = restored.Normalize()

	if err := config.ValidateNotifyConfig(restored); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := config.ValidateNotifyProxies(restored, proxies); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for _, ch := range restored.Channels {
		if !ch.Enabled {
			continue
		}
		impl, ok := notify.LookupChannel(ch.Type)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("不支持的渠道类型 %q", ch.Type)})
			return
		}
		if err := impl.ValidateParams(ch.Params); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	if err := config.SaveNotifyConfig(notifyPath, restored); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := ReloadRuntimeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "已写入配置但热重载失败: " + err.Error()})
		return
	}

	var botStatus telegram.BotStatus
	if bot := currentTelegramBot(); bot != nil {
		botStatus = bot.Status()
	}
	updatedCfg, _, _ := notifyStateSnapshot()
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"config":    maskNotifyConfig(updatedCfg),
		"botStatus": botStatus,
	})
}

func testNotifyChannel(c *gin.Context) {
	var input config.NotifyChannelConfig
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式无效: " + err.Error()})
		return
	}

	currentCfg, proxies, _ := notifyStateSnapshot()
	restored := restoreMaskedChannel(input, currentCfg.Channels)
	chNorm := (config.NotifyConfig{Channels: []config.NotifyChannelConfig{restored}}).Normalize().Channels[0]
	if err := config.ValidateNotifyProxies(config.NotifyConfig{Channels: []config.NotifyChannelConfig{chNorm}}, proxies); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	router := proxy.NewProxyRouter(proxies, nil)
	d := currentNotifyDispatcher()
	if d == nil {
		d = notify.NewDispatcher(currentCfg, router)
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	if err := d.TestChannel(ctx, chNorm, router); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "测试通知发送成功"})
}

func testTelegramBotHandler(c *gin.Context) {
	var req struct {
		Bot         config.TelegramBotConfig `json:"bot"`
		SendMessage bool                     `json:"sendMessage"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式无效: " + err.Error()})
		return
	}

	currentCfg, proxies, _ := notifyStateSnapshot()
	botCfg := restoreMaskedBot(req.Bot, currentCfg.TelegramBot)
	botCfg = (config.NotifyConfig{TelegramBot: botCfg}).Normalize().TelegramBot
	if err := config.ValidateNotifyProxies(config.NotifyConfig{TelegramBot: botCfg}, proxies); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	router := proxy.NewProxyRouter(proxies, nil)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	username, err := telegram.TestBotConnection(ctx, botCfg, router)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "连接 Telegram Bot 失败: " + err.Error()})
		return
	}

	msg := fmt.Sprintf("Bot @%s 连接正常", username)
	if req.SendMessage && len(botCfg.EffectiveNotifyChatIDs()) > 0 {
		d := currentNotifyDispatcher()
		if d == nil {
			d = notify.NewDispatcher(currentCfg, router)
		}
		if err := d.TestTelegramBotNotify(ctx, botCfg, router); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Bot @%s 已连通，但发送测试消息失败: %v", username, err)})
			return
		}
		msg = fmt.Sprintf("Bot @%s 连通正常，已发送测试消息", username)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"botUsername": username,
		"message":     msg,
	})
}

func getNotifyLogs(c *gin.Context) {
	var logs []notify.DeliveryLog
	if d := currentNotifyDispatcher(); d != nil {
		logs = d.RecentLogs()
	}
	var botStatus telegram.BotStatus
	if bot := currentTelegramBot(); bot != nil {
		botStatus = bot.Status()
	}
	c.JSON(http.StatusOK, gin.H{
		"logs":      logs,
		"botStatus": botStatus,
	})
}

func notifyStateSnapshot() (config.NotifyConfig, []config.ProxyConfig, string) {
	runtimeState.mu.RLock()
	defer runtimeState.mu.RUnlock()
	if runtimeState.cfg == nil {
		return config.DefaultNotifyConfig(), nil, ""
	}
	return runtimeState.cfg.Notify, append([]config.ProxyConfig(nil), runtimeState.cfg.Proxies...), runtimeState.cfg.NotifyPath
}

func maskNotifyConfig(cfg config.NotifyConfig) config.NotifyConfig {
	out := cfg.Normalize()
	if out.TelegramBot.BotToken != "" {
		out.TelegramBot.BotToken = notifySecretMask
	}
	for i := range out.Channels {
		maskedParams := make(map[string]string, len(out.Channels[i].Params))
		for k, v := range out.Channels[i].Params {
			if sensitiveNotifyParamKeys[k] && v != "" {
				maskedParams[k] = notifySecretMask
			} else {
				maskedParams[k] = v
			}
		}
		out.Channels[i].Params = maskedParams
	}
	return out
}

func restoreMaskedNotifyConfig(candidate, current config.NotifyConfig) config.NotifyConfig {
	out := candidate
	out.TelegramBot = restoreMaskedBot(candidate.TelegramBot, current.TelegramBot)
	out.Channels = make([]config.NotifyChannelConfig, 0, len(candidate.Channels))
	for _, ch := range candidate.Channels {
		out.Channels = append(out.Channels, restoreMaskedChannel(ch, current.Channels))
	}
	return out
}

func restoreMaskedBot(candidate, current config.TelegramBotConfig) config.TelegramBotConfig {
	if strings.TrimSpace(candidate.BotToken) == notifySecretMask {
		candidate.BotToken = current.BotToken
	}
	return candidate
}

func restoreMaskedChannel(candidate config.NotifyChannelConfig, existing []config.NotifyChannelConfig) config.NotifyChannelConfig {
	var prevParams map[string]string
	for _, item := range existing {
		if item.ID == candidate.ID {
			prevParams = item.Params
			break
		}
	}
	if prevParams == nil || candidate.Params == nil {
		return candidate
	}
	nextParams := make(map[string]string, len(candidate.Params))
	for k, v := range candidate.Params {
		nk := strings.ToLower(strings.TrimSpace(k))
		if sensitiveNotifyParamKeys[nk] && strings.TrimSpace(v) == notifySecretMask {
			if prevVal, ok := prevParams[nk]; ok {
				nextParams[nk] = prevVal
				continue
			}
		}
		nextParams[nk] = v
	}
	candidate.Params = nextParams
	return candidate
}

func newNotifyChannelID(channelType string) string {
	prefix := strings.ToLower(strings.TrimSpace(channelType))
	if prefix == "" {
		prefix = "ch"
	}
	prefix = strings.ReplaceAll(prefix, "_", "-")
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b)
}
