package exclusion

// HardcodedPatterns returns security-sensitive paths that are always excluded
// from workspace sync, regardless of .abxignore content.
func HardcodedPatterns() []string {
	return []string{
		"**/.ssh/**",
		"**/.aws/**",
		".env",
		"**/*.pem",
		"**/*key",
		"**/*_key",
		"**/.gnupg/**",
		"*.p12",
		"*.pfx",
		".netrc",
		".npmrc",
	}
}
