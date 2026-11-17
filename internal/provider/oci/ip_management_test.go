package oci

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestChangeIPRejectsWhenNoPublicIP(t *testing.T) {
	svc, closeFn := newTestOCIService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/instances/inst-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "inst-1", "displayName": "vm-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/vnicAttachments":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"vnicId": "vnic-1", "lifecycleState": "ATTACHED"}})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/vnics/vnic-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "vnic-1", "isPrimary": true, "subnetId": "sub-1", "displayName": "vnic-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/subnets/sub-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "sub-1", "vcnId": "vcn-1", "cidrBlock": "10.0.0.0/24"})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/privateIps":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"id": "priv-1", "ipAddress": "10.0.0.2", "isPrimary": true}})
		case r.Method == http.MethodPost && r.URL.Path == "/20160918/publicIps/actions/getByPrivateIpId":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/ipv6":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	defer closeFn()

	_, err := svc.ChangeIP("inst-1")
	if err == nil || !strings.Contains(err.Error(), "未绑定公网 IP") {
		t.Fatalf("expected error rejecting ChangeIP with no public IP, got: %v", err)
	}
}

func TestIPv6AttachAndDeleteBeforeCreateSwap(t *testing.T) {
	origPoll := ociIPPollInterval
	ociIPPollInterval = 5 * time.Millisecond
	defer func() { ociIPPollInterval = origPoll }()

	var sequence []string
	ipv6List := []map[string]interface{}{
		{"id": "v6-128", "ipAddress": "2603:c020:1:2::1111", "cidrPrefixLength": 128, "subnetId": "sub-1", "vnicId": "vnic-1"},
		{"id": "v6-80", "ipAddress": "2603:c020:1:2:1000::", "cidrPrefixLength": 80, "subnetId": "sub-1", "vnicId": "vnic-1"},
	}

	svc, closeFn := newTestOCIService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/instances/inst-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "inst-1", "displayName": "vm-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/vnicAttachments":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"vnicId": "vnic-1", "lifecycleState": "ATTACHED"}})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/vnics/vnic-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "vnic-1", "isPrimary": true, "subnetId": "sub-1", "displayName": "vnic-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/subnets/sub-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             "sub-1",
				"vcnId":          "vcn-1",
				"cidrBlock":      "10.0.0.0/24",
				"ipv6CidrBlocks": []string{"2603:c020:1:2::/64"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/privateIps":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"id": "priv-1", "ipAddress": "10.0.0.2", "isPrimary": true}})
		case r.Method == http.MethodPost && r.URL.Path == "/20160918/publicIps/actions/getByPrivateIpId":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/publicIps":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/ipv6":
			_ = json.NewEncoder(w).Encode(ipv6List)
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/ipv6/v6-128":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodDelete && r.URL.Path == "/20160918/ipv6/v6-128":
			sequence = append(sequence, "DELETE:v6-128")
			ipv6List = ipv6List[1:]
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/20160918/ipv6":
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			sequence = append(sequence, "POST:/ipv6")
			newAddr := "2603:c020:1:2::2222"
			if v, ok := body["ipAddress"].(string); ok && v != "" {
				newAddr = v
			}
			created := map[string]interface{}{
				"id":               "v6-new",
				"ipAddress":        newAddr,
				"cidrPrefixLength": body["cidrPrefixLength"],
				"subnetId":         "sub-1",
				"vnicId":           "vnic-1",
			}
			ipv6List = append(ipv6List, created)
			_ = json.NewEncoder(w).Encode(created)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	defer closeFn()

	// Reject prefix length outside 80-128 or not divisible by 4
	if _, err := svc.AttachIPv6("inst-1", OCIAttachIPv6Request{
		VNICID:           "vnic-1",
		IPv6SubnetCIDR:   "2603:c020:1:2::/64",
		CIDRPrefixLength: 64,
		AssignMode:       "auto",
	}); err == nil {
		t.Fatalf("expected error for cidrPrefixLength=64")
	}
	if _, err := svc.AttachIPv6("inst-1", OCIAttachIPv6Request{
		VNICID:           "vnic-1",
		IPv6SubnetCIDR:   "2603:c020:1:2::/64",
		CIDRPrefixLength: 127,
		AssignMode:       "auto",
	}); err == nil {
		t.Fatalf("expected error for non-multiple-of-4 cidrPrefixLength=127")
	}

	// Reject manual IPv6 outside selected prefix
	if _, err := svc.AttachIPv6("inst-1", OCIAttachIPv6Request{
		VNICID:           "vnic-1",
		IPv6SubnetCIDR:   "2603:c020:1:2::/64",
		CIDRPrefixLength: 128,
		AssignMode:       "manual",
		IPAddress:        "2603:c020:9999:9999::1",
	}); err == nil {
		t.Fatalf("expected error for IPv6 outside subnet prefix")
	}

	// Reject changing /80 prefix IPv6
	if _, err := svc.ChangeIPv6("inst-1", "vnic-1", "v6-80"); err == nil || !strings.Contains(err.Error(), "128") {
		t.Fatalf("expected /80 change rejection, got: %v", err)
	}

	// Change /128 IPv6 -> must DELETE first then POST
	res, err := svc.ChangeIPv6("inst-1", "vnic-1", "v6-128")
	if err != nil {
		t.Fatalf("ChangeIPv6 failed: %v", err)
	}
	if res.OldIPAddress != "2603:c020:1:2::1111" || res.NewIPAddress != "2603:c020:1:2::2222" {
		t.Fatalf("unexpected ChangeIPv6 result: %+v", res)
	}
	if len(sequence) != 2 || sequence[0] != "DELETE:v6-128" || sequence[1] != "POST:/ipv6" {
		t.Fatalf("expected DELETE before POST, got sequence=%v", sequence)
	}
}

