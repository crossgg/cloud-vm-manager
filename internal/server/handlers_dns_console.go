package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/crossgg/cloud-vm-manager/internal/dns"
	"github.com/gin-gonic/gin"
)

type dnsAccountView struct {
	Name            string              `json:"name"`
	Provider        string              `json:"provider"`
	Edition         string              `json:"edition,omitempty"`
	Remark          string              `json:"remark,omitempty"`
	Domains         []string            `json:"domains"`
	ZoneID          string              `json:"zone_id,omitempty"`
	AccessKeyID     string              `json:"access_key_id,omitempty"`
	SecretID        string              `json:"secret_id,omitempty"`
	TokenID         string              `json:"token_id,omitempty"`
	HasSecret       bool                `json:"has_secret"`
	SecretMasked    string              `json:"secret_masked,omitempty"`
	Capabilities    dns.DNSCapabilities `json:"capabilities"`
	SupportsProxied bool                `json:"supportsProxied"`
}

func snapshotDNSConfig() ([]config.DNSAccountConfig, []config.DNSBinding) {
	runtimeState.mu.RLock()
	defer runtimeState.mu.RUnlock()
	if runtimeState.cfg == nil {
		return nil, nil
	}
	accounts := collectMergedDNSAccounts(runtimeState.cfg)
	bindings := append([]config.DNSBinding(nil), runtimeState.cfg.DNSBindings...)
	return accounts, bindings
}

func collectMergedDNSAccounts(cfg *config.Config) []config.DNSAccountConfig {
	if cfg == nil {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]config.DNSAccountConfig, 0, len(cfg.DNSAccounts)+len(cfg.Cloudflare))
	for _, acc := range cfg.DNSAccounts {
		if strings.TrimSpace(acc.Name) == "" {
			continue
		}
		norm := acc
		if norm.Provider == "" {
			norm.Provider = "cloudflare"
		}
		seen[norm.Name] = struct{}{}
		out = append(out, norm)
	}
	for _, cf := range cfg.Cloudflare {
		if strings.TrimSpace(cf.Name) == "" {
			continue
		}
		if _, ok := seen[cf.Name]; ok {
			continue
		}
		seen[cf.Name] = struct{}{}
		out = append(out, config.DNSAccountConfig{
			Name:     cf.Name,
			Provider: "cloudflare",
			APIToken: cf.APIToken,
			ZoneID:   cf.ZoneID,
			Domains:  append([]string(nil), cf.Domains...),
		})
	}
	return out
}

func dnsServiceSnapshot() *dns.CloudflareService {
	runtimeState.mu.RLock()
	defer runtimeState.mu.RUnlock()
	return runtimeState.cloudflare
}

