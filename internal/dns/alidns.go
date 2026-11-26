package dns

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

const defaultAlidnsEndpoint = "https://alidns.aliyuncs.com/"

var aliNonceSeq uint64

type AlidnsClient struct {
	account  config.DNSAccountConfig
	client   *http.Client
	endpoint string
}

func NewAlidnsClient(account config.DNSAccountConfig, httpClient *http.Client) *AlidnsClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &AlidnsClient{
		account:  account,
		client:   httpClient,
		endpoint: defaultAlidnsEndpoint,
	}
}

func (c *AlidnsClient) Capabilities() DNSCapabilities {
	return DNSCapabilities{
		Provider:        "alidns",
		SupportsProxied: false,
		SupportsLine:    true,
		SupportsStatus:  true,
		SupportsRemark:  true,
		DefaultLine:     "default",
		AvailableLines:  []string{"default", "telecom", "unicom", "mobile", "edu", "oversea", "internal"},
	}
}

func (c *AlidnsClient) ResolveConfiguredDomains() ([]DNSDomainInfo, error) {
	seen := make(map[string]struct{})
	out := make([]DNSDomainInfo, 0, len(c.account.Domains))
	for _, d := range c.account.Domains {
		norm := normalizeRootDomain(d)
		if norm == "" {
			continue
		}
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		out = append(out, DNSDomainInfo{Domain: norm})
	}
	return out, nil
}

func (c *AlidnsClient) ListRecords(domain string) ([]DNSRecord, error) {
	root := normalizeRootDomain(domain)
	if root == "" {
		return nil, fmt.Errorf("domain is required")
	}

	var records []DNSRecord
	page := 1
	const pageSize = 500
	for {
		var resp struct {
			TotalCount    int `json:"TotalCount"`
			DomainRecords struct {
				Record []struct {
					RecordId   string `json:"RecordId"`
					DomainName string `json:"DomainName"`
					RR         string `json:"RR"`
					Type       string `json:"Type"`
					Value      string `json:"Value"`
					TTL        int    `json:"TTL"`
					Priority   int    `json:"Priority"`
					Line       string `json:"Line"`
					Status     string `json:"Status"`
					Remark     string `json:"Remark"`
				} `json:"Record"`
			} `json:"DomainRecords"`
		}

		err := c.doRPC("DescribeDomainRecords", map[string]string{
			"DomainName": root,
			"PageNumber": strconv.Itoa(page),
			"PageSize":   strconv.Itoa(pageSize),
		}, &resp)
		if err != nil {
			return nil, err
		}

		for _, item := range resp.DomainRecords.Record {
			var prio *int
			t := strings.ToUpper(strings.TrimSpace(item.Type))
			if t == "MX" || item.Priority > 0 {
				p := item.Priority
				prio = &p
			}
			status := "ENABLE"
			if strings.EqualFold(item.Status, "DISABLE") {
				status = "DISABLE"
			}
			recDomain := normalizeRootDomain(item.DomainName)
			if recDomain == "" {
				recDomain = root
			}
			rr := item.RR
			if rr == "" {
				rr = "@"
			}
			records = append(records, DNSRecord{
				ID:       item.RecordId,
				Domain:   recDomain,
				Name:     rr,
				FQDN:     rrToFQDN(rr, recDomain),
				Type:     t,
				Value:    item.Value,
				TTL:      item.TTL,
				Priority: prio,
				Line:     item.Line,
				Status:   status,
				Remark:   item.Remark,
			})
		}

		if len(resp.DomainRecords.Record) == 0 || len(records) >= resp.TotalCount {
			break
		}
		page++
	}
	return records, nil
}

