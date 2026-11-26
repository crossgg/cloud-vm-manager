package dns

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

type mockRoundTripFunc func(*http.Request) (*http.Response, error)

func (f mockRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestParsePreviewExportBINDZone(t *testing.T) {
	zone := `
$ORIGIN example.com.
$TTL 300
@       IN  SOA ns1.example.com. hostmaster.example.com. (
                2026010101 ; serial
                7200       ; refresh
                900        ; retry
                1209600    ; expire
                300 )      ; minimum
@       IN  NS  ns1.example.com.
sub     IN  NS  ns2.other.net.
@       600 IN  A   1.2.3.4
        IN  AAAA 2001:db8::1
www     IN  CNAME example.com.
@       IN  MX  10 mail.example.com.
@       IN  TXT "v=spf1 include:_spf.example.com ~all"
_sip._tcp IN SRV 5 10 5060 sip.example.com.
@       IN  CAA 0 issue "letsencrypt.org"
`
	parsed, warnings, err := ParseBINDZone("example.com", zone)
	if err != nil {
		t.Fatalf("ParseBINDZone failed: %v", err)
	}
	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings (SOA + apex NS), got %d: %v", len(warnings), warnings)
	}
	if len(parsed) != 8 {
		t.Fatalf("expected 8 importable records, got %d: %+v", len(parsed), parsed)
	}
	if parsed[0].Name != "sub" || parsed[0].Type != "NS" || parsed[0].Value != "ns2.other.net" {
		t.Fatalf("unexpected delegated NS record: %+v", parsed[0])
	}
	if parsed[2].Name != "@" || parsed[2].Type != "AAAA" || parsed[2].Value != "2001:db8::1" {
		t.Fatalf("expected inherited @ owner on AAAA record, got %+v", parsed[2])
	}
	if parsed[4].Type != "MX" || parsed[4].Priority == nil || *parsed[4].Priority != 10 {
		t.Fatalf("unexpected MX record: %+v", parsed[4])
	}
	if parsed[5].Type != "TXT" || parsed[5].Value != "v=spf1 include:_spf.example.com ~all" {
		t.Fatalf("unexpected TXT record: %+v", parsed[5])
	}

	existing := []DNSRecord{
		{ID: "rec-1", Domain: "example.com", Name: "@", Type: "A", Value: "9.9.9.9", TTL: 600},
	}
	preview, err := PreviewBINDImport("example.com", zone, existing)
	if err != nil {
		t.Fatalf("PreviewBINDImport failed: %v", err)
	}
	if preview.CreateCount != 7 || preview.ConflictCount != 1 || preview.SkipCount != 2 {
		t.Fatalf("unexpected preview counts: %+v", preview)
	}

	exported := ExportBINDZone("example.com", parsed)
	if !strings.Contains(exported, "$ORIGIN example.com.") {
		t.Fatalf("expected $ORIGIN in exported zone:\n%s", exported)
	}
	if !strings.Contains(exported, `"v=spf1 include:_spf.example.com ~all"`) {
		t.Fatalf("expected quoted TXT in exported zone:\n%s", exported)
	}
	if !strings.Contains(exported, "2001:db8::1") {
		t.Fatalf("expected AAAA value in exported zone:\n%s", exported)
	}
}

