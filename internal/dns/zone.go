package dns

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// ParseBINDZone parses an RFC 1035 BIND zone file into normalized DNSRecord entries.
// System SOA records and apex (@) NS records are returned in warnings and skipped from importable records.
func ParseBINDZone(rootDomain, content string) ([]DNSRecord, []string, error) {
	root := normalizeRootDomain(rootDomain)
	if root == "" {
		return nil, nil, fmt.Errorf("目标根域名不能为空")
	}

	logicalLines, err := joinZoneLogicalLines(content)
	if err != nil {
		return nil, nil, err
	}

	origin := root
	defaultTTL := 600
	lastOwner := "@"
	records := make([]DNSRecord, 0)
	warnings := make([]string, 0)

	for _, line := range logicalLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "$ORIGIN") {
			fields := tokenizeZoneLine(trimmed)
			if len(fields) >= 2 {
				val := strings.TrimSuffix(strings.ToLower(fields[1]), ".")
				if val != "" {
					origin = val
				}
			}
			continue
		}
		if strings.HasPrefix(upper, "$TTL") {
			fields := tokenizeZoneLine(trimmed)
			if len(fields) >= 2 {
				if parsedTTL, ok := parseZoneTTL(fields[1]); ok && parsedTTL > 0 {
					defaultTTL = parsedTTL
				}
			}
			continue
		}
		if strings.HasPrefix(upper, "$") {
			continue
		}

		// Leading whitespace means owner is inherited from previous record
		startsWithSpace := len(line) > 0 && unicode.IsSpace(rune(line[0]))
		tokens := tokenizeZoneLine(trimmed)
		if len(tokens) == 0 {
			continue
		}

		idx := 0
		ownerToken := lastOwner
		if !startsWithSpace {
			ownerToken = tokens[0]
			idx = 1
		}
		lastOwner = ownerToken

		ttl := defaultTTL
		// Optional [TTL] [CLASS] or [CLASS] [TTL]
		for step := 0; step < 2 && idx < len(tokens); step++ {
			tUpper := strings.ToUpper(tokens[idx])
			if tUpper == "IN" || tUpper == "CH" || tUpper == "HS" {
				idx++
				continue
			}
			if parsedTTL, ok := parseZoneTTL(tokens[idx]); ok {
				if parsedTTL > 0 {
					ttl = parsedTTL
				}
				idx++
				continue
			}
			break
		}

		if idx >= len(tokens) {
			continue
		}
		recType := strings.ToUpper(tokens[idx])
		idx++
		if idx >= len(tokens) {
			continue
		}

		fqdn := resolveZoneOwnerFQDN(ownerToken, origin, root)
		rr := splitFQDNToRR(fqdn, root)
		fqdn = rrToFQDN(rr, root)

		if recType == "SOA" {
			warnings = append(warnings, fmt.Sprintf("跳过系统 SOA 记录 (%s)", fqdn))
			continue
		}
		if recType == "NS" && rr == "@" {
			warnings = append(warnings, fmt.Sprintf("跳过根域系统 NS 记录 (%s -> %s)", fqdn, strings.TrimSuffix(tokens[idx], ".")))
			continue
		}

		rec := DNSRecord{
			Domain: root,
			Name:   rr,
			FQDN:   fqdn,
			Type:   recType,
			TTL:    ttl,
			Status: "ENABLE",
		}

		switch recType {
		case "MX":
			if idx+1 >= len(tokens) {
				warnings = append(warnings, fmt.Sprintf("跳过格式不完整的 MX 记录: %s", trimmed))
				continue
			}
			prio, err := strconv.Atoi(tokens[idx])
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("跳过无效 MX 优先级记录: %s", trimmed))
				continue
			}
			rec.Priority = &prio
			rec.Value = normalizeZoneTarget(tokens[idx+1], origin)
		case "SRV":
			if idx+3 >= len(tokens) {
				warnings = append(warnings, fmt.Sprintf("跳过格式不完整的 SRV 记录: %s", trimmed))
				continue
			}
			prio, err := strconv.Atoi(tokens[idx])
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("跳过无效 SRV 优先级记录: %s", trimmed))
				continue
			}
			rec.Priority = &prio
			target := normalizeZoneTarget(tokens[idx+3], origin)
			rec.Value = fmt.Sprintf("%s %s %s", tokens[idx+1], tokens[idx+2], target)
		case "TXT":
			rec.Value = strings.Join(tokens[idx:], "")
		case "CNAME", "NS", "PTR":
			rec.Value = normalizeZoneTarget(tokens[idx], origin)
		case "CAA":
			if idx+2 >= len(tokens) {
				rec.Value = strings.Join(tokens[idx:], " ")
			} else {
				rec.Value = fmt.Sprintf("%s %s \"%s\"", tokens[idx], tokens[idx+1], tokens[idx+2])
			}
		default:
			rec.Value = strings.Join(tokens[idx:], " ")
		}

		if strings.TrimSpace(rec.Value) == "" {
			continue
		}
		records = append(records, rec)
	}

	return records, warnings, nil
}

