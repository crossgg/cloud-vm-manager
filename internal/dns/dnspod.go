package dns

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

const (
	dnspodBaseCN   = "https://dnsapi.cn"
	dnspodBaseIntl = "https://api.dnspod.com"
)

type DNSPodClient struct {
	account config.DNSAccountConfig
	client  *http.Client
}

func NewDNSPodClient(account config.DNSAccountConfig, httpClient *http.Client) *DNSPodClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &DNSPodClient{
		account: account,
		client:  httpClient,
	}
}

func (c *DNSPodClient) isIntl() bool {
	return strings.EqualFold(strings.TrimSpace(c.account.Edition), "intl")
}

func (c *DNSPodClient) baseURL() string {
	if c.isIntl() {
		return dnspodBaseIntl
	}
	return dnspodBaseCN
}

func (c *DNSPodClient) defaultLine() string {
	if c.isIntl() {
		return "Default"
	}
	return "默认"
}

func (c *DNSPodClient) Capabilities() DNSCapabilities {
	edition := "cn"
	lines := []string{"默认", "电信", "联通", "移动", "教育网", "境外", "搜索引擎"}
	if c.isIntl() {
		edition = "intl"
		lines = []string{"Default", "Mainland China", "Overseas"}
	}
	return DNSCapabilities{
		Provider:        "dnspod",
		Edition:         edition,
		SupportsProxied: false,
		SupportsLine:    true,
		SupportsStatus:  true,
		SupportsRemark:  true,
		DefaultLine:     c.defaultLine(),
		AvailableLines:  lines,
	}
}

