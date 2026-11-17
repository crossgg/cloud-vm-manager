package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBlockConfig(t *testing.T) {
	cfg, err := parseBlockConfig(`
oci=begin
[shuai-jp]
user=ocid1.user.oc1.example
fingerprint=fp
tenancy=ocid1.tenancy.oc1.example
compartment_id=ocid1.compartment.oc1.example
region=ap-tokyo-1
key_file=/app/keys/oci.pem
oci=end

gcp=begin
[gcp01]
project_id=project-1
client_email=bot@example.iam.gserviceaccount.com
key_file=/app/keys/gcp01.pem
gcp=end

azure=begin
[az001]
subscription_id=sub-1
appId=client-1
password=secret-1
tenant=tenant-1
resource_group=rg-1
location=koreacentral
[az002]
subscription_id=sub-2
client_id=client-2
client_secret=secret-2
tenant_id=tenant-2
resource_group=rg-2
location=eastus
azure=end
`)
	if err != nil {
		t.Fatalf("parseBlockConfig returned error: %v", err)
	}

	if len(cfg.AzureAccounts) != 2 {
		t.Fatalf("expected 2 azure accounts, got %d", len(cfg.AzureAccounts))
	}
	if cfg.AzureAccounts[0].Name != "az001" || cfg.AzureAccounts[0].ClientId != "client-1" {
		t.Fatalf("unexpected first azure account: %+v", cfg.AzureAccounts[0])
	}
	if cfg.AzureAccounts[1].Name != "az002" || cfg.AzureAccounts[1].ClientSecret != "secret-2" {
		t.Fatalf("unexpected second azure account: %+v", cfg.AzureAccounts[1])
	}
	if len(cfg.GCPAccounts) != 1 || cfg.GCPAccounts[0].ProjectID != "project-1" {
		t.Fatalf("unexpected gcp accounts: %+v", cfg.GCPAccounts)
	}
	if len(cfg.OCIAccounts) != 1 || cfg.OCIAccounts[0].Region != "ap-tokyo-1" || cfg.OCIAccounts[0].CompartmentID == "" {
		t.Fatalf("unexpected oci accounts: %+v", cfg.OCIAccounts)
	}
}

func TestSaveAndLoadAzureCreditConfig(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "config.conf")
	initial := `azure=begin
[az001]
group=azure
subscription_id=sub-1
tenant_id=tenant-1
client_id=client-1
client_secret=secret-1
resource_group=rg-1
location=koreacentral
azure=end
`
	if err := os.WriteFile(confPath, []byte(initial), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	want := AzureCreditMonitorConfig{
		Enabled:              true,
		Interval:             1800,
		InitialCredit:        100,
		StartDate:            "2026-04-01",
		ExpireDate:           "2027-04-01",
		MinRemainingCredit:   10,
		DaysBeforeExpire:     7,
		MaxMonthlyCost:       15,
		TriggerAction:        "cleanup_orphans",
		ConfirmAutoDeleteAll: false,
	}
	if err := SaveAzureCreditConfig(confPath, "az001", want); err != nil {
		t.Fatalf("SaveAzureCreditConfig failed: %v", err)
	}

	raw, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	cfg, err := parseBlockConfig(string(raw))
	if err != nil {
		t.Fatalf("parseBlockConfig failed: %v", err)
	}
	if len(cfg.AzureAccounts) != 1 {
		t.Fatalf("expected 1 azure account, got %d", len(cfg.AzureAccounts))
	}
	got := cfg.AzureAccounts[0].CreditMonitor
	if !got.Enabled || got.Interval != 1800 || got.InitialCredit != 100 ||
		got.StartDate != "2026-04-01" || got.ExpireDate != "2027-04-01" ||
		got.MinRemainingCredit != 10 || got.DaysBeforeExpire != 7 ||
		got.MaxMonthlyCost != 15 || got.TriggerAction != "cleanup_orphans" {
		t.Fatalf("unexpected credit config: %+v", got)
	}
	if !strings.Contains(string(raw), "subscription_id=sub-1") {
		t.Fatalf("existing account keys were lost: %s", string(raw))
	}
}
