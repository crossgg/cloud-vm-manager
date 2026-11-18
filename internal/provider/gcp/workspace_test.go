package gcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

func TestGCPWorkspaceCreateOptionsLaunchAndResourceCleanup(t *testing.T) {
	origBase := gcpComputeBaseURL
	origPoll := gcpPollInterval
	gcpPollInterval = 2 * time.Millisecond
	defer func() {
		gcpComputeBaseURL = origBase
		gcpPollInterval = origPoll
	}()

	var insertedBody map[string]interface{}
	var switchedCustom, patchedSubnet bool
	var deletedPaths []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case r.Method == http.MethodGet && p == "/projects/proj-1/zones":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]interface{}{{"name": "us-central1-a", "status": "UP"}},
			})
		case r.Method == http.MethodGet && p == "/projects/proj-1/zones/us-central1-a/machineTypes":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]interface{}{{"name": "e2-micro", "guestCpus": float64(2), "memoryMb": float64(1024)}},
			})
		case r.Method == http.MethodGet && p == "/projects/proj-1/global/networks":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]interface{}{{"name": "default", "autoCreateSubnetworks": !switchedCustom}},
			})
		case r.Method == http.MethodGet && p == "/projects/proj-1/global/networks/default":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":                  "default",
				"autoCreateSubnetworks": !switchedCustom,
			})
		case r.Method == http.MethodPost && p == "/projects/proj-1/global/networks/default/switchToCustomMode":
			switchedCustom = true
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"name": "op-sw", "status": "DONE"})
		case r.Method == http.MethodGet && p == "/projects/proj-1/regions/us-central1/subnetworks":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]interface{}{{"name": "default", "network": "default", "ipCidrRange": "10.128.0.0/20", "stackType": "IPV4_ONLY"}},
			})
		case r.Method == http.MethodGet && p == "/projects/proj-1/regions/us-central1/subnetworks/default":
			st := "IPV4_ONLY"
			at := ""
			if patchedSubnet {
				st = "IPV4_IPV6"
				at = "EXTERNAL"
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":           "default",
				"fingerprint":    "fp-1",
				"stackType":      st,
				"ipv6AccessType": at,
			})
		case r.Method == http.MethodPatch && p == "/projects/proj-1/regions/us-central1/subnetworks/default":
			patchedSubnet = true
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"name": "op-sub", "status": "DONE"})
		case r.Method == http.MethodPost && p == "/projects/proj-1/zones/us-central1-a/instances":
			_ = json.NewDecoder(r.Body).Decode(&insertedBody)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"name": "op-ins", "status": "DONE"})
		case r.Method == http.MethodGet && p == "/projects/proj-1/zones/us-central1-a/instances/vm-new":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":        "vm-new",
				"zone":        "zones/us-central1-a",
				"machineType": "zones/us-central1-a/machineTypes/e2-micro",
				"status":      "RUNNING",
			})
		case r.Method == http.MethodGet && p == "/projects/proj-1/aggregated/disks":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": map[string]interface{}{
					"zones/us-central1-a": map[string]interface{}{
						"disks": []map[string]interface{}{
							{"name": "orphan-disk", "zone": "zones/us-central1-a", "sizeGb": "20", "status": "READY"},
						},
					},
				},
			})
		case r.Method == http.MethodGet && p == "/projects/proj-1/global/snapshots":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]interface{}{{"name": "snap-1", "diskSizeGb": "10", "status": "READY"}},
			})
		case r.Method == http.MethodGet && p == "/projects/proj-1/aggregated/addresses":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": map[string]interface{}{
					"regions/us-central1": map[string]interface{}{
						"addresses": []map[string]interface{}{
							{"name": "idle-ip", "region": "regions/us-central1", "address": "34.1.2.3", "status": "RESERVED"},
						},
					},
				},
			})
		case r.Method == http.MethodDelete:
			deletedPaths = append(deletedPaths, p)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"name": "op-del", "status": "DONE"})
		default:
			t.Fatalf("unexpected GCP request: %s %s", r.Method, p)
		}
	}))
	defer srv.Close()
	gcpComputeBaseURL = srv.URL

	svc := NewGCPService(config.GCPConfig{Name: "gcp-1", ProjectID: "proj-1"})
	svc.token = "tok"
	svc.tokenExp = time.Now().Add(time.Hour)

	opts, err := svc.GetCreateOptions("")
	if err != nil {
		t.Fatalf("GetCreateOptions failed: %v", err)
	}
	if opts.SelectedZone != "us-central1-a" || len(opts.MachineTypes) == 0 || len(opts.Images) == 0 {
		t.Fatalf("unexpected GCP create options: %+v", opts)
	}

	vm, logs, err := svc.LaunchInstance(GCPLaunchInstanceRequest{
		Name:                "vm-new",
		Zone:                "us-central1-a",
		MachineType:         "e2-micro",
		SourceImage:         "projects/debian-cloud/global/images/family/debian-12",
		DiskType:            "pd-standard",
		DiskSizeGB:          10,
		Network:             "default",
		Subnetwork:          "default",
		AssignPublicIPv4:    true,
		AssignIPv6:          true,
		ConfirmIrreversible: true,
		Spot:                true,
		SSHUser:             "debian",
		SSHPublicKey:        "ssh-ed25519 AAAATEST",
	})
	if err != nil {
		t.Fatalf("LaunchInstance failed: %v logs=%v", err, logs)
	}
	if vm["name"] != "vm-new" || !switchedCustom || !patchedSubnet {
		t.Fatalf("unexpected launch state: vm=%+v switched=%v patched=%v", vm, switchedCustom, patchedSubnet)
	}
	if sched, _ := insertedBody["scheduling"].(map[string]interface{}); sched["provisioningModel"] != "SPOT" {
		t.Fatalf("expected SPOT scheduling: %+v", insertedBody)
	}

	scan, err := svc.ScanResources()
	if err != nil {
		t.Fatalf("ScanResources failed: %v", err)
	}
	if scan.Summary.OrphanDiskCount != 1 || scan.Summary.SnapshotCount != 1 || scan.Summary.UnusedStaticIPs != 1 {
		t.Fatalf("unexpected scan summary: %+v", scan.Summary)
	}

	deleted, errs := svc.CleanupResources([]string{
		"disk|us-central1-a|orphan-disk",
		"snapshot|global|snap-1",
		"address|us-central1|idle-ip",
	})
	if len(errs) != 0 || len(deleted) != 3 || len(deletedPaths) != 3 {
		t.Fatalf("CleanupResources failed: deleted=%v errs=%v paths=%v", deleted, errs, deletedPaths)
	}
}