func (c *DNSPodClient) ResolveConfiguredDomains() ([]DNSDomainInfo, error) {
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

func (c *DNSPodClient) ListRecords(domain string) ([]DNSRecord, error) {
	root := normalizeRootDomain(domain)
	if root == "" {
		return nil, fmt.Errorf("domain is required")
	}

	var records []DNSRecord
	offset := 0
	const length = 500
	for {
		var resp struct {
			Status struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"status"`
			Info struct {
				RecordTotal json.Number `json:"record_total"`
			} `json:"info"`
			Records []struct {
				ID      string      `json:"id"`
				Name    string      `json:"name"`
				Type    string      `json:"type"`
				Value   string      `json:"value"`
				TTL     json.Number `json:"ttl"`
				MX      json.Number `json:"mx"`
				Line    string      `json:"line"`
				LineID  string      `json:"line_id"`
				Enabled string      `json:"enabled"`
				Status  string      `json:"status"`
				Remark  string      `json:"remark"`
			} `json:"records"`
		}

		err := c.doForm("/Record.List", map[string]string{
			"domain": root,
			"offset": strconv.Itoa(offset),
			"length": strconv.Itoa(length),
		}, &resp)
		if err != nil {
			if strings.Contains(err.Error(), "(code 10)") {
				return records, nil
			}
			return nil, err
		}

		for _, item := range resp.Records {
			t := strings.ToUpper(strings.TrimSpace(item.Type))
			ttlVal, _ := strconv.Atoi(item.TTL.String())
			mxVal, _ := strconv.Atoi(item.MX.String())
			var prio *int
			if t == "MX" || mxVal > 0 {
				p := mxVal
				prio = &p
			}
			status := "ENABLE"
			if item.Enabled == "0" || strings.EqualFold(item.Status, "disable") {
				status = "DISABLE"
			}
			rr := item.Name
			if rr == "" {
				rr = "@"
			}
			records = append(records, DNSRecord{
				ID:       item.ID,
				Domain:   root,
				Name:     rr,
				FQDN:     rrToFQDN(rr, root),
				Type:     t,
				Value:    item.Value,
				TTL:      ttlVal,
				Priority: prio,
				Line:     item.Line,
				LineID:   item.LineID,
				Status:   status,
				Remark:   item.Remark,
			})
		}

		total, _ := strconv.Atoi(resp.Info.RecordTotal.String())
		if len(resp.Records) == 0 || (total > 0 && len(records) >= total) || len(resp.Records) < length {
			break
		}
		offset += len(resp.Records)
	}
	return records, nil
}

func (c *DNSPodClient) CreateRecord(domain string, rec DNSRecord) (*DNSRecord, error) {
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

	params := map[string]string{
		"domain":      root,
		"sub_domain":  rr,
		"record_type": recType,
		"record_line": line,
		"value":       rec.Value,
		"ttl":         strconv.Itoa(ttl),
	}
	if recType == "MX" {
		p := 10
		if rec.Priority != nil && *rec.Priority > 0 {
			p = *rec.Priority
		}
		params["mx"] = strconv.Itoa(p)
	}
	if strings.EqualFold(rec.Status, "DISABLE") {
		params["status"] = "disable"
	} else {
		params["status"] = "enable"
	}

	var resp struct {
		Record struct {
			ID json.Number `json:"id"`
		} `json:"record"`
	}
	if err := c.doForm("/Record.Create", params, &resp); err != nil {
		return nil, err
	}

	recID := resp.Record.ID.String()
	if strings.TrimSpace(rec.Remark) != "" && recID != "" {
		_ = c.doForm("/Record.Remark", map[string]string{
			"domain":    root,
			"record_id": recID,
			"remark":    rec.Remark,
		}, nil)
	}

	created := rec
	created.ID = recID
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

func (c *DNSPodClient) UpdateRecord(domain string, recordID string, rec DNSRecord) (*DNSRecord, error) {
	root := normalizeRootDomain(domain)
	if root == "" || recordID == "" {
		return nil, fmt.Errorf("domain and record id are required")
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

	params := map[string]string{
		"domain":      root,
		"record_id":   recordID,
		"sub_domain":  rr,
		"record_type": recType,
		"record_line": line,
		"value":       rec.Value,
		"ttl":         strconv.Itoa(ttl),
	}
	if recType == "MX" {
		p := 10
		if rec.Priority != nil && *rec.Priority > 0 {
			p = *rec.Priority
		}
		params["mx"] = strconv.Itoa(p)
	}
	if strings.EqualFold(rec.Status, "DISABLE") {
		params["status"] = "disable"
	} else if strings.EqualFold(rec.Status, "ENABLE") {
		params["status"] = "enable"
	}

	if err := c.doForm("/Record.Modify", params, nil); err != nil {
		return nil, err
	}
	if rec.Remark != "" {
		_ = c.doForm("/Record.Remark", map[string]string{
			"domain":    root,
			"record_id": recordID,
			"remark":    rec.Remark,
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

func (c *DNSPodClient) DeleteRecord(domain string, recordID string) error {
	root := normalizeRootDomain(domain)
	if root == "" || recordID == "" {
		return fmt.Errorf("domain and record id are required")
	}
	return c.doForm("/Record.Remove", map[string]string{
		"domain":    root,
		"record_id": recordID,
	}, nil)
}

func (c *DNSPodClient) SetRecordStatus(domain string, recordID string, enabled bool) error {
	root := normalizeRootDomain(domain)
	if root == "" || recordID == "" {
		return fmt.Errorf("domain and record id are required")
	}
	status := "disable"
	if enabled {
		status = "enable"
	}
	return c.doForm("/Record.Status", map[string]string{
		"domain":    root,
		"record_id": recordID,
		"status":    status,
	}, nil)
}

func (c *DNSPodClient) UpsertBindingRecord(binding config.DNSBinding, ip string) error {
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

func (c *DNSPodClient) doForm(path string, params map[string]string, out interface{}) error {
	if strings.TrimSpace(c.account.TokenID) == "" || strings.TrimSpace(c.account.Token) == "" {
		return fmt.Errorf("dnspod token_id and token are required")
	}

	form := url.Values{}
	form.Set("login_token", strings.TrimSpace(c.account.TokenID)+","+strings.TrimSpace(c.account.Token))
	form.Set("format", "json")
	if c.isIntl() {
		form.Set("lang", "en")
	} else {
		form.Set("lang", "cn")
	}
	for k, v := range params {
		form.Set(k, v)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL()+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "CloudVMManager/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var statusEnvelope struct {
		Status struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"status"`
	}
	if err := json.Unmarshal(raw, &statusEnvelope); err == nil && statusEnvelope.Status.Code != "" && statusEnvelope.Status.Code != "1" {
		return fmt.Errorf("dnspod %s (code %s): %s", path, statusEnvelope.Status.Code, statusEnvelope.Status.Message)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("dnspod %s failed (%d): %s", path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return err
		}
	}
	return nil
}
