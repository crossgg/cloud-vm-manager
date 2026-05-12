package main

import "testing"

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