func maskSecretPreview(val string) string {
	s := strings.TrimSpace(val)
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

func buildDNSAccountView(acc config.DNSAccountConfig, svc *dns.CloudflareService) dnsAccountView {
	prov := strings.ToLower(strings.TrimSpace(acc.Provider))
	if prov == "" {
		prov = "cloudflare"
	}
	secret := acc.APIToken
	switch prov {
	case "alidns":
		secret = acc.AccessKeySecret
	case "tencent":
		secret = acc.SecretKey
	case "dnspod":
		secret = acc.Token
	}

	var caps dns.DNSCapabilities
	if svc != nil {
		if cli, err := svc.ClientFor(acc.Name); err == nil {
			caps = cli.Capabilities()
		}
	}
	if caps.Provider == "" {
		caps = dns.DNSCapabilities{
			Provider:        prov,
			Edition:         acc.Edition,
			SupportsProxied: prov == "cloudflare",
			SupportsLine:    prov != "cloudflare",
			SupportsStatus:  prov != "cloudflare",
			SupportsRemark:  true,
		}
	}

	domains := append([]string(nil), acc.Domains...)
	if domains == nil {
		domains = []string{}
	}

	return dnsAccountView{
		Name:            acc.Name,
		Provider:        prov,
		Edition:         acc.Edition,
		Remark:          acc.Remark,
		Domains:         domains,
		ZoneID:          acc.ZoneID,
		AccessKeyID:     acc.AccessKeyID,
		SecretID:        acc.SecretID,
		TokenID:         acc.TokenID,
		HasSecret:       strings.TrimSpace(secret) != "",
		SecretMasked:    maskSecretPreview(secret),
		Capabilities:    caps,
		SupportsProxied: caps.SupportsProxied,
	}
}

func listDNSAccounts(c *gin.Context) {
	accounts, _ := snapshotDNSConfig()
	svc := dnsServiceSnapshot()
	views := make([]dnsAccountView, 0, len(accounts))
	for _, acc := range accounts {
		views = append(views, buildDNSAccountView(acc, svc))
	}
	c.JSON(http.StatusOK, gin.H{"accounts": views})
}

type dnsAccountInput struct {
	Name            string   `json:"name"`
	Provider        string   `json:"provider"`
	Edition         string   `json:"edition"`
	Remark          string   `json:"remark"`
	Domains         []string `json:"domains"`
	DomainsText     string   `json:"domains_text"`
	ZoneID          string   `json:"zone_id"`
	APIToken        string   `json:"api_token"`
	AccessKeyID     string   `json:"access_key_id"`
	AccessKeySecret string   `json:"access_key_secret"`
	SecretID        string   `json:"secret_id"`
	SecretKey       string   `json:"secret_key"`
	TokenID         string   `json:"token_id"`
	Token           string   `json:"token"`
}

func saveDNSAccountsHandler(c *gin.Context) {
	var req struct {
		Mode         string            `json:"mode"` // "upsert" | "delete" | "" (bulk)
		OriginalName string            `json:"original_name"`
		Name         string            `json:"name"`
		Account      *dnsAccountInput  `json:"account"`
		Accounts     []dnsAccountInput `json:"accounts"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式无效: " + err.Error()})
		return
	}

	existingAccounts, existingBindings := snapshotDNSConfig()
	existingByName := make(map[string]config.DNSAccountConfig, len(existingAccounts))
	for _, a := range existingAccounts {
		existingByName[a.Name] = a
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	var nextAccounts []config.DNSAccountConfig

	switch mode {
	case "delete":
		target := strings.TrimSpace(req.Name)
		if target == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "账号名称不能为空"})
			return
		}
		for _, a := range existingAccounts {
			if a.Name != target {
				nextAccounts = append(nextAccounts, a)
			}
		}
	case "upsert":
		if req.Account == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 account 参数"})
			return
		}
		prevLookup := strings.TrimSpace(req.OriginalName)
		if prevLookup == "" {
			prevLookup = strings.TrimSpace(req.Account.Name)
		}
		converted, err := convertDNSAccountInput(*req.Account, existingByName[prevLookup])
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		replaced := false
		for _, a := range existingAccounts {
			if a.Name == prevLookup || a.Name == converted.Name {
				if !replaced {
					nextAccounts = append(nextAccounts, converted)
					replaced = true
				}
				continue
			}
			nextAccounts = append(nextAccounts, a)
		}
		if !replaced {
			nextAccounts = append(nextAccounts, converted)
		}
	default:
		for _, item := range req.Accounts {
			converted, err := convertDNSAccountInput(item, existingByName[strings.TrimSpace(item.Name)])
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			nextAccounts = append(nextAccounts, converted)
		}
	}

	if err := config.SaveAllDNSConfig(currentDNSPath(), nextAccounts, existingBindings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存 DNS 账号配置失败: " + err.Error()})
		return
	}
	if err := ReloadRuntimeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重载配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func convertDNSAccountInput(in dnsAccountInput, prev config.DNSAccountConfig) (config.DNSAccountConfig, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return config.DNSAccountConfig{}, fmt.Errorf("DNS 账号名称不能为空")
	}
	prov := strings.ToLower(strings.TrimSpace(in.Provider))
	if prov == "" {
		prov = "cloudflare"
	}

	domains := config.SplitDomainList(strings.Join(in.Domains, ","))
	if len(domains) == 0 && strings.TrimSpace(in.DomainsText) != "" {
		domains = config.SplitDomainList(in.DomainsText)
	}

	out := config.DNSAccountConfig{
		Name:     name,
		Provider: prov,
		Edition:  strings.ToLower(strings.TrimSpace(in.Edition)),
		Remark:   strings.TrimSpace(in.Remark),
		Domains:  domains,
	}

	switch prov {
	case "cloudflare":
		out.ZoneID = strings.TrimSpace(in.ZoneID)
		out.APIToken = strings.TrimSpace(in.APIToken)
		if out.APIToken == "" && strings.EqualFold(prev.Provider, "cloudflare") {
			out.APIToken = prev.APIToken
		}
		if out.ZoneID == "" || out.APIToken == "" {
			return config.DNSAccountConfig{}, fmt.Errorf("Cloudflare 账号 %q 需要填写 API Token 和 Zone ID", name)
		}
	case "alidns":
		out.AccessKeyID = strings.TrimSpace(in.AccessKeyID)
		out.AccessKeySecret = strings.TrimSpace(in.AccessKeySecret)
		if out.AccessKeySecret == "" && strings.EqualFold(prev.Provider, "alidns") {
			out.AccessKeySecret = prev.AccessKeySecret
		}
		if out.AccessKeyID == "" || out.AccessKeySecret == "" {
			return config.DNSAccountConfig{}, fmt.Errorf("阿里云 DNS 账号 %q 需要填写 AccessKey ID 和 AccessKey Secret", name)
		}
		if len(out.Domains) == 0 {
			return config.DNSAccountConfig{}, fmt.Errorf("阿里云 DNS 账号 %q 请至少配置一个需要管理的域名", name)
		}
	case "tencent":
		if out.Edition != "intl" {
			out.Edition = "cn"
		}
		out.SecretID = strings.TrimSpace(in.SecretID)
		out.SecretKey = strings.TrimSpace(in.SecretKey)
		if out.SecretKey == "" && strings.EqualFold(prev.Provider, "tencent") {
			out.SecretKey = prev.SecretKey
		}
		if out.SecretID == "" || out.SecretKey == "" {
			return config.DNSAccountConfig{}, fmt.Errorf("腾讯云 DNS 账号 %q 需要填写 SecretId 和 SecretKey", name)
		}
		if len(out.Domains) == 0 {
			return config.DNSAccountConfig{}, fmt.Errorf("腾讯云 DNS 账号 %q 请至少配置一个需要管理的域名", name)
		}
	case "dnspod":
		if out.Edition != "intl" {
			out.Edition = "cn"
		}
		out.TokenID = strings.TrimSpace(in.TokenID)
		out.Token = strings.TrimSpace(in.Token)
		if out.Token == "" && strings.EqualFold(prev.Provider, "dnspod") {
			out.Token = prev.Token
		}
		if out.TokenID == "" || out.Token == "" {
			return config.DNSAccountConfig{}, fmt.Errorf("DNSPod 账号 %q 需要填写 Token ID 和 Token", name)
		}
		if len(out.Domains) == 0 {
			return config.DNSAccountConfig{}, fmt.Errorf("DNSPod 账号 %q 请至少配置一个需要管理的域名", name)
		}
	default:
		return config.DNSAccountConfig{}, fmt.Errorf("不支持的 DNS 服务商 %q", prov)
	}
	return out, nil
}

func resolveConsoleClientAndDomain(accountName, requestedDomain string) (dns.DNSClient, string, error) {
	svc := dnsServiceSnapshot()
	if svc == nil {
		return nil, "", fmt.Errorf("DNS 服务未初始化")
	}
	accName := strings.TrimSpace(accountName)
	if accName == "" {
		return nil, "", fmt.Errorf("请选择 DNS 账号")
	}
	client, err := svc.ClientFor(accName)
	if err != nil {
		return nil, "", err
	}
	domains, err := client.ResolveConfiguredDomains()
	if err != nil {
		return nil, "", err
	}
	if len(domains) == 0 {
		return nil, "", fmt.Errorf("账号 %q 未配置可管理的域名，请在「DNS 账号配置」中设置", accName)
	}
	reqDom := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(requestedDomain), "."))
	if reqDom == "" {
		return client, domains[0].Domain, nil
	}
	for _, d := range domains {
		if strings.EqualFold(d.Domain, reqDom) {
			return client, d.Domain, nil
		}
	}
	return nil, "", fmt.Errorf("域名 %q 不在账号 %q 已配置的管理域名列表中", requestedDomain, accName)
}

func getDNSConsoleDomains(c *gin.Context) {
	accName := strings.TrimSpace(c.Query("account"))
	svc := dnsServiceSnapshot()
	if svc == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "DNS 服务未初始化"})
		return
	}
	client, err := svc.ClientFor(accName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	domains, err := client.ResolveConfiguredDomains()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if domains == nil {
		domains = []dns.DNSDomainInfo{}
	}
	c.JSON(http.StatusOK, gin.H{
		"account":      accName,
		"capabilities": client.Capabilities(),
		"domains":      domains,
	})
}

func listDNSConsoleRecords(c *gin.Context) {
	client, domain, err := resolveConsoleClientAndDomain(c.Query("account"), c.Query("domain"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	records, err := client.ListRecords(domain)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if records == nil {
		records = []dns.DNSRecord{}
	}
	c.JSON(http.StatusOK, gin.H{
		"account":      c.Query("account"),
		"domain":       domain,
		"capabilities": client.Capabilities(),
		"records":      records,
	})
}

func createDNSConsoleRecord(c *gin.Context) {
	var req struct {
		Account string        `json:"account"`
		Domain  string        `json:"domain"`
		Record  dns.DNSRecord `json:"record"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式无效: " + err.Error()})
		return
	}
	client, domain, err := resolveConsoleClientAndDomain(req.Account, req.Domain)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	created, err := client.CreateRecord(domain, req.Record)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "record": created})
}

