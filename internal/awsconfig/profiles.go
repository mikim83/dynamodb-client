// Package awsconfig reads the profiles in the shared AWS files and builds the
// SDK configuration the same way the AWS CLI does.
package awsconfig

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Profile is a named profile from ~/.aws/config or ~/.aws/credentials.
type Profile struct {
	Name       string
	Region     string
	RoleARN    string
	SSO        bool
	StaticKeys bool
}

// Kind is a short description of how the profile gets its credentials.
func (p Profile) Kind() string {
	switch {
	case p.RoleARN != "":
		return "assume role"
	case p.SSO:
		return "SSO"
	case p.StaticKeys:
		return "access keys"
	default:
		return ""
	}
}

// ConfigFile and CredentialsFile honour AWS_CONFIG_FILE and
// AWS_SHARED_CREDENTIALS_FILE, like the CLI and the SDK.
func ConfigFile() string {
	if f := os.Getenv("AWS_CONFIG_FILE"); f != "" {
		return f
	}
	return filepath.Join(home(), ".aws", "config")
}

func CredentialsFile() string {
	if f := os.Getenv("AWS_SHARED_CREDENTIALS_FILE"); f != "" {
		return f
	}
	return filepath.Join(home(), ".aws", "credentials")
}

func home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

// ListProfiles returns every profile defined in the shared files, "default"
// first and the rest alphabetically. Missing files are not an error.
func ListProfiles() ([]Profile, error) {
	byName := map[string]*Profile{}
	get := func(name string) *Profile {
		p, ok := byName[name]
		if !ok {
			p = &Profile{Name: name}
			byName[name] = p
		}
		return p
	}

	if err := parseINI(ConfigFile(), func(section, key, value string) {
		var name string
		switch {
		case section == "default":
			name = "default"
		case strings.HasPrefix(section, "profile "):
			name = strings.TrimSpace(strings.TrimPrefix(section, "profile "))
		default:
			return // sso-session, services and other non-profile sections
		}
		p := get(name)
		switch key {
		case "":
		case "region":
			p.Region = value
		case "role_arn":
			p.RoleARN = value
		case "sso_session", "sso_start_url", "sso_account_id":
			p.SSO = true
		case "aws_access_key_id":
			p.StaticKeys = true
		}
	}); err != nil {
		return nil, err
	}

	if err := parseINI(CredentialsFile(), func(section, key, value string) {
		p := get(section)
		if key == "aws_access_key_id" {
			p.StaticKeys = true
		}
	}); err != nil {
		return nil, err
	}

	out := make([]Profile, 0, len(byName))
	for _, p := range byName {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == "default" || out[j].Name == "default" {
			return out[i].Name == "default"
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// parseINI calls fn once per section header (key "") and once per key.
func parseINI(path string, fn func(section, key, value string)) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	section := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			fn(section, "", "")
			continue
		}
		if section == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue // continuation line of a nested block (e.g. s3 =)
		}
		fn(section, strings.TrimSpace(k), strings.TrimSpace(v))
	}
	return sc.Err()
}
