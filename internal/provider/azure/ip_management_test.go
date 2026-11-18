package azure

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/crossgg/cloud-vm-manager/internal/provider"
)

func newMockAzureService(handler func(*http.Request) (*http.Response, error)) *AzureService {
	svc := NewAzureAccountService(&config.Config{}, config.AzureConfig{
		Name:          "az-test",
		SubId:         "sub-1",
		ResourceGroup: "rg-main",
		Location:      "japaneast",
	})
	svc.token = "tok"
	svc.tokenExp = time.Now().Add(time.Hour)
	svc.Client = &http.Client{Transport: roundTripFunc(handler)}
	return svc
}

func TestAzureIPManagementEnableDualStackAndSwapIPv6(t *testing.T) {
	origPoll := azurePollInterval
	azurePollInterval = 2 * time.Millisecond
	defer func() { azurePollInterval = origPoll }()

	vnet := map[string]interface{}{
		"id":       "/subscriptions/sub-1/resourceGroups/rg-net/providers/Microsoft.Network/virtualNetworks/vnet-1",
		"name":     "vnet-1",
		"location": "japaneast",
		"properties": map[string]interface{}{
			"addressSpace": map[string]interface{}{
				"addressPrefixes": []interface{}{"10.0.0.0/16"},
			},
			"subnets": []interface{}{
				map[string]interface{}{
					"id":   "/subscriptions/sub-1/resourceGroups/rg-net/providers/Microsoft.Network/virtualNetworks/vnet-1/subnets/default",
					"name": "default",
					"properties": map[string]interface{}{
						"addressPrefix": "10.0.0.0/24",
					},
				},
			},
		},
	}

	subnet := map[string]interface{}{
		"id":   "/subscriptions/sub-1/resourceGroups/rg-net/providers/Microsoft.Network/virtualNetworks/vnet-1/subnets/default",
		"name": "default",
		"properties": map[string]interface{}{
			"addressPrefix": "10.0.0.0/24",
		},
	}

	nic := map[string]interface{}{
		"id":       "/subscriptions/sub-1/resourceGroups/rg-net/providers/Microsoft.Network/networkInterfaces/nic-1",
		"name":     "nic-1",
		"location": "japaneast",
		"properties": map[string]interface{}{
			"macAddress": "00-0D-3A-01-02-03",
			"ipConfigurations": []interface{}{
				map[string]interface{}{
					"id":   "/subscriptions/sub-1/resourceGroups/rg-net/providers/Microsoft.Network/networkInterfaces/nic-1/ipConfigurations/ipconfig1",
					"name": "ipconfig1",
					"properties": map[string]interface{}{
						"primary":                   true,
						"privateIPAddress":          "10.0.0.4",
						"privateIPAddressVersion":   "IPv4",
						"privateIPAllocationMethod": "Dynamic",
						"subnet": map[string]interface{}{
							"id": subnet["id"],
						},
						"publicIPAddress": map[string]interface{}{
							"id": "/subscriptions/sub-1/resourceGroups/rg-net/providers/Microsoft.Network/publicIPAddresses/pip-v4-old",
						},
					},
				},
			},
		},
	}

	publicIPs := map[string]map[string]interface{}{
		"/subscriptions/sub-1/resourceGroups/rg-net/providers/Microsoft.Network/publicIPAddresses/pip-v4-old": {
			"id":       "/subscriptions/sub-1/resourceGroups/rg-net/providers/Microsoft.Network/publicIPAddresses/pip-v4-old",
			"name":     "pip-v4-old",
			"location": "japaneast",
			"zones":    []interface{}{"1"},
			"sku":      map[string]interface{}{"name": "Standard"},
			"properties": map[string]interface{}{
				"ipAddress":                "20.1.1.10",
				"publicIPAllocationMethod": "Static",
				"publicIPAddressVersion":   "IPv4",
				"ipConfiguration":          map[string]interface{}{"id": "ipconfig1"},
			},
		},
		"/subscriptions/sub-1/resourceGroups/rg-net/providers/Microsoft.Network/publicIPAddresses/pip-unused": {
			"id":       "/subscriptions/sub-1/resourceGroups/rg-net/providers/Microsoft.Network/publicIPAddresses/pip-unused",
			"name":     "pip-unused",
			"location": "japaneast",
			"sku":      map[string]interface{}{"name": "Standard"},
			"properties": map[string]interface{}{
				"ipAddress":                "20.1.1.99",
				"publicIPAllocationMethod": "Static",
				"publicIPAddressVersion":   "IPv4",
			},
		},
	}

	v6Seq := 0
	var deletedOldV4, deletedOldV6 bool

	svc := newMockAzureService(func(req *http.Request) (*http.Response, error) {
		path := req.URL.Path
		switch {
		case req.Method == http.MethodGet && strings.HasSuffix(path, "/virtualMachines/vm-1"):
			return jsonResponse(http.StatusOK, map[string]interface{}{
				"id":       "/subscriptions/sub-1/resourceGroups/rg-main/providers/Microsoft.Compute/virtualMachines/vm-1",
				"name":     "vm-1",
				"location": "japaneast",
				"zones":    []interface{}{"1"},
				"properties": map[string]interface{}{
					"hardwareProfile": map[string]interface{}{"vmSize": "Standard_B1s"},
					"networkProfile": map[string]interface{}{
						"networkInterfaces": []interface{}{
							map[string]interface{}{"id": nic["id"]},
						},
					},
					"instanceView": map[string]interface{}{
						"statuses": []interface{}{map[string]interface{}{"code": "PowerState/running"}},
					},
				},
			}), nil

		case req.Method == http.MethodGet && path == nic["id"]:
			return jsonResponse(http.StatusOK, nic), nil
		case req.Method == http.MethodPut && path == nic["id"]:
			var updated map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&updated)
			nic = updated
			resp := jsonResponse(http.StatusAccepted, map[string]interface{}{})
			resp.Header.Set("Azure-AsyncOperation", "https://management.azure.com/ops/nic-1")
			return resp, nil
		case req.Method == http.MethodGet && path == "/ops/nic-1":
			return jsonResponse(http.StatusOK, map[string]interface{}{"status": "Succeeded"}), nil

		case req.Method == http.MethodGet && path == vnet["id"]:
			return jsonResponse(http.StatusOK, vnet), nil
		case req.Method == http.MethodPut && path == vnet["id"]:
			var updated map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&updated)
			vnet = updated
			return jsonResponse(http.StatusOK, vnet), nil

		case req.Method == http.MethodGet && path == subnet["id"]:
			return jsonResponse(http.StatusOK, subnet), nil
		case req.Method == http.MethodPut && path == subnet["id"]:
			var updated map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&updated)
			subnet = updated
			return jsonResponse(http.StatusOK, subnet), nil

		case req.Method == http.MethodGet && path == "/subscriptions/sub-1/providers/Microsoft.Network/publicIPAddresses":
			vals := make([]map[string]interface{}, 0, len(publicIPs))
			for _, v := range publicIPs {
				vals = append(vals, v)
			}
			return jsonResponse(http.StatusOK, map[string]interface{}{"value": vals}), nil

		case req.Method == http.MethodGet && strings.Contains(path, "/publicIPAddresses/"):
			if p, ok := publicIPs[path]; ok {
				return jsonResponse(http.StatusOK, p), nil
			}
			return jsonResponse(http.StatusNotFound, map[string]interface{}{"error": "not found"}), nil

		case req.Method == http.MethodPut && strings.Contains(path, "/publicIPAddresses/"):
			var body map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&body)
			props, _ := body["properties"].(map[string]interface{})
			ver := stringValFromMap(props, "publicIPAddressVersion")
			name := lastPathSegment(path)
			addr := "20.2.2.20"
			if ver == "IPv6" {
				v6Seq++
				if v6Seq == 1 {
					addr = "2603:1040:100::1"
				} else {
					addr = "2603:1040:100::2"
				}
			}
			created := map[string]interface{}{
				"id":       path,
				"name":     name,
				"location": body["location"],
				"zones":    body["zones"],
				"sku":      body["sku"],
				"properties": map[string]interface{}{
					"ipAddress":                addr,
					"publicIPAllocationMethod": "Static",
					"publicIPAddressVersion":   ver,
				},
			}
			publicIPs[path] = created
			return jsonResponse(http.StatusCreated, created), nil

		case req.Method == http.MethodDelete && strings.Contains(path, "/publicIPAddresses/"):
			if strings.HasSuffix(path, "pip-v4-old") {
				deletedOldV4 = true
			}
			if strings.Contains(path, "ip-v6-") {
				deletedOldV6 = true
			}
			delete(publicIPs, path)
			return jsonResponse(http.StatusNoContent, nil), nil

		default:
			t.Fatalf("unexpected Azure request: %s %s", req.Method, path)
			return nil, nil
		}
	})

	info, err := svc.GetIPManagement("vm-1")
	if err != nil {
		t.Fatalf("GetIPManagement failed: %v", err)
	}
	if info.Provider != "azure" || len(info.VNICs) != 1 || len(info.ReservedIPs) != 1 {
		t.Fatalf("unexpected Azure IPManagementInfo: %+v", info)
	}
	if info.VNICs[0].IPv6Setup == nil || !info.VNICs[0].IPv6Setup.CanEnable || info.VNICs[0].IPv6Setup.SubnetHasIPv6 {
		t.Fatalf("unexpected initial IPv6Setup: %+v", info.VNICs[0].IPv6Setup)
	}

	// Enable subnet IPv6
	subCIDR, _, err := svc.EnableSubnetIPv6("vm-1", provider.EnableIPv6Request{VNICID: nic["id"].(string)})
	if err != nil {
		t.Fatalf("EnableSubnetIPv6 failed: %v", err)
	}
	if !strings.HasSuffix(subCIDR, "/64") || !strings.Contains(subCIDR, ":") {
		t.Fatalf("expected /64 IPv6 subnet CIDR, got %s", subCIDR)
	}

	// Attach IPv6
	v6Info, err := svc.AttachIPv6("vm-1", provider.AttachIPv6Request{VNICID: nic["id"].(string)})
	if err != nil {
		t.Fatalf("AttachIPv6 failed: %v", err)
	}
	if v6Info.IPAddress != "2603:1040:100::1" || !v6Info.CanChange {
		t.Fatalf("unexpected attached IPv6: %+v", v6Info)
	}

	// GetVM should expose IPv6s & IPv6
	vmInfo, err := svc.GetVM("vm-1")
	if err != nil {
		t.Fatalf("GetVM failed: %v", err)
	}
	if vmInfo["ipv6"] != "2603:1040:100::1" {
		t.Fatalf("expected GetVM ipv6=2603:1040:100::1, got %v", vmInfo["ipv6"])
	}

	// Change IPv6
	swapV6, err := svc.ChangeIPv6("vm-1", nic["id"].(string), v6Info.ID)
	if err != nil {
		t.Fatalf("ChangeIPv6 failed: %v", err)
	}
	if swapV6.OldIPAddress != "2603:1040:100::1" || swapV6.NewIPAddress != "2603:1040:100::2" || !deletedOldV6 {
		t.Fatalf("unexpected ChangeIPv6 result: %+v deletedOldV6=%v", swapV6, deletedOldV6)
	}

	// Change IPv4 (uses LRO & deletes old IP)
	swapV4, err := svc.ChangePrivateIPPublicIP("vm-1", "")
	if err != nil {
		t.Fatalf("ChangePrivateIPPublicIP failed: %v", err)
	}
	if swapV4.NewIPAddress != "20.2.2.20" || !deletedOldV4 {
		t.Fatalf("unexpected ChangePrivateIPPublicIP result: %+v deletedOldV4=%v", swapV4, deletedOldV4)
	}

	// Delete IPv6
	oldRemoved, err := svc.DeleteIPv6("vm-1", v6Info.ID)
	if err != nil || oldRemoved != "2603:1040:100::2" {
		t.Fatalf("DeleteIPv6 failed: old=%q err=%v", oldRemoved, err)
	}
}

