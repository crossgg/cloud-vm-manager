package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func SaveAuthConfig(path string, auth AuthConfig) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	if HasUnsafeConfigValue(auth.Username) {
		return fmt.Errorf("auth username contains unsupported characters")
	}
	if HasUnsafeConfigLineValue(auth.PasswordHash) || HasUnsafeConfigLineValue(auth.SessionSecret) {
		return fmt.Errorf("auth configuration contains unsupported control characters")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	if strings.Contains(content, "=begin") {
		return writeFileAtomic(path, []byte(upsertAuthBlock(content, auth)), 0600)
	}
	return saveYAMLAuthConfig(path, data, auth)
}

func upsertAuthBlock(content string, auth AuthConfig) string {
	block := renderAuthBlock(auth)
	lines := strings.Split(content, "\n")
	start, end := -1, -1
	for i, raw := range lines {
		line := strings.ToLower(strings.TrimSpace(raw))
		if line == "auth=begin" {
			start = i
			continue
		}
		if start >= 0 && line == "auth=end" {
			end = i
			break
		}
	}

	if start >= 0 && end >= start {
		next := make([]string, 0, len(lines)-(end-start)+strings.Count(block, "\n")+1)
		next = append(next, lines[:start]...)
		next = append(next, strings.Split(block, "\n")...)
		next = append(next, lines[end+1:]...)
		return strings.Join(next, "\n")
	}

	trimmed := strings.TrimRight(content, "\r\n")
	if trimmed == "" {
		return block + "\n"
	}
	return trimmed + "\n\n" + block + "\n"
}

func renderAuthBlock(auth AuthConfig) string {
	return strings.Join([]string{
		"auth=begin",
		"[main]",
		"enabled=" + strconv.FormatBool(auth.Enabled),
		"username=" + auth.Username,
		"password_hash=" + auth.PasswordHash,
		"session_secret=" + auth.SessionSecret,
		"session_hours=" + strconv.Itoa(auth.SessionHours),
		"cookie_secure=" + strconv.FormatBool(auth.CookieSecure),
		"auth=end",
	}, "\n")
}

func saveYAMLAuthConfig(path string, data []byte, auth AuthConfig) error {
	var values map[string]interface{}
	if err := yaml.Unmarshal(data, &values); err != nil {
		return err
	}
	if values == nil {
		values = map[string]interface{}{}
	}
	values["auth"] = map[string]interface{}{
		"enabled":        auth.Enabled,
		"username":       auth.Username,
		"password_hash":  auth.PasswordHash,
		"session_secret": auth.SessionSecret,
		"session_hours":  auth.SessionHours,
		"cookie_secure":  auth.CookieSecure,
	}
	next, err := yaml.Marshal(values)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, next, 0600)
}

func SaveUpdateConfig(path string, update UpdateConfig) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	if HasUnsafeConfigLineValue(update.DownloadProxy) {
		return fmt.Errorf("download_proxy contains unsupported characters")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	if strings.Contains(content, "=begin") {
		return writeFileAtomic(path, []byte(upsertUpdateBlock(content, update)), 0600)
	}
	return saveYAMLUpdateConfig(path, data, update)
}

func upsertUpdateBlock(content string, update UpdateConfig) string {
	block := renderUpdateBlock(update)
	lines := strings.Split(content, "\n")
	start, end := -1, -1
	for i, raw := range lines {
		line := strings.ToLower(strings.TrimSpace(raw))
		if line == "update=begin" {
			start = i
			continue
		}
		if start >= 0 && line == "update=end" {
			end = i
			break
		}
	}

	if start >= 0 && end >= start {
		next := make([]string, 0, len(lines)-(end-start)+strings.Count(block, "\n")+1)
		next = append(next, lines[:start]...)
		next = append(next, strings.Split(block, "\n")...)
		next = append(next, lines[end+1:]...)
		return strings.Join(next, "\n")
	}

	trimmed := strings.TrimRight(content, "\r\n")
	if trimmed == "" {
		return block + "\n"
	}
	return trimmed + "\n\n" + block + "\n"
}

func renderUpdateBlock(update UpdateConfig) string {
	return strings.Join([]string{
		"update=begin",
		"[main]",
		"download_proxy=" + update.DownloadProxy,
		"update=end",
	}, "\n")
}