func updateDNSConsoleRecord(c *gin.Context) {
	var req struct {
		Account string        `json:"account"`
		Domain  string        `json:"domain"`
		ID      string        `json:"id"`
		Record  dns.DNSRecord `json:"record"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式无效: " + err.Error()})
		return
	}
	client, domain, err := resolveConsoleClientAndDomain(req.Account, req.Domain)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	recordID := strings.TrimSpace(req.ID)
	if recordID == "" {
		recordID = strings.TrimSpace(req.Record.ID)
	}
	updated, err := client.UpdateRecord(domain, recordID, req.Record)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "record": updated})
}

func deleteDNSConsoleRecord(c *gin.Context) {
	client, domain, err := resolveConsoleClientAndDomain(c.Query("account"), c.Query("domain"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	recordID := strings.TrimSpace(c.Query("id"))
	if recordID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "记录 ID 不能为空"})
		return
	}
	if err := client.DeleteRecord(domain, recordID); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func setDNSConsoleRecordStatus(c *gin.Context) {
	var req struct {
		Account string `json:"account"`
		Domain  string `json:"domain"`
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式无效: " + err.Error()})
		return
	}
	client, domain, err := resolveConsoleClientAndDomain(req.Account, req.Domain)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := client.SetRecordStatus(domain, strings.TrimSpace(req.ID), req.Enabled); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func exportDNSZoneHandler(c *gin.Context) {
	client, domain, err := resolveConsoleClientAndDomain(c.Query("account"), c.Query("domain"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	records, err := client.ListRecords(domain)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	zoneText := dns.ExportBINDZone(domain, records)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zone"`, domain))
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(zoneText))
}

