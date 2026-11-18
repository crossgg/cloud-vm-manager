package updater

import (
	"archive/zip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGithubReleaseJSONIncludesBody(t *testing.T) {
	raw := `{"tag_name":"v1.2.0","name":"v1.2.0","body":"### 新增\n- 支持策略管理","html_url":"https://github.com/crossgg/cloud-vm-manager/releases/tag/v1.2.0","assets":[]}`
	var rel GithubRelease
	if err := json.Unmarshal([]byte(raw), &rel); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if rel.TagName != "v1.2.0" || rel.Body != "### 新增\n- 支持策略管理" {
		t.Fatalf("unexpected release parsed: %+v", rel)
	}
}

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
		if got := UpdateAssetName(tt.goos, tt.goarch, tt.goarm); got != tt.want {
			t.Fatalf("updateAssetName(%q, %q, %q) = %q, want %q", tt.goos, tt.goarch, tt.goarm, got, tt.want)
		}
	}
}

func TestFileSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := FileSHA256(path)
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
	if got := ProxiedDownloadURL(rawURL, ""); got != rawURL {
		t.Fatalf("expected empty proxy to keep raw URL, got %q", got)
	}
	want := "https://gh-proxy.com/" + rawURL
	if got := ProxiedDownloadURL(rawURL, "https://gh-proxy.com"); got != want {
		t.Fatalf("proxiedDownloadURL = %q, want %q", got, want)
	}
}

func TestReplaceDirectoryRollback(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "public")
	source := filepath.Join(root, "update", "public")
	writeTestAsset(t, filepath.Join(target, "index.html"), "old")
	writeTestAsset(t, filepath.Join(source, "index.html"), "new")

	replacement, err := ReplaceDirectory(source, target)
	if err != nil {
		t.Fatalf("replaceDirectory returned error: %v", err)
	}
	assertTestAsset(t, filepath.Join(target, "index.html"), "new")
	if err := replacement.Rollback(); err != nil {
		t.Fatalf("rollback returned error: %v", err)
	}
	assertTestAsset(t, filepath.Join(target, "index.html"), "old")
}

func TestReplaceDirectoryCommit(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "public")
	source := filepath.Join(root, "update", "public")
	writeTestAsset(t, filepath.Join(target, "index.html"), "old")
	writeTestAsset(t, filepath.Join(source, "index.html"), "new")

	replacement, err := ReplaceDirectory(source, target)
	if err != nil {
		t.Fatalf("replaceDirectory returned error: %v", err)
	}
	if err := replacement.Commit(); err != nil {
		t.Fatalf("commit returned error: %v", err)
	}
	assertTestAsset(t, filepath.Join(target, "index.html"), "new")
	if _, err := os.Stat(target + ".previous"); !os.IsNotExist(err) {
		t.Fatalf("expected backup directory to be removed, got %v", err)
	}
}

func writeTestAsset(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertTestAsset(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("asset %s = %q, want %q", path, data, want)
	}
}

func TestValidateDownloadProxyURL(t *testing.T) {
	if _, err := ValidateDownloadProxyURL("https://gh-proxy.com/"); err != nil {
		t.Fatalf("expected valid https proxy, got %v", err)
	}
	for _, bad := range []string{
		"file:///etc/passwd",
		"https://user:pass@proxy.example.com/",
		"https://proxy.example.com/?a=1",
		"https://proxy.example.com/\n[auth]",
	} {
		if _, err := ValidateDownloadProxyURL(bad); err == nil {
			t.Fatalf("expected error for %q, got nil", bad)
		}
	}
}

func TestFetchLatestReleaseDoesNotLeakTokenToProxy(t *testing.T) {
	var sawAuthHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuthHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.0.0","assets":[]}`))
	}))
	defer srv.Close()

	t.Setenv("GITHUB_TOKEN", "ghp_secret_token_should_not_leak")
	rel, err := FetchLatestRelease(srv.URL)
	if err != nil {
		t.Fatalf("FetchLatestRelease failed: %v", err)
	}
	if rel.TagName != "v1.0.0" {
		t.Fatalf("unexpected tag: %q", rel.TagName)
	}
	if sawAuthHeader != "" {
		t.Fatalf("expected empty Authorization header when using proxy, got %q", sawAuthHeader)
	}
}

func TestExtractArchiveFromZipRejectsZipSlip(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "evil.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, content := range map[string]string{
		"cloud-vm-manager":       "bin",
		"public/../../pwned.txt": "evil",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	destBin := filepath.Join(root, "bin")
	destPublic := filepath.Join(root, "out", "public")
	if _, err := extractArchiveFromZip(zipPath, destBin, destPublic); err == nil || !strings.Contains(err.Error(), "escapes target directory") {
		t.Fatalf("expected Zip Slip rejection error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "pwned.txt")); !os.IsNotExist(err) {
		t.Fatalf("evil file should not exist outside target dir")
	}
}