func (c *AlidnsClient) CreateRecord(domain string, rec DNSRecord) (*DNSRecord, error) {
	root := normalizeRootDomain(domain)
	if root == "" {
		return nil, fmt.Errorf("domain is required")
	}
	rr := splitFQDNToRR(rec.Name, root)
	recType := strings.ToUpper(strings.TrimSpace(rec.Type))
	if recType == "" {
		return nil, fmt.Errorf("record type is required")
	}
	line := strings.TrimSpace(rec.Line)
	if line == "" {
		line = "default"
	}
	ttl := rec.TTL
	if ttl <= 0 {
		ttl = 600
	}

	params := map[string]string{
		"DomainName": root,
		"RR":         rr,
		"Type":       recType,
		"Value":      rec.Value,
		"TTL":        strconv.Itoa(ttl),
		"Line":       line,
	}
	if recType == "MX" {
		p := 10
		if rec.Priority != nil && *rec.Priority > 0 {
			p = *rec.Priority
		}
		params["Priority"] = strconv.Itoa(p)
	}

	var resp struct {
		RecordId string `json:"RecordId"`
	}
	if err := c.doRPC("AddDomainRecord", params, &resp); err != nil {
		return nil, err
	}

	if strings.TrimSpace(rec.Remark) != "" && resp.RecordId != "" {
		_ = c.doRPC("UpdateDomainRecordRemark", map[string]string{
			"RecordId": resp.RecordId,
			"Remark":   rec.Remark,
		}, nil)
	}
	if strings.EqualFold(rec.Status, "DISABLE") && resp.RecordId != "" {
		_ = c.SetRecordStatus(root, resp.RecordId, false)
	}

	created := rec
	created.ID = resp.RecordId
	created.Domain = root
	created.Name = rr
	created.FQDN = rrToFQDN(rr, root)
	created.Type = recType
	created.TTL = ttl
	created.Line = line
	if created.Status == "" {
		created.Status = "ENABLE"
	}
	return &created, nil
}

func (c *AlidnsClient) UpdateRecord(domain string, recordID string, rec DNSRecord) (*DNSRecord, error) {
	root := normalizeRootDomain(domain)
	if recordID == "" {
		return nil, fmt.Errorf("record id is required")
	}
	rr := splitFQDNToRR(rec.Name, root)
	recType := strings.ToUpper(strings.TrimSpace(rec.Type))
	line := strings.TrimSpace(rec.Line)
	if line == "" {
		line = "default"
	}
	ttl := rec.TTL
	if ttl <= 0 {
		ttl = 600
	}

	params := map[string]string{
		"RecordId": recordID,
		"RR":       rr,
		"Type":     recType,
		"Value":    rec.Value,
		"TTL":      strconv.Itoa(ttl),
		"Line":     line,
	}
	if recType == "MX" {
		p := 10
		if rec.Priority != nil && *rec.Priority > 0 {
			p = *rec.Priority
		}
		params["Priority"] = strconv.Itoa(p)
	}

	if err := c.doRPC("UpdateDomainRecord", params, nil); err != nil {
		// Alidns returns DomainRecordDuplicate if no core field changed; allow remark/status update to continue.
		if !strings.Contains(err.Error(), "DomainRecordDuplicate") {
			return nil, err
		}
	}

	if rec.Remark != "" {
		_ = c.doRPC("UpdateDomainRecordRemark", map[string]string{
			"RecordId": recordID,
			"Remark":   rec.Remark,
		}, nil)
	}
	if strings.EqualFold(rec.Status, "DISABLE") || strings.EqualFold(rec.Status, "ENABLE") {
		_ = c.SetRecordStatus(root, recordID, strings.EqualFold(rec.Status, "ENABLE"))
	}

	updated := rec
	updated.ID = recordID
	updated.Domain = root
	updated.Name = rr
	updated.FQDN = rrToFQDN(rr, root)
	updated.Type = recType
	updated.TTL = ttl
	updated.Line = line
	return &updated, nil
}

func (c *AlidnsClient) DeleteRecord(domain string, recordID string) error {
	if recordID == "" {
		return fmt.Errorf("record id is required")
	}
	return c.doRPC("DeleteDomainRecord", map[string]string{
		"RecordId": recordID,
	}, nil)
}

