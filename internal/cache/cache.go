package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type AccountVMCache struct {
	Provider  string    `json:"provider"`
	Account   string    `json:"account"`
	Group     string    `json:"group"`
	Total     int       `json:"total"`
	Running   int       `json:"running"`
	Stopped   int       `json:"stopped"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type VMCacheData struct {
	UpdatedAt time.Time                 `json:"updatedAt"`
	Accounts  map[string]AccountVMCache `json:"accounts"`
}

type VMCacheStore struct {
	mu       sync.RWMutex
	filePath string
	data     VMCacheData
}

var globalVMCache *VMCacheStore

func InitVMCache(configPath string) *VMCacheStore {
	filePath := resolveVMCachePath(configPath)
	store := &VMCacheStore{
		filePath: filePath,
		data: VMCacheData{
			Accounts: make(map[string]AccountVMCache),
		},
	}
	store.load()
	globalVMCache = store
	return store
}

func Global() *VMCacheStore {
	return globalVMCache
}

func ServiceKey(provider, account string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + ":" + strings.TrimSpace(account)
}

func serviceKey(provider, account string) string {
	return ServiceKey(provider, account)
}

func resolveVMCachePath(configPath string) string {
	if configPath != "" {
		dir := filepath.Dir(configPath)
		if dir != "" && dir != "." {
			return filepath.Join(dir, "vm_cache.json")
		}
	}
	return "vm_cache.json"
}

func (s *VMCacheStore) load() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.filePath == "" {
		return
	}

	bytes, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}

	var data VMCacheData
	if err := json.Unmarshal(bytes, &data); err != nil {
		return
	}
	if data.Accounts == nil {
		data.Accounts = make(map[string]AccountVMCache)
	}
	s.data = data
}

func (s *VMCacheStore) save() {
	if s.filePath == "" {
		return
	}

	bytes, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return
	}

	_ = os.MkdirAll(filepath.Dir(s.filePath), 0755)
	_ = os.WriteFile(s.filePath, bytes, 0644)
}

func (s *VMCacheStore) UpdateAccount(provider, account, group string, vms []map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := ServiceKey(provider, account)
	running := 0
	stopped := 0

	for _, vm := range vms {
		status, _ := vm["status"].(string)
		statusLower := strings.ToLower(status)
		if statusLower == "vm running" || statusLower == "running" {
			running++
		} else if strings.Contains(statusLower, "stop") || strings.Contains(statusLower, "deallocated") || strings.Contains(statusLower, "terminated") {
			stopped++
		}
	}

	now := time.Now()
	s.data.Accounts[key] = AccountVMCache{
		Provider:  provider,
		Account:   account,
		Group:     group,
		Total:     len(vms),
		Running:   running,
		Stopped:   stopped,
		UpdatedAt: now,
	}
	s.data.UpdatedAt = now
	s.save()
}

func (s *VMCacheStore) GetOverview(activeAccounts []gin.H) gin.H {
	s.mu.RLock()
	defer s.mu.RUnlock()

	providerCounts := make(map[string]int)
	configuredKeys := make(map[string]gin.H)

	for _, acc := range activeAccounts {
		provider, _ := acc["provider"].(string)
		account, _ := acc["account"].(string)
		if provider != "" {
			providerCounts[provider]++
		}
		if provider != "" && account != "" {
			configuredKeys[ServiceKey(provider, account)] = acc
		}
	}

	totalVMs := 0
	totalRunning := 0
	totalStopped := 0
	cachedCount := 0
	var latestUpdate time.Time

	accountSummaries := make([]AccountVMCache, 0)

	for key, acc := range configuredKeys {
		if cache, exists := s.data.Accounts[key]; exists {
			cachedCount++
			totalVMs += cache.Total
			totalRunning += cache.Running
			totalStopped += cache.Stopped
			if cache.UpdatedAt.After(latestUpdate) {
				latestUpdate = cache.UpdatedAt
			}
			accountSummaries = append(accountSummaries, cache)
		} else {
			provider, _ := acc["provider"].(string)
			account, _ := acc["account"].(string)
			group, _ := acc["group"].(string)
			accountSummaries = append(accountSummaries, AccountVMCache{
				Provider: provider,
				Account:  account,
				Group:    group,
			})
		}
	}

	var latestUpdateStr string
	if !latestUpdate.IsZero() {
		latestUpdateStr = latestUpdate.Format("2006-01-02 15:04:05")
	}

	return gin.H{
		"configCount":        len(activeAccounts),
		"providerCounts":     providerCounts,
		"instanceCount":      totalVMs,
		"runningCount":       totalRunning,
		"stoppedCount":       totalStopped,
		"cachedAccountCount": cachedCount,
		"lastUpdatedAt":      latestUpdateStr,
		"accounts":           accountSummaries,
	}
}
