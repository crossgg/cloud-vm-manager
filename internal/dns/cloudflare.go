package dns

import (
	"bytes"
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

type CloudflareService struct {
	accounts map[string]config.DNSAccountConfig
	bindings []config.DNSBinding
	client   *http.Client
}

func NewCloudflareService(cfg *config.Config) *CloudflareService {
	accounts := make(map[string]config.DNSAccountConfig)
	var bindings []config.DNSBinding
	if cfg != nil {
		for _, a := range cfg.DNSAccounts {
			if a.Name == "" {
				continue
			}
			acc := a
			if acc.Provider == "" {
				acc.Provider = "cloudflare"
			}
			accounts[acc.Name] = acc
		}
		for _, cf := range cfg.Cloudflare {
			if cf.Name == "" {
				continue
			}
			if _, exists := accounts[cf.Name]; exists {
				continue
			}
			accounts[cf.Name] = config.DNSAccountConfig{
				Name:     cf.Name,
				Provider: "cloudflare",
				APIToken: cf.APIToken,
				ZoneID:   cf.ZoneID,
				Domains:  cf.Domains,
			}
		}
		bindings = append(bindings, cfg.DNSBindings...)
	}
	return &CloudflareService{
		accounts: accounts,
		bindings: bindings,
		client:   &http.Client{Timeout: 20 * time.Second},
	}
}

func (s *CloudflareService) ClientFor(accountName string) (DNSClient, error) {
	if s == nil {
		return nil, fmt.Errorf("dns service is nil")
	}
	acc, ok := s.accounts[strings.TrimSpace(accountName)]
	if !ok {
		return nil, fmt.Errorf("dns account %q not found", accountName)
	}
	switch strings.ToLower(strings.TrimSpace(acc.Provider)) {
	case "", "cloudflare":
		return &cloudflareClient{account: acc, client: s.client}, nil
	case "alidns":
		return NewAlidnsClient(acc, s.client), nil
	case "tencent":
		return NewTencentClient(acc, s.client), nil
	case "dnspod":
		return NewDNSPodClient(acc, s.client), nil
	default:
		return nil, fmt.Errorf("unsupported dns provider %q", acc.Provider)
	}
}

func (s *CloudflareService) UpdateForVM(provider, account, vmID, ip string) []string {
	trimmed := strings.TrimSpace(ip)
	if trimmed == "" {
		return nil
	}
	if strings.Contains(trimmed, ":") {
		return s.UpdateForVMByIPs(provider, account, vmID, "", trimmed)
	}
	return s.UpdateForVMByIPs(provider, account, vmID, trimmed, "")
}

func (s *CloudflareService) UpdateForVMByIPs(provider, account, vmID, ipv4, ipv6 string) []string {
	if s == nil || len(s.bindings) == 0 {
		return nil
	}
	var logs []string
	for _, binding := range s.bindings {
		if !binding.Matches(provider, account, vmID) {
			continue
		}
		recType := strings.ToUpper(strings.TrimSpace(binding.Type))
		if recType == "" {
			recType = "A"
		}
		targetIP := strings.TrimSpace(ipv4)
		if recType == "AAAA" {
			targetIP = strings.TrimSpace(ipv6)
		}
		if targetIP == "" {
			continue
		}
		targetAcc := binding.TargetAccountName()
		client, err := s.ClientFor(targetAcc)
		if err != nil {
			logs = append(logs, fmt.Sprintf("DNS update failed (%s %s -> %s): %v", binding.Domain, recType, targetIP, err))
			continue
		}
		if err := client.UpsertBindingRecord(binding, targetIP); err != nil {
			logs = append(logs, fmt.Sprintf("DNS update failed (%s %s -> %s): %v", binding.Domain, recType, targetIP, err))
			continue
		}
		logs = append(logs, fmt.Sprintf("DNS updated (%s): %s [%s] -> %s", targetAcc, binding.Domain, recType, targetIP))
	}
	return logs
}

func (s *CloudflareService) HasBinding(provider, account, vmID string) bool {
	if s == nil {
		return false
	}
	for _, b := range s.bindings {
		if b.Matches(provider, account, vmID) {
			return true
		}
	}
	return false
}

func (s *CloudflareService) BindingTypes(provider, account, vmID string) (hasA, hasAAAA bool) {
	if s == nil {
		return false, false
	}
	for _, b := range s.bindings {
		if !b.Matches(provider, account, vmID) {
			continue
		}
		t := strings.ToUpper(strings.TrimSpace(b.Type))
		if t == "AAAA" {
			hasAAAA = true
		} else {
			hasA = true
		}
	}
	return hasA, hasAAAA
}

type cloudflareClient struct {
	account config.DNSAccountConfig
	client  *http.Client
}

func (c *cloudflareClient) Capabilities() DNSCapabilities {
	return DNSCapabilities{
		Provider:        "cloudflare",
		SupportsProxied: true,
		SupportsLine:    false,
		SupportsStatus:  false,
		SupportsRemark:  true,
	}
}

func (c *cloudflareClient) ResolveConfiguredDomains() ([]DNSDomainInfo, error) {
	if len(c.account.Domains) > 0 {
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
			out = append(out, DNSDomainInfo{Domain: norm, ZoneID: c.account.ZoneID})
		}
		if len(out) > 0 {
			return out, nil
		}
	}

	zoneID := strings.TrimSpace(c.account.ZoneID)
	if zoneID == "" {
		return nil, nil
	}
	// Resolve only the single configured Zone ID; never enumerate all account zones.
	var resp struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Result struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
	}
	endpoint := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s", url.PathEscape(zoneID))
	if err := c.doJSON(http.MethodGet, endpoint, nil, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("cloudflare zone lookup failed: %v", resp.Errors)
	}
	name := normalizeRootDomain(resp.Result.Name)
	if name == "" {
		return nil, nil
	}
	return []DNSDomainInfo{{Domain: name, ZoneID: zoneID}}, nil
}

