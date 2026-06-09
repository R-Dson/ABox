package exclusion

// HardcodedPatterns returns security-sensitive paths that are always excluded
// from workspace sync, regardless of .abxignore content.
func HardcodedPatterns() []string {
	return []string{
		"**/.ssh",
		"**/.ssh/**",
		"**/.aws",
		"**/.aws/**",
		"**/.env",
		"**/.env.*",
		"**/.kube/config",
		"**/.docker/config.json",
		"**/.config/gcloud/application_default_credentials.json",
		"**/.azure",
		"**/.azure/**",
		"**/.pypirc",
		"**/.netlify",
		"**/.netlify/**",
		"**/.gnupg",
		"**/.gnupg/**",
		"**/.netrc",
		"**/.npmrc",
		"**/.yarnrc",
		"**/.cargo/credentials",
		"**/.git/credentials",
		"**/id_rsa",
		"**/id_ed25519",
		"**/*.pem",
		"**/*.key",
		"**/*_key",
		"**/*.p12",
		"**/*.pfx",
	}
}
