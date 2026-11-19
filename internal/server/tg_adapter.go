package server

import (
	"fmt"
	"strings"

	"github.com/crossgg/cloud-vm-manager/internal/bot/telegram"
	"github.com/crossgg/cloud-vm-manager/internal/cache"
	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/crossgg/cloud-vm-manager/internal/provider/azure"
	"github.com/crossgg/cloud-vm-manager/internal/provider/oci"
)

type serverTGBackend struct{}

func (s *serverTGBackend) AccountsByProvider(providerName string) []telegram.AccountSummary {
	accounts := cloudAccountsSnapshot()
	out := make([]telegram.AccountSummary, 0)
	for _, acc := range accounts {
		p, _ := acc["provider"].(string)
		if !strings.EqualFold(p, providerName) {
			continue
		}
		name, _ := acc["account"].(string)
		group, _ := acc["group"].(string)
		out = append(out, telegram.AccountSummary{
			Provider: strings.ToLower(p),
			Account:  name,
			Group:    group,
		})
	}
	return out
}

func (s *serverTGBackend) GetOverview() telegram.OverviewSummary {
	accounts := cloudAccountsSnapshot()
	cStore := cache.Global()
	if cStore == nil {
		return telegram.OverviewSummary{ConfigCount: len(accounts)}
	}
	ov := cStore.GetOverview(accounts)
	return telegram.OverviewSummary{
		ConfigCount:        intFromAny(ov["configCount"]),
		InstanceCount:      intFromAny(ov["instanceCount"]),
		RunningCount:       intFromAny(ov["runningCount"]),
		StoppedCount:       intFromAny(ov["stoppedCount"]),
		CachedAccountCount: intFromAny(ov["cachedAccountCount"]),
	}
}

func (s *serverTGBackend) ListVMs(providerName, account string) ([]telegram.VMSummary, error) {
	service, _, ok := serviceSnapshot(providerName, account)
	if !ok || service == nil {
		return nil, fmt.Errorf("账号 %s/%s 不存在", providerName, account)
	}
	vms, err := service.ListVMs()
	if err != nil {
		return nil, err
	}
	if cStore := cache.Global(); cStore != nil {
		group := ""
		for _, acc := range cloudAccountsSnapshot() {
			p, _ := acc["provider"].(string)
			a, _ := acc["account"].(string)
			if strings.EqualFold(p, providerName) && a == account {
				group, _ = acc["group"].(string)
				break
			}
		}
		cStore.UpdateAccount(providerName, account, group, vms)
	}
	out := make([]telegram.VMSummary, 0, len(vms))
	for _, vm := range vms {
		id := strFromAny(vm["id"])
		name := strFromAny(vm["name"])
		if name == "" {
			name = id
		}
		pubIP, _ := publicIPAddress(vm)
		ipv6, _ := primaryIPv6Address(vm)
		out = append(out, telegram.VMSummary{
			ID:       id,
			Name:     name,
			State:    strFromAny(vm["status"]),
			PublicIP: pubIP,
			IPv6:     ipv6,
			Region:   strFromAny(vm["location"]),
			Shape:    strFromAny(vm["vmSize"]),
		})
	}
	return out, nil
}

func (s *serverTGBackend) GetVM(providerName, account, vmID string) (*telegram.VMDetail, error) {
	service, cloudflare, ok := serviceSnapshot(providerName, account)
	if !ok || service == nil {
		return nil, fmt.Errorf("账号 %s/%s 不存在", providerName, account)
	}
	vm, err := service.GetVM(vmID)
	if err != nil {
		return nil, err
	}
	id := strFromAny(vm["id"])
	if id == "" {
		id = vmID
	}
	name := strFromAny(vm["name"])
	if name == "" {
		name = id
	}
	pubIP, _ := publicIPAddress(vm)
	ipv6, _ := primaryIPv6Address(vm)
	if ipv6 == "" {
		raw6 := strFromAny(vm["ipv6"])
		if raw6 != "" && raw6 != "未分配" && raw6 != "N/A" {
			ipv6 = raw6
		}
	}
	hasDNS := cloudflare != nil && cloudflare.HasBinding(providerName, account, id)
	return &telegram.VMDetail{
		ID:            id,
		Name:          name,
		State:         strFromAny(vm["status"]),
		PublicIP:      pubIP,
		PrivateIP:     strFromAny(vm["privateIP"]),
		IPv6:          ipv6,
		Region:        strFromAny(vm["location"]),
		Shape:         strFromAny(vm["vmSize"]),
		HasDNSBinding: hasDNS,
	}, nil
}

func (s *serverTGBackend) PowerVM(providerName, account, vmID, action string) error {
	service, _, ok := serviceSnapshot(providerName, account)
	if !ok || service == nil {
		return fmt.Errorf("账号 %s/%s 不存在", providerName, account)
	}
	var err error
	switch strings.ToLower(action) {
	case "start":
		err = service.StartVM(vmID)
	case "stop":
		err = service.StopVM(vmID)
	case "restart":
		err = service.RestartVM(vmID)
	default:
		return fmt.Errorf("不支持的操作 %q", action)
	}
	if err != nil {
		return err
	}
	emitVMPowerEvent(providerName, account, vmID, action, "TG Bot")
	return nil
}

