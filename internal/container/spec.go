package container

import (
	"os"
	"sync"

	"github.com/r-dson/abox/internal/config"
	_ "embed"
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
	f.Write(seccompProfile)
	f.Close()
	return f.Name()
})

// SeccompProfilePath returns the path to the materialized seccomp profile.
func SeccompProfilePath() string {
	return seccompPath()
}

// BuildSpec creates a ContainerSpec from profile, session, and config.
// This replaces the string-building functions in container.sh.
func BuildSpec(profile config.EditorProfile, sess *Session, workdir string, cfg *config.Config) ContainerSpec {
	spec := ContainerSpec{
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

// ContainerSpec is the container creation specification.
// Re-exports runtime.ContainerSpec to avoid circular imports within the package.
type ContainerSpec = struct {
	Name        string
	Image       string
	Cmd         []string
	Env         []string
	User        string
	WorkingDir  string
	Tty         bool
	OpenStdin   bool
	Binds       []string
	CapDrop     []string
	CapAdd      []string
	SecurityOpt []string
	NetworkMode string
	AutoRemove  bool
	Memory      int64
	NanoCPUs    int64
}