func (c *cloudflareClient) ListRecords(domain string) ([]DNSRecord, error) {
	zoneID := strings.TrimSpace(c.account.ZoneID)
	if zoneID == "" {
		return nil, fmt.Errorf("cloudflare zone_id is missing")
	}
	root := normalizeRootDomain(domain)
	if root == "" && len(c.account.Domains) > 0 {
		root = normalizeRootDomain(c.account.Domains[0])
	}

	var out []DNSRecord
	page := 1
	const perPage = 100
	for {
		var resp struct {
			Success bool `json:"success"`
			Errors  []struct {
				Message string `json:"message"`
			} `json:"errors"`
			ResultInfo struct {
				TotalPages int `json:"total_pages"`
			} `json:"result_info"`
			Result []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Name     string `json:"name"`
				Content  string `json:"content"`
				TTL      int    `json:"ttl"`
				Priority *int   `json:"priority,omitempty"`
				Proxied  bool   `json:"proxied"`
				Comment  string `json:"comment,omitempty"`
			} `json:"result"`
		}

		endpoint := fmt.Sprintf(
			"https://api.cloudflare.com/client/v4/zones/%s/dns_records?per_page=%d&page=%d",
			url.PathEscape(zoneID), perPage, page,
		)
		if err := c.doJSON(http.MethodGet, endpoint, nil, &resp); err != nil {
			return nil, err
		}
		if !resp.Success {
			return nil, fmt.Errorf("cloudflare list records failed: %v", resp.Errors)
		}

		for _, item := range resp.Result {
			fqdn := normalizeRootDomain(item.Name)
			recDomain := root
			if recDomain == "" {
				recDomain = fqdn
			}
			rr := splitFQDNToRR(fqdn, recDomain)
			out = append(out, DNSRecord{
				ID:       item.ID,
				Domain:   recDomain,
				Name:     rr,
				FQDN:     fqdn,
				Type:     strings.ToUpper(strings.TrimSpace(item.Type)),
				Value:    item.Content,
				TTL:      item.TTL,
				Priority: item.Priority,
				Proxied:  item.Proxied,
				Status:   "ENABLE",
				Remark:   item.Comment,
			})
		}

		if len(resp.Result) == 0 || (resp.ResultInfo.TotalPages > 0 && page >= resp.ResultInfo.TotalPages) || len(resp.Result) < perPage {
			break
		}
		page++
	}
	return out, nil
}