func previewDNSImportHandler(c *gin.Context) {
	var req struct {
		Account string `json:"account"`
		Domain  string `json:"domain"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式无效: " + err.Error()})
		return
	}
	client, domain, err := resolveConsoleClientAndDomain(req.Account, req.Domain)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	existing, err := client.ListRecords(domain)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	preview, err := dns.PreviewBINDImport(domain, req.Content, existing)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, preview)
}

func applyDNSImportHandler(c *gin.Context) {
	var req struct {
		Account   string `json:"account"`
		Domain    string `json:"domain"`
		Content   string `json:"content"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式无效: " + err.Error()})
		return
	}
	client, domain, err := resolveConsoleClientAndDomain(req.Account, req.Domain)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	existing, err := client.ListRecords(domain)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	preview, err := dns.PreviewBINDImport(domain, req.Content, existing)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result := dns.DNSImportExecuteResult{
		Success: true,
		Skipped: len(preview.Warnings),
		Logs:    append([]string(nil), preview.Warnings...),
	}
	for _, item := range preview.Items {
		switch item.Action {
		case "conflict":
			if !req.Overwrite {
				result.Skipped++
				result.Logs = append(result.Logs, fmt.Sprintf("跳过冲突记录: %s %s (%s)", item.Record.Name, item.Record.Type, item.Record.Value))
				continue
			}
			if _, err := client.UpdateRecord(domain, item.Target, item.Record); err != nil {
				result.Failed++
				result.Logs = append(result.Logs, fmt.Sprintf("覆盖失败 %s %s: %v", item.Record.Name, item.Record.Type, err))
			} else {
				result.Updated++
				result.Logs = append(result.Logs, fmt.Sprintf("已覆盖 %s %s -> %s", item.Record.Name, item.Record.Type, item.Record.Value))
			}
		case "create":
			if _, err := client.CreateRecord(domain, item.Record); err != nil {
				result.Failed++
				result.Logs = append(result.Logs, fmt.Sprintf("创建失败 %s %s: %v", item.Record.Name, item.Record.Type, err))
			} else {
				result.Created++
				result.Logs = append(result.Logs, fmt.Sprintf("已新增 %s %s -> %s", item.Record.Name, item.Record.Type, item.Record.Value))
			}
		}
	}
	result.Success = result.Failed == 0
	c.JSON(http.StatusOK, result)
}

