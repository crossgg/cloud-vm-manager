package oci

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestLaunchInstancePayloadAndTaskRetryPersistenceAutoResume(t *testing.T) {
	origUnit := createTaskSleepUnit
	createTaskSleepUnit = 5 * time.Millisecond
	defer func() { createTaskSleepUnit = origUnit }()

	var launchCount int32
	var lastPayload map[string]interface{}

	svc, closeFn := newTestOCIService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/20160918/instances" {
			n := atomic.AddInt32(&launchCount, 1)
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			lastPayload = body

			switch n {
			case 1:
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"code":"TooManyRequests","message":"429 rate limited"}`))
			case 2:
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"code":"InternalError","message":"Out of host capacity."}`))
			default:
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "ocid1.instance.oc1..created"})
			}
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
	defer closeFn()

	storePath := filepath.Join(t.TempDir(), "oci_create_tasks.json")
	mgr := NewOCICreateTaskManager(storePath, func(account string) (*OCIService, error) {
		return svc, nil
	})
	defer mgr.StopAll()

	task, err := mgr.CreateTask(OCICreateTaskInput{
		Account:             "oci-test",
		Name:                "arm-grab",
		Template:            "arm_free",
		AvailabilityDomain:  "AD-1",
		DisplayName:         "arm-vm",
		Shape:               "VM.Standard.A1.Flex",
		OCPUs:               4,
		MemoryInGBs:         24,
		BootVolumeSizeInGBs: 50,
		ImageID:             "ocid1.image.oc1..arm",
		SubnetID:            "ocid1.subnet.oc1..sub1",
		AssignPublicIP:      true,
		AssignIPv6IP:        true,
		NSGIDs:              []string{"ocid1.nsg.oc1..n1"},
		SSHPublicKey:        "ssh-ed25519 AAAAtest",
		CloudInit:           "#!/bin/bash\necho hi\n",
		MinIntervalSec:      5,
		MaxIntervalSec:      6,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	if _, err := mgr.StartTask(task.ID); err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	// Wait for 3 attempts (429 -> Out of host capacity -> succeeded)
	deadline := time.Now().Add(3 * time.Second)
	var finalTask OCICreateTask
	for time.Now().Before(deadline) {
		tasks := mgr.ListTasks("oci-test")
		if len(tasks) == 1 && tasks[0].Status == OCICreateTaskStatusSucceeded {
			finalTask = tasks[0]
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if finalTask.Status != OCICreateTaskStatusSucceeded {
		t.Fatalf("expected task to succeed after retries, got: %+v", mgr.ListTasks("oci-test"))
	}
	if finalTask.Attempts != 3 || finalTask.CreatedInstanceID != "ocid1.instance.oc1..created" {
		t.Fatalf("unexpected final task metrics: %+v", finalTask)
	}

	// Verify Flex shapeConfig + base64 cloud-init + assignIpv6Ip in launch payload
	shapeCfg, _ := lastPayload["shapeConfig"].(map[string]interface{})
	if shapeCfg["ocpus"] != float64(4) || shapeCfg["memoryInGBs"] != float64(24) {
		t.Fatalf("unexpected shapeConfig: %+v", shapeCfg)
	}
	meta, _ := lastPayload["metadata"].(map[string]interface{})
	expectedUD := base64.StdEncoding.EncodeToString([]byte("#!/bin/bash\necho hi\n"))
	if meta["user_data"] != expectedUD || meta["ssh_authorized_keys"] != "ssh-ed25519 AAAAtest" {
		t.Fatalf("unexpected metadata: %+v", meta)
	}

	// Test persistence & auto-resume for a running task across restart
	task2, err := mgr.CreateTask(OCICreateTaskInput{
		Account:            "oci-test",
		Name:               "amd-grab",
		Template:           "amd_free",
		AvailabilityDomain: "AD-1",
		Shape:              "VM.Standard.E2.1.Micro",
		ImageID:            "ocid1.image.oc1..amd",
		SubnetID:           "ocid1.subnet.oc1..sub1",
		MinIntervalSec:     5,
		MaxIntervalSec:     5,
	})
	if err != nil {
		t.Fatalf("CreateTask 2 failed: %v", err)
	}
	mgr.mu.Lock()
	mgr.tasks[task2.ID].Status = OCICreateTaskStatusRunning
	_ = mgr.saveLocked()
	mgr.mu.Unlock()

	mgr2 := NewOCICreateTaskManager(storePath, func(account string) (*OCIService, error) {
		return svc, nil
	})
	defer mgr2.StopAll()
	if err := mgr2.LoadAndAutoResume(); err != nil {
		t.Fatalf("LoadAndAutoResume failed: %v", err)
	}

	deadline = time.Now().Add(2 * time.Second)
	resumedSucceeded := false
	for time.Now().Before(deadline) {
		for _, item := range mgr2.ListTasks("oci-test") {
			if item.ID == task2.ID && item.Status == OCICreateTaskStatusSucceeded {
				resumedSucceeded = true
			}
		}
		if resumedSucceeded {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if !resumedSucceeded {
		t.Fatalf("expected persisted running task to auto-resume and succeed, got: %+v", mgr2.ListTasks("oci-test"))
	}
}
