package dns

import (
	"fmt"
	"strings"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

type DNSRecord struct {
	ID       string `json:"id"`
	Domain   string `json:"domain"`
	Name     string `json:"name"`               // Host/RR e.g. "@", "www", "*"
	FQDN     string `json:"fqdn"`               // Full domain e.g. "www.example.com"
	Type     string `json:"type"`               // A, AAAA, CNAME, MX, TXT, NS, SRV, CAA
	Value    string `json:"value"`
	TTL      int    `json:"ttl"`
	Priority *int   `json:"priority,omitempty"` // MX / SRV priority
	Proxied  bool   `json:"proxied,omitempty"`  // Cloudflare only
	Line     string `json:"line,omitempty"`     // Alidns / Tencent / DNSPod
	LineID   string `json:"lineId,omitempty"`
	Status   string `json:"status,omitempty"`   // "ENABLE" | "DISABLE"
	Remark   string `json:"remark,omitempty"`
}

type DNSCapabilities struct {
	Provider        string   `json:"provider"`
	Edition         string   `json:"edition,omitempty"`
	SupportsProxied bool     `json:"supportsProxied"`
	SupportsLine    bool     `json:"supportsLine"`
	SupportsStatus  bool     `json:"supportsStatus"`
	SupportsRemark  bool     `json:"supportsRemark"`
	DefaultLine     string   `json:"defaultLine,omitempty"`
	AvailableLines  []string `json:"availableLines,omitempty"`
}

type DNSDomainInfo struct {
	Domain string `json:"domain"`
	ZoneID string `json:"zoneId,omitempty"`
}

type DNSImportPreviewItem struct {
	Action string    `json:"action"` // "create" | "conflict" | "skip_system"
	Reason string    `json:"reason,omitempty"`
	Record DNSRecord `json:"record"`
	Target string    `json:"existingId,omitempty"`
	OldVal string    `json:"existingValue,omitempty"`
}

type DNSImportPreviewResult struct {
	Domain        string                 `json:"domain"`
	TotalParsed   int                    `json:"totalParsed"`
	CreateCount   int                    `json:"createCount"`
	ConflictCount int                    `json:"conflictCount"`
	SkipCount     int                    `json:"skipCount"`
	Warnings      []string               `json:"warnings,omitempty"`
	Items         []DNSImportPreviewItem `json:"items"`
}

type DNSImportExecuteResult struct {
	Success  bool     `json:"success"`
	Created  int      `json:"created"`
	Updated  int      `json:"updated"`
	Skipped  int      `json:"skipped"`
	Failed   int      `json:"failed"`
	Logs     []string `json:"logs"`
}

type DNSClient interface {
	Capabilities() DNSCapabilities
	ResolveConfiguredDomains() ([]DNSDomainInfo, error)
	ListRecords(domain string) ([]DNSRecord, error)
	CreateRecord(domain string, rec DNSRecord) (*DNSRecord, error)
	UpdateRecord(domain string, recordID string, rec DNSRecord) (*DNSRecord, error)
	DeleteRecord(domain string, recordID string) error
	SetRecordStatus(domain string, recordID string, enabled bool) error
	UpsertBindingRecord(binding config.DNSBinding, ip string) error
}

func normalizeRootDomain(domain string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
}

func splitFQDNToRR(fqdnOrRR, rootDomain string) string {
	root := normalizeRootDomain(rootDomain)
	raw := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(fqdnOrRR), "."))
	if raw == "" || raw == "@" || (root != "" && raw == root) {
		return "@"
	}
	if root != "" && strings.HasSuffix(raw, "."+root) {
		rr := strings.TrimSuffix(raw, "."+root)
		if rr == "" {
			return "@"
		}
		return rr
	}
	return raw
}

func rrToFQDN(rrOrFQDN, rootDomain string) string {
	root := normalizeRootDomain(rootDomain)
	raw := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(rrOrFQDN), "."))
	if raw == "" || raw == "@" {
		return root
	}
	if root == "" {
		return raw
	}
	if raw == root || strings.HasSuffix(raw, "."+root) {
		return raw
	}
	return raw + "." + root
}

func matchConfiguredDomain(fqdn string, configured []string) (rootDomain, rr string, err error) {
	target := normalizeRootDomain(fqdn)
	if target == "" {
		return "", "", fmt.Errorf("domain is empty")
	}
	best := ""
	for _, c := range configured {
		cand := normalizeRootDomain(c)
		if cand == "" {
			continue
		}
		if target == cand || strings.HasSuffix(target, "."+cand) {
			if len(cand) > len(best) {
				best = cand
			}
		}
	}
	if best != "" {
		return best, splitFQDNToRR(target, best), nil
	}
	// If only one configured domain is present and target is a relative RR (no dot), use it.
	if len(configured) == 1 && !strings.Contains(target, ".") {
		best = normalizeRootDomain(configured[0])
		return best, target, nil
	}
	// Fallback: derive last two labels if no configured domains list was specified
	if len(configured) == 0 {
		parts := strings.Split(target, ".")
		if len(parts) >= 2 {
			best = strings.Join(parts[len(parts)-2:], ".")
			return best, splitFQDNToRR(target, best), nil
		}
		return target, "@", nil
	}
	return "", "", fmt.Errorf("域名 %q 不在账号配置的管理域名列表 (%s) 中", fqdn, strings.Join(configured, ", "))
}
