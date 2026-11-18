package oci

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestOCIScanAndCleanupIdleResources(t *testing.T) {
	var deletedPaths []string

	svc, closeFn := newTestOCIService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case r.Method == http.MethodGet && p == "/20160918/availabilityDomains":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"name": "AD-1"}})
		case r.Method == http.MethodGet && p == "/20160918/instances":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "inst-1", "displayName": "vm-1", "lifecycleState": "RUNNING", "availabilityDomain": "AD-1"},
			})
		case r.Method == http.MethodGet && p == "/20160918/bootVolumeAttachments":
			if bvFilter := r.URL.Query().Get("bootVolumeId"); bvFilter != "" && bvFilter != "bv-used" {
				_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "bva-1", "bootVolumeId": "bv-used", "instanceId": "inst-1", "lifecycleState": "ATTACHED"},
			})
		case r.Method == http.MethodGet && p == "/20160918/bootVolumes":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "bv-used", "displayName": "boot-used", "sizeInGBs": 50, "lifecycleState": "AVAILABLE", "availabilityDomain": "AD-1"},
				{"id": "bv-idle", "displayName": "boot-idle", "sizeInGBs": 100, "lifecycleState": "AVAILABLE", "availabilityDomain": "AD-1"},
			})
		case r.Method == http.MethodGet && p == "/20160918/bootVolumes/bv-idle":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "bv-idle", "availabilityDomain": "AD-1", "compartmentId": "ocid1.compartment.oc1..test"})
		case r.Method == http.MethodGet && p == "/20160918/volumeAttachments":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
		case r.Method == http.MethodGet && p == "/20160918/volumes":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "vol-idle", "displayName": "data-idle", "sizeInGBs": 50, "lifecycleState": "AVAILABLE", "availabilityDomain": "AD-1"},
			})
		case r.Method == http.MethodGet && p == "/20160918/volumes/vol-idle":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "vol-idle", "availabilityDomain": "AD-1", "compartmentId": "ocid1.compartment.oc1..test"})
		case r.Method == http.MethodGet && p == "/20160918/publicIps":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "pub-used", "ipAddress": "1.1.1.1", "privateIpId": "priv-1", "lifecycleState": "ASSIGNED"},
				{"id": "pub-idle", "ipAddress": "2.2.2.2", "privateIpId": "", "displayName": "unused-res", "lifecycleState": "AVAILABLE"},
			})
		case r.Method == http.MethodGet && p == "/20160918/publicIps/pub-used":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "pub-used", "ipAddress": "1.1.1.1", "privateIpId": "priv-1"})
		case r.Method == http.MethodGet && p == "/20160918/publicIps/pub-idle":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "pub-idle", "ipAddress": "2.2.2.2", "privateIpId": ""})
		case r.Method == http.MethodGet && p == "/20160918/bootVolumeBackups":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "bvb-1", "displayName": "bv-snap", "uniqueSizeInGBs": 20, "lifecycleState": "AVAILABLE"},
			})
		case r.Method == http.MethodGet && p == "/20160918/volumeBackups":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
		case r.Method == http.MethodGet && p == "/20160918/images":
			// One official image (compartmentId empty), one custom image matching compartment
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "img-official", "displayName": "Oracle-Linux-9", "compartmentId": "", "lifecycleState": "AVAILABLE", "sizeInMBs": 4096},
				{"id": "img-custom", "displayName": "my-custom-img", "compartmentId": svcCompartmentFallback, "lifecycleState": "AVAILABLE", "sizeInMBs": 8192},
			})
		case r.Method == http.MethodDelete:
			deletedPaths = append(deletedPaths, p)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, p)
		}
	})
	defer closeFn()

	// Ensure compartment matches mock custom image
	svc.Account.CompartmentID = svcCompartmentFallback

	res, err := svc.ScanIdleResources()
	if err != nil {
		t.Fatalf("ScanIdleResources failed: %v", err)
	}
	if res.Summary.IdleVolumeCount != 2 || res.Summary.IdleVolumeGB != 150 || res.Summary.IdleReservedIPs != 1 || res.Summary.BackupCount != 1 || res.Summary.CustomImageCount != 1 {
		t.Fatalf("unexpected OCI resource summary: %+v", res.Summary)
	}

	// Refuse deleting assigned reserved IP
	deleted, errs := svc.CleanupIdleResources([]string{"reserved_ip|pub-used"})
	if len(deleted) != 0 || len(errs) != 1 || !strings.Contains(errs[0], "绑定使用中") {
		t.Fatalf("expected refusal on in-use reserved IP, deleted=%v errs=%v", deleted, errs)
	}

	// Delete idle resources
	deleted, errs = svc.CleanupIdleResources([]string{
		"boot_volume|bv-idle",
		"block_volume|vol-idle",
		"reserved_ip|pub-idle",
		"boot_backup|bvb-1",
		"custom_image|img-custom",
	})
	if len(errs) != 0 || len(deleted) != 5 || len(deletedPaths) != 5 {
		t.Fatalf("CleanupIdleResources failed: deleted=%v errs=%v paths=%v", deleted, errs, deletedPaths)
	}
}

const svcCompartmentFallback = "ocid1.compartment.oc1..test"