func saveYAMLUpdateConfig(path string, data []byte, update UpdateConfig) error {
	var values map[string]interface{}
	if err := yaml.Unmarshal(data, &values); err != nil {
		return err
	}
	if values == nil {
		values = map[string]interface{}{}
	}
	values["update"] = map[string]interface{}{
		"download_proxy": update.DownloadProxy,
	}
	next, err := yaml.Marshal(values)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, next, 0600)
}

// SaveDNSConfig writes cloudflare accounts and dns bindings while preserving non-cloudflare DNS accounts on disk.
func SaveDNSConfig(path string, cloudflareAccounts []CloudflareConfig, bindings []DNSBinding) error {
	if path == "" {
		return fmt.Errorf("dns config path is empty")
	}

	var mergedAccounts []DNSAccountConfig
	for _, cf := range cloudflareAccounts {
		if cf.Name == "" {
			continue
		}
		mergedAccounts = append(mergedAccounts, DNSAccountConfig{
			Name:     cf.Name,
			Provider: "cloudflare",
			Remark:   cf.Remark,
			Domains:  cf.Domains,
			APIToken: cf.APIToken,
			ZoneID:   cf.ZoneID,
		})
	}
	if existingRaw, err := os.ReadFile(path); err == nil {
		tmpCfg := &Config{}
		if mergeDNSConfig(tmpCfg, string(existingRaw)) == nil {
			for _, acc := range tmpCfg.DNSAccounts {
				if acc.Provider != "cloudflare" {
					mergedAccounts = append(mergedAccounts, acc)
				}
			}
		}
	}
	return SaveAllDNSConfig(path, mergedAccounts, bindings)
}

