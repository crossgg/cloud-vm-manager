package main

type ChangeIPResult struct {
	Success      bool     `json:"success"`
	Message      string   `json:"message"`
	NewIPAddress string   `json:"newIpAddress,omitempty"`
	Logs         []string `json:"logs,omitempty"`
}

type AccountBalance struct {
	Provider string  `json:"provider"`
	Account  string  `json:"account"`
	Total    float64 `json:"total"`
	Currency string  `json:"currency"`
	Note     string  `json:"note,omitempty"`
}

type CloudService interface {
	ListVMs() ([]map[string]interface{}, error)
	GetVM(id string) (map[string]interface{}, error)
	StartVM(id string) error
	StopVM(id string) error
	RestartVM(id string) error
	ChangeIP(id string) (*ChangeIPResult, error)
}
