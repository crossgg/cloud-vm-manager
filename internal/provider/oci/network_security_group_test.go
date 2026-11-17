package oci

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAppendUniqueString(t *testing.T) {
	values := appendUniqueString([]string{"nsg-1"}, "nsg-1")
	if len(values) != 1 {
		t.Fatalf("expected duplicate value to be ignored: %+v", values)
	}

	values = appendUniqueString(values, "nsg-2")
	if len(values) != 2 || values[1] != "nsg-2" {
		t.Fatalf("expected new value to be appended: %+v", values)
	}
}

func TestChunkStrings(t *testing.T) {
	chunks := chunkStrings([]string{"a", "b", "c"}, 2)
	if len(chunks) != 2 || len(chunks[0]) != 2 || len(chunks[1]) != 1 {
		t.Fatalf("unexpected chunks: %+v", chunks)
	}
}

func TestNSGAttachDetachRenameAndDelete(t *testing.T) {
	vnicNSGs := []string{}
	nsgNames := map[string]string{
		"nsg-1": "existing-nsg",
		"nsg-2": "another-nsg",
	}
	deletedNSG := ""

	svc, closeFn := newTestOCIService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		switch {
		case r.Method == http.MethodGet && path == "/20160918/vnicAttachments":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"vnicId": "vnic-1", "lifecycleState": "ATTACHED"},
			})
		case r.Method == http.MethodGet && path == "/20160918/vnics/vnic-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":       "vnic-1",
				"subnetId": "subnet-1",
				"vcnId":    "vcn-1",
				"nsgIds":   vnicNSGs,
			})
		case r.Method == http.MethodPut && path == "/20160918/vnics/vnic-1":
			var body struct {
				NSGIDs []string `json:"nsgIds"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			vnicNSGs = body.NSGIDs
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "vnic-1", "nsgIds": vnicNSGs})
		case r.Method == http.MethodGet && path == "/20160918/privateIps":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "priv-1", "ipAddress": "10.0.0.2", "isPrimary": true},
			})
		case r.Method == http.MethodGet && path == "/20160918/networkSecurityGroups":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "nsg-1", "displayName": nsgNames["nsg-1"], "vcnId": "vcn-1", "lifecycleState": "AVAILABLE"},
				{"id": "nsg-2", "displayName": nsgNames["nsg-2"], "vcnId": "vcn-1", "lifecycleState": "AVAILABLE"},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/20160918/networkSecurityGroups/") && strings.HasSuffix(path, "/securityRules"):
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
		case r.Method == http.MethodGet && path == "/20160918/networkSecurityGroups/nsg-1/vnics":
			if len(vnicNSGs) > 0 && vnicNSGs[0] == "nsg-1" {
				_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"vnicId": "vnic-1"}})
			} else {
				_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
			}
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/20160918/networkSecurityGroups/"):
			id := strings.TrimPrefix(path, "/20160918/networkSecurityGroups/")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "displayName": nsgNames[id], "vcnId": "vcn-1"})
		case r.Method == http.MethodPut && path == "/20160918/networkSecurityGroups/nsg-1":
			var body struct {
				DisplayName string `json:"displayName"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			nsgNames["nsg-1"] = body.DisplayName
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "nsg-1", "displayName": body.DisplayName, "vcnId": "vcn-1"})
		case r.Method == http.MethodDelete && path == "/20160918/networkSecurityGroups/nsg-1":
			deletedNSG = "nsg-1"
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, path)
		}
	})
	defer closeFn()

	// Initially unattached -> available should show both nsg-1 and nsg-2
	attached, available, vcnID, err := svc.ListInstanceNetworkSecurityGroups("inst-1")
	if err != nil {
		t.Fatalf("ListInstanceNetworkSecurityGroups failed: %v", err)
	}
	if vcnID != "vcn-1" || len(attached) != 0 || len(available) != 2 {
		t.Fatalf("unexpected initial NSG state: vcn=%s attached=%d available=%d", vcnID, len(attached), len(available))
	}

	// Attach existing NSG
	if err := svc.AttachNetworkSecurityGroup("inst-1", "nsg-1"); err != nil {
		t.Fatalf("AttachNetworkSecurityGroup failed: %v", err)
	}
	if len(vnicNSGs) != 1 || vnicNSGs[0] != "nsg-1" {
		t.Fatalf("expected nsg-1 attached, got %+v", vnicNSGs)
	}

	// Deleting while attached should fail with unbind prompt
	if err := svc.DeleteNetworkSecurityGroup("nsg-1"); err == nil || !strings.Contains(err.Error(), "解绑") {
		t.Fatalf("expected unbind prompt error when deleting bound NSG, got %v", err)
	}

	// Rename NSG
	renamed, err := svc.RenameNetworkSecurityGroup("nsg-1", "renamed-nsg")
	if err != nil || renamed.Name != "renamed-nsg" {
		t.Fatalf("RenameNetworkSecurityGroup failed: renamed=%+v err=%v", renamed, err)
	}

	// Detach then delete
	if err := svc.DetachNetworkSecurityGroup("inst-1", "nsg-1"); err != nil {
		t.Fatalf("DetachNetworkSecurityGroup failed: %v", err)
	}
	if err := svc.DeleteNetworkSecurityGroup("nsg-1"); err != nil {
		t.Fatalf("DeleteNetworkSecurityGroup failed after detach: %v", err)
	}
	if deletedNSG != "nsg-1" {
		t.Fatalf("expected DELETE /networkSecurityGroups/nsg-1 to be called")
	}
}
