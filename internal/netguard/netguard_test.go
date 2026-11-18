package netguard

import (
	"net"
	"testing"
)

func TestValidateIP(t *testing.T) {
	tests := []struct {
		name         string
		ip           string
		allowPrivate bool
		wantErr      bool
	}{
		{"public v4 allowed strict", "8.8.8.8", false, false},
		{"public v4 allowed permissive", "8.8.8.8", true, false},
		{"metadata always blocked permissive", "169.254.169.254", true, true},
		{"metadata always blocked strict", "169.254.169.254", false, true},
		{"ipv4 mapped metadata always blocked", "::ffff:169.254.169.254", true, true},
		{"link local v6 always blocked", "fe80::1", true, true},
		{"aws imds v6 always blocked", "fd00:ec2::254", true, true},
		{"unspecified always blocked", "0.0.0.0", true, true},
		{"loopback allowed when private allowed", "127.0.0.1", true, false},
		{"loopback blocked when strict", "127.0.0.1", false, true},
		{"private rfc1918 allowed when private allowed", "192.168.1.10", true, false},
		{"private rfc1918 blocked when strict", "192.168.1.10", false, true},
		{"docker bridge allowed when private allowed", "172.17.0.1", true, false},
		{"docker bridge blocked when strict", "172.17.0.1", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			err := ValidateIP(ip, tt.allowPrivate)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateIP(%q, %v) err=%v, wantErr=%v", tt.ip, tt.allowPrivate, err, tt.wantErr)
			}
		})
	}
}

func TestIsPrivateProxyBlockedEnv(t *testing.T) {
	t.Setenv("BLOCK_PRIVATE_PROXY", "")
	if IsPrivateProxyBlocked() {
		t.Fatal("expected BLOCK_PRIVATE_PROXY=empty to allow private proxies")
	}
	t.Setenv("BLOCK_PRIVATE_PROXY", "true")
	if !IsPrivateProxyBlocked() {
		t.Fatal("expected BLOCK_PRIVATE_PROXY=true to block private proxies")
	}
}
