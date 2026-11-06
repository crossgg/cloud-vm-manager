package config

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// SaveCloudAccount creates or updates a cloud account (azure, oci, gcp) in the config file.
func SaveCloudAccount(path string, provider string, accountName string, fields map[string]string) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	accountName = strings.TrimSpace(accountName)
	if provider == "" || accountName == "" {
		return fmt.Errorf("provider and account name are required")
	}

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(data)

	// Block format: provider=begin ... provider=end
	startTag := provider + "=begin"
	endTag := provider + "=end"

	// Normalize CRLF to LF for processing
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	blockStart, blockEnd := -1, -1
	for i, rawLine := range lines {
		line := strings.ToLower(strings.TrimSpace(rawLine))
		if line == startTag {
			blockStart = i
		} else if blockStart >= 0 && line == endTag {
			blockEnd = i
			break
		}
	}

	// Prepare the lines for this account section
	var accountLines []string
	accountLines = append(accountLines, "["+accountName+"]")
	var keys []string
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// Place 'group' first if present
	if g, ok := fields["group"]; ok && strings.TrimSpace(g) != "" {
		accountLines = append(accountLines, "group="+strings.TrimSpace(g))
	}
	for _, k := range keys {
		if k == "group" {
			continue
		}
		val := strings.TrimSpace(fields[k])
		if val != "" {
			accountLines = append(accountLines, k+"="+val)
		}
	}

	if blockStart == -1 || blockEnd == -1 {
		// Provider block does not exist, append to content
		newBlock := "\n" + startTag + "\n" + strings.Join(accountLines, "\n") + "\n" + endTag + "\n"
		trimmed := strings.TrimRight(normalized, "\n")
		var out string
		if trimmed == "" {
			out = strings.TrimLeft(newBlock, "\n")
		} else {
			out = trimmed + newBlock
		}
		return os.WriteFile(path, []byte(out), 0600)
	}

	// Provider block exists. Search for [accountName] within [blockStart+1, blockEnd)
	accStart, accEnd := -1, -1
	for i := blockStart + 1; i < blockEnd; i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currSection := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			if accStart >= 0 {
				accEnd = i
				break
			}
			if strings.EqualFold(currSection, accountName) {
				accStart = i
			}
		}
	}
	if accStart >= 0 && accEnd == -1 {
		accEnd = blockEnd
	}

	var newLines []string
	if accStart >= 0 {
		// Replace existing account section
		newLines = append(newLines, lines[:accStart]...)
		newLines = append(newLines, accountLines...)
		newLines = append(newLines, lines[accEnd:]...)
	} else {
		// Insert before blockEnd
		newLines = append(newLines, lines[:blockEnd]...)
		if blockEnd > 0 && strings.TrimSpace(lines[blockEnd-1]) != "" && strings.TrimSpace(lines[blockEnd-1]) != startTag {
			newLines = append(newLines, "")
		}
		newLines = append(newLines, accountLines...)
		newLines = append(newLines, lines[blockEnd:]...)
	}

	return os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0600)
}

// DeleteCloudAccount removes an account section from the given provider block.
func DeleteCloudAccount(path string, provider string, accountName string) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	accountName = strings.TrimSpace(accountName)
	if provider == "" || accountName == "" {
		return fmt.Errorf("provider and account name are required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)

	startTag := provider + "=begin"
	endTag := provider + "=end"

	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	blockStart, blockEnd := -1, -1
	for i, rawLine := range lines {
		line := strings.ToLower(strings.TrimSpace(rawLine))
		if line == startTag {
			blockStart = i
		} else if blockStart >= 0 && line == endTag {
			blockEnd = i
			break
		}
	}

	if blockStart == -1 || blockEnd == -1 {
		return nil
	}

	accStart, accEnd := -1, -1
	for i := blockStart + 1; i < blockEnd; i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currSection := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			if accStart >= 0 {
				accEnd = i
				break
			}
			if strings.EqualFold(currSection, accountName) {
				accStart = i
			}
		}
	}
	if accStart >= 0 && accEnd == -1 {
		accEnd = blockEnd
	}

	if accStart >= 0 {
		var newLines []string
		newLines = append(newLines, lines[:accStart]...)
		newLines = append(newLines, lines[accEnd:]...)
		return os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0600)
	}

	return nil
}
