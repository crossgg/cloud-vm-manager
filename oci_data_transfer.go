package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// DataTransferConfig holds the configuration for OCI data transfer monitoring.
type DataTransferConfig struct {
	Enabled    bool    `json:"enabled"`
	Interval   int     `json:"interval"`   // detection interval in seconds, default 300
	Threshold  float64 `json:"threshold"`  // threshold in GB, default 9000 (OCI free tier is ~10TB)
	AutoStop   bool    `json:"autoStop"`   // auto-stop instances when threshold exceeded
	StopMethod string  `json:"stopMethod"` // "soft" or "hard"
}

// DataTransferResult holds the result of a data transfer usage query.
type DataTransferResult struct {
	UsageGB    float64   `json:"usageGB"`
	Threshold  float64   `json:"threshold"`
	Percentage float64   `json:"percentage"`
	QueryTime  time.Time `json:"queryTime"`
	Error      string    `json:"error,omitempty"`
}

// OCIDataTransferMonitor manages periodic data transfer monitoring for an OCI account.
type OCIDataTransferMonitor struct {
	service    *OCIService
	account    OCIConfig
	mu         sync.RWMutex
	config     DataTransferConfig
	stopChan   chan struct{}
	running    bool
	lastResult *DataTransferResult
	logs       []string
}

// NewOCIDataTransferMonitor creates a new monitor instance.
func NewOCIDataTransferMonitor(service *OCIService, account OCIConfig, config DataTransferConfig) *OCIDataTransferMonitor {
	if config.Interval <= 0 {
		config.Interval = 300
	}
	if config.Threshold <= 0 {
		config.Threshold = 9000
	}
	if config.StopMethod == "" {
		config.StopMethod = "soft"
	}
	return &OCIDataTransferMonitor{
		service: service,
		account: account,
		config:  config,
	}
}

// QueryNow performs an immediate data transfer usage query.
func (m *OCIDataTransferMonitor) QueryNow() *DataTransferResult {
	usageGB, err := m.service.QueryDataTransfer()
	m.mu.Lock()
	defer m.mu.Unlock()
	threshold := m.config.Threshold
	if threshold <= 0 {
		threshold = 9000
	}
	result := &DataTransferResult{
		UsageGB:   usageGB,
		Threshold: threshold,
		QueryTime: time.Now(),
	}
	if err != nil {
		result.Error = err.Error()
	} else {
		result.Percentage = (usageGB / threshold) * 100
	}
	m.lastResult = result
	return result
}

// Start begins periodic monitoring.
func (m *OCIDataTransferMonitor) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.stopChan = make(chan struct{})
	interval := m.config.Interval
	autoStop := m.config.AutoStop
	stopMethod := m.config.StopMethod
	threshold := m.config.Threshold
	m.mu.Unlock()

	go func() {
		ticker := time.NewTicker(time.Duration(interval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-m.stopChan:
				return
			case <-ticker.C:
				result := m.QueryNow()
				if result.Error == "" {
					m.addLog(fmt.Sprintf("周期检测：当月已用 %.2f GB / %.0f GB (%.1f%%)", result.UsageGB, result.Threshold, result.Percentage))
				} else {
					m.addLog(fmt.Sprintf("周期检测失败：%s", result.Error))
				}

				if autoStop && result.Error == "" && result.UsageGB > threshold {
					m.addLog(fmt.Sprintf("⚠️ 用量 %.2f GB 超过阈值 %.0f GB，正在自动停止实例...", result.UsageGB, threshold))
					m.autoStopInstances(stopMethod)
				}
			}
		}
	}()
}

// Stop stops the periodic monitoring.
func (m *OCIDataTransferMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running && m.stopChan != nil {
		close(m.stopChan)
		m.running = false
	}
}

// IsRunning returns whether the monitor is currently running.
func (m *OCIDataTransferMonitor) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// GetConfig returns the current config.
func (m *OCIDataTransferMonitor) GetConfig() DataTransferConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// UpdateConfig updates the configuration. If the monitor is running, it restarts.
func (m *OCIDataTransferMonitor) UpdateConfig(config DataTransferConfig) {
	wasRunning := m.IsRunning()
	if wasRunning {
		m.Stop()
	}
	m.mu.Lock()
	if config.Interval <= 0 {
		config.Interval = 300
	}
	if config.Threshold <= 0 {
		config.Threshold = 9000
	}
	if config.StopMethod == "" {
		config.StopMethod = "soft"
	}
	m.config = config
	if m.lastResult != nil {
		m.lastResult.Threshold = config.Threshold
		if config.Threshold > 0 {
			m.lastResult.Percentage = (m.lastResult.UsageGB / config.Threshold) * 100
		}
	}
	m.mu.Unlock()
	if config.Enabled {
		m.Start()
	}
}