func PreviewBINDImport(rootDomain, content string, existing []DNSRecord) (*DNSImportPreviewResult, error) {
	parsed, warnings, err := ParseBINDZone(rootDomain, content)
	if err != nil {
		return nil, err
	}

	// Build lookup map by (RR, Type) -> existing record(s)
	type key struct {
		rr      string
		recType string
	}
	existingByKey := make(map[key]DNSRecord)
	for _, ex := range existing {
		k := key{
			rr:      splitFQDNToRR(ex.Name, rootDomain),
			recType: strings.ToUpper(ex.Type),
		}
		if _, exists := existingByKey[k]; !exists {
			existingByKey[k] = ex
		}
	}

	res := &DNSImportPreviewResult{
		Domain:      normalizeRootDomain(rootDomain),
		TotalParsed: len(parsed),
		Warnings:    warnings,
		SkipCount:   len(warnings),
		Items:       make([]DNSImportPreviewItem, 0, len(parsed)),
	}

	for _, rec := range parsed {
		k := key{
			rr:      splitFQDNToRR(rec.Name, rootDomain),
			recType: strings.ToUpper(rec.Type),
		}
		if ex, ok := existingByKey[k]; ok {
			res.ConflictCount++
			res.Items = append(res.Items, DNSImportPreviewItem{
				Action: "conflict",
				Reason: "线上已存在同名同类型记录",
				Record: rec,
				Target: ex.ID,
				OldVal: ex.Value,
			})
		} else {
			res.CreateCount++
			res.Items = append(res.Items, DNSImportPreviewItem{
				Action: "create",
				Record: rec,
			})
		}
	}

	return res, nil
}

func ExportBINDZone(rootDomain string, records []DNSRecord) string {
	root := normalizeRootDomain(rootDomain)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(";; BIND Zone Export for %s\n", root))
	sb.WriteString(fmt.Sprintf("$ORIGIN %s.\n", root))
	sb.WriteString("$TTL 600\n\n")

	sorted := make([]DNSRecord, len(records))
	copy(sorted, records)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Type != sorted[j].Type {
			return sorted[i].Type < sorted[j].Type
		}
		return sorted[i].Name < sorted[j].Name
	})

	for _, r := range sorted {
		rr := splitFQDNToRR(r.Name, root)
		owner := rr
		if owner != "@" {
			owner = rrToFQDN(rr, root) + "."
		}
		ttl := r.TTL
		if ttl <= 0 {
			ttl = 600
		}
		t := strings.ToUpper(strings.TrimSpace(r.Type))
		val := strings.TrimSpace(r.Value)

		prefix := ""
		if strings.EqualFold(r.Status, "DISABLE") {
			prefix = "; [DISABLED] "
		}

		switch t {
		case "MX":
			prio := 10
			if r.Priority != nil {
				prio = *r.Priority
			}
			sb.WriteString(fmt.Sprintf("%s%-24s %d\tIN\t%s\t%d %s\n", prefix, owner, ttl, t, prio, formatZoneTarget(val)))
		case "SRV":
			prio := 0
			if r.Priority != nil {
				prio = *r.Priority
			}
			parts := strings.Fields(val)
			if len(parts) == 3 {
				sb.WriteString(fmt.Sprintf("%s%-24s %d\tIN\t%s\t%d %s %s %s\n", prefix, owner, ttl, t, prio, parts[0], parts[1], formatZoneTarget(parts[2])))
			} else {
				sb.WriteString(fmt.Sprintf("%s%-24s %d\tIN\t%s\t%s\n", prefix, owner, ttl, t, val))
			}
		case "CNAME", "NS", "PTR":
			sb.WriteString(fmt.Sprintf("%s%-24s %d\tIN\t%s\t%s\n", prefix, owner, ttl, t, formatZoneTarget(val)))
		case "TXT":
			escaped := strings.ReplaceAll(val, `"`, `\"`)
			sb.WriteString(fmt.Sprintf("%s%-24s %d\tIN\t%s\t\"%s\"\n", prefix, owner, ttl, t, escaped))
		default:
			sb.WriteString(fmt.Sprintf("%s%-24s %d\tIN\t%s\t%s\n", prefix, owner, ttl, t, val))
		}
	}
	return sb.String()
}

