package dns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

type CloudflareService struct {
	accounts map[string]config.CloudflareConfig
	bindings []config.DNSBinding
	client   *http.Client
}

func NewCloudflareService(cfg *config.Config) *CloudflareService {
	accounts := make(map[string]config.CloudflareConfig)
	if cfg != nil {
		for _, account := range cfg.Cloudflare {
			accounts[account.Name] = account
		}
	}
	var bindings []config.DNSBinding
	if cfg != nil {
		bindings = cfg.DNSBindings
	}
	return &CloudflareService{
		accounts: accounts,
		bindings: bindings,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (cf *CloudflareService) UpdateForVM(provider, account, vmID, ip string) []string {
	if cf == nil || strings.TrimSpace(ip) == "" {
		return nil
	}
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed != nil && parsed.To4() == nil {
		return cf.UpdateForVMByIPs(provider, account, vmID, "", strings.TrimSpace(ip))
	}
	return cf.UpdateForVMByIPs(provider, account, vmID, strings.TrimSpace(ip), "")
}

func (cf *CloudflareService) UpdateForVMByIPs(provider, account, vmID, ipv4, ipv6 string) []string {
	ipv4 = strings.TrimSpace(ipv4)
	ipv6 = strings.TrimSpace(ipv6)
	if cf == nil || (ipv4 == "" && ipv6 == "") {
		return nil
	}

	var logs []string
	for _, binding := range cf.bindings {
		if !bindingMatches(binding, provider, account, vmID) {
			continue
		}

		recordType := strings.ToUpper(strings.TrimSpace(binding.Type))
		if recordType == "" {
			recordType = "A"
		}

		targetIP := ""
		switch recordType {
		case "A":
			if ipv4 == "" {
				continue
			}
			parsed := net.ParseIP(ipv4)
			if parsed == nil || parsed.To4() == nil {
				logs = append(logs, fmt.Sprintf("跳过 %s (A): %q 不是有效的 IPv4 地址", binding.Domain, ipv4))
				continue
			}
			targetIP = ipv4
		case "AAAA":
			if ipv6 == "" {
				continue
			}
			parsed := net.ParseIP(ipv6)
			if parsed == nil || parsed.To4() != nil {
				logs = append(logs, fmt.Sprintf("跳过 %s (AAAA): %q 不是有效的 IPv6 地址", binding.Domain, ipv6))
				continue
			}
			targetIP = ipv6
		default:
			logs = append(logs, fmt.Sprintf("跳过 %s (%s): 不支持的自动 IP 记录类型", binding.Domain, recordType))
			continue
		}

		cfAccount, ok := cf.accounts[binding.Cloudflare]
		if !ok {
			logs = append(logs, fmt.Sprintf("Cloudflare 配置 %s 不存在，跳过 %s", binding.Cloudflare, binding.Domain))
			continue
		}

		if err := cf.upsertRecord(cfAccount, binding, targetIP); err != nil {
			logs = append(logs, fmt.Sprintf("Cloudflare 更新失败 %s (%s) -> %s: %v", binding.Domain, recordType, targetIP, err))
			continue
		}
		logs = append(logs, fmt.Sprintf("Cloudflare 已更新 %s (%s) -> %s", binding.Domain, recordType, targetIP))
	}
	return logs
}

func (cf *CloudflareService) HasBinding(provider, account, vmID string) bool {
	if cf == nil {
		return false
	}
	for _, binding := range cf.bindings {
		if bindingMatches(binding, provider, account, vmID) {
			return true
		}
	}
	return false
}

func (cf *CloudflareService) BindingTypes(provider, account, vmID string) (hasA bool, hasAAAA bool) {
	if cf == nil {
		return false, false
	}
	for _, binding := range cf.bindings {
		if !bindingMatches(binding, provider, account, vmID) {
			continue
		}
		recordType := strings.ToUpper(strings.TrimSpace(binding.Type))
		if recordType == "" || recordType == "A" {
			hasA = true
		} else if recordType == "AAAA" {
			hasAAAA = true
		}
	}
	return hasA, hasAAAA
}

func bindingMatches(binding config.DNSBinding, provider, account, vmID string) bool {
	if binding.Provider != "" && !strings.EqualFold(binding.Provider, provider) {
		return false
	}
	if binding.Account != "" && binding.Account != account {
		return false
	}
	return binding.VM == "" || binding.VM == vmID
}

func (cf *CloudflareService) upsertRecord(account config.CloudflareConfig, binding config.DNSBinding, ip string) error {
	if account.APIToken == "" {
		return fmt.Errorf("api_token is empty")
	}
	if account.ZoneID == "" {
		return fmt.Errorf("zone_id is empty")
	}
	if binding.Domain == "" {
		return fmt.Errorf("domain is empty")
	}

	recordType := binding.Type
	if recordType == "" {
		recordType = "A"
	}
	recordID, err := cf.findRecordID(account, recordType, binding.Domain)
	if err != nil {
		return err
	}

	payload := map[string]interface{}{
		"type":    recordType,
		"name":    binding.Domain,
		"content": ip,
		"ttl":     binding.TTL,
		"proxied": binding.Proxied,
	}
	if binding.TTL == 0 {
		payload["ttl"] = 1
	}

	method := "POST"
	endpoint := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", url.PathEscape(account.ZoneID))
	if recordID != "" {
		method = "PATCH"
		endpoint = fmt.Sprintf("%s/%s", endpoint, url.PathEscape(recordID))
	}

	return cf.cloudflareJSON(account.APIToken, method, endpoint, payload, nil)
}

func (cf *CloudflareService) findRecordID(account config.CloudflareConfig, recordType, domain string) (string, error) {
	query := url.Values{
		"type": {recordType},
		"name": {domain},
	}
	endpoint := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/zones/%s/dns_records?%s",
		url.PathEscape(account.ZoneID),
		query.Encode(),
	)

	var records []struct {
		ID string `json:"id"`
	}
	if err := cf.cloudflareJSON(account.APIToken, "GET", endpoint, nil, &records); err != nil {
		return "", err
	}
	if len(records) == 0 {
		return "", nil
	}
	return records[0].ID, nil
}

func (cf *CloudflareService) cloudflareJSON(token, method, endpoint string, body interface{}, out interface{}) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := cf.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Cloudflare API returned %d: %s", resp.StatusCode, string(data))
	}

	var envelope struct {
		Success bool            `json:"success"`
		Errors  []interface{}   `json:"errors"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	if !envelope.Success {
		return fmt.Errorf("Cloudflare API error: %v", envelope.Errors)
	}
	if out != nil && len(envelope.Result) > 0 {
		return json.Unmarshal(envelope.Result, out)
	}
	return nil
}