// GetLastResult returns the last query result.
func (m *OCIDataTransferMonitor) GetLastResult() *DataTransferResult {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastResult
}

// GetLogs returns recent monitor logs.
func (m *OCIDataTransferMonitor) GetLogs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	logs := make([]string, len(m.logs))
	copy(logs, m.logs)
	return logs
}

func (m *OCIDataTransferMonitor) addLog(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	m.logs = append(m.logs, fmt.Sprintf("[%s] %s", timestamp, msg))
	if len(m.logs) > 100 {
		m.logs = m.logs[len(m.logs)-100:]
	}
}

func (m *OCIDataTransferMonitor) autoStopInstances(stopMethod string) {
	instances, err := m.service.ListVMs()
	if err != nil {
		m.addLog(fmt.Sprintf("获取实例列表失败：%s", err.Error()))
		return
	}
	action := "SOFTSTOP"
	if stopMethod == "hard" {
		action = "STOP"
	}
	for _, instance := range instances {
		status, _ := instance["status"].(string)
		if status != "VM running" {
			continue
		}
		id, _ := instance["id"].(string)
		name, _ := instance["name"].(string)
		if id == "" {
			continue
		}
		m.addLog(fmt.Sprintf("正在停止实例 %s (%s)，方式：%s", name, id, action))
		if err := m.service.instanceAction(id, action); err != nil {
			m.addLog(fmt.Sprintf("停止实例 %s 失败：%s", name, err.Error()))
		} else {
			m.addLog(fmt.Sprintf("实例 %s 已发送停止命令", name))
		}
	}
}

// QueryDataTransfer queries the current month's data transfer usage from OCI Monitoring API.
func (o *OCIService) QueryDataTransfer() (float64, error) {
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0)

	body := map[string]interface{}{
		"namespace": "oci_vcn",
		"query":     "VnicToNetworkBytes[1d].sum()",
		"startTime": monthStart.Format(time.RFC3339),
		"endTime":   monthEnd.Format(time.RFC3339),
	}

	var result []interface{}
	err := o.monitoringRequest("POST",
		"/actions/summarizeMetricsData",
		url.Values{
			"compartmentId":            {o.account.Tenancy},
			"compartmentIdInSubtree":   {"true"},
		},
		body, &result)
	if err != nil {
		return 0, err
	}

	var totalBytes float64
	for _, item := range result {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		points, ok := itemMap["aggregatedDatapoints"].([]interface{})
		if !ok {
			continue
		}
		for _, p := range points {
			point, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			val, ok := point["value"].(float64)
			if !ok {
				// try via json.Number
				if num, ok := point["value"].(json.Number); ok {
					val, _ = num.Float64()
				}
			}
			totalBytes += val
		}
	}

	usageGB := totalBytes / (1024 * 1024 * 1024)
	return usageGB, nil
}

// monitoringRequest sends a request to OCI Monitoring API (telemetry endpoint).
func (o *OCIService) monitoringRequest(method, path string, query url.Values, body interface{}, out interface{}) error {
	endpoint := o.monitoringEndpoint(path, query)
	return o.doRequest(method, endpoint, body, out, false)
}

// monitoringEndpoint builds the URL for OCI Monitoring API.
func (o *OCIService) monitoringEndpoint(path string, query url.Values) string {
	u := url.URL{
		Scheme: "https",
		Host:   "telemetry." + o.account.Region + ".oraclecloud.com",
		Path:   "/20180401/metrics" + path,
	}
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	return u.String()
}

// ----- API Handlers -----

// dtMonitors stores per-account data transfer monitors.
var dtMonitors = struct {
	mu       sync.RWMutex
	monitors map[string]*OCIDataTransferMonitor
}{
	monitors: make(map[string]*OCIDataTransferMonitor),
}

func getOrCreateDTMonitor(account string) *OCIDataTransferMonitor {
	dtMonitors.mu.RLock()
	monitor, ok := dtMonitors.monitors[account]
	dtMonitors.mu.RUnlock()
	if ok {
		return monitor
	}
	return nil
}

func setDTMonitor(account string, monitor *OCIDataTransferMonitor) {
	dtMonitors.mu.Lock()
	defer dtMonitors.mu.Unlock()
	dtMonitors.monitors[account] = monitor
}

func initDTMonitors() {
	runtimeState.mu.RLock()
	cfg := runtimeState.cfg
	runtimeState.mu.RUnlock()
	if cfg == nil {
		return
	}

	for _, account := range cfg.OCIAccounts {
		service, _, ok := serviceSnapshot("oci", account.Name)
		if !ok {
			continue
		}
		ociService, ok := service.(*OCIService)
		if !ok {
			continue
		}
		dtCfg := account.DTMonitor
		monitor := NewOCIDataTransferMonitor(ociService, account, dtCfg)
		setDTMonitor(account.Name, monitor)
		if dtCfg.Enabled {
			monitor.Start()
			fmt.Printf("OCI data transfer monitor started for account %s (interval=%ds, threshold=%.0fGB)\n",
				account.Name, dtCfg.Interval, dtCfg.Threshold)
		}
	}
}

