package oci

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBootVolumeLifecycleResizeVPUDetachAttachDelete(t *testing.T) {
	origPoll := ociIPPollInterval
	ociIPPollInterval = 5 * time.Millisecond
	defer func() { ociIPPollInterval = origPoll }()

	var bvSize int64 = 98
	var bvVPU int64 = 10
	instState := "RUNNING"
	attState := "ATTACHED"
	var deletedBV bool

	svc, closeFn := newTestOCIService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/availabilityDomains":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"name": "AD-1"}})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/instances":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{
				"id":                 "inst-1",
				"displayName":        "us-arm-1",
				"lifecycleState":     instState,
				"availabilityDomain": "AD-1",
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/instances/inst-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":                 "inst-1",
				"displayName":        "us-arm-1",
				"lifecycleState":     instState,
				"availabilityDomain": "AD-1",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/bootVolumes":
			if deletedBV {
				_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{
				"id":                 "bv-1",
				"displayName":        "us-arm-1 (Boot Volume)",
				"sizeInGBs":          bvSize,
				"vpusPerGB":          bvVPU,
				"lifecycleState":     "AVAILABLE",
				"availabilityDomain": "AD-1",
				"compartmentId":      "ocid1.compartment.oc1..comp",
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/bootVolumes/bv-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":                 "bv-1",
				"displayName":        "us-arm-1 (Boot Volume)",
				"sizeInGBs":          bvSize,
				"vpusPerGB":          bvVPU,
				"lifecycleState":     "AVAILABLE",
				"availabilityDomain": "AD-1",
				"compartmentId":      "ocid1.compartment.oc1..comp",
			})
		case r.Method == http.MethodPut && r.URL.Path == "/20160918/bootVolumes/bv-1":
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if v, ok := body["sizeInGBs"].(float64); ok && v > 0 {
				bvSize = int64(v)
			}
			if v, ok := body["vpusPerGB"].(float64); ok && v > 0 {
				bvVPU = int64(v)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":                 "bv-1",
				"displayName":        "us-arm-1 (Boot Volume)",
				"sizeInGBs":          bvSize,
				"vpusPerGB":          bvVPU,
				"lifecycleState":     "AVAILABLE",
				"availabilityDomain": "AD-1",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/bootVolumeAttachments":
			if attState == "NONE" {
				_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{
				"id":             "bva-1",
				"bootVolumeId":   "bv-1",
				"instanceId":     "inst-1",
				"lifecycleState": attState,
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/bootVolumeAttachments/bva-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             "bva-1",
				"bootVolumeId":   "bv-1",
				"instanceId":     "inst-1",
				"lifecycleState": attState,
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/20160918/bootVolumeAttachments/bva-1":
			attState = "DETACHED"
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/20160918/bootVolumeAttachments":
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["bootVolumeId"] != "bv-1" || body["instanceId"] != "inst-1" {
				t.Fatalf("unexpected attach payload: %+v", body)
			}
			attState = "ATTACHED"
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             "bva-1",
				"bootVolumeId":   "bv-1",
				"instanceId":     "inst-1",
				"lifecycleState": attState,
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/20160918/bootVolumes/bv-1":
			deletedBV = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	defer closeFn()

	// 1. Instance boot volume view
	instBV, err := svc.GetInstanceBootVolume("inst-1")
	if err != nil {
		t.Fatalf("GetInstanceBootVolume failed: %v", err)
	}
	if instBV.BootVolume == nil || instBV.BootVolume.SizeInGBs != 98 || !instBV.BootVolume.Attached {
		t.Fatalf("unexpected GetInstanceBootVolume result: %+v", instBV)
	}

	// 2. Reject shrink & invalid VPU; allow resize + VPU update
	var badSize int64 = 50
	if _, err := svc.UpdateBootVolume("bv-1", &badSize, nil); err == nil {
		t.Fatalf("expected error shrinking boot volume")
	}
	var badVPU int64 = 15
	if _, err := svc.UpdateBootVolume("bv-1", nil, &badVPU); err == nil {
		t.Fatalf("expected error for invalid VPU=15")
	}
	var newSize int64 = 120
	var newVPU int64 = 20
	updated, err := svc.UpdateBootVolume("bv-1", &newSize, &newVPU)
	if err != nil || updated.SizeInGBs != 120 || updated.VpusPerGB != 20 {
		t.Fatalf("UpdateBootVolume failed: %+v err=%v", updated, err)
	}

	// 3. Reject detach while RUNNING, reject delete while attached
	if err := svc.DetachBootVolume("bva-1"); err == nil || !strings.Contains(err.Error(), "STOPPED") {
		t.Fatalf("expected RUNNING detach rejection, got: %v", err)
	}
	if err := svc.DeleteBootVolume("bv-1"); err == nil || !strings.Contains(err.Error(), "卸载") {
		t.Fatalf("expected attached delete rejection, got: %v", err)
	}

	// 4. Stop instance -> detach -> reattach -> detach -> delete
	instState = "STOPPED"
	if err := svc.DetachBootVolume("bva-1"); err != nil {
		t.Fatalf("DetachBootVolume while STOPPED failed: %v", err)
	}
	if _, err := svc.AttachBootVolume("bv-1", "inst-1"); err != nil {
		t.Fatalf("AttachBootVolume failed: %v", err)
	}
	if err := svc.DetachBootVolume("bva-1"); err != nil {
		t.Fatalf("second DetachBootVolume failed: %v", err)
	}
	if err := svc.DeleteBootVolume("bv-1"); err != nil {
		t.Fatalf("DeleteBootVolume failed: %v", err)
	}
	if !deletedBV {
		t.Fatalf("expected DeleteBootVolume to execute DELETE")
	}
}