func TestEnableSubnetIPv6WithTwoDigitHex(t *testing.T) {
	origPoll := ociIPPollInterval
	ociIPPollInterval = 5 * time.Millisecond
	defer func() { ociIPPollInterval = origPoll }()

	derived, err := deriveSubnetIPv6CIDR("2603:c020:1234:5600::/56", "7E")
	if err != nil {
		t.Fatalf("deriveSubnetIPv6CIDR failed: %v", err)
	}
	if derived != "2603:c020:1234:567e::/64" {
		t.Fatalf("expected 2603:c020:1234:567e::/64, got %s", derived)
	}

	var vcnEnabled, subnetAdded, routeUpdated, slUpdated bool
	var vcnGetsAfterAdd int
	vcnIPv6 := []string{}
	subIPv6 := []string{}
	slEgress := []map[string]interface{}{}

	svc, closeFn := newTestOCIService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/instances/inst-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "inst-1", "displayName": "vm-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/vnicAttachments":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"vnicId": "vnic-1", "lifecycleState": "ATTACHED"}})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/vnics/vnic-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "vnic-1", "isPrimary": true, "subnetId": "sub-1", "vcnId": "vcn-1", "displayName": "vnic-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/subnets":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "sub-other", "vcnId": "vcn-1", "ipv6CidrBlocks": []string{"2603:c020:1234:5601::/64"}},
				{"id": "sub-1", "vcnId": "vcn-1", "ipv6CidrBlocks": subIPv6},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/subnets/sub-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":              "sub-1",
				"vcnId":           "vcn-1",
				"routeTableId":    "rt-1",
				"securityListIds": []string{"sl-1"},
				"cidrBlock":       "10.0.0.0/24",
				"ipv6CidrBlocks":  subIPv6,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/privateIps":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"id": "priv-1", "ipAddress": "10.0.0.2", "isPrimary": true}})
		case r.Method == http.MethodPost && r.URL.Path == "/20160918/publicIps/actions/getByPrivateIpId":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/publicIps":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/ipv6":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/internetGateways":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"id": "ocid1.internetgateway.oc1..igw1", "isEnabled": true}})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/vcns/vcn-1":
			if vcnEnabled {
				vcnGetsAfterAdd++
				if vcnGetsAfterAdd >= 2 {
					vcnIPv6 = []string{"2603:c020:1234:5600::/56"}
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "vcn-1", "ipv6CidrBlocks": vcnIPv6})
		case r.Method == http.MethodPost && r.URL.Path == "/20160918/vcns/vcn-1/actions/addIpv6Cidr":
			vcnEnabled = true
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/20160918/subnets/sub-1/actions/addIpv6Cidr":
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["ipv6CidrBlock"] != "2603:c020:1234:567e::/64" {
				t.Fatalf("unexpected subnet ipv6CidrBlock: %+v", body)
			}
			subnetAdded = true
			subIPv6 = []string{"2603:c020:1234:567e::/64"}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/routeTables/rt-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "rt-1",
				"routeRules": []map[string]interface{}{
					{"destination": "0.0.0.0/0", "destinationType": "CIDR_BLOCK", "networkEntityId": "ocid1.internetgateway.oc1..igw1"},
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/20160918/routeTables/rt-1":
			routeUpdated = true
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/securityLists/sl-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":                   "sl-1",
				"displayName":          "default-sl",
				"ingressSecurityRules": []map[string]interface{}{},
				"egressSecurityRules":  slEgress,
			})
		case r.Method == http.MethodPut && r.URL.Path == "/20160918/securityLists/sl-1":
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			rawEgress, _ := body["egressSecurityRules"].([]interface{})
			for _, item := range rawEgress {
				if m, ok := item.(map[string]interface{}); ok && m["destination"] == "::/0" {
					slUpdated = true
				}
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	defer closeFn()

	info, err := svc.GetIPManagement("inst-1")
	if err != nil {
		t.Fatalf("GetIPManagement failed: %v", err)
	}
	if len(info.VNICs) != 1 || info.VNICs[0].IPv6Setup == nil || info.VNICs[0].IPv6Setup.SuggestedHex != "7E" || len(info.VNICs[0].IPv6Setup.UsedHexes) != 1 || info.VNICs[0].IPv6Setup.UsedHexes[0] != "01" {
		t.Fatalf("unexpected IPv6Setup inspection: %+v", info.VNICs[0].IPv6Setup)
	}

	cidr, logs, err := svc.EnableSubnetIPv6("inst-1", OCIEnableIPv6Request{
		VNICID:                "vnic-1",
		HexByte:               "7e",
		AddDefaultRoute:       true,
		AddSecurityListEgress: false,
	})
	if err != nil {
		t.Fatalf("EnableSubnetIPv6 failed: %v", err)
	}
	if cidr != "2603:c020:1234:567e::/64" || !vcnEnabled || !subnetAdded || !routeUpdated || slUpdated {
		t.Fatalf("unexpected EnableSubnetIPv6 outcome: cidr=%s vcn=%v sub=%v rt=%v sl=%v", cidr, vcnEnabled, subnetAdded, routeUpdated, slUpdated)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "防火墙管理") {
		t.Fatalf("expected firewall reminder log, got: %v", logs)
	}

	_, _, err = svc.EnableSubnetIPv6("inst-1", OCIEnableIPv6Request{
		VNICID:                "vnic-1",
		HexByte:               "7e",
		AddDefaultRoute:       false,
		AddSecurityListEgress: true,
	})
	if err != nil {
		t.Fatalf("EnableSubnetIPv6 (sl egress) failed: %v", err)
	}
	if !slUpdated {
		t.Fatalf("expected security list ::/0 egress update when explicitly requested")
	}
}

