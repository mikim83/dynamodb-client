package awsconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListProfiles(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	creds := filepath.Join(dir, "credentials")
	os.WriteFile(cfg, []byte(`
# comment
[default]
region = eu-west-1

[profile prod]
role_arn = arn:aws:iam::123456789012:role/read
source_profile = default
region = eu-west-1

[profile dev]
sso_session = corp
s3 =
  max_concurrent_requests = 10

[sso-session corp]
sso_start_url = https://example.awsapps.com/start
`), 0o600)
	os.WriteFile(creds, []byte(`
[default]
aws_access_key_id = AKIAEXAMPLE
aws_secret_access_key = secret

[legacy]
aws_access_key_id = AKIAEXAMPLE2
`), 0o600)
	t.Setenv("AWS_CONFIG_FILE", cfg)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", creds)

	got, err := ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, p := range got {
		names = append(names, p.Name)
	}
	want := []string{"default", "dev", "legacy", "prod"}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("got %v, want %v", names, want)
		}
	}
	kinds := map[string]string{"default": "access keys", "dev": "SSO", "legacy": "access keys", "prod": "assume role"}
	for _, p := range got {
		if p.Kind() != kinds[p.Name] {
			t.Errorf("%s: kind %q, want %q", p.Name, p.Kind(), kinds[p.Name])
		}
	}
}

func TestListProfilesNoFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "nope"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "nope2"))
	got, err := ListProfiles()
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}