func (s *serverTGBackend) ChangeVMIP(providerName, account, vmID string) (*telegram.ChangeIPOutcome, error) {
	service, cloudflare, ok := serviceSnapshot(providerName, account)
	if !ok || service == nil {
		return nil, fmt.Errorf("账号 %s/%s 不存在", providerName, account)
	}
	result, err := service.ChangeIP(vmID)
	if err != nil {
		return nil, err
	}
	emitVMChangeIPEvent(providerName, account, vmID, result.NewIPAddress, result.Message, "TG Bot")
	hasDNS := cloudflare != nil && cloudflare.HasBinding(providerName, account, vmID)
	return &telegram.ChangeIPOutcome{
		Success:       result.Success,
		Message:       result.Message,
		NewIP:         result.NewIPAddress,
		Logs:          result.Logs,
		HasDNSBinding: hasDNS,
	}, nil
}

func (s *serverTGBackend) UpdateVMDNS(providerName, account, vmID string) ([]string, error) {
	service, cloudflare, ok := serviceSnapshot(providerName, account)
	if !ok || service == nil {
		return nil, fmt.Errorf("账号 %s/%s 不存在", providerName, account)
	}
	if cloudflare == nil || !cloudflare.HasBinding(providerName, account, vmID) {
		return nil, fmt.Errorf("该实例未配置 Cloudflare DNS 绑定")
	}
	vm, err := service.GetVM(vmID)
	if err != nil {
		return nil, err
	}
	ipv4, _ := publicIPAddress(vm)
	ipv6, _ := primaryIPv6Address(vm)
	hasA, hasAAAA := cloudflare.BindingTypes(providerName, account, vmID)
	if (!hasA || ipv4 == "") && (!hasAAAA || ipv6 == "") {
		return nil, fmt.Errorf("实例当前没有可同步的公网 IPv4 / IPv6")
	}
	logs := cloudflare.UpdateForVMByIPs(providerName, account, vmID, ipv4, ipv6)
	return logs, nil
}

func (s *serverTGBackend) ListOCICreateTasks(account string) ([]telegram.OCITaskSummary, error) {
	ociCreateTaskStore.mu.RLock()
	manager := ociCreateTaskStore.manager
	ociCreateTaskStore.mu.RUnlock()
	if manager == nil {
		return nil, fmt.Errorf("OCI 创建任务管理器未初始化")
	}
	tasks := manager.ListTasks(account)
	out := make([]telegram.OCITaskSummary, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, telegram.OCITaskSummary{
			ID:          t.ID,
			Name:        t.Name,
			Account:     t.Account,
			Status:      t.Status,
			Shape:       t.Shape,
			Attempts:    int(t.Attempts),
			LastMessage: t.LastMessage,
			CreatedVMID: t.CreatedInstanceID,
		})
	}
	return out, nil
}

func (s *serverTGBackend) GetOCICreateTask(account, taskID string) (*telegram.OCITaskDetail, error) {
	ociCreateTaskStore.mu.RLock()
	manager := ociCreateTaskStore.manager
	ociCreateTaskStore.mu.RUnlock()
	if manager == nil {
		return nil, fmt.Errorf("OCI 创建任务管理器未初始化")
	}
	tasks := manager.ListTasks(account)
	for _, t := range tasks {
		if t.ID == taskID {
			return toTGTaskDetail(t), nil
		}
	}
	return nil, fmt.Errorf("未找到抢购任务 %s", taskID)
}

func (s *serverTGBackend) StartOCICreateTask(account, taskID string) (*telegram.OCITaskDetail, error) {
	ociCreateTaskStore.mu.RLock()
	manager := ociCreateTaskStore.manager
	ociCreateTaskStore.mu.RUnlock()
	if manager == nil {
		return nil, fmt.Errorf("OCI 创建任务管理器未初始化")
	}
	task, err := manager.StartTask(taskID)
	if err != nil {
		return nil, err
	}
	return toTGTaskDetail(task), nil
}

func (s *serverTGBackend) StopOCICreateTask(account, taskID string) (*telegram.OCITaskDetail, error) {
	ociCreateTaskStore.mu.RLock()
	manager := ociCreateTaskStore.manager
	ociCreateTaskStore.mu.RUnlock()
	if manager == nil {
		return nil, fmt.Errorf("OCI 创建任务管理器未初始化")
	}
	task, err := manager.StopTask(taskID)
	if err != nil {
		return nil, err
	}
	return toTGTaskDetail(task), nil
}

func (s *serverTGBackend) GetOCIDTMonitor(account string) (*telegram.OCIDTStatus, error) {
	return buildOCIDTStatus(account, false)
}

func (s *serverTGBackend) QueryOCIDTMonitorNow(account string) (*telegram.OCIDTStatus, error) {
	return buildOCIDTStatus(account, true)
}