func TestOCIGetRetriesOnTransientEOF(t *testing.T) {
	origDelay := ociTransientRetryDelay
	ociTransientRetryDelay = 5 * time.Millisecond
	defer func() { ociTransientRetryDelay = origDelay }()

	attempts := 0
	svc, closeFn := newTestOCIService(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					_ = conn.Close()
					return
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "inst-1", "displayName": "vm-1"})
	})
	defer closeFn()

	var out map[string]interface{}
	if err := svc.requestJSON(http.MethodGet, "/instances/inst-1", nil, nil, &out, false); err != nil {
		t.Fatalf("expected GET to recover from transient EOF, got: %v", err)
	}
	if attempts != 2 || out["id"] != "inst-1" {
		t.Fatalf("unexpected retry attempts=%d out=%+v", attempts, out)
	}
}

func TestExtractSubnetIPv6CIDRsDeduplicatesExpandedAndCompressed(t *testing.T) {
	subnet := map[string]interface{}{
		"ipv6CidrBlocks": []interface{}{"2603:c021:8023:7bee:0000:0000:0000:0000/64"},
		"ipv6CidrBlock":  "2603:c021:8023:7bee::/64",
	}
	got := extractSubnetIPv6CIDRs(subnet)
	if len(got) != 1 || got[0] != "2603:c021:8023:7bee:0000:0000:0000:0000/64" {
		t.Fatalf("expected single expanded deduplicated CIDR, got %v", got)
	}
}

func TestRouteTablesListAndSaveRules(t *testing.T) {
	var putPayload map[string]interface{}
	svc, closeFn := newTestOCIService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/routeTables":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{
				"id":          "rt-1",
				"displayName": "Default Route Table",
				"vcnId":       "vcn-1",
				"routeRules": []map[string]interface{}{{
					"destination":     "0.0.0.0/0",
					"destinationType": "CIDR_BLOCK",
					"networkEntityId": "igw-1",
				}},
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/vcns":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"id": "vcn-1", "displayName": "main-vcn"}})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/subnets":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"id": "sub-1", "displayName": "public-subnet", "routeTableId": "rt-1"}})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/internetGateways":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"id": "igw-1", "vcnId": "vcn-1", "displayName": "igw-main"}})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/natGateways":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/serviceGateways":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/localPeeringGateways":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/drgAttachments":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/routeTables/rt-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":          "rt-1",
				"displayName": "Default Route Table",
			})
		case r.Method == http.MethodPut && r.URL.Path == "/20160918/routeTables/rt-1":
			_ = json.NewDecoder(r.Body).Decode(&putPayload)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	defer closeFn()

	tables, err := svc.ListAccountRouteTables()
	if err != nil {
		t.Fatalf("ListAccountRouteTables failed: %v", err)
	}
	if len(tables) != 1 || tables[0].VCNName != "main-vcn" || len(tables[0].SubnetNames) != 1 || len(tables[0].AvailableEntities) != 1 {
		t.Fatalf("unexpected route tables: %+v", tables)
	}

	err = svc.SaveAccountRouteTableRules("rt-1", []OCIRouteRule{
		{Destination: "0.0.0.0/0", DestinationType: "CIDR_BLOCK", NetworkEntityID: "igw-1"},
		{Destination: "::/0", DestinationType: "CIDR_BLOCK", NetworkEntityID: "igw-1", Description: "IPv6 default"},
	})
	if err != nil {
		t.Fatalf("SaveAccountRouteTableRules failed: %v", err)
	}
	rawRules, _ := putPayload["routeRules"].([]interface{})
	if len(rawRules) != 2 {
		t.Fatalf("expected 2 saved route rules, got %+v", putPayload)
	}
}
