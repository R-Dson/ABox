package container

import (
	"os"
	"sync"

	_ "embed"

	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/runtime"
)

//go:embed testdata/seccomp.json
var seccompProfile []byte

// seccompPath materializes the embedded seccomp profile to a temp file.
// The Docker API requires a filesystem path, not inline JSON.
var seccompPath = sync.OnceValue(func() string {
	f, err := os.CreateTemp("", "abox-seccomp-*.json")
	if err != nil {
		return ""
	}
	_, _ = f.Write(seccompProfile)
	f.Close()
	return f.Name()
})

// SeccompProfilePath returns the path to the materialized seccomp profile.
func SeccompProfilePath() string {
	return seccompPath()
}

// BuildSpec creates a ContainerSpec from profile, session, and config.
// This replaces the string-building functions in container.sh.
func BuildSpec(profile config.EditorProfile, sess *Session, workdir string, cfg *config.Config) Spec {
	spec := Spec{
		Image:       profile.ImageTag,
		Cmd:         []string{profile.CmdName},
		Env:         buildEnv(profile),
		WorkingDir:  "/workspace",
		Tty:         true,
		OpenStdin:   true,
		CapDrop:     []string{"ALL"},
		CapAdd:      []string{"CHOWN", "SETUID", "SETGID"},
		SecurityOpt: []string{"no-new-privileges", "seccomp=" + seccompPath()},
		AutoRemove:  true,
	}

	// Resource limits
	if cfg.MemoryLimit != "" {
		spec.Memory = parseMemoryBytes(cfg.MemoryLimit)
	}
	if cfg.CPULimit > 0 {
		spec.NanoCPUs = int64(cfg.CPULimit * 1e9)
	}

	// Volume mounts
	spec.Binds = buildBinds(profile, sess, workdir)

	return spec
}

// Spec is the container creation specification.
type Spec = runtime.ContainerSpec
