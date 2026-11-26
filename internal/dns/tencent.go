package dns

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

const (
	tencentDNSPodHostCN   = "dnspod.tencentcloudapi.com"
	tencentDNSPodHostIntl = "dnspod.intl.tencentcloudapi.com"
	tencentDNSPodVersion  = "2021-03-23"
)

type TencentClient struct {
	account config.DNSAccountConfig
	client  *http.Client
	nowFunc func() time.Time
}

func NewTencentClient(account config.DNSAccountConfig, httpClient *http.Client) *TencentClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &TencentClient{
		account: account,
		client:  httpClient,
		nowFunc: time.Now,
	}
}

func (c *TencentClient) isIntl() bool {
	return strings.EqualFold(strings.TrimSpace(c.account.Edition), "intl")
}

func (c *TencentClient) host() string {
	if c.isIntl() {
		return tencentDNSPodHostIntl
	}
	return tencentDNSPodHostCN
}

func (c *TencentClient) defaultLine() string {
	if c.isIntl() {
		return "Default"
	}
	return "默认"
}

func (c *TencentClient) Capabilities() DNSCapabilities {
	edition := "cn"
	lines := []string{"默认", "电信", "联通", "移动", "教育网", "境外", "搜索引擎", "百度", "Google"}
	if c.isIntl() {
		edition = "intl"
		lines = []string{"Default", "Mainland China", "Overseas"}
	}
	return DNSCapabilities{
		Provider:        "tencent",
		Edition:         edition,
		SupportsProxied: false,
		SupportsLine:    true,
		SupportsStatus:  true,
		SupportsRemark:  true,
		DefaultLine:     c.defaultLine(),
		AvailableLines:  lines,
	}
}

