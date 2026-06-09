package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateAssetName(t *testing.T) {
	tests := []struct {
		goos   string
		goarch string
		goarm  string
		want   string
	}{
		{"linux", "amd64", "", "cloud-vm-manager_linux_amd64.tar.gz"},
		{"linux", "arm64", "", "cloud-vm-manager_linux_arm64.tar.gz"},
		{"linux", "arm", "7", "cloud-vm-manager_linux_armv7.tar.gz"},
		{"windows", "amd64", "", "cloud-vm-manager_windows_amd64.zip"},
	}

	for _, tt := range tests {
		if got := updateAssetName(tt.goos, tt.goarch, tt.goarm); got != tt.want {
			t.Fatalf("updateAssetName(%q, %q, %q) = %q, want %q", tt.goos, tt.goarch, tt.goarm, got, tt.want)
		}
	}
}

func TestFileSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := fileSHA256(path)
	if err != nil {
		t.Fatalf("fileSHA256 returned error: %v", err)
	}
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Fatalf("fileSHA256 = %q, want %q", got, want)
	}
}

func TestProxiedDownloadURL(t *testing.T) {
	const rawURL = "https://github.com/owner/repo/releases/download/v1/app.tar.gz"
	if got := proxiedDownloadURL(rawURL, ""); got != rawURL {
		t.Fatalf("expected empty proxy to keep raw URL, got %q", got)
	}
	want := "https://gh-proxy.com/" + rawURL
	if got := proxiedDownloadURL(rawURL, "https://gh-proxy.com"); got != want {
		t.Fatalf("proxiedDownloadURL = %q, want %q", got, want)
	}
}