func listDNSBindingsV2(c *gin.Context) {
	accounts, bindings := snapshotDNSConfig()
	accProv := make(map[string]string, len(accounts))
	for _, a := range accounts {
		accProv[a.Name] = a.Provider
	}
	out := make([]gin.H, 0, len(bindings))
	for _, b := range bindings {
		target := b.TargetAccountName()
		out = append(out, gin.H{
			"name":         b.Name,
			"cloudflare":   target,
			"dns_account":  target,
			"dns_provider": accProv[target],
			"provider":     b.Provider,
			"account":      b.Account,
			"vm":           b.VM,
			"domain":       b.Domain,
			"type":         b.Type,
			"ttl":          b.TTL,
			"proxied":      b.Proxied,
		})
	}
	c.JSON(http.StatusOK, gin.H{"bindings": out})
}

func saveDNSBindingsV2(c *gin.Context) {
	var req struct {
		Bindings []config.DNSBinding `json:"bindings"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式无效: " + err.Error()})
		return
	}
	accounts, _ := snapshotDNSConfig()
	accByName := make(map[string]config.DNSAccountConfig, len(accounts))
	for _, a := range accounts {
		accByName[a.Name] = a
	}

	cleaned := make([]config.DNSBinding, 0, len(req.Bindings))
	for _, b := range req.Bindings {
		if strings.TrimSpace(b.Provider) == "" || strings.TrimSpace(b.Account) == "" || strings.TrimSpace(b.VM) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "VM 绑定必须明确指定云厂商、账号和实例名称，不允许通配符绑定"})
			return
		}
		target := b.TargetAccountName()
		if target == "" || strings.TrimSpace(b.Domain) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "DNS 账号和绑定域名不能为空"})
			return
		}
		b.DNSAccount = target
		b.Cloudflare = ""
		if acc, ok := accByName[target]; ok && !strings.EqualFold(acc.Provider, "cloudflare") {
			b.Proxied = false
		}
		cleaned = append(cleaned, b)
	}

	if err := config.SaveAllDNSConfig(currentDNSPath(), accounts, cleaned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存绑定配置失败: " + err.Error()})
		return
	}
	if err := ReloadRuntimeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重载配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func deleteDNSBindingV2(c *gin.Context) {
	var req struct {
		Index    *int   `json:"index"`
		Provider string `json:"provider"`
		Account  string `json:"account"`
		VM       string `json:"vm"`
		Domain   string `json:"domain"`
		Type     string `json:"type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式无效"})
		return
	}
	accounts, bindings := snapshotDNSConfig()
	next := make([]config.DNSBinding, 0, len(bindings))
	removed := false
	for i, b := range bindings {
		matchIdx := req.Index != nil && *req.Index == i
		matchFields := req.Index == nil &&
			b.Provider == req.Provider &&
			b.Account == req.Account &&
			b.VM == req.VM &&
			strings.EqualFold(b.Domain, req.Domain) &&
			(req.Type == "" || strings.EqualFold(b.Type, req.Type))
		if !removed && (matchIdx || matchFields) {
			removed = true
			continue
		}
		next = append(next, b)
	}
	if err := config.SaveAllDNSConfig(currentDNSPath(), accounts, next); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除绑定失败: " + err.Error()})
		return
	}
	if err := ReloadRuntimeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重载配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func getVMDNSBindingsV2(c *gin.Context) {
	providerName := strings.TrimSpace(c.Param("provider"))
	accountName := strings.TrimSpace(c.Param("account"))
	vmName := strings.TrimSpace(c.Param("name"))

	accounts, bindings := snapshotDNSConfig()
	svc := dnsServiceSnapshot()

	cfNames := make([]string, 0)
	accountSummaries := make([]gin.H, 0, len(accounts))
	accProv := make(map[string]string, len(accounts))
	for _, a := range accounts {
		view := buildDNSAccountView(a, svc)
		accProv[a.Name] = view.Provider
		if view.Provider == "cloudflare" {
			cfNames = append(cfNames, a.Name)
		}
		accountSummaries = append(accountSummaries, gin.H{
			"name":            view.Name,
			"provider":        view.Provider,
			"edition":         view.Edition,
			"domains":         view.Domains,
			"supportsProxied": view.SupportsProxied,
		})
	}

	matched := make([]gin.H, 0)
	for _, b := range bindings {
		if !b.Matches(providerName, accountName, vmName) {
			continue
		}
		target := b.TargetAccountName()
		matched = append(matched, gin.H{
			"name":         b.Name,
			"cloudflare":   target,
			"dns_account":  target,
			"dns_provider": accProv[target],
			"provider":     b.Provider,
			"account":      b.Account,
			"vm":           b.VM,
			"domain":       b.Domain,
			"type":         b.Type,
			"ttl":          b.TTL,
			"proxied":      b.Proxied,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"bindings":            matched,
		"cloudflare_accounts": cfNames,
		"dns_accounts":        accountSummaries,
	})
}

func saveVMDNSBindingsV2(c *gin.Context) {
	providerName := strings.TrimSpace(c.Param("provider"))
	accountName := strings.TrimSpace(c.Param("account"))
	vmName := strings.TrimSpace(c.Param("name"))
	if providerName == "" || accountName == "" || vmName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "VM 标识不完整"})
		return
	}

	var req struct {
		Bindings []config.DNSBinding `json:"bindings"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式无效: " + err.Error()})
		return
	}

	accounts, existingBindings := snapshotDNSConfig()
	accByName := make(map[string]config.DNSAccountConfig, len(accounts))
	for _, a := range accounts {
		accByName[a.Name] = a
	}

	kept := make([]config.DNSBinding, 0, len(existingBindings)+len(req.Bindings))
	for _, b := range existingBindings {
		if !b.Matches(providerName, accountName, vmName) {
			kept = append(kept, b)
		}
	}

	for i, b := range req.Bindings {
		target := b.TargetAccountName()
		domain := strings.TrimSpace(b.Domain)
		if target == "" || domain == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请为每条绑定选择 DNS 账号并填写域名"})
			return
		}
		acc, ok := accByName[target]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("DNS 账号 %q 不存在", target)})
			return
		}
		recType := strings.ToUpper(strings.TrimSpace(b.Type))
		if recType != "AAAA" {
			recType = "A"
		}
		proxied := b.Proxied
		if !strings.EqualFold(acc.Provider, "cloudflare") {
			proxied = false
		}
		ttl := b.TTL
		if ttl <= 0 {
			if strings.EqualFold(acc.Provider, "cloudflare") {
				ttl = 1
			} else {
				ttl = 600
			}
		}
		name := strings.TrimSpace(b.Name)
		if name == "" {
			name = fmt.Sprintf("%s-%d", vmName, i+1)
		}
		kept = append(kept, config.DNSBinding{
			Name:       name,
			DNSAccount: target,
			Provider:   providerName,
			Account:    accountName,
			VM:         vmName,
			Domain:     domain,
			Type:       recType,
			TTL:        ttl,
			Proxied:    proxied,
		})
	}

	if err := config.SaveAllDNSConfig(currentDNSPath(), accounts, kept); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存 VM DNS 绑定失败: " + err.Error()})
		return
	}
	if err := ReloadRuntimeConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重载配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
