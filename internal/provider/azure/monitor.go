package azure

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

type AzureMonitorStatus struct {
	Running        bool                            `json:"running"`
	Config         config.AzureCreditMonitorConfig `json:"config"`
	LastOverview   *AzureCostOverview              `json:"lastOverview,omitempty"`
	Triggered      bool                            `json:"triggered"`
	TriggerReasons []string                        `json:"triggerReasons,omitempty"`
	ActionExecuted bool                            `json:"actionExecuted"`
	Logs           []string                        `json:"logs"`
}

type AzureCreditMonitor struct {
	service        *AzureService
	account        config.AzureConfig
	mu             sync.RWMutex
	config         config.AzureCreditMonitorConfig
	stopChan       chan struct{}
	running        bool
	lastOverview   *AzureCostOverview
	triggered      bool
	triggerReasons []string
	actionExecuted bool
	logs           []string
	onAlert        func(account config.AzureConfig, ov *AzureCostOverview, reasons []string, action string, actionLogs []string)
}

func (m *AzureCreditMonitor) SetOnAlert(fn func(account config.AzureConfig, ov *AzureCostOverview, reasons []string, action string, actionLogs []string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onAlert = fn
}

func NewAzureCreditMonitor(service *AzureService, account config.AzureConfig, cfg config.AzureCreditMonitorConfig) *AzureCreditMonitor {
	cfg.Normalize()
	return &AzureCreditMonitor{
		service: service,
		account: account,
		config:  cfg,
	}
}

// QueryNow refreshes the cost & credit overview without executing destructive actions.
func (m *AzureCreditMonitor) QueryNow() (*AzureCostOverview, error) {
	m.mu.RLock()
	cfg := m.config
	m.mu.RUnlock()

	ov, err := m.service.GetCostAndCreditOverview(cfg)
	if err != nil {
		return nil, err
	}

	triggered, reasons := EvaluateCreditTriggers(cfg, ov)

	m.mu.Lock()
	m.lastOverview = ov
	m.triggered = triggered
	m.triggerReasons = reasons
	m.mu.Unlock()

	return ov, nil
}

// EvaluateCreditTriggers checks configured thresholds against an AzureCostOverview.
func EvaluateCreditTriggers(cfg config.AzureCreditMonitorConfig, ov *AzureCostOverview) (bool, []string) {
	if ov == nil {
		return false, nil
	}
	var reasons []string

	if cfg.DaysBeforeExpire > 0 && ov.DaysUntilExpire != nil {
		if *ov.DaysUntilExpire <= cfg.DaysBeforeExpire {
			if *ov.DaysUntilExpire < 0 {
				reasons = append(reasons, fmt.Sprintf("赠金已过期 %d 天", -*ov.DaysUntilExpire))
			} else {
				reasons = append(reasons, fmt.Sprintf("距离赠金到期仅剩 %d 天 (≤ %d 天)", *ov.DaysUntilExpire, cfg.DaysBeforeExpire))
			}
		}
	}

	if cfg.MinRemainingCredit > 0 && ov.CostStatus == "ok" && ov.EstimatedRemainingUSD != nil {
		if *ov.EstimatedRemainingUSD <= cfg.MinRemainingCredit {
			reasons = append(reasons, fmt.Sprintf("估算剩余赠金 $%.2f USD (≤ $%.2f USD)", *ov.EstimatedRemainingUSD, cfg.MinRemainingCredit))
		}
	}

	if cfg.MaxMonthlyCost > 0 && ov.CostStatus == "ok" {
		if ov.MonthToDateCostUSD >= cfg.MaxMonthlyCost {
			reasons = append(reasons, fmt.Sprintf("本月消费已达 $%.2f USD (≥ $%.2f USD)", ov.MonthToDateCostUSD, cfg.MaxMonthlyCost))
		}
	}

	return len(reasons) > 0, reasons
}

func (m *AzureCreditMonitor) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.stopChan = make(chan struct{})
	stopCh := m.stopChan
	interval := m.config.Interval
	if interval < 300 {
		interval = 300
	}
	m.mu.Unlock()

	m.addLog(fmt.Sprintf("已启动周期监控（周期 %d 秒，触发动作: %s）", interval, m.GetConfig().TriggerAction))

	go func() {
		// Perform initial check shortly after start
		m.runCycle()

		ticker := time.NewTicker(time.Duration(interval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				m.runCycle()
			}
		}
	}()
}