// SaveAllDNSConfig writes all DNS accounts (cloudflare + alidns + tencent + dnspod) and dns bindings to dns.conf.
func SaveAllDNSConfig(path string, dnsAccounts []DNSAccountConfig, bindings []DNSBinding) error {
	if path == "" {
		return fmt.Errorf("dns config path is empty")
	}

	cfMap := make(map[string]DNSAccountConfig)
	var cfOrder []string
	otherMap := make(map[string]DNSAccountConfig)
	var otherOrder []string

	for _, acc := range dnsAccounts {
		if acc.Name == "" {
			continue
		}
		if HasUnsafeConfigValue(acc.Name) {
			return fmt.Errorf("dns account name %q contains unsupported characters", acc.Name)
		}
		if HasUnsafeConfigLineValue(acc.Remark) || HasUnsafeConfigLineValue(acc.Provider) ||
			HasUnsafeConfigLineValue(acc.Edition) || HasUnsafeConfigLineValue(acc.APIToken) ||
			HasUnsafeConfigLineValue(acc.ZoneID) || HasUnsafeConfigLineValue(acc.AccessKeyID) ||
			HasUnsafeConfigLineValue(acc.AccessKeySecret) || HasUnsafeConfigLineValue(acc.SecretID) ||
			HasUnsafeConfigLineValue(acc.SecretKey) || HasUnsafeConfigLineValue(acc.TokenID) ||
			HasUnsafeConfigLineValue(acc.Token) {
			return fmt.Errorf("dns account %q contains unsupported control characters", acc.Name)
		}
		for _, d := range acc.Domains {
			if HasUnsafeConfigLineValue(d) {
				return fmt.Errorf("dns account %q domain %q contains unsupported control characters", acc.Name, d)
			}
		}

		prov := strings.ToLower(strings.TrimSpace(acc.Provider))
		if prov == "" {
			prov = "cloudflare"
		}
		acc.Provider = prov

		if prov == "cloudflare" {
			if _, exists := cfMap[acc.Name]; !exists {
				cfOrder = append(cfOrder, acc.Name)
			}
			cfMap[acc.Name] = acc
		} else {
			if _, exists := otherMap[acc.Name]; !exists {
				otherOrder = append(otherOrder, acc.Name)
			}
			otherMap[acc.Name] = acc
		}
	}

	// Deduplicate dns bindings by name
	dnsMap := make(map[string]DNSBinding)
	var dnsOrder []string
	for _, b := range bindings {
		if b.Name == "" {
			continue
		}
		if HasUnsafeConfigValue(b.Name) {
			return fmt.Errorf("dns binding name %q contains unsupported characters", b.Name)
		}
		targetAcc := b.TargetAccountName()
		if HasUnsafeConfigLineValue(targetAcc) || HasUnsafeConfigLineValue(b.Provider) ||
			HasUnsafeConfigLineValue(b.Account) || HasUnsafeConfigLineValue(b.VM) ||
			HasUnsafeConfigLineValue(b.Domain) || HasUnsafeConfigLineValue(b.Type) {
			return fmt.Errorf("dns binding %q contains unsupported control characters", b.Name)
		}
		if _, exists := dnsMap[b.Name]; !exists {
			dnsOrder = append(dnsOrder, b.Name)
		}
		dnsMap[b.Name] = b
	}

	var sb strings.Builder

	// Write cloudflare block (backward-compatible)
	sb.WriteString("cloudflare=begin\n")
	for _, name := range cfOrder {
		cf := cfMap[name]
		sb.WriteString("[" + cf.Name + "]\n")
		if cf.Remark != "" {
			sb.WriteString("remark=" + cf.Remark + "\n")
		}
		sb.WriteString("api_token=" + cf.APIToken + "\n")
		sb.WriteString("zone_id=" + cf.ZoneID + "\n")
		if len(cf.Domains) > 0 {
			sb.WriteString("domains=" + strings.Join(cf.Domains, ",") + "\n")
		}
	}
	sb.WriteString("cloudflare=end\n\n")

	// Write multi-cloud dns_accounts block
	if len(otherOrder) > 0 {
		sb.WriteString("dns_accounts=begin\n")
		for _, name := range otherOrder {
			acc := otherMap[name]
			sb.WriteString("[" + acc.Name + "]\n")
			sb.WriteString("provider=" + acc.Provider + "\n")
			if acc.Edition != "" {
				sb.WriteString("edition=" + acc.Edition + "\n")
			}
			if acc.Remark != "" {
				sb.WriteString("remark=" + acc.Remark + "\n")
			}
			if len(acc.Domains) > 0 {
				sb.WriteString("domains=" + strings.Join(acc.Domains, ",") + "\n")
			}
			switch acc.Provider {
			case "alidns":
				sb.WriteString("access_key_id=" + acc.AccessKeyID + "\n")
				sb.WriteString("access_key_secret=" + acc.AccessKeySecret + "\n")
			case "tencent":
				sb.WriteString("secret_id=" + acc.SecretID + "\n")
				sb.WriteString("secret_key=" + acc.SecretKey + "\n")
			case "dnspod":
				sb.WriteString("token_id=" + acc.TokenID + "\n")
				sb.WriteString("token=" + acc.Token + "\n")
			}
		}
		sb.WriteString("dns_accounts=end\n\n")
	}

	// Write dns block
	sb.WriteString("dns=begin\n")
	for _, name := range dnsOrder {
		b := dnsMap[name]
		targetAcc := b.TargetAccountName()
		sb.WriteString("[" + b.Name + "]\n")
		sb.WriteString("dns_account=" + targetAcc + "\n")
		sb.WriteString("cloudflare=" + targetAcc + "\n")
		sb.WriteString("provider=" + b.Provider + "\n")
		sb.WriteString("account=" + b.Account + "\n")
		sb.WriteString("vm=" + b.VM + "\n")
		sb.WriteString("domain=" + b.Domain + "\n")
		sb.WriteString("type=" + valueOrDefault(b.Type, "A") + "\n")
		sb.WriteString("ttl=" + strconv.Itoa(b.TTL) + "\n")
		sb.WriteString("proxied=" + strconv.FormatBool(b.Proxied) + "\n")
	}
	sb.WriteString("dns=end\n")

	return writeFileAtomic(path, []byte(sb.String()), 0600)
}

// ParseDNSConfContent parses raw dns.conf content and returns cloudflare accounts and bindings.
// Used for the upload/import feature.
func ParseDNSConfContent(content string) ([]CloudflareConfig, []DNSBinding, error) {
	cfg := &Config{}
	if err := mergeDNSConfig(cfg, content); err != nil {
		return nil, nil, err
	}
	return cfg.Cloudflare, cfg.DNSBindings, nil
}