func (c *AlidnsClient) SetRecordStatus(domain string, recordID string, enabled bool) error {
	if recordID == "" {
		return fmt.Errorf("record id is required")
	}
	status := "Disable"
	if enabled {
		status = "Enable"
	}
	return c.doRPC("SetDomainRecordStatus", map[string]string{
		"RecordId": recordID,
		"Status":   status,
	}, nil)
}

func (c *AlidnsClient) UpsertBindingRecord(binding config.DNSBinding, ip string) error {
	rootDomain, rr, err := matchConfiguredDomain(binding.Domain, c.account.Domains)
	if err != nil {
		return err
	}
	recType := strings.ToUpper(strings.TrimSpace(binding.Type))
	if recType == "" {
		recType = "A"
	}
	ttl := binding.TTL
	if ttl <= 0 {
		ttl = 600
	}

	records, err := c.ListRecords(rootDomain)
	if err != nil {
		return err
	}
	for _, r := range records {
		if strings.EqualFold(r.Name, rr) && strings.EqualFold(r.Type, recType) {
			if r.Value == ip && r.TTL == ttl {
				return nil
			}
			line := r.Line
			if line == "" {
				line = "default"
			}
			_, err := c.UpdateRecord(rootDomain, r.ID, DNSRecord{
				Name:   rr,
				Type:   recType,
				Value:  ip,
				TTL:    ttl,
				Line:   line,
				Status: r.Status,
			})
			return err
		}
	}

	_, err = c.CreateRecord(rootDomain, DNSRecord{
		Name:   rr,
		Type:   recType,
		Value:  ip,
		TTL:    ttl,
		Line:   "default",
		Status: "ENABLE",
	})
	return err
}

func (c *AlidnsClient) doRPC(action string, extra map[string]string, out interface{}) error {
	if strings.TrimSpace(c.account.AccessKeyID) == "" || strings.TrimSpace(c.account.AccessKeySecret) == "" {
		return fmt.Errorf("alidns credentials are missing")
	}

	nonce := fmt.Sprintf("%d-%d", time.Now().UTC().UnixNano(), atomic.AddUint64(&aliNonceSeq, 1))
	params := map[string]string{
		"Action":           action,
		"Format":           "JSON",
		"Version":          "2015-01-09",
		"AccessKeyId":      c.account.AccessKeyID,
		"SignatureMethod":  "HMAC-SHA1",
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"SignatureVersion": "1.0",
		"SignatureNonce":   nonce,
	}
	for k, v := range extra {
		params[k] = v
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		parts = append(parts, aliPercentEncode(k)+"="+aliPercentEncode(params[k]))
	}
	canonicalQuery := strings.Join(parts, "&")
	stringToSign := "POST&" + aliPercentEncode("/") + "&" + aliPercentEncode(canonicalQuery)

	mac := hmac.New(sha1.New, []byte(c.account.AccessKeySecret+"&"))
	_, _ = mac.Write([]byte(stringToSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	bodyStr := canonicalQuery + "&Signature=" + aliPercentEncode(sig)
	endpoint := c.endpoint
	if endpoint == "" {
		endpoint = defaultAlidnsEndpoint
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(bodyStr))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var apiErr struct {
		Code      string `json:"Code"`
		Message   string `json:"Message"`
		RequestId string `json:"RequestId"`
	}
	if json.Unmarshal(raw, &apiErr) == nil && apiErr.Code != "" {
		return fmt.Errorf("alidns %s (%s): %s", action, apiErr.Code, apiErr.Message)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("alidns %s failed (%d): %s", action, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return err
		}
	}
	return nil
}

func aliPercentEncode(s string) string {
	esc := url.QueryEscape(s)
	esc = strings.ReplaceAll(esc, "+", "%20")
	esc = strings.ReplaceAll(esc, "*", "%2A")
	esc = strings.ReplaceAll(esc, "%7E", "~")
	return esc
}