func buildOCIDTStatus(account string, queryNow bool) (*telegram.OCIDTStatus, error) {
	service, _, ok := serviceSnapshot("oci", account)
	if !ok || service == nil {
		return nil, fmt.Errorf("OCI 账号 %q 不存在", account)
	}
	ociSvc, ok := service.(*oci.OCIService)
	if !ok {
		return nil, fmt.Errorf("账号 %q 不是 OCI 服务", account)
	}
	monitor := getOrCreateDTMonitor(account)
	if monitor == nil {
		ociCfg := findOCIConfig(account)
		cfg := ociCfg.DTMonitor
		if cfg.Threshold <= 0 {
			cfg.Threshold = 9000
		}
		if cfg.Interval <= 0 {
			cfg.Interval = 300
		}
		if cfg.StopMethod == "" {
			cfg.StopMethod = "soft"
		}
		monitor = oci.NewOCIDataTransferMonitor(ociSvc, ociCfg, cfg)
		monitor.SetOnAlert(onOCIDTAlertCallback())
		setDTMonitor(account, monitor)
	}
	var last *oci.DataTransferResult
	if queryNow {
		last = monitor.QueryNow()
	} else {
		last = monitor.GetLastResult()
	}
	cfg := monitor.GetConfig()
	st := &telegram.OCIDTStatus{
		Account:    account,
		Running:    monitor.IsRunning(),
		AutoStop:   cfg.AutoStop,
		StopMethod: cfg.StopMethod,
		Interval:   cfg.Interval,
		Threshold:  cfg.Threshold,
		Logs:       monitor.GetLogs(),
	}
	if last != nil {
		st.UsageGB = last.UsageGB
		st.Percentage = last.Percentage
		st.QueryError = last.Error
		if !last.QueryTime.IsZero() {
			st.QueryTime = last.QueryTime.Format("2006-01-02 15:04:05")
		}
	}
	return st, nil
}

func (s *serverTGBackend) GetAzureCreditMonitor(account string) (*telegram.AzureMonitorSnapshot, error) {
	return buildAzureMonitorSnapshot(account, false)
}

func (s *serverTGBackend) QueryAzureCreditMonitorNow(account string) (*telegram.AzureMonitorSnapshot, error) {
	return buildAzureMonitorSnapshot(account, true)
}

func buildAzureMonitorSnapshot(account string, queryNow bool) (*telegram.AzureMonitorSnapshot, error) {
	service, _, ok := serviceSnapshot("azure", account)
	if !ok || service == nil {
		return nil, fmt.Errorf("Azure 账号 %q 不存在", account)
	}
	azSvc, ok := service.(*azure.AzureService)
	if !ok {
		return nil, fmt.Errorf("账号 %q 不是 Azure 服务", account)
	}
	monitor := getOrCreateAzureMonitor(account, azSvc)
	if queryNow {
		if _, err := monitor.QueryNow(); err != nil {
			return nil, err
		}
	}
	st := monitor.GetStatus()
	snap := &telegram.AzureMonitorSnapshot{
		Account:        account,
		Running:        st.Running,
		Interval:       st.Config.Interval,
		TriggerAction:  st.Config.TriggerAction,
		Triggered:      st.Triggered,
		TriggerReasons: st.TriggerReasons,
		ActionExecuted: st.ActionExecuted,
		Logs:           st.Logs,
	}
	if st.LastOverview != nil {
		snap.MonthCostUSD = st.LastOverview.MonthToDateCostUSD
		snap.PeriodCostUSD = st.LastOverview.PeriodTotalCostUSD
		snap.RemainingUSD = st.LastOverview.EstimatedRemainingUSD
		snap.DaysUntilExpire = st.LastOverview.DaysUntilExpire
		snap.CostStatus = st.LastOverview.CostStatus
		snap.CostMessage = st.LastOverview.CostMessage
		if !st.LastOverview.QueriedAt.IsZero() {
			snap.QueriedAt = st.LastOverview.QueriedAt.Format("2006-01-02 15:04:05")
		}
	}
	return snap, nil
}

func toTGTaskDetail(t oci.OCICreateTask) *telegram.OCITaskDetail {
	return &telegram.OCITaskDetail{
		OCITaskSummary: telegram.OCITaskSummary{
			ID:          t.ID,
			Name:        t.Name,
			Account:     t.Account,
			Status:      t.Status,
			Shape:       t.Shape,
			Attempts:    int(t.Attempts),
			LastMessage: t.LastMessage,
			CreatedVMID: t.CreatedInstanceID,
		},
		AvailabilityDomain: t.AvailabilityDomain,
		OCPUs:              t.OCPUs,
		MemoryInGBs:        t.MemoryInGBs,
		LastAttemptAt:      t.LastAttemptAt,
		LastError:          t.LastError,
		Logs:               t.Logs,
	}
}

func intFromAny(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func strFromAny(v interface{}) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// Keep unused config import clean if needed
var _ = config.ProxyFallbackNone