// DTMonitorConfigFields returns the config key-value map for OCI data transfer monitor settings.
func DTMonitorConfigFields(dtCfg DataTransferConfig) map[string]string {
	return map[string]string{
		"dt_monitor_enabled":     strconv.FormatBool(dtCfg.Enabled),
		"dt_monitor_interval":    strconv.Itoa(dtCfg.Interval),
		"dt_monitor_threshold":   strconv.FormatFloat(dtCfg.Threshold, 'f', -1, 64),
		"dt_monitor_auto_stop":   strconv.FormatBool(dtCfg.AutoStop),
		"dt_monitor_stop_method": dtCfg.StopMethod,
	}
}

// AzureCreditConfigFields returns the config key-value map for Azure credit/cost monitor settings.
func AzureCreditConfigFields(azCfg AzureCreditMonitorConfig) map[string]string {
	azCfg.Normalize()
	return map[string]string{
		"az_credit_enabled":            strconv.FormatBool(azCfg.Enabled),
		"az_credit_interval":           strconv.Itoa(azCfg.Interval),
		"az_credit_initial":            strconv.FormatFloat(azCfg.InitialCredit, 'f', -1, 64),
		"az_credit_start_date":         azCfg.StartDate,
		"az_credit_expire_date":        azCfg.ExpireDate,
		"az_credit_min_remaining":      strconv.FormatFloat(azCfg.MinRemainingCredit, 'f', -1, 64),
		"az_credit_days_before_expire": strconv.Itoa(azCfg.DaysBeforeExpire),
		"az_credit_max_monthly_cost":   strconv.FormatFloat(azCfg.MaxMonthlyCost, 'f', -1, 64),
		"az_credit_trigger_action":     azCfg.TriggerAction,
		"az_credit_confirm_delete_all": strconv.FormatBool(azCfg.ConfirmAutoDeleteAll),
	}
}

// SaveDTMonitorConfig persists data transfer monitor settings for an OCI account.
// It updates the dt_monitor_* keys within the OCI config block for the given account.
func SaveDTMonitorConfig(path string, accountName string, dtCfg DataTransferConfig) error {
	return saveBlockAccountKeys(path, "oci", accountName, DTMonitorConfigFields(dtCfg))
}

// SaveAzureCreditConfig persists credit & cost monitor settings for an Azure account.
func SaveAzureCreditConfig(path string, accountName string, azCfg AzureCreditMonitorConfig) error {
	return saveBlockAccountKeys(path, "azure", accountName, AzureCreditConfigFields(azCfg))
}

func saveBlockAccountKeys(path string, provider string, accountName string, keys map[string]string) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.Contains(content, "=begin") {
		return fmt.Errorf("monitor config persistence only supported for block config format")
	}

	sortedKeys := make([]string, 0, len(keys))
	for k := range keys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	lines := strings.Split(content, "\n")
	startTag := strings.ToLower(provider) + "=begin"
	endTag := strings.ToLower(provider) + "=end"
	inBlock := false
	inSection := false
	sectionName := ""

	updatedKeys := map[string]bool{}
	result := make([]string, 0, len(lines)+len(keys))

	flushMissing := func() {
		if inSection && strings.EqualFold(sectionName, accountName) {
			for _, key := range sortedKeys {
				if !updatedKeys[key] {
					val := strings.TrimSpace(keys[key])
					if val != "" {
						result = append(result, key+"="+val)
					}
					updatedKeys[key] = true
				}
			}
		}
	}

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		lower := strings.ToLower(line)

		if lower == startTag {
			inBlock = true
			result = append(result, rawLine)
			continue
		}
		if lower == endTag {
			flushMissing()
			inBlock = false
			inSection = false
			sectionName = ""
			result = append(result, rawLine)
			continue
		}

		if inBlock {
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				flushMissing()
				sectionName = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
				inSection = true
				result = append(result, rawLine)
				continue
			}

			if inSection && strings.EqualFold(sectionName, accountName) {
				key, _, hasEquals := strings.Cut(line, "=")
				if hasEquals {
					trimmedKey := strings.TrimSpace(key)
					if newVal, managed := keys[trimmedKey]; managed {
						updatedKeys[trimmedKey] = true
						trimmedVal := strings.TrimSpace(newVal)
						if trimmedVal != "" {
							result = append(result, trimmedKey+"="+trimmedVal)
						}
						continue
					}
				}
			}
		}

		result = append(result, rawLine)
	}

	return writeFileAtomic(path, []byte(strings.Join(result, "\n")), 0600)
}