func (c *cloudflareClient) CreateRecord(domain string, rec DNSRecord) (*DNSRecord, error) {
	zoneID := strings.TrimSpace(c.account.ZoneID)
	if zoneID == "" {
		return nil, fmt.Errorf("cloudflare zone_id is missing")
	}
	root := normalizeRootDomain(domain)
	recType := strings.ToUpper(strings.TrimSpace(rec.Type))
	fqdn := rrToFQDN(rec.Name, root)
	if fqdn == "" {
		fqdn = strings.TrimSpace(rec.Name)
	}
	ttl := rec.TTL
	if ttl <= 0 {
		ttl = 1
	}

	payload := map[string]interface{}{
		"type":    recType,
		"name":    fqdn,
		"content": rec.Value,
		"ttl":     ttl,
	}
	if supportsCFProxy(recType) {
		payload["proxied"] = rec.Proxied
	}
	if rec.Priority != nil && (recType == "MX" || recType == "SRV") {
		payload["priority"] = *rec.Priority
	}
	if strings.TrimSpace(rec.Remark) != "" {
		payload["comment"] = strings.TrimSpace(rec.Remark)
	}

	var resp struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Result struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Name     string `json:"name"`
			Content  string `json:"content"`
			TTL      int    `json:"ttl"`
			Priority *int   `json:"priority,omitempty"`
			Proxied  bool   `json:"proxied"`
			Comment  string `json:"comment,omitempty"`
		} `json:"result"`
	}

	endpoint := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", url.PathEscape(zoneID))
	if err := c.doJSON(http.MethodPost, endpoint, payload, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("cloudflare create failed: %v", resp.Errors)
	}

	resDomain := root
	if resDomain == "" {
		resDomain = normalizeRootDomain(resp.Result.Name)
	}
	return &DNSRecord{
		ID:       resp.Result.ID,
		Domain:   resDomain,
		Name:     splitFQDNToRR(resp.Result.Name, resDomain),
		FQDN:     normalizeRootDomain(resp.Result.Name),
		Type:     strings.ToUpper(resp.Result.Type),
		Value:    resp.Result.Content,
		TTL:      resp.Result.TTL,
		Priority: resp.Result.Priority,
		Proxied:  resp.Result.Proxied,
		Status:   "ENABLE",
		Remark:   resp.Result.Comment,
	}, nil
}

func (c *cloudflareClient) UpdateRecord(domain string, recordID string, rec DNSRecord) (*DNSRecord, error) {
	zoneID := strings.TrimSpace(c.account.ZoneID)
	if zoneID == "" || recordID == "" {
		return nil, fmt.Errorf("cloudflare zone_id and record id are required")
	}
	root := normalizeRootDomain(domain)
	recType := strings.ToUpper(strings.TrimSpace(rec.Type))
	fqdn := rrToFQDN(rec.Name, root)
	if fqdn == "" {
		fqdn = strings.TrimSpace(rec.Name)
	}
	ttl := rec.TTL
	if ttl <= 0 {
		ttl = 1
	}

	payload := map[string]interface{}{
		"type":    recType,
		"name":    fqdn,
		"content": rec.Value,
		"ttl":     ttl,
	}
	if supportsCFProxy(recType) {
		payload["proxied"] = rec.Proxied
	}
	if rec.Priority != nil && (recType == "MX" || recType == "SRV") {
		payload["priority"] = *rec.Priority
	}
	if rec.Remark != "" {
		payload["comment"] = rec.Remark
	}

	var resp struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Result struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Name     string `json:"name"`
			Content  string `json:"content"`
			TTL      int    `json:"ttl"`
			Priority *int   `json:"priority,omitempty"`
			Proxied  bool   `json:"proxied"`
			Comment  string `json:"comment,omitempty"`
		} `json:"result"`
	}

	endpoint := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s",
		url.PathEscape(zoneID), url.PathEscape(recordID))
	if err := c.doJSON(http.MethodPut, endpoint, payload, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("cloudflare update failed: %v", resp.Errors)
	}

	resDomain := root
	if resDomain == "" {
		resDomain = normalizeRootDomain(resp.Result.Name)
	}
	return &DNSRecord{
		ID:       resp.Result.ID,
		Domain:   resDomain,
		Name:     splitFQDNToRR(resp.Result.Name, resDomain),
		FQDN:     normalizeRootDomain(resp.Result.Name),
		Type:     strings.ToUpper(resp.Result.Type),
		Value:    resp.Result.Content,
		TTL:      resp.Result.TTL,
		Priority: resp.Result.Priority,
		Proxied:  resp.Result.Proxied,
		Status:   "ENABLE",
		Remark:   resp.Result.Comment,
	}, nil
}

