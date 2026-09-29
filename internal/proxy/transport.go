package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
	xproxy "golang.org/x/net/proxy"
)

type ProxyRouter struct {
	proxies  map[string]config.ProxyConfig
	bindings map[string]config.ProxyBinding
}

func NewProxyRouter(proxies []config.ProxyConfig, bindings []config.ProxyBinding) *ProxyRouter {
	router := &ProxyRouter{
		proxies:  make(map[string]config.ProxyConfig, len(proxies)),
		bindings: make(map[string]config.ProxyBinding, len(bindings)),
	}
	for _, p := range proxies {
		router.proxies[p.ID] = p
	}
	for _, b := range bindings {
		router.bindings[config.ProxyBindingKey(b.Provider, b.Account)] = b
	}
	return router
}

func (r *ProxyRouter) HasBinding(provider, account string) bool {
	if r == nil {
		return false
	}
	_, ok := r.bindings[config.ProxyBindingKey(provider, account)]
	return ok
}

func (r *ProxyRouter) HTTPClient(provider, account string, timeout time.Duration) (*http.Client, error) {
	if r == nil {
		return &http.Client{Timeout: timeout}, nil
	}
	binding, ok := r.bindings[config.ProxyBindingKey(provider, account)]
	if !ok {
		return &http.Client{Timeout: timeout}, nil
	}
	primary, ok := r.proxies[binding.ProxyID]
	if !ok {
		return nil, fmt.Errorf("primary proxy %q not found", binding.ProxyID)
	}
	primaryTransport, err := TransportForProxy(primary.URL)
	if err != nil {
		return nil, err
	}
	fallbackID := config.NormalizeProxyFallback(binding.FallbackProxyID)
	if fallbackID == config.ProxyFallbackNone {
		return &http.Client{Timeout: timeout, Transport: primaryTransport}, nil
	}

	var fallback http.RoundTripper
	if fallbackID == config.ProxyFallbackDirect {
		fallback = DirectTransport()
	} else {
		fallbackProxy, ok := r.proxies[fallbackID]
		if !ok {
			return nil, fmt.Errorf("fallback proxy %q not found", fallbackID)
		}
		fallback, err = TransportForProxy(fallbackProxy.URL)
		if err != nil {
			return nil, err
		}
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &fallbackRoundTripper{
			primary:  primaryTransport,
			fallback: fallback,
		},
	}, nil
}

func TransportForProxy(raw string) (http.RoundTripper, error) {
	normalized, err := config.NormalizeProxyURL(raw)
	if err != nil {
		return nil, err
	}
	parsed, _ := url.Parse(normalized)
	if parsed.Scheme == "http" || parsed.Scheme == "https" {
		transport := DirectTransport()
		transport.Proxy = http.ProxyURL(parsed)
		return transport, nil
	}

	var auth *xproxy.Auth
	if parsed.User != nil {
		password, _ := parsed.User.Password()
		auth = &xproxy.Auth{User: parsed.User.Username(), Password: password}
	}
	dialer, err := xproxy.SOCKS5("tcp", parsed.Host, auth, xproxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("create SOCKS5 dialer: %w", err)
	}
	transport := DirectTransport()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if contextDialer, ok := dialer.(xproxy.ContextDialer); ok {
			return contextDialer.DialContext(ctx, network, address)
		}
		return dialer.Dial(network, address)
	}
	return transport, nil
}

func DirectTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return transport
}

type fallbackRoundTripper struct {
	primary  http.RoundTripper
	fallback http.RoundTripper
}

func (t *fallbackRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.primary.RoundTrip(req)
	if err == nil && !isProxyGatewayFailure(resp.StatusCode) {
		return resp, nil
	}
	if resp != nil {
		if resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}
	fallbackRequest, cloneErr := cloneRequestForRetry(req)
	if cloneErr != nil {
		if err != nil {
			return nil, err
		}
		return nil, cloneErr
	}
	return t.fallback.RoundTrip(fallbackRequest)
}

func isProxyGatewayFailure(status int) bool {
	return status == http.StatusProxyAuthRequired || status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func cloneRequestForRetry(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if req.Body == nil {
		return clone, nil
	}
	if req.GetBody == nil {
		return nil, fmt.Errorf("request body cannot be replayed through fallback proxy")
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	clone.Body = body
	return clone, nil
}
