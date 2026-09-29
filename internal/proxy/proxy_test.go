package proxy

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestFallbackRoundTripperReplaysRequestBody(t *testing.T) {
	var primaryBody string
	var fallbackBody string
	transport := &fallbackRoundTripper{
		primary: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			data, _ := io.ReadAll(req.Body)
			primaryBody = string(data)
			return nil, errors.New("primary unavailable")
		}),
		fallback: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			data, _ := io.ReadAll(req.Body)
			fallbackBody = string(data)
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	}

	req, err := http.NewRequest(http.MethodPost, "https://management.azure.com/test", bytes.NewBufferString(`{"action":"start"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	_ = resp.Body.Close()
	if primaryBody != `{"action":"start"}` || fallbackBody != primaryBody {
		t.Fatalf("body replay mismatch: primary=%q fallback=%q", primaryBody, fallbackBody)
	}
}

func TestProxyRouterUsesAccountRoute(t *testing.T) {
	router := NewProxyRouter(
		[]config.ProxyConfig{{ID: "primary", URL: "http://127.0.0.1:8080"}},
		[]config.ProxyBinding{{Provider: "azure", Account: "account", ProxyID: "primary", FallbackProxyID: config.ProxyFallbackDirect}},
	)
	routed, err := router.HTTPClient("azure", "account", 60*time.Second)
	if err != nil {
		t.Fatalf("HTTPClient: %v", err)
	}
	if _, ok := routed.Transport.(*fallbackRoundTripper); !ok {
		t.Fatalf("routed client transport = %T, want *fallbackRoundTripper", routed.Transport)
	}
	routedTransport := routed.Transport.(*fallbackRoundTripper)
	directFallback, ok := routedTransport.fallback.(*http.Transport)
	if !ok {
		t.Fatalf("direct fallback transport = %T, want *http.Transport", routedTransport.fallback)
	}
	if directFallback.Proxy != nil {
		t.Fatal("direct fallback must not use environment proxy settings")
	}
	direct, err := router.HTTPClient("azure", "unbound", 60*time.Second)
	if err != nil {
		t.Fatalf("HTTPClient unbound: %v", err)
	}
	if direct.Transport != nil {
		t.Fatalf("unbound account transport = %T, want default direct transport", direct.Transport)
	}
	if routed.Timeout != 60*time.Second {
		t.Fatalf("routed timeout = %v", routed.Timeout)
	}
}

func TestProxyRouterSupportsNoFallback(t *testing.T) {
	router := NewProxyRouter(
		[]config.ProxyConfig{{ID: "primary", URL: "http://127.0.0.1:8080"}},
		[]config.ProxyBinding{{Provider: "azure", Account: "account", ProxyID: "primary", FallbackProxyID: config.ProxyFallbackNone}},
	)
	routed, err := router.HTTPClient("azure", "account", 60*time.Second)
	if err != nil {
		t.Fatalf("HTTPClient: %v", err)
	}
	if _, ok := routed.Transport.(*fallbackRoundTripper); ok {
		t.Fatal("none fallback must not install a fallback round tripper")
	}
	primary, ok := routed.Transport.(*http.Transport)
	if !ok || primary.Proxy == nil {
		t.Fatalf("primary proxy transport = %T, want configured *http.Transport", routed.Transport)
	}
}
