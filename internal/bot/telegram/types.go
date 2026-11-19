package telegram

import "time"

type AccountSummary struct {
	Provider string
	Account  string
	Group    string
}

type VMSummary struct {
	ID       string
	Name     string
	State    string
	PublicIP string
	IPv6     string
	Region   string
	Shape    string
}

type VMDetail struct {
	ID            string
	Name          string
	State         string
	PublicIP      string
	PrivateIP     string
	IPv6          string
	Region        string
	Shape         string
	OS            string
	HasDNSBinding bool
	RawSummary    string
}

type ChangeIPOutcome struct {
	Success       bool
	Message       string
	OldIP         string
	NewIP         string
	Logs          []string
	HasDNSBinding bool
}

type OCITaskSummary struct {
	ID           string
	Name         string
	Account      string
	Status       string
	Shape        string
	Attempts     int
	LastMessage  string
	CreatedVMID  string
}

type OCITaskDetail struct {
	OCITaskSummary
	AvailabilityDomain string
	OCPUs              float64
	MemoryInGBs        float64
	LastAttemptAt      string
	LastError          string
	Logs               []string
}

type OCIDTStatus struct {
	Account    string
	Running    bool
	AutoStop   bool
	StopMethod string
	Interval   int
	Threshold  float64
	UsageGB    float64
	Percentage float64
	QueryTime  string
	QueryError string
	Logs       []string
}

type AzureMonitorSnapshot struct {
	Account          string
	Running          bool
	Interval         int
	TriggerAction    string
	Triggered        bool
	TriggerReasons   []string
	ActionExecuted   bool
	MonthCostUSD     float64
	PeriodCostUSD    float64
	RemainingUSD     *float64
	DaysUntilExpire  *int
	CostStatus       string
	CostMessage      string
	QueriedAt        string
	Logs             []string
}

type OverviewSummary struct {
	ConfigCount        int
	InstanceCount      int
	RunningCount       int
	StoppedCount       int
	CachedAccountCount int
}

type Backend interface {
	AccountsByProvider(provider string) []AccountSummary
	GetOverview() OverviewSummary
	ListVMs(provider, account string) ([]VMSummary, error)
	GetVM(provider, account, vmID string) (*VMDetail, error)
	PowerVM(provider, account, vmID, action string) error
	ChangeVMIP(provider, account, vmID string) (*ChangeIPOutcome, error)
	UpdateVMDNS(provider, account, vmID string) ([]string, error)
	ListOCICreateTasks(account string) ([]OCITaskSummary, error)
	GetOCICreateTask(account, taskID string) (*OCITaskDetail, error)
	StartOCICreateTask(account, taskID string) (*OCITaskDetail, error)
	StopOCICreateTask(account, taskID string) (*OCITaskDetail, error)
	GetOCIDTMonitor(account string) (*OCIDTStatus, error)
	QueryOCIDTMonitorNow(account string) (*OCIDTStatus, error)
	GetAzureCreditMonitor(account string) (*AzureMonitorSnapshot, error)
	QueryAzureCreditMonitorNow(account string) (*AzureMonitorSnapshot, error)
}

type BotStatus struct {
	Enabled           bool   `json:"enabled"`
	Running           bool   `json:"running"`
	Connected         bool   `json:"connected"`
	BotUsername       string `json:"botUsername,omitempty"`
	ProxyID           string `json:"proxyId"`
	FallbackProxyID   string `json:"fallbackProxyId"`
	LastPollAt        string `json:"lastPollAt,omitempty"`
	LastError         string `json:"lastError,omitempty"`
	UpdatesProcessed  int64  `json:"updatesProcessed"`
	WhitelistWarning  string `json:"whitelistWarning,omitempty"`
	AutoNotify        bool   `json:"autoNotify"`
	UpdatedAt         time.Time `json:"-"`
}
