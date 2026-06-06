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
		"**/*.pem",
		"**/*key",
		"**/*_key",
		"**/.gnupg",
		"**/.gnupg/**",
		"**/*.p12",
		"**/*.pfx",
		"**/.netrc",
		"**/.npmrc",
	}
}
