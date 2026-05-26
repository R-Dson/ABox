package container

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	_ "embed"

	"github.com/docker/go-units"
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

// Spec is the container creation specification.
type Spec = runtime.ContainerSpec

// BuildSpec creates a ContainerSpec from profile, session, and config.
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

	if cfg.MemoryLimit != "" {
		spec.Memory = parseMemoryBytes(cfg.MemoryLimit)
	}
	if cfg.CPULimit > 0 {
		spec.NanoCPUs = int64(cfg.CPULimit * 1e9)
	}

	spec.Binds = buildBinds(sess, workdir)
	spec.NetworkMode = ResolveNetworkMode(sess, cfg)
	return spec
}

func buildEnv(profile config.EditorProfile) []string {
	env := []string{
		fmt.Sprintf("HOST_UID=%d", os.Getuid()),
		fmt.Sprintf("HOST_GID=%d", os.Getgid()),
	}

	for _, key := range profile.EnvVars {
		if val, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+val)
		}
	}

	if sshSocket := sshAgentSocket(); sshSocket != "" {
		env = append(env, "SSH_AUTH_SOCK=/tmp/ssh-agent.sock")
	}

	return env
}

func buildBinds(sess *Session, workdir string) []string {
	home := config.HomeDir()
	var binds []string

	binds = append(binds,
		sess.ConfigVol()+":/vol/config",
		sess.CacheVol()+":/vol/cache",
		sess.StateVol()+":/vol/state",
		sess.ShareVol()+":/vol/share",
	)

	if sess.WorkspaceVol() != "" {
		binds = append(binds, sess.WorkspaceVol()+":/workspace")
	} else {
		binds = append(binds, workdir+":/workspace")
	}

	if gc := filepath.Join(home, ".gitconfig"); fileExists(gc) {
		binds = append(binds, gc+":/home/agent/.gitconfig:ro,z")
	}

	if sock := sshAgentSocket(); sock != "" {
		binds = append(binds, sock+":/tmp/ssh-agent.sock:ro")
	} else if sshDir := filepath.Join(home, ".ssh"); dirExists(sshDir) {
		binds = append(binds, sshDir+":/home/agent/.ssh:ro,z")
	}

	return binds
}

func sshAgentSocket() string {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return ""
	}
	if _, err := os.Stat(sock); err != nil {
		return ""
	}
	return sock
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func parseMemoryBytes(s string) int64 {
	b, err := units.RAMInBytes(s)
	if err != nil {
		slog.Debug("invalid memory limit, using no limit", "input", s, "error", err)
		return 0
	}
	return b
}