func (c *cloudflareClient) DeleteRecord(domain string, recordID string) error {
	zoneID := strings.TrimSpace(c.account.ZoneID)
	if zoneID == "" || recordID == "" {
		return fmt.Errorf("cloudflare zone_id and record id are required")
	}
	var resp struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	endpoint := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s",
		url.PathEscape(zoneID), url.PathEscape(recordID))
	if err := c.doJSON(http.MethodDelete, endpoint, nil, &resp); err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("cloudflare delete failed: %v", resp.Errors)
	}
	return nil
}

func (c *cloudflareClient) SetRecordStatus(domain string, recordID string, enabled bool) error {
	return fmt.Errorf("cloudflare does not support pause/enable record status toggle")
}

func (c *cloudflareClient) UpsertBindingRecord(binding config.DNSBinding, ip string) error {
	zoneID := strings.TrimSpace(c.account.ZoneID)
	if strings.TrimSpace(c.account.APIToken) == "" || zoneID == "" {
		return fmt.Errorf("cloudflare credentials are incomplete")
	}
	recType := strings.ToUpper(strings.TrimSpace(binding.Type))
	if recType == "" {
		recType = "A"
	}
	ttl := binding.TTL
	if ttl <= 0 {
		ttl = 1
	}

	fqdn := binding.Domain
	if len(c.account.Domains) > 0 {
		root, rr, err := matchConfiguredDomain(binding.Domain, c.account.Domains)
		if err != nil {
			return err
		}
		fqdn = rrToFQDN(rr, root)
	}

	existingID, err := c.findRecordID(fqdn, recType)
	if err != nil {
		return err
	}

	payload := map[string]interface{}{
		"type":    recType,
		"name":    fqdn,
		"content": ip,
		"ttl":     ttl,
		"proxied": binding.Proxied,
	}

	method := http.MethodPost
	endpoint := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", url.PathEscape(zoneID))
	if existingID != "" {
		method = http.MethodPut
		endpoint = fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s",
			url.PathEscape(zoneID), url.PathEscape(existingID))
	}

	var resp struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := c.doJSON(method, endpoint, payload, &resp); err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("cloudflare update failed: %v", resp.Errors)
	}
	return nil
}

func (c *cloudflareClient) findRecordID(name, recType string) (string, error) {
	endpoint := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/zones/%s/dns_records?type=%s&name=%s",
		url.PathEscape(c.account.ZoneID),
		url.QueryEscape(recType),
		url.QueryEscape(name),
	)
	var resp struct {
		Success bool `json:"success"`
		Result  []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := c.doJSON(http.MethodGet, endpoint, nil, &resp); err != nil {
		return "", err
	}
	if len(resp.Result) > 0 {
		return resp.Result[0].ID, nil
	}
	return "", nil
}

func (c *cloudflareClient) doJSON(method, endpoint string, body interface{}, out interface{}) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.account.APIToken)
	req.Header.Set("Content-Type", "application/json")
	client := c.client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("cloudflare api %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return err
		}
	}
	return nil
}

func supportsCFProxy(recType string) bool {
	switch strings.ToUpper(strings.TrimSpace(recType)) {
	case "A", "AAAA", "CNAME":
		return true
	default:
		return false
	}
}

func parsePositiveInt(s string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