func TestAzureBasicSKUBlocksIPv6Enable(t *testing.T) {
	subnetID := "/subscriptions/sub-1/resourceGroups/rg-main/providers/Microsoft.Network/virtualNetworks/vnet-1/subnets/default"
	nic := map[string]interface{}{
		"id":   "/subscriptions/sub-1/resourceGroups/rg-main/providers/Microsoft.Network/networkInterfaces/nic-1",
		"name": "nic-1",
		"properties": map[string]interface{}{
			"ipConfigurations": []interface{}{
				map[string]interface{}{
					"id":   "cfg-1",
					"name": "ipconfig1",
					"properties": map[string]interface{}{
						"primary":                 true,
						"privateIPAddress":        "10.0.0.4",
						"privateIPAddressVersion": "IPv4",
						"subnet":                  map[string]interface{}{"id": subnetID},
						"publicIPAddress": map[string]interface{}{
							"id": "/subscriptions/sub-1/resourceGroups/rg-main/providers/Microsoft.Network/publicIPAddresses/pip-basic",
						},
					},
				},
			},
		},
	}

	svc := newMockAzureService(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/subscriptions/sub-1/resourceGroups/rg-main/providers/Microsoft.Compute/virtualMachines/vm-basic":
			return jsonResponse(http.StatusOK, map[string]interface{}{
				"id":       "/subscriptions/sub-1/resourceGroups/rg-main/providers/Microsoft.Compute/virtualMachines/vm-basic",
				"name":     "vm-basic",
				"location": "japaneast",
				"properties": map[string]interface{}{
					"networkProfile": map[string]interface{}{
						"networkInterfaces": []interface{}{map[string]interface{}{"id": nic["id"]}},
					},
				},
			}), nil
		case nic["id"]:
			return jsonResponse(http.StatusOK, nic), nil
		case "/subscriptions/sub-1/resourceGroups/rg-main/providers/Microsoft.Network/publicIPAddresses/pip-basic":
			return jsonResponse(http.StatusOK, map[string]interface{}{
				"id":   "/subscriptions/sub-1/resourceGroups/rg-main/providers/Microsoft.Network/publicIPAddresses/pip-basic",
				"name": "pip-basic",
				"sku":  map[string]interface{}{"name": "Basic"},
				"properties": map[string]interface{}{
					"ipAddress":                "20.1.1.1",
					"publicIPAllocationMethod": "Dynamic",
					"publicIPAddressVersion":   "IPv4",
				},
			}), nil
		default:
			return jsonResponse(http.StatusOK, map[string]interface{}{"properties": map[string]interface{}{}}), nil
		}
	})

	_, _, err := svc.EnableSubnetIPv6("vm-basic", provider.EnableIPv6Request{})
	if err == nil || !strings.Contains(err.Error(), "Basic SKU") {
		t.Fatalf("expected Basic SKU blocking error, got: %v", err)
	}
}
