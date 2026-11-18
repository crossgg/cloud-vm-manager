package azure

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAzureCreateDefaultNetworkAndLaunchInstance(t *testing.T) {
	origPoll := azurePollInterval
	azurePollInterval = 2 * time.Millisecond
	defer func() { azurePollInterval = origPoll }()

	var createdVMBody map[string]interface{}
	var deletedResources []string

	svc := newMockAzureService(func(req *http.Request) (*http.Response, error) {
		p := req.URL.Path
		switch {
		case req.Method == http.MethodGet && p == "/subscriptions/sub-1/resourcegroups":
			return jsonResponse(http.StatusOK, map[string]interface{}{
				"value": []map[string]interface{}{{"name": "rg-main"}},
			}), nil
		case req.Method == http.MethodGet && strings.HasSuffix(p, "/resourcegroups/rg-main"):
			return jsonResponse(http.StatusOK, map[string]interface{}{"name": "rg-main"}), nil
		case req.Method == http.MethodGet && strings.Contains(p, "/vmSizes"):
			return jsonResponse(http.StatusOK, map[string]interface{}{
				"value": []map[string]interface{}{
					{"name": "Standard_B1s", "numberOfCores": 1, "memoryInMB": 1024},
				},
			}), nil
		case req.Method == http.MethodGet && p == "/subscriptions/sub-1/providers/Microsoft.Network/virtualNetworks":
			return jsonResponse(http.StatusOK, map[string]interface{}{"value": []interface{}{}}), nil
		case req.Method == http.MethodGet && p == "/subscriptions/sub-1/providers/Microsoft.Network/networkSecurityGroups":
			return jsonResponse(http.StatusOK, map[string]interface{}{"value": []interface{}{}}), nil
		case req.Method == http.MethodPut && strings.Contains(p, "/networkSecurityGroups/"):
			return jsonResponse(http.StatusCreated, map[string]interface{}{"id": p}), nil
		case req.Method == http.MethodPut && strings.Contains(p, "/virtualNetworks/"):
			var body map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&body)
			return jsonResponse(http.StatusCreated, body), nil
		case req.Method == http.MethodPut && strings.Contains(p, "/publicIPAddresses/"):
			return jsonResponse(http.StatusCreated, map[string]interface{}{
				"id":         p,
				"name":       lastPathSegment(p),
				"properties": map[string]interface{}{"ipAddress": "20.1.2.3"},
			}), nil
		case req.Method == http.MethodPut && strings.Contains(p, "/networkInterfaces/"):
			return jsonResponse(http.StatusCreated, map[string]interface{}{"id": p}), nil
		case req.Method == http.MethodPut && strings.Contains(p, "/virtualMachines/vm-new"):
			_ = json.NewDecoder(req.Body).Decode(&createdVMBody)
			return jsonResponse(http.StatusCreated, map[string]interface{}{"id": p, "name": "vm-new"}), nil
		case req.Method == http.MethodPut && strings.Contains(p, "/virtualMachines/vm-fail"):
			return jsonResponse(http.StatusBadRequest, map[string]interface{}{"error": "quota exceeded"}), nil
		case req.Method == http.MethodGet && strings.HasSuffix(p, "/virtualMachines/vm-new"):
			return jsonResponse(http.StatusOK, map[string]interface{}{
				"name":     "vm-new",
				"location": "japaneast",
				"properties": map[string]interface{}{
					"hardwareProfile": map[string]interface{}{"vmSize": "Standard_B1s"},
				},
			}), nil
		case req.Method == http.MethodDelete:
			deletedResources = append(deletedResources, p)
			return jsonResponse(http.StatusNoContent, nil), nil
		default:
			t.Fatalf("unexpected Azure request: %s %s", req.Method, p)
			return nil, nil
		}
	})

	opts, err := svc.GetCreateOptions("japaneast")
	if err != nil {
		t.Fatalf("GetCreateOptions failed: %v", err)
	}
	if len(opts.VMSizes) == 0 || len(opts.Images) == 0 {
		t.Fatalf("expected vmSizes and images in create options")
	}

	vmRes, logs, err := svc.LaunchInstance(AzureLaunchInstanceRequest{
		Name:           "vm-new",
		ResourceGroup:  "rg-main",
		Location:       "japaneast",
		VMSize:         "Standard_B1s",
		ImageID:        "ubuntu-2404",
		AssignPublicV4: true,
		AssignIPv6:     true,
		Spot:           true,
		AdminUsername:  "azureuser",
		AuthType:       "ssh",
		SSHPublicKey:   "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItest",
	})
	if err != nil {
		t.Fatalf("LaunchInstance failed: %v logs=%v", err, logs)
	}
	if vmRes["name"] != "vm-new" {
		t.Fatalf("unexpected created vm: %+v", vmRes)
	}
	props, _ := createdVMBody["properties"].(map[string]interface{})
	if props["priority"] != "Spot" {
		t.Fatalf("expected Spot priority in VM body: %+v", props)
	}

	// Verify rollback deletes created NIC/PublicIPs when VM creation fails
	deletedResources = nil
	_, _, err = svc.LaunchInstance(AzureLaunchInstanceRequest{
		Name:           "vm-fail",
		ResourceGroup:  "rg-main",
		Location:       "japaneast",
		SubnetID:       "/subscriptions/sub-1/resourceGroups/rg-main/providers/Microsoft.Network/virtualNetworks/v/subnets/default",
		AssignPublicV4: true,
		AssignIPv6:     false,
		AdminUsername:  "azureuser",
		AuthType:       "password",
		AdminPassword:  "P@ssword123456!",
	})
	if err == nil || len(deletedResources) < 2 {
		t.Fatalf("expected rollback of NIC and PublicIP on VM failure, err=%v deleted=%v", err, deletedResources)
	}
}