func (m *AzureCreditMonitor) runCycle() {
	ov, err := m.QueryNow()
	if err != nil {
		m.addLog(fmt.Sprintf("周期检测失败：%v", err))
		return
	}
	if ov.CostStatus != "ok" {
		m.addLog(fmt.Sprintf("费用查询异常：%s", ov.CostMessage))
		// Note: expiration-date trigger can still fire even if cost API failed
	} else {
		remText := "未配置初始赠金"
		if ov.EstimatedRemainingUSD != nil {
			remText = fmt.Sprintf("估算剩余 $%.2f", *ov.EstimatedRemainingUSD)
		}
		expText := "未配置到期日"
		if ov.DaysUntilExpire != nil {
			expText = fmt.Sprintf("距到期 %d 天", *ov.DaysUntilExpire)
		}
		m.addLog(fmt.Sprintf("周期检测：本月 $%.2f / 累计 $%.2f USD | %s | %s",
			ov.MonthToDateCostUSD, ov.PeriodTotalCostUSD, remText, expText))
	}

	m.mu.RLock()
	cfg := m.config
	triggered := m.triggered
	reasons := append([]string(nil), m.triggerReasons...)
	alreadyActed := m.actionExecuted
	m.mu.RUnlock()

	if !triggered {
		return
	}

	reasonSummary := strings.Join(reasons, "；")
	m.addLog("⚠️ 触发监控阈值：" + reasonSummary)

	m.mu.RLock()
	alertCB := m.onAlert
	m.mu.RUnlock()
	emitAlert := func(action string, actionLogs []string) {
		if alertCB != nil {
			go alertCB(m.account, ov, reasons, action, actionLogs)
		}
	}

	if cfg.TriggerAction == "alert" {
		emitAlert("alert", nil)
		return
	}
	if alreadyActed {
		m.addLog("已在本周期策略下执行过自动动作，跳过重复执行（如需重置请重新保存配置或重启监控）")
		return
	}

	switch cfg.TriggerAction {
	case "deallocate":
		m.addLog("正在自动停止（Deallocate）所有 Azure 虚拟机...")
		stopped, errs, err := m.service.DeallocateAllVMs()
		if err != nil {
			errMsg := fmt.Sprintf("❌ 自动停机失败：%v", err)
			m.addLog(errMsg)
			emitAlert("deallocate", []string{errMsg})
			return
		}
		var actionLogs []string
		if len(stopped) > 0 {
			msg := fmt.Sprintf("✅ 已发起停机释放 VM：%s", strings.Join(stopped, ", "))
			m.addLog(msg)
			actionLogs = append(actionLogs, msg)
		} else {
			msg := "未检测到可停机的 VM"
			m.addLog(msg)
			actionLogs = append(actionLogs, msg)
		}
		for _, e := range errs {
			msg := "❌ 停机异常：" + e
			m.addLog(msg)
			actionLogs = append(actionLogs, msg)
		}
		if len(errs) == 0 {
			m.markActionExecuted()
		}
		emitAlert("deallocate", actionLogs)

	case "cleanup_orphans":
		m.addLog("正在自动清理孤立计费资源（未绑定公网 IP、未挂载磁盘、孤立网卡、快照）...")
		res, err := m.service.CleanupResources(AzureCleanupRequest{
			Mode:               "orphans",
			IncludeOrphanIPs:   true,
			IncludeOrphanDisks: true,
			IncludeOrphanNICs:  true,
			IncludeSnapshots:   true,
		})
		if err != nil {
			errMsg := fmt.Sprintf("❌ 清理孤立资源失败：%v", err)
			m.addLog(errMsg)
			emitAlert("cleanup_orphans", []string{errMsg})
			return
		}
		for _, line := range res.Logs {
			m.addLog(line)
		}
		if res.Success {
			m.markActionExecuted()
		}
		emitAlert("cleanup_orphans", res.Logs)

	case "delete_all":
		if !cfg.ConfirmAutoDeleteAll {
			msg := "⚠️ 触发动作设为“自动删除全部计费资源”，但未勾选二次安全确认开关，已拦截删除仅作告警"
			m.addLog(msg)
			emitAlert("alert", []string{msg})
			return
		}
		m.addLog("🚨 已确认高危自动清理：正在按依赖顺序删除全部计费资源...")
		res, err := m.service.CleanupResources(AzureCleanupRequest{
			Mode:           "all_billable",
			IncludeNetwork: false,
		})
		if err != nil {
			errMsg := fmt.Sprintf("❌ 自动清空计费资源失败：%v", err)
			m.addLog(errMsg)
			emitAlert("delete_all", []string{errMsg})
			return
		}
		for _, line := range res.Logs {
			m.addLog(line)
		}
		if res.Success {
			m.markActionExecuted()
		}
		emitAlert("delete_all", res.Logs)
	}
}

func (m *AzureCreditMonitor) markActionExecuted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actionExecuted = true
}

func (m *AzureCreditMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running && m.stopChan != nil {
		close(m.stopChan)
		m.running = false
		m.logs = append(m.logs, fmt.Sprintf("[%s] 监控已停止", time.Now().Format("15:04:05")))
		if len(m.logs) > 80 {
			m.logs = m.logs[len(m.logs)-80:]
		}
	}
}

func (m *AzureCreditMonitor) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

func (m *AzureCreditMonitor) GetConfig() config.AzureCreditMonitorConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

func (m *AzureCreditMonitor) UpdateConfig(cfg config.AzureCreditMonitorConfig) {
	cfg.Normalize()
	wasRunning := m.IsRunning()
	if wasRunning {
		m.Stop()
	}
	m.mu.Lock()
	m.config = cfg
	m.actionExecuted = false
	m.mu.Unlock()
	if cfg.Enabled || wasRunning {
		if cfg.Enabled {
			m.Start()
		}
	}
}

func (m *AzureCreditMonitor) GetStatus() AzureMonitorStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	logsCopy := make([]string, len(m.logs))
	copy(logsCopy, m.logs)
	reasonsCopy := make([]string, len(m.triggerReasons))
	copy(reasonsCopy, m.triggerReasons)
	return AzureMonitorStatus{
		Running:        m.running,
		Config:         m.config,
		LastOverview:   m.lastOverview,
		Triggered:      m.triggered,
		TriggerReasons: reasonsCopy,
		ActionExecuted: m.actionExecuted,
		Logs:           logsCopy,
	}
}

func (m *AzureCreditMonitor) addLog(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	m.logs = append(m.logs, entry)
	if len(m.logs) > 80 {
		m.logs = m.logs[len(m.logs)-80:]
	}
}