func formatZoneTarget(val string) string {
	trimmed := strings.TrimSpace(val)
	if trimmed == "" || trimmed == "@" || strings.HasSuffix(trimmed, ".") {
		return trimmed
	}
	return trimmed + "."
}

func resolveZoneOwnerFQDN(ownerToken, origin, root string) string {
	tok := strings.TrimSpace(ownerToken)
	if tok == "" || tok == "@" {
		return origin
	}
	if strings.HasSuffix(tok, ".") {
		return strings.ToLower(strings.TrimSuffix(tok, "."))
	}
	return strings.ToLower(tok) + "." + origin
}

func normalizeZoneTarget(target, origin string) string {
	tok := strings.TrimSpace(target)
	if tok == "" {
		return ""
	}
	if tok == "@" {
		return origin
	}
	if strings.HasSuffix(tok, ".") {
		return strings.TrimSuffix(tok, ".")
	}
	if !strings.Contains(tok, ".") && origin != "" {
		return tok + "." + origin
	}
	return tok
}

func parseZoneTTL(tok string) (int, bool) {
	if tok == "" {
		return 0, false
	}
	if v, err := strconv.Atoi(tok); err == nil && v >= 0 {
		return v, true
	}
	// Support BIND duration suffixes like 1h, 10m, 1d, 1w
	lower := strings.ToLower(tok)
	last := lower[len(lower)-1]
	numPart := lower[:len(lower)-1]
	n, err := strconv.Atoi(numPart)
	if err != nil || n < 0 {
		return 0, false
	}
	switch last {
	case 's':
		return n, true
	case 'm':
		return n * 60, true
	case 'h':
		return n * 3600, true
	case 'd':
		return n * 86400, true
	case 'w':
		return n * 604800, true
	default:
		return 0, false
	}
}

func joinZoneLogicalLines(content string) ([]string, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	rawLines := strings.Split(content, "\n")
	out := make([]string, 0, len(rawLines))
	var buf strings.Builder
	parenDepth := 0

	for _, raw := range rawLines {
		clean := stripZoneComment(raw)
		if strings.TrimSpace(clean) == "" && parenDepth == 0 {
			continue
		}
		if parenDepth == 0 {
			buf.Reset()
		} else {
			buf.WriteByte(' ')
		}

		inQuote := false
		escaped := false
		for i := 0; i < len(clean); i++ {
			ch := clean[i]
			if escaped {
				buf.WriteByte(ch)
				escaped = false
				continue
			}
			if ch == '\\' {
				buf.WriteByte(ch)
				escaped = true
				continue
			}
			if ch == '"' {
				inQuote = !inQuote
				buf.WriteByte(ch)
				continue
			}
			if !inQuote {
				if ch == '(' {
					parenDepth++
					continue
				}
				if ch == ')' {
					if parenDepth > 0 {
						parenDepth--
					}
					continue
				}
			}
			buf.WriteByte(ch)
		}

		if parenDepth == 0 {
			out = append(out, buf.String())
		}
	}
	if parenDepth > 0 {
		return nil, fmt.Errorf("BIND Zone 文件括号未闭合")
	}
	return out, nil
}

func stripZoneComment(line string) string {
	inQuote := false
	escaped := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			inQuote = !inQuote
			continue
		}
		if !inQuote && ch == ';' {
			return line[:i]
		}
	}
	return line
}

func tokenizeZoneLine(line string) []string {
	tokens := make([]string, 0)
	var cur strings.Builder
	inQuote := false
	hasToken := false

	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inQuote {
			if ch == '\\' && i+1 < len(line) {
				i++
				cur.WriteByte(line[i])
				continue
			}
			if ch == '"' {
				inQuote = false
				continue
			}
			cur.WriteByte(ch)
			continue
		}
		if ch == '"' {
			inQuote = true
			hasToken = true
			continue
		}
		if unicode.IsSpace(rune(ch)) {
			if hasToken {
				tokens = append(tokens, cur.String())
				cur.Reset()
				hasToken = false
			}
			continue
		}
		cur.WriteByte(ch)
		hasToken = true
	}
	if hasToken {
		tokens = append(tokens, cur.String())
	}
	return tokens
}
