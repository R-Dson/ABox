package seccomp

import _ "embed"

// ABoxDefault is the production seccomp profile used by ABox containers.
//
//go:embed abox-default.json
var ABoxDefault []byte
