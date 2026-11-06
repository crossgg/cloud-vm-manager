package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndDeleteCloudAccount(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "config.conf")

	initialContent := `oci=begin
[oci01]
group=oci
user=ocid1.user.oc1.test
key_file=/app/config/keys/oci.pem
oci=end
`
	if err := os.WriteFile(confPath, []byte(initialContent), 0600); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	// 1. Add new GCP account
	gcpFields := map[string]string{
		"group":        "my-gcp",
		"project_id":   "test-proj-123",
		"client_email": "sa@test-proj-123.iam.gserviceaccount.com",
		"key_file":     "/app/config/keys/gcp.pem",
	}
	if err := SaveCloudAccount(confPath, "gcp", "gcp01", gcpFields); err != nil {
		t.Fatalf("SaveCloudAccount (gcp): %v", err)
	}

	data, _ := os.ReadFile(confPath)
	content := string(data)
	if !strings.Contains(content, "gcp=begin") || !strings.Contains(content, "[gcp01]") || !strings.Contains(content, "project_id=test-proj-123") {
		t.Fatalf("config does not contain saved GCP account: %s", content)
	}

	// 2. Update existing OCI account
	ociUpdateFields := map[string]string{
		"group":    "oci-prod",
		"user":     "ocid1.user.oc1.updated",
		"key_file": "/app/config/keys/oci_new.pem",
	}
	if err := SaveCloudAccount(confPath, "oci", "oci01", ociUpdateFields); err != nil {
		t.Fatalf("SaveCloudAccount update (oci): %v", err)
	}

	data, _ = os.ReadFile(confPath)
	content = string(data)
	if !strings.Contains(content, "user=ocid1.user.oc1.updated") || strings.Contains(content, "user=ocid1.user.oc1.test") {
		t.Fatalf("OCI account was not updated properly: %s", content)
	}

	// 3. Delete OCI account
	if err := DeleteCloudAccount(confPath, "oci", "oci01"); err != nil {
		t.Fatalf("DeleteCloudAccount: %v", err)
	}
	data, _ = os.ReadFile(confPath)
	content = string(data)
	if strings.Contains(content, "[oci01]") {
		t.Fatalf("OCI account still exists after delete: %s", content)
	}
}