func TestAlidnsClientCRUDAndUpsert(t *testing.T) {
	var calledActions []string
	var updatedValue string
	client := &http.Client{
		Transport: mockRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host != "alidns.aliyuncs.com" {
				t.Fatalf("unexpected host: %s", req.URL.Host)
			}
			bodyBytes, _ := io.ReadAll(req.Body)
			vals, err := url.ParseQuery(string(bodyBytes))
			if err != nil {
				t.Fatalf("failed to parse alidns form body: %v", err)
			}
			if vals.Get("Signature") == "" || vals.Get("AccessKeyId") != "ak-test" {
				t.Fatalf("missing signature or access key in body: %s", string(bodyBytes))
			}
			action := vals.Get("Action")
			calledActions = append(calledActions, action)
			switch action {
			case "DescribeDomainRecords":
				if vals.Get("PageNumber") == "1" {
					return jsonResponse(200, `{
						"TotalCount": 2,
						"DomainRecords": {
							"Record": [
								{"RecordId":"101","DomainName":"example.com","RR":"www","Type":"A","Value":"1.1.1.1","TTL":600,"Line":"default","Status":"ENABLE"}
							]
						}
					}`), nil
				}
				return jsonResponse(200, `{
					"TotalCount": 2,
					"DomainRecords": {
						"Record": [
							{"RecordId":"102","DomainName":"example.com","RR":"@","Type":"A","Value":"2.2.2.2","TTL":600,"Line":"default","Status":"ENABLE"}
						]
					}
				}`), nil
			case "UpdateDomainRecord":
				updatedValue = vals.Get("Value")
				return jsonResponse(200, `{"RecordId":"101"}`), nil
			case "AddDomainRecord":
				return jsonResponse(200, `{"RecordId":"103"}`), nil
			case "SetDomainRecordStatus":
				return jsonResponse(200, `{"RecordId":"101"}`), nil
			case "DeleteDomainRecord":
				return jsonResponse(200, `{"RecordId":"101"}`), nil
			default:
				t.Fatalf("unexpected alidns action: %s", action)
			}
			return nil, nil
		}),
	}

	ali := NewAlidnsClient(config.DNSAccountConfig{
		Name:            "ali-main",
		Provider:        "alidns",
		AccessKeyID:     "ak-test",
		AccessKeySecret: "sk-test",
		Domains:         []string{"example.com"},
	}, client)

	domains, err := ali.ResolveConfiguredDomains()
	if err != nil || len(domains) != 1 || domains[0].Domain != "example.com" {
		t.Fatalf("unexpected configured domains: %+v err=%v", domains, err)
	}

	recs, err := ali.ListRecords("example.com")
	if err != nil {
		t.Fatalf("ListRecords failed: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 paginated records, got %d", len(recs))
	}

	err = ali.UpsertBindingRecord(config.DNSBinding{
		Domain: "www.example.com",
		Type:   "A",
		TTL:    600,
	}, "8.8.8.8")
	if err != nil {
		t.Fatalf("UpsertBindingRecord failed: %v", err)
	}
	if updatedValue != "8.8.8.8" {
		t.Fatalf("expected updated IP 8.8.8.8, got %q", updatedValue)
	}

	for _, act := range calledActions {
		if act == "DescribeDomains" {
			t.Fatalf("must not enumerate account domains via DescribeDomains")
		}
	}
}

func TestTencentClientCNAndIntlEndpoints(t *testing.T) {
	for _, tc := range []struct {
		edition      string
		expectedHost string
		expectedLine string
	}{
		{edition: "cn", expectedHost: "dnspod.tencentcloudapi.com", expectedLine: "默认"},
		{edition: "intl", expectedHost: "dnspod.intl.tencentcloudapi.com", expectedLine: "Default"},
	} {
		t.Run(tc.edition, func(t *testing.T) {
			var sawCreateLine string
			client := &http.Client{
				Transport: mockRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					if req.URL.Host != tc.expectedHost {
						t.Fatalf("expected host %s, got %s", tc.expectedHost, req.URL.Host)
					}
					auth := req.Header.Get("Authorization")
					if !strings.HasPrefix(auth, "TC3-HMAC-SHA256 Credential=sid-1/2026-01-02/dnspod/tc3_request, SignedHeaders=content-type;host;x-tc-action, Signature=") {
						t.Fatalf("unexpected TC3 auth header: %s", auth)
					}
					action := req.Header.Get("X-TC-Action")
					bodyBytes, _ := io.ReadAll(req.Body)
					var payload map[string]interface{}
					_ = json.Unmarshal(bodyBytes, &payload)

					switch action {
					case "DescribeRecordList":
						return jsonResponse(200, `{"Response":{"Error":{"Code":"ResourceNotFound.NoDataOfRecord","Message":"empty"},"RequestId":"r1"}}`), nil
					case "CreateRecord":
						sawCreateLine, _ = payload["RecordLine"].(string)
						return jsonResponse(200, `{"Response":{"RecordId":999,"RequestId":"r2"}}`), nil
					default:
						t.Fatalf("unexpected tencent action: %s", action)
					}
					return nil, nil
				}),
			}

			cli := NewTencentClient(config.DNSAccountConfig{
				Name:      "tx",
				Provider:  "tencent",
				Edition:   tc.edition,
				SecretID:  "sid-1",
				SecretKey: "skey-1",
				Domains:   []string{"example.org"},
			}, client)
			cli.nowFunc = func() time.Time {
				return time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
			}

			recs, err := cli.ListRecords("example.org")
			if err != nil {
				t.Fatalf("expected empty list on NoDataOfRecord, got err=%v", err)
			}
			if len(recs) != 0 {
				t.Fatalf("expected 0 records, got %d", len(recs))
			}

			err = cli.UpsertBindingRecord(config.DNSBinding{
				Domain: "node1.example.org",
				Type:   "A",
				TTL:    600,
			}, "5.5.5.5")
			if err != nil {
				t.Fatalf("UpsertBindingRecord failed: %v", err)
			}
			if sawCreateLine != tc.expectedLine {
				t.Fatalf("expected default line %q, got %q", tc.expectedLine, sawCreateLine)
			}
		})
	}
}

