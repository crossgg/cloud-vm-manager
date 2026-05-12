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
