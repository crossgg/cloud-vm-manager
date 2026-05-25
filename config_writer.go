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