func TestDNSPodClientCNAndIntlEndpoints(t *testing.T) {
	for _, tc := range []struct {
		edition      string
		expectedHost string
		expectedLine string
	}{
		{edition: "cn", expectedHost: "dnsapi.cn", expectedLine: "默认"},
		{edition: "intl", expectedHost: "api.dnspod.com", expectedLine: "Default"},
	} {
		t.Run(tc.edition, func(t *testing.T) {
			var sawCreateLine string
			client := &http.Client{
				Transport: mockRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					if req.URL.Host != tc.expectedHost {
						t.Fatalf("expected host %s, got %s", tc.expectedHost, req.URL.Host)
					}
					bodyBytes, _ := io.ReadAll(req.Body)
					vals, _ := url.ParseQuery(string(bodyBytes))
					if vals.Get("login_token") != "123,tok-abc" {
						t.Fatalf("unexpected login_token: %s", vals.Get("login_token"))
					}
					switch req.URL.Path {
					case "/Record.List":
						return jsonResponse(200, `{"status":{"code":"10","message":"No records"}}`), nil
					case "/Record.Create":
						sawCreateLine = vals.Get("record_line")
						return jsonResponse(200, `{"status":{"code":"1","message":"Action completed successful"},"record":{"id":"555"}}`), nil
					default:
						t.Fatalf("unexpected path: %s", req.URL.Path)
					}
					return nil, nil
				}),
			}

			cli := NewDNSPodClient(config.DNSAccountConfig{
				Name:     "dp",
				Provider: "dnspod",
				Edition:  tc.edition,
				TokenID:  "123",
				Token:    "tok-abc",
				Domains:  []string{"example.net"},
			}, client)

			recs, err := cli.ListRecords("example.net")
			if err != nil || len(recs) != 0 {
				t.Fatalf("expected empty list for code 10, got len=%d err=%v", len(recs), err)
			}

			err = cli.UpsertBindingRecord(config.DNSBinding{
				Domain: "v6.example.net",
				Type:   "AAAA",
				TTL:    600,
			}, "2001:db8::2")
			if err != nil {
				t.Fatalf("UpsertBindingRecord failed: %v", err)
			}
			if sawCreateLine != tc.expectedLine {
				t.Fatalf("expected default line %q, got %q", tc.expectedLine, sawCreateLine)
			}
		})
	}
}

func TestStrictBindingNonWildcardMatching(t *testing.T) {
	var requests []string
	cfg := &config.Config{
		DNSAccounts: []config.DNSAccountConfig{
			{
				Name:            "ali-main",
				Provider:        "alidns",
				AccessKeyID:     "ak",
				AccessKeySecret: "sk",
				Domains:         []string{"example.com"},
			},
		},
		DNSBindings: []config.DNSBinding{
			// Incomplete wildcard-like bindings MUST be ignored!
			{Name: "bad-empty-vm", DNSAccount: "ali-main", Provider: "oci", Account: "acc1", VM: "", Domain: "bad1.example.com", Type: "A"},
			{Name: "bad-empty-provider", DNSAccount: "ali-main", Provider: "", Account: "acc1", VM: "vm-1", Domain: "bad2.example.com", Type: "A"},
			// Exact match binding
			{Name: "good", DNSAccount: "ali-main", Provider: "oci", Account: "acc1", VM: "vm-1", Domain: "good.example.com", Type: "A"},
			// Different VM binding
			{Name: "other-vm", DNSAccount: "ali-main", Provider: "oci", Account: "acc1", VM: "vm-2", Domain: "other.example.com", Type: "A"},
		},
	}

	svc := NewCloudflareService(cfg)
	svc.client = &http.Client{
		Transport: mockRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			bodyBytes, _ := io.ReadAll(req.Body)
			vals, _ := url.ParseQuery(string(bodyBytes))
			action := vals.Get("Action")
			requests = append(requests, action+":"+vals.Get("RR"))
			switch action {
			case "DescribeDomainRecords":
				return jsonResponse(200, `{"TotalCount":0,"DomainRecords":{"Record":[]}}`), nil
			case "AddDomainRecord":
				return jsonResponse(200, `{"RecordId":"901"}`), nil
			default:
				return jsonResponse(200, `{}`), nil
			}
		}),
	}

	if svc.HasBinding("oci", "acc1", "vm-999") {
		t.Fatalf("expected vm-999 not to match any incomplete wildcard binding")
	}
	if !svc.HasBinding("oci", "acc1", "vm-1") {
		t.Fatalf("expected vm-1 to match exact binding")
	}

	logs := svc.UpdateForVMByIPs("oci", "acc1", "vm-1", "9.8.7.6", "")
	if len(logs) != 1 || !strings.Contains(logs[0], "DNS updated") {
		t.Fatalf("unexpected UpdateForVMByIPs logs: %v", logs)
	}
	if len(requests) != 2 || requests[1] != "AddDomainRecord:good" {
		t.Fatalf("expected only AddDomainRecord:good, got %v", requests)
	}
}
