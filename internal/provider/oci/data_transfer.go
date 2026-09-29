package oci

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

// config.DataTransferConfig holds the configuration for OCI data transfer monitoring.


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
	account    config.OCIConfig
	mu         sync.RWMutex
	config     config.DataTransferConfig
	stopChan   chan struct{}
	running    bool
	lastResult *DataTransferResult
	logs       []string
}

// NewOCIDataTransferMonitor creates a new monitor instance.
func NewOCIDataTransferMonitor(service *OCIService, account config.OCIConfig, config config.DataTransferConfig) *OCIDataTransferMonitor {
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
func (m *OCIDataTransferMonitor) GetConfig() config.DataTransferConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// UpdateConfig updates the configuration. If the monitor is running, it restarts.
func (m *OCIDataTransferMonitor) UpdateConfig(config config.DataTransferConfig) {
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
			"compartmentId":            {o.Account.Tenancy},
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
		Host:   "telemetry." + o.Account.Region + ".oraclecloud.com",
		Path:   "/20180401/metrics" + path,
	}
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	return u.String()
}

