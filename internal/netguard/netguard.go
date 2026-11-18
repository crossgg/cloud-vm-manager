package netguard

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"syscall"
	"time"
)

var (
	cgnatPrefix     = netip.MustParsePrefix("100.64.0.0/10")
	zeroNetPrefix   = netip.MustParsePrefix("0.0.0.0/8")
	awsIMDSv6Prefix = netip.MustParsePrefix("fd00:ec2::254/128")
)

// IsPrivateProxyBlocked reports whether BLOCK_PRIVATE_PROXY is enabled.
func IsPrivateProxyBlocked() bool {
	val := strings.ToLower(strings.TrimSpace(os.Getenv("BLOCK_PRIVATE_PROXY")))
	return val == "1" || val == "true" || val == "yes" || val == "on"
}

// AllowPrivateProxy reports whether local/private proxy addresses are permitted.
func AllowPrivateProxy() bool {
	return !IsPrivateProxyBlocked()
}

// ValidateIP checks whether a resolved IP address is safe to dial.
// Cloud metadata and link-local/unspecified/multicast addresses are always blocked.
// Loopback and private network addresses are blocked when allowPrivate is false.
func ValidateIP(ip net.IP, allowPrivate bool) error {
	if ip == nil {
		return fmt.Errorf("empty IP address")
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return fmt.Errorf("invalid IP address")
	}
	addr = addr.Unmap()

	if addr.IsUnspecified() || zeroNetPrefix.Contains(addr) {
		return fmt.Errorf("blocked unspecified IP address %s", addr)
	}
	if addr.IsMulticast() {
		return fmt.Errorf("blocked multicast IP address %s", addr)
	}
	if addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || awsIMDSv6Prefix.Contains(addr) {
		return fmt.Errorf("blocked link-local/metadata IP address %s", addr)
	}

	if !allowPrivate {
		if addr.IsLoopback() {
			return fmt.Errorf("blocked loopback IP address %s (BLOCK_PRIVATE_PROXY is enabled)", addr)
		}
		if addr.IsPrivate() || cgnatPrefix.Contains(addr) {
			return fmt.Errorf("blocked private network IP address %s (BLOCK_PRIVATE_PROXY is enabled)", addr)
		}
	}
	return nil
}

// Dialer returns a net.Dialer whose Control hook validates resolved IPs before connecting.
func Dialer(timeout time.Duration, allowPrivate bool) *net.Dialer {
	return &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				host = address
			}
			// Strip IPv6 zone if present
			if idx := strings.Index(host, "%"); idx >= 0 {
				host = host[:idx]
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("invalid dial IP %q", host)
			}
			return ValidateIP(ip, allowPrivate)
		},
	}
}

// NewRestrictedTransport returns an http.Transport guarded against SSRF.
func NewRestrictedTransport(timeout time.Duration, allowPrivate bool) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.IdleConnTimeout = 15 * time.Second
	transport.DialContext = Dialer(timeout, allowPrivate).DialContext
	return transport
}

// NewRestrictedHTTPClient returns an http.Client using a restricted transport and validating redirects.
func NewRestrictedHTTPClient(timeout time.Duration, allowPrivate bool, allowRedirects bool) *http.Client {
	client := &http.Client{
		Timeout:   timeout,
		Transport: NewRestrictedTransport(timeout, allowPrivate),
	}
	if !allowRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	} else {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			scheme := strings.ToLower(req.URL.Scheme)
			if scheme != "http" && scheme != "https" {
				return fmt.Errorf("blocked redirect to unsupported scheme %q", req.URL.Scheme)
			}
			return nil
		}
	}
	return client
}
