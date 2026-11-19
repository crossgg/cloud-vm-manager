package server

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/auth"
	"github.com/crossgg/cloud-vm-manager/internal/bot/telegram"
	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/crossgg/cloud-vm-manager/internal/notify"
	"github.com/crossgg/cloud-vm-manager/internal/provider/azure"
	"github.com/crossgg/cloud-vm-manager/internal/provider/oci"
	"github.com/crossgg/cloud-vm-manager/internal/proxy"
)

var notifyRuntime = struct {
	mu         sync.RWMutex
	dispatcher *notify.Dispatcher
	bot        *telegram.BotService
}{}

func syncNotifyRuntime(cfg *config.Config, authSvc *auth.AuthService) {
	if cfg == nil {
		return
	}
	proxyRouter := proxy.NewProxyRouter(cfg.Proxies, cfg.ProxyBindings)

	notifyRuntime.mu.Lock()
	if notifyRuntime.dispatcher == nil {
		notifyRuntime.dispatcher = notify.NewDispatcher(cfg.Notify, proxyRouter)
	} else {
		notifyRuntime.dispatcher.Update(cfg.Notify, proxyRouter)
	}
	if notifyRuntime.bot == nil {
		notifyRuntime.bot = telegram.NewBotService(cfg.Notify.TelegramBot, proxyRouter, &serverTGBackend{})
	}
	bot := notifyRuntime.bot
	notifyRuntime.mu.Unlock()

	if authSvc != nil {
		authSvc.SetOnLoginEvent(func(ev auth.LoginEvent) {
			emitAuthLoginEvent(ev)
		})
	}

	if err := bot.Reload(cfg.Notify.TelegramBot, proxyRouter); err != nil {
		fmt.Printf("warning: telegram bot reload failed: %v\n", err)
	}
}

func currentNotifyDispatcher() *notify.Dispatcher {
	notifyRuntime.mu.RLock()
	defer notifyRuntime.mu.RUnlock()
	return notifyRuntime.dispatcher
}

func currentTelegramBot() *telegram.BotService {
	notifyRuntime.mu.RLock()
	defer notifyRuntime.mu.RUnlock()
	return notifyRuntime.bot
}

func emitNotification(ev notify.Event) {
	if d := currentNotifyDispatcher(); d != nil {
		d.Emit(ev)
	}
}

func emitOCITaskSucceededEvent(task oci.OCICreateTask) {
	var parts []string
	parts = append(parts, fmt.Sprintf("任务名称: %s", task.Name))
	parts = append(parts, fmt.Sprintf("规格: %s (%.0f OCPU / %.0f GB)", task.Shape, task.OCPUs, task.MemoryInGBs))
	if task.AvailabilityDomain != "" {
		parts = append(parts, "可用域: "+task.AvailabilityDomain)
	}
	parts = append(parts, fmt.Sprintf("尝试次数: %d", task.Attempts))

	if task.CreatedInstanceID != "" {
		if svc, _, ok := serviceSnapshot("oci", task.Account); ok && svc != nil {
			if vm, err := svc.GetVM(task.CreatedInstanceID); err == nil {
				if pubIP, ok := publicIPAddress(vm); ok {
					parts = append(parts, "公网 IP: "+pubIP)
				}
				if ipv6, ok := primaryIPv6Address(vm); ok {
					parts = append(parts, "IPv6: "+ipv6)
				}
			}
		}
	}

	emitNotification(notify.Event{
		Type:         config.NotifyEventOCITaskSucceeded,
		Severity:     notify.SeverityInfo,
		Title:        "OCI 抢购任务成功创建实例",
		Provider:     "oci",
		Account:      task.Account,
		ResourceID:   task.CreatedInstanceID,
		ResourceName: valueOrEmpty(task.DisplayName, task.Name),
		Content:      strings.Join(parts, "\n"),
		Timestamp:    time.Now(),
	})
}

func onOCIDTAlertCallback() func(account config.OCIConfig, result oci.DataTransferResult, autoStopped bool, actionLogs []string) {
	return func(account config.OCIConfig, result oci.DataTransferResult, autoStopped bool, actionLogs []string) {
		severity := notify.SeverityWarning
		title := "OCI 出站流量超过阈值"
		if autoStopped {
			severity = notify.SeverityCritical
			title = "OCI 流量超限，已触发自动停机"
		}
		lines := []string{
			fmt.Sprintf("当前用量: %.2f GB / %.0f GB (%.1f%%)", result.UsageGB, result.Threshold, result.Percentage),
		}
		if len(actionLogs) > 0 {
			lines = append(lines, "执行结果:\n"+strings.Join(actionLogs, "\n"))
		}
		emitNotification(notify.Event{
			Type:      config.NotifyEventOCIDTAlert,
			Severity:  severity,
			Title:     title,
			Provider:  "oci",
			Account:   account.Name,
			DedupeKey: "oci_dt:" + account.Name,
			Content:   strings.Join(lines, "\n"),
			Timestamp: time.Now(),
		})
	}
}

