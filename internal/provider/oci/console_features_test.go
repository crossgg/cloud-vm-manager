package oci

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestGetCreateOptionsUsesIdentityHostForADsAndCreatesDefaultNetwork(t *testing.T) {
	var seenADHost string
	var createdVCN, createdIGW, createdSubnet, updatedSL bool

	svc, closeFn := newTestOCIService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		origHost := r.Header.Get("X-Orig-Host")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/availabilityDomains":
			seenADHost = origHost
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"name": "AD-1"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/shapes":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"shape": "VM.Standard.A1.Flex", "ocpus": 1, "memoryInGBs": 6},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/vcns":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/subnets":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/networkSecurityGroups":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/images":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "img-1", "displayName": "Ubuntu 24.04", "operatingSystem": "Canonical Ubuntu"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/20160918/vcns":
			createdVCN = true
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":                    "vcn-new",
				"displayName":           "default-vcn",
				"cidrBlock":             "10.0.0.0/16",
				"defaultRouteTableId":   "rt-new",
				"defaultSecurityListId": "sl-new",
				"ipv6CidrBlocks":        []string{"2603:c020:1000:2000::/56"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/20160918/internetGateways":
			createdIGW = true
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "igw-new"})
		case r.Method == http.MethodPut && r.URL.Path == "/20160918/routeTables/rt-new":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/20160918/securityLists/sl-new":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":                   "sl-new",
				"displayName":          "Default Security List",
				"ingressSecurityRules": []map[string]interface{}{},
				"egressSecurityRules":  []map[string]interface{}{},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/20160918/securityLists/sl-new":
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			rawEgress, _ := body["egressSecurityRules"].([]interface{})
			for _, item := range rawEgress {
				if m, ok := item.(map[string]interface{}); ok && m["destination"] == "::/0" && m["protocol"] == "all" {
					updatedSL = true
				}
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/20160918/subnets":
			createdSubnet = true
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["ipv6CidrBlock"] != "2603:c020:1000:2000::/64" {
				t.Fatalf("expected derived /64 ipv6CidrBlock, got: %+v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             "sub-new",
				"vcnId":          "vcn-new",
				"displayName":    "public-subnet",
				"cidrBlock":      "10.0.0.0/24",
				"ipv6CidrBlocks": []string{"2603:c020:1000:2000::/64"},
			})
		default:
			t.Fatalf("unexpected request: %s %s (Host=%s)", r.Method, r.URL.Path, origHost)
		}
	})
	defer closeFn()

	opts, err := svc.GetCreateOptions("VM.Standard.A1.Flex")
	if err != nil {
		t.Fatalf("GetCreateOptions failed: %v", err)
	}
	if seenADHost != "identity.us-ashburn-1.oraclecloud.com" {
		t.Fatalf("expected AD request to hit identity host, got: %s", seenADHost)
	}
	if len(opts.AvailabilityDomains) != 1 || len(opts.Images) != 1 {
		t.Fatalf("unexpected create options: %+v", opts)
	}

	netRes, err := svc.CreateDefaultNetwork(true)
	if err != nil {
		t.Fatalf("CreateDefaultNetwork failed: %v", err)
	}
	if !createdVCN || !createdIGW || !createdSubnet || !updatedSL || netRes.Subnet.ID != "sub-new" {
		t.Fatalf("unexpected default network result: %+v updatedSL=%v", netRes, updatedSL)
	}
}

