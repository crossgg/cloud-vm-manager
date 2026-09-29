package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectPublicDirPrefersCompleteRuntimeUpdate(t *testing.T) {
	root := t.TempDir()
	updated := filepath.Join(root, "runtime", "public")
	bundled := filepath.Join(root, "bundled", "public")
	writeTestAsset(t, filepath.Join(bundled, "index.html"), "bundled")

	if got := SelectPublicDir(updated, bundled); got != bundled {
		t.Fatalf("selectPublicDir without update = %q, want %q", got, bundled)
	}
	writeTestAsset(t, filepath.Join(updated, "index.html"), "updated")
	if got := SelectPublicDir(updated, bundled); got != updated {
		t.Fatalf("selectPublicDir with update = %q, want %q", got, updated)
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