func (c *TencentClient) ResolveConfiguredDomains() ([]DNSDomainInfo, error) {
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

func (c *TencentClient) ListRecords(domain string) ([]DNSRecord, error) {
	root := normalizeRootDomain(domain)
	if root == "" {
		return nil, fmt.Errorf("domain is required")
	}

	var records []DNSRecord
	offset := 0
	const limit = 3000
	for {
		var resp struct {
			Response struct {
				RecordCountInfo struct {
					TotalCount int `json:"TotalCount"`
				} `json:"RecordCountInfo"`
				RecordList []struct {
					RecordId uint64 `json:"RecordId"`
					Name     string `json:"Name"`
					Type     string `json:"Type"`
					Value    string `json:"Value"`
					TTL      int    `json:"TTL"`
					MX       int    `json:"MX"`
					Line     string `json:"Line"`
					LineId   string `json:"LineId"`
					Status   string `json:"Status"`
					Remark   string `json:"Remark"`
				} `json:"RecordList"`
			} `json:"Response"`
		}

		err := c.doTC3("DescribeRecordList", map[string]interface{}{
			"Domain": root,
			"Offset": offset,
			"Limit":  limit,
		}, &resp)
		if err != nil {
			if strings.Contains(err.Error(), "ResourceNotFound.NoDataOfRecord") {
				return records, nil
			}
			return nil, err
		}

		for _, item := range resp.Response.RecordList {
			t := strings.ToUpper(strings.TrimSpace(item.Type))
			var prio *int
			if t == "MX" || item.MX > 0 {
				p := item.MX
				prio = &p
			}
			status := "ENABLE"
			if strings.EqualFold(item.Status, "DISABLE") {
				status = "DISABLE"
			}
			rr := item.Name
			if rr == "" {
				rr = "@"
			}
			records = append(records, DNSRecord{
				ID:       strconv.FormatUint(item.RecordId, 10),
				Domain:   root,
				Name:     rr,
				FQDN:     rrToFQDN(rr, root),
				Type:     t,
				Value:    item.Value,
				TTL:      item.TTL,
				Priority: prio,
				Line:     item.Line,
				LineID:   item.LineId,
				Status:   status,
				Remark:   item.Remark,
			})
		}

		if len(resp.Response.RecordList) == 0 || len(records) >= resp.Response.RecordCountInfo.TotalCount {
			break
		}
		offset += len(resp.Response.RecordList)
	}
	return records, nil
}

func (c *TencentClient) CreateRecord(domain string, rec DNSRecord) (*DNSRecord, error) {
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
		line = c.defaultLine()
	}
	ttl := rec.TTL
	if ttl <= 0 {
		ttl = 600
	}

	payload := map[string]interface{}{
		"Domain":     root,
		"SubDomain":  rr,
		"RecordType": recType,
		"RecordLine": line,
		"Value":      rec.Value,
		"TTL":        ttl,
	}
	if recType == "MX" {
		p := 10
		if rec.Priority != nil && *rec.Priority > 0 {
			p = *rec.Priority
		}
		payload["MX"] = p
	}
	if strings.EqualFold(rec.Status, "DISABLE") {
		payload["Status"] = "DISABLE"
	} else {
		payload["Status"] = "ENABLE"
	}

	var resp struct {
		Response struct {
			RecordId uint64 `json:"RecordId"`
		} `json:"Response"`
	}
	if err := c.doTC3("CreateRecord", payload, &resp); err != nil {
		return nil, err
	}

	if strings.TrimSpace(rec.Remark) != "" && resp.Response.RecordId > 0 {
		_ = c.doTC3("ModifyRecordRemark", map[string]interface{}{
			"Domain":   root,
			"RecordId": resp.Response.RecordId,
			"Remark":   rec.Remark,
		}, nil)
	}

	created := rec
	created.ID = strconv.FormatUint(resp.Response.RecordId, 10)
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

func (c *TencentClient) UpdateRecord(domain string, recordID string, rec DNSRecord) (*DNSRecord, error) {
	root := normalizeRootDomain(domain)
	if root == "" {
		return nil, fmt.Errorf("domain is required")
	}
	idNum, err := strconv.ParseUint(strings.TrimSpace(recordID), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid tencent record id %q: %w", recordID, err)
	}
	rr := splitFQDNToRR(rec.Name, root)
	recType := strings.ToUpper(strings.TrimSpace(rec.Type))
	line := strings.TrimSpace(rec.Line)
	if line == "" {
		line = c.defaultLine()
	}
	ttl := rec.TTL
	if ttl <= 0 {
		ttl = 600
	}

	payload := map[string]interface{}{
		"Domain":     root,
		"RecordId":   idNum,
		"SubDomain":  rr,
		"RecordType": recType,
		"RecordLine": line,
		"Value":      rec.Value,
		"TTL":        ttl,
	}
	if recType == "MX" {
		p := 10
		if rec.Priority != nil && *rec.Priority > 0 {
			p = *rec.Priority
		}
		payload["MX"] = p
	}
	if strings.EqualFold(rec.Status, "DISABLE") {
		payload["Status"] = "DISABLE"
	} else if strings.EqualFold(rec.Status, "ENABLE") {
		payload["Status"] = "ENABLE"
	}

	if err := c.doTC3("ModifyRecord", payload, nil); err != nil {
		return nil, err
	}

	if rec.Remark != "" {
		_ = c.doTC3("ModifyRecordRemark", map[string]interface{}{
			"Domain":   root,
			"RecordId": idNum,
			"Remark":   rec.Remark,
		}, nil)
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

func (c *TencentClient) DeleteRecord(domain string, recordID string) error {
	root := normalizeRootDomain(domain)
	idNum, err := strconv.ParseUint(strings.TrimSpace(recordID), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid tencent record id %q: %w", recordID, err)
	}
	return c.doTC3("DeleteRecord", map[string]interface{}{
		"Domain":   root,
		"RecordId": idNum,
	}, nil)
}

func (c *TencentClient) SetRecordStatus(domain string, recordID string, enabled bool) error {
	root := normalizeRootDomain(domain)
	idNum, err := strconv.ParseUint(strings.TrimSpace(recordID), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid tencent record id %q: %w", recordID, err)
	}
	status := "DISABLE"
	if enabled {
		status = "ENABLE"
	}
	return c.doTC3("ModifyRecordStatus", map[string]interface{}{
		"Domain":   root,
		"RecordId": idNum,
		"Status":   status,
	}, nil)
}

func (c *TencentClient) UpsertBindingRecord(binding config.DNSBinding, ip string) error {
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
				line = c.defaultLine()
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
		Line:   c.defaultLine(),
		Status: "ENABLE",
	})
	return err
}

func (c *TencentClient) doTC3(action string, payload interface{}, out interface{}) error {
	if strings.TrimSpace(c.account.SecretID) == "" || strings.TrimSpace(c.account.SecretKey) == "" {
		return fmt.Errorf("tencent cloud credentials are missing")
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	host := c.host()
	now := c.nowFunc().UTC()
	timestamp := now.Unix()
	date := now.Format("2006-01-02")

	contentType := "application/json; charset=utf-8"
	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\nx-tc-action:%s\n",
		contentType, host, strings.ToLower(action))
	signedHeaders := "content-type;host;x-tc-action"
	hashedPayload := sha256Hex(payloadBytes)
	canonicalRequest := fmt.Sprintf("POST\n/\n\n%s\n%s\n%s",
		canonicalHeaders, signedHeaders, hashedPayload)

	credentialScope := fmt.Sprintf("%s/dnspod/tc3_request", date)
	stringToSign := fmt.Sprintf("TC3-HMAC-SHA256\n%d\n%s\n%s",
		timestamp, credentialScope, sha256Hex([]byte(canonicalRequest)))

	secretDate := hmacSHA256([]byte("TC3"+c.account.SecretKey), date)
	secretService := hmacSHA256(secretDate, "dnspod")
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))

	authorization := fmt.Sprintf(
		"TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		c.account.SecretID, credentialScope, signedHeaders, signature,
	)

	req, err := http.NewRequest(http.MethodPost, "https://"+host+"/", bytes.NewReader(payloadBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", tencentDNSPodVersion)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("Authorization", authorization)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var errEnvelope struct {
		Response struct {
			Error *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"Response"`
	}
	if json.Unmarshal(raw, &errEnvelope) == nil && errEnvelope.Response.Error != nil {
		return fmt.Errorf("tencent %s (%s): %s", action, errEnvelope.Response.Error.Code, errEnvelope.Response.Error.Message)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("tencent %s failed (%d): %s", action, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return err
		}
	}
	return nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, msg string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(msg))
	return mac.Sum(nil)
}