func TestOverviewIAMObjectStorageAndTerminate(t *testing.T) {
	var seenUsageHost, seenTerminateQuery string
	var uploadedKeyBody, createdPolicyBody, updatedPolicyBody map[string]interface{}
	var deletedPolicy bool

	svc, closeFn := newTestOCIService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		host := r.Header.Get("X-Orig-Host")
		path := r.URL.Path

		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/regionSubscriptions"):
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"regionName": "us-phoenix-1", "isHomeRegion": true},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/20160918/tenancies/"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"name": "my-tenancy"})
		case r.Method == http.MethodPost && path == "/20200107/usage":
			seenUsageHost = host
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"NotAuthorizedOrNotFound","message":"no permission"}`))
		case r.Method == http.MethodGet && path == "/20160918/availabilityDomains":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"name": "AD-1"}})
		case r.Method == http.MethodGet && path == "/20190729/limitValues":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"name": "standard-a1-core-count", "scopeType": "AD", "value": 4},
			})
		case r.Method == http.MethodGet && strings.Contains(path, "/resourceAvailability"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"used": 2, "available": 2})
		case r.Method == http.MethodGet && path == "/20160918/users":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "user-1", "name": "alice", "lifecycleState": "ACTIVE"},
			})
		case r.Method == http.MethodGet && path == "/20160918/groups":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "grp-1", "name": "Administrators", "lifecycleState": "ACTIVE"},
			})
		case r.Method == http.MethodGet && path == "/20160918/userGroupMemberships":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "mem-1", "userId": "user-1", "groupId": "grp-1"},
			})
		case r.Method == http.MethodGet && path == "/20160918/compartments":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "comp-1", "name": "prod", "lifecycleState": "ACTIVE"},
			})
		case r.Method == http.MethodGet && path == "/20160918/policies":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":             "pol-1",
					"name":           "admin-policy",
					"compartmentId":  r.URL.Query().Get("compartmentId"),
					"statements":     []string{"Allow group Administrators to manage all-resources in tenancy"},
					"lifecycleState": "ACTIVE",
				},
			})
		case r.Method == http.MethodPost && path == "/20160918/policies":
			_ = json.NewDecoder(r.Body).Decode(&createdPolicyBody)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             "pol-2",
				"name":           createdPolicyBody["name"],
				"description":    createdPolicyBody["description"],
				"compartmentId":  createdPolicyBody["compartmentId"],
				"statements":     createdPolicyBody["statements"],
				"lifecycleState": "ACTIVE",
			})
		case r.Method == http.MethodPut && path == "/20160918/policies/pol-2":
			_ = json.NewDecoder(r.Body).Decode(&updatedPolicyBody)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             "pol-2",
				"name":           "ops-policy",
				"description":    updatedPolicyBody["description"],
				"compartmentId":  "ocid1.tenancy.oc1..test",
				"statements":     updatedPolicyBody["statements"],
				"lifecycleState": "ACTIVE",
			})
		case r.Method == http.MethodDelete && path == "/20160918/policies/pol-2":
			deletedPolicy = true
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && path == "/20160918/users/user-1/apiKeys":
			_ = json.NewDecoder(r.Body).Decode(&uploadedKeyBody)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"keyId":          "tenancy/user-1/fp1",
				"fingerprint":    "fp1",
				"lifecycleState": "ACTIVE",
			})
		case r.Method == http.MethodGet && path == "/n/":
			_ = json.NewEncoder(w).Encode("my-ns")
		case r.Method == http.MethodGet && path == "/n/my-ns/b":
			for _, f := range r.URL.Query()["fields"] {
				if f == "approximateCount" || f == "approximateSize" {
					t.Fatalf("ListBuckets must not request invalid field %q", f)
				}
			}
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"name": "bucket-1", "namespace": "my-ns"},
			})
		case r.Method == http.MethodGet && path == "/n/my-ns/b/bucket-1":
			fields := strings.Join(r.URL.Query()["fields"], ",")
			if !strings.Contains(fields, "approximateCount") || !strings.Contains(fields, "approximateSize") {
				t.Fatalf("GetBucket should request approximateCount and approximateSize, got: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":             "bucket-1",
				"namespace":        "my-ns",
				"publicAccessType": "ObjectRead",
				"approximateCount": 3,
				"approximateSize":  1024,
			})
		case r.Method == http.MethodPut && path == "/n/my-ns/b/bucket-1/o/dir/hello.txt":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete && path == "/20160918/instances/inst-1":
			seenTerminateQuery = r.URL.RawQuery
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s (Host=%s)", r.Method, path, host)
		}
	})
	defer closeFn()

	overview, err := svc.GetOverview()
	if err != nil {
		t.Fatalf("GetOverview failed: %v", err)
	}
	if overview.HomeRegion != "us-phoenix-1" || seenUsageHost != "usageapi.us-phoenix-1.oci.oraclecloud.com" {
		t.Fatalf("expected usage query on home region us-phoenix-1, got home=%s usageHost=%s", overview.HomeRegion, seenUsageHost)
	}
	if overview.CostStatus != "unauthorized" {
		t.Fatalf("expected graceful unauthorized costStatus, got: %s", overview.CostStatus)
	}
	if len(overview.Quotas) != 1 || overview.Quotas[0].Available != 2 {
		t.Fatalf("unexpected quotas: %+v", overview.Quotas)
	}

	iamData, err := svc.ListIAMData()
	if err != nil {
		t.Fatalf("ListIAMData failed: %v", err)
	}
	if len(iamData.Users) != 1 || len(iamData.Users[0].Groups) != 1 || iamData.Users[0].Groups[0].GroupName != "Administrators" {
		t.Fatalf("unexpected IAM users/groups: %+v", iamData)
	}
	if len(iamData.Compartments) != 2 || len(iamData.Policies) != 1 {
		t.Fatalf("expected root + prod compartments and 1 policy, got comps=%+v policies=%+v", iamData.Compartments, iamData.Policies)
	}

	createdPol, err := svc.CreateIAMPolicy("", "ops-policy", "", []string{"Allow group ops to manage instance-family in tenancy", "  "})
	if err != nil || createdPol.ID != "pol-2" || len(createdPol.Statements) != 1 {
		t.Fatalf("CreateIAMPolicy failed: pol=%+v err=%v", createdPol, err)
	}
	updatedPol, err := svc.UpdateIAMPolicy("pol-2", "updated desc", []string{"Allow group ops to read all-resources in tenancy"})
	if err != nil || len(updatedPol.Statements) != 1 || updatedPolicyBody["description"] != "updated desc" {
		t.Fatalf("UpdateIAMPolicy failed: pol=%+v err=%v", updatedPol, err)
	}
	if err := svc.DeleteIAMPolicy("pol-2"); err != nil || !deletedPolicy {
		t.Fatalf("DeleteIAMPolicy failed: deleted=%v err=%v", deletedPolicy, err)
	}

	keyRes, err := svc.UploadUserAPIKey("user-1", "", true)
	if err != nil {
		t.Fatalf("UploadUserAPIKey failed: %v", err)
	}
	if keyRes.PrivateKeyPEM == "" || !strings.Contains(stringValue(uploadedKeyBody["key"]), "BEGIN PUBLIC KEY") {
		t.Fatalf("expected generated RSA keypair upload, got: %+v", uploadedKeyBody)
	}

	ns, buckets, err := svc.ListBuckets()
	if err != nil || ns != "my-ns" || len(buckets) != 1 {
		t.Fatalf("ListBuckets failed: ns=%s buckets=%+v err=%v", ns, buckets, err)
	}
	bucketDetail, err := svc.GetBucket("bucket-1")
	if err != nil || bucketDetail.PublicAccessType != "ObjectRead" || bucketDetail.ApproximateCount != 3 || bucketDetail.ApproximateSize != 1024 {
		t.Fatalf("GetBucket failed: detail=%+v err=%v", bucketDetail, err)
	}
	if err := svc.PutObject("bucket-1", "dir/hello.txt", "text/plain", []byte("hi")); err != nil {
		t.Fatalf("PutObject failed: %v", err)
	}

	if err := svc.TerminateInstance("inst-1", true); err != nil {
		t.Fatalf("TerminateInstance failed: %v", err)
	}
	if seenTerminateQuery != "preserveBootVolume=true" {
		t.Fatalf("expected preserveBootVolume=true query, got %s", seenTerminateQuery)
	}
}
