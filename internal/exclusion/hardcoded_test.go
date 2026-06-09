package exclusion_test

import (
	"testing"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/r-dson/abox/internal/exclusion"
)

func TestHardcodedPatterns_CommonCredentialStores(t *testing.T) {
	patterns := exclusion.HardcodedPatterns()
	tests := []struct {
		path string
		want bool
	}{
		{path: ".env.local", want: true},
		{path: "services/api/.env.production", want: true},
		{path: ".kube/config", want: true},
		{path: ".docker/config.json", want: true},
		{path: ".config/gcloud/application_default_credentials.json", want: true},
		{path: ".azure/accessTokens.json", want: true},
		{path: ".pypirc", want: true},
		{path: ".netlify/config.json", want: true},
		{path: ".npmrc", want: true},
		{path: ".yarnrc", want: true},
		{path: ".cargo/credentials", want: true},
		{path: ".git/credentials", want: true},
		{path: "id_ed25519", want: true},
		{path: "certs/prod.key", want: true},
		{path: "secrets/api_key", want: true},
		{path: "monkey", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			matched := false
			for _, p := range patterns {
				if ok, _ := doublestar.Match(p, tt.path); ok {
					matched = true
					break
				}
			}
			if matched != tt.want {
				t.Fatalf("path %q matched=%v, want %v", tt.path, matched, tt.want)
			}
		})
	}
}

func TestHardcodedPatterns(t *testing.T) {
	patterns := exclusion.HardcodedPatterns()
	if len(patterns) == 0 {
		t.Fatal("HardcodedPatterns() returned empty list")
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"ssh dir", ".ssh", true},
		{"ssh key", ".ssh/id_rsa", true},
		{"aws dir", ".aws", true},
		{"aws creds", ".aws/credentials", true},
		{"env file", ".env", true},
		{"nested env file", "services/api/.env", true},
		{"pem file", "server.pem", true},
		{"nested p12 file", "certs/client.p12", true},
		{"nested pfx file", "certs/client.pfx", true},
		{"nested key", "secrets/prod.key", true},
		{"nested netrc", "deploy/.netrc", true},
		{"nested npmrc", "frontend/.npmrc", true},
		{"gnupg dir", ".gnupg", true},
		{"normal code", "main.go", false},
		{"src dir", "src/lib/util.rs", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := false
			for _, p := range patterns {
				if ok, _ := doublestar.Match(p, tt.path); ok {
					matched = true
					break
				}
			}
			if matched != tt.want {
				t.Errorf("path %q matched=%v, want %v", tt.path, matched, tt.want)
			}
		})
	}
}