func stopAllDTMonitors() {
	dtMonitors.mu.Lock()
	defer dtMonitors.mu.Unlock()
	for _, monitor := range dtMonitors.monitors {
		monitor.Stop()
	}
	dtMonitors.monitors = make(map[string]*OCIDataTransferMonitor)
}

// getOCIDataTransfer handles GET /api/oci/:account/data-transfer — manual query
func getOCIDataTransfer(c *gin.Context) {
	account := c.Param("account")
	service, _, ok := serviceSnapshot("oci", account)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("OCI account %s not found", account)})
		return
	}
	ociService, ok := service.(*OCIService)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not an OCI service"})
		return
	}

	monitor := getOrCreateDTMonitor(account)
	if monitor == nil {
		ociCfg := findOCIConfig(account)
		monitor = NewOCIDataTransferMonitor(ociService, ociCfg, DataTransferConfig{
			Threshold: 9000,
		})
		setDTMonitor(account, monitor)
	}

	result := monitor.QueryNow()
	c.JSON(http.StatusOK, result)
}

// getDataTransferConfig handles GET /api/oci/:account/data-transfer/config
func getDataTransferConfig(c *gin.Context) {
	account := c.Param("account")
	monitor := getOrCreateDTMonitor(account)
	if monitor == nil {
		c.JSON(http.StatusOK, DataTransferConfig{
			Interval:  300,
			Threshold: 9000,
			StopMethod: "soft",
		})
		return
	}
	cfg := monitor.GetConfig()
	c.JSON(http.StatusOK, cfg)
}

// saveDataTransferConfig handles POST /api/oci/:account/data-transfer/config
func saveDataTransferConfig(c *gin.Context) {
	account := c.Param("account")
	var payload DataTransferConfig
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	service, _, ok := serviceSnapshot("oci", account)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("OCI account %s not found", account)})
		return
	}
	ociService, ok := service.(*OCIService)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not an OCI service"})
		return
	}

	monitor := getOrCreateDTMonitor(account)
	if monitor == nil {
		ociCfg := findOCIConfig(account)
		monitor = NewOCIDataTransferMonitor(ociService, ociCfg, payload)
		setDTMonitor(account, monitor)
	}
	monitor.UpdateConfig(payload)

	// Update in-memory config
	runtimeState.mu.Lock()
	if runtimeState.cfg != nil {
		for i, a := range runtimeState.cfg.OCIAccounts {
			if a.Name == account {
				runtimeState.cfg.OCIAccounts[i].DTMonitor = payload
				break
			}
		}
	}
	runtimeState.mu.Unlock()

	// Persist to config file
	configPath := currentConfigPath()
	if err := SaveDTMonitorConfig(configPath, account, payload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("saved in memory but failed to persist: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "数据传输监控配置已保存。",
		"config":  payload,
	})
}

// startDataTransferMonitor handles POST /api/oci/:account/data-transfer/start
func startDataTransferMonitor(c *gin.Context) {
	account := c.Param("account")
	monitor := getOrCreateDTMonitor(account)
	if monitor == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "monitor not found, please save config first"})
		return
	}
	monitor.Start()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "周期检测已启动。"})
}

// stopDataTransferMonitor handles POST /api/oci/:account/data-transfer/stop
func stopDataTransferMonitor(c *gin.Context) {
	account := c.Param("account")
	monitor := getOrCreateDTMonitor(account)
	if monitor == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "monitor not found"})
		return
	}
	monitor.Stop()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "周期检测已停止。"})
}

// getDataTransferMonitorStatus handles GET /api/oci/:account/data-transfer/status
func getDataTransferMonitorStatus(c *gin.Context) {
	account := c.Param("account")
	monitor := getOrCreateDTMonitor(account)
	if monitor == nil {
		c.JSON(http.StatusOK, gin.H{
			"running":    false,
			"lastResult": nil,
			"config":     DataTransferConfig{Interval: 300, Threshold: 9000, StopMethod: "soft"},
			"logs":       []string{},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"running":    monitor.IsRunning(),
		"lastResult": monitor.GetLastResult(),
		"config":     monitor.GetConfig(),
		"logs":       monitor.GetLogs(),
	})
}

func findOCIConfig(accountName string) OCIConfig {
	runtimeState.mu.RLock()
	defer runtimeState.mu.RUnlock()
	if runtimeState.cfg == nil {
		return OCIConfig{}
	}
	for _, a := range runtimeState.cfg.OCIAccounts {
		if a.Name == accountName {
			return a
		}
	}
	return OCIConfig{}
}
