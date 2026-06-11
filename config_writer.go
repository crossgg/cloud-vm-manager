package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

func SaveAuthConfig(path string, auth AuthConfig) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	if strings.Contains(content, "=begin") {
		return os.WriteFile(path, []byte(upsertAuthBlock(content, auth)), 0600)
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
	return os.WriteFile(path, next, 0600)
}

func SaveUpdateConfig(path string, update UpdateConfig) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	if strings.Contains(content, "=begin") {
		return os.WriteFile(path, []byte(upsertUpdateBlock(content, update)), 0600)
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
	return os.WriteFile(path, next, 0600)
}

// SaveDNSConfig writes the entire dns.conf file from the given cloudflare accounts and dns bindings.
// It performs deduplication: duplicate cloudflare names or dns binding names are merged (last wins).
func SaveDNSConfig(path string, cloudflareAccounts []CloudflareConfig, bindings []DNSBinding) error {
	if path == "" {
		return fmt.Errorf("dns config path is empty")
	}

	// Deduplicate cloudflare accounts by name
	cfMap := make(map[string]CloudflareConfig)
	var cfOrder []string
	for _, cf := range cloudflareAccounts {
		if cf.Name == "" {
			continue
		}
		if _, exists := cfMap[cf.Name]; !exists {
			cfOrder = append(cfOrder, cf.Name)
		}
		cfMap[cf.Name] = cf
	}

	// Deduplicate dns bindings by name
	dnsMap := make(map[string]DNSBinding)
	var dnsOrder []string
	for _, b := range bindings {
		if b.Name == "" {
			continue
		}
		if _, exists := dnsMap[b.Name]; !exists {
			dnsOrder = append(dnsOrder, b.Name)
		}
		dnsMap[b.Name] = b
	}

	var sb strings.Builder

	// Write cloudflare block
	sb.WriteString("cloudflare=begin\n")
	for _, name := range cfOrder {
		cf := cfMap[name]
		sb.WriteString("[" + cf.Name + "]\n")
		if cf.Remark != "" {
			sb.WriteString("remark=" + cf.Remark + "\n")
		}
		sb.WriteString("api_token=" + cf.APIToken + "\n")
		sb.WriteString("zone_id=" + cf.ZoneID + "\n")
	}
	sb.WriteString("cloudflare=end\n")

	sb.WriteString("\n")

	// Write dns block
	sb.WriteString("dns=begin\n")
	for _, name := range dnsOrder {
		b := dnsMap[name]
		sb.WriteString("[" + b.Name + "]\n")
		sb.WriteString("cloudflare=" + b.Cloudflare + "\n")
		sb.WriteString("provider=" + b.Provider + "\n")
		sb.WriteString("account=" + b.Account + "\n")
		sb.WriteString("vm=" + b.VM + "\n")
		sb.WriteString("domain=" + b.Domain + "\n")
		sb.WriteString("type=" + valueOrDefault(b.Type, "A") + "\n")
		sb.WriteString("ttl=" + strconv.Itoa(b.TTL) + "\n")
		sb.WriteString("proxied=" + strconv.FormatBool(b.Proxied) + "\n")
	}
	sb.WriteString("dns=end\n")

	return os.WriteFile(path, []byte(sb.String()), 0600)
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

// SaveDTMonitorConfig persists data transfer monitor settings for an OCI account.
// It updates the dt_monitor_* keys within the OCI config block for the given account.
func SaveDTMonitorConfig(path string, accountName string, dtCfg DataTransferConfig) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	if !strings.Contains(content, "=begin") {
		return fmt.Errorf("dt_monitor config persistence only supported for block config format")
	}

	lines := strings.Split(content, "\n")
	inOCI := false
	inSection := false
	sectionName := ""
	insertionLine := -1

	dtKeys := map[string]string{
		"dt_monitor_enabled":     strconv.FormatBool(dtCfg.Enabled),
		"dt_monitor_interval":    strconv.Itoa(dtCfg.Interval),
		"dt_monitor_threshold":   strconv.FormatFloat(dtCfg.Threshold, 'f', -1, 64),
		"dt_monitor_auto_stop":   strconv.FormatBool(dtCfg.AutoStop),
		"dt_monitor_stop_method": dtCfg.StopMethod,
	}

	// Track which dt keys we've already updated in the file
	updatedKeys := map[string]bool{}
	result := make([]string, 0, len(lines)+len(dtKeys))

	for i, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		lower := strings.ToLower(line)

		if lower == "oci=begin" {
			inOCI = true
			result = append(result, rawLine)
			continue
		}
		if lower == "oci=end" {
			// Before closing the OCI block, insert any remaining dt keys for the target account
			if inSection && sectionName == accountName {
				for key, value := range dtKeys {
					if !updatedKeys[key] {
						result = append(result, key+"="+value)
						updatedKeys[key] = true
					}
				}
			}
			inOCI = false
			inSection = false
			sectionName = ""
			result = append(result, rawLine)
			continue
		}

		if inOCI {
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				// Before switching to a new section, insert remaining dt keys if we were in the target section
				if inSection && sectionName == accountName {
					for key, value := range dtKeys {
						if !updatedKeys[key] {
							result = append(result, key+"="+value)
							updatedKeys[key] = true
						}
					}
				}
				sectionName = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
				inSection = true
				_ = insertionLine
				insertionLine = i
				result = append(result, rawLine)
				continue
			}

			// Check if this line is a dt_monitor key to update
			if inSection && sectionName == accountName {
				key, _, hasEquals := strings.Cut(line, "=")
				if hasEquals {
					trimmedKey := strings.TrimSpace(key)
					if newVal, isDTKey := dtKeys[trimmedKey]; isDTKey {
						result = append(result, trimmedKey+"="+newVal)
						updatedKeys[trimmedKey] = true
						continue
					}
				}
			}
		}

		result = append(result, rawLine)
	}

	return os.WriteFile(path, []byte(strings.Join(result, "\n")), 0600)
}

