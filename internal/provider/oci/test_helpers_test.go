package oci

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/crossgg/cloud-vm-manager/internal/config"
)

type testRoundTripFunc func(*http.Request) (*http.Response, error)

func (f testRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func newTestOCIService(t *testing.T, handler http.HandlerFunc) (*OCIService, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate test RSA key: %v", err)
	}

	svc := NewOCIService(config.OCIConfig{
		Name:          "oci-test",
		User:          "ocid1.user.oc1..test",
		Fingerprint:   "aa:bb:cc",
		Tenancy:       "ocid1.tenancy.oc1..test",
		CompartmentID: "ocid1.compartment.oc1..test",
		Region:        "us-ashburn-1",
	})
	svc.key = key
	svc.Client = &http.Client{
		Transport: testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			cloned := req.Clone(req.Context())
			cloned.Header.Set("X-Orig-Host", req.URL.Host)
			cloned.URL.Scheme = "http"
			cloned.URL.Host = strings.TrimPrefix(srv.URL, "http://")
			return srv.Client().Transport.RoundTrip(cloned)
		}),
	}
	return svc, srv.Close
}