func onAzureCreditAlertCallback() func(account config.AzureConfig, ov *azure.AzureCostOverview, reasons []string, action string, actionLogs []string) {
	return func(account config.AzureConfig, ov *azure.AzureCostOverview, reasons []string, action string, actionLogs []string) {
		severity := notify.SeverityWarning
		title := "Azure 赠金/费用监控触发告警"
		if action != "" && action != "alert" {
			severity = notify.SeverityCritical
			title = fmt.Sprintf("Azure 监控触发自动动作 (%s)", action)
		}
		var lines []string
		if len(reasons) > 0 {
			lines = append(lines, "触发原因: "+strings.Join(reasons, "；"))
		}
		if ov != nil && ov.CostStatus == "ok" {
			lines = append(lines, fmt.Sprintf("本月消费: $%.2f USD | 累计: $%.2f USD", ov.MonthToDateCostUSD, ov.PeriodTotalCostUSD))
			if ov.EstimatedRemainingUSD != nil {
				lines = append(lines, fmt.Sprintf("估算剩余赠金: $%.2f USD", *ov.EstimatedRemainingUSD))
			}
			if ov.DaysUntilExpire != nil {
				lines = append(lines, fmt.Sprintf("距到期天数: %d 天", *ov.DaysUntilExpire))
			}
		}
		if len(actionLogs) > 0 {
			lines = append(lines, "执行日志:\n"+strings.Join(actionLogs, "\n"))
		}
		emitNotification(notify.Event{
			Type:      config.NotifyEventAzureCreditAlert,
			Severity:  severity,
			Title:     title,
			Provider:  "azure",
			Account:   account.Name,
			DedupeKey: "azure_credit:" + account.Name,
			Content:   strings.Join(lines, "\n"),
			Timestamp: time.Now(),
		})
	}
}

func emitVMChangeIPEvent(providerName, account, vmID, newIP, message, source string) {
	var lines []string
	if source != "" {
		lines = append(lines, "操作来源: "+source)
	}
	if newIP != "" {
		lines = append(lines, "新公网 IP: "+newIP)
	}
	if strings.TrimSpace(message) != "" {
		lines = append(lines, message)
	}
	emitNotification(notify.Event{
		Type:         config.NotifyEventVMChangeIP,
		Severity:     notify.SeverityInfo,
		Title:        "实例更换公网 IP 完成",
		Provider:     providerName,
		Account:      account,
		ResourceID:   vmID,
		ResourceName: vmID,
		Content:      strings.Join(lines, "\n"),
		Timestamp:    time.Now(),
	})
}

func emitVMPowerEvent(providerName, account, vmID, action, source string) {
	actionText := action
	switch strings.ToLower(action) {
	case "start":
		actionText = "开机"
	case "stop":
		actionText = "关机"
	case "restart":
		actionText = "重启"
	}
	lines := []string{fmt.Sprintf("已执行操作: %s", actionText)}
	if source != "" {
		lines = append(lines, "操作来源: "+source)
	}
	emitNotification(notify.Event{
		Type:         config.NotifyEventVMPower,
		Severity:     notify.SeverityInfo,
		Title:        fmt.Sprintf("实例%s指令已发送", actionText),
		Provider:     providerName,
		Account:      account,
		ResourceID:   vmID,
		ResourceName: vmID,
		Content:      strings.Join(lines, "\n"),
		Timestamp:    time.Now(),
	})
}

func emitAuthLoginEvent(ev auth.LoginEvent) {
	if ev.Success {
		lines := []string{
			fmt.Sprintf("登录账号: %s", valueOrEmpty(ev.Username, "-")),
			fmt.Sprintf("来源 IP: %s", valueOrEmpty(ev.ClientIP, "-")),
		}
		if ev.UserAgent != "" {
			lines = append(lines, "客户端: "+ev.UserAgent)
		}
		emitNotification(notify.Event{
			Type:      config.NotifyEventAuthLogin,
			Severity:  notify.SeverityInfo,
			Title:     "管理面板登录成功提醒",
			Content:   strings.Join(lines, "\n"),
			Timestamp: ev.Time,
		})
		return
	}
	if ev.Locked {
		lines := []string{
			fmt.Sprintf("尝试账号: %s", valueOrEmpty(ev.Username, "-")),
			fmt.Sprintf("来源 IP: %s", valueOrEmpty(ev.ClientIP, "-")),
			fmt.Sprintf("连续失败次数: %d（已临时锁定）", ev.Failures),
		}
		if ev.UserAgent != "" {
			lines = append(lines, "客户端: "+ev.UserAgent)
		}
		emitNotification(notify.Event{
			Type:      config.NotifyEventAuthLogin,
			Severity:  notify.SeverityCritical,
			Title:     "管理面板登录异常锁定告警",
			Content:   strings.Join(lines, "\n"),
			Timestamp: ev.Time,
		})
	}
}

func valueOrEmpty(s, fallback string) string {
	if strings.TrimSpace(s) != "" {
		return s
	}
	return fallback
}
