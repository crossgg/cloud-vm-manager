package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestVMCacheStore(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "vm_cache.json")

	store := &VMCacheStore{
		filePath: cachePath,
		data: VMCacheData{
			Accounts: make(map[string]AccountVMCache),
		},
	}

	mockVMs := []map[string]interface{}{
		{"status": "VM running"},
		{"status": "VM running"},
		{"status": "VM stopped"},
		{"status": "VM deallocated"},
	}

	store.UpdateAccount("azure", "test-acc", "dev-group", mockVMs)

	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("expected cache file to exist: %v", err)
	}

	// Reload into new store
	newStore := &VMCacheStore{
		filePath: cachePath,
		data: VMCacheData{
			Accounts: make(map[string]AccountVMCache),
		},
	}
	newStore.load()

	key := serviceKey("azure", "test-acc")
	acc, exists := newStore.data.Accounts[key]
	if !exists {
		t.Fatalf("expected account %s in cache", key)
	}
	if acc.Total != 4 || acc.Running != 2 || acc.Stopped != 2 {
		t.Errorf("expected 4 total, 2 running, 2 stopped; got %+v", acc)
	}

	// Test GetOverview
	activeAccounts := []gin.H{
		{"provider": "azure", "account": "test-acc", "group": "dev-group"},
		{"provider": "gcp", "account": "gcp-acc", "group": "prod"},
	}
	overview := newStore.GetOverview(activeAccounts)

	if overview["configCount"] != 2 {
		t.Errorf("expected configCount 2, got %v", overview["configCount"])
	}
	if overview["instanceCount"] != 4 {
		t.Errorf("expected instanceCount 4, got %v", overview["instanceCount"])
	}
	if overview["runningCount"] != 2 {
		t.Errorf("expected runningCount 2, got %v", overview["runningCount"])
	}
	if overview["stoppedCount"] != 2 {
		t.Errorf("expected stoppedCount 2, got %v", overview["stoppedCount"])
	}
	if overview["cachedAccountCount"] != 1 {
		t.Errorf("expected cachedAccountCount 1, got %v", overview["cachedAccountCount"])
	}
}
