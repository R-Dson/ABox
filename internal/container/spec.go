package container

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "embed"

	"github.com/docker/go-units"
	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/runtime"
)

const (
	containerHomeDir     = "/home/agent"
	containerWorkDir     = "/workspace"
	fileConfigVolumePath = "/vol/config"
	readOnlyBindFlags    = "ro,z"
)

//go:embed testdata/seccomp.json
var seccompProfile []byte

// seccompPath materializes the embedded seccomp profile to a temp file.
// The Docker API requires a filesystem path, not inline JSON.
// Validates the JSON after writing to prevent corruption.
var seccompPath = sync.OnceValues(func() (string, error) {
	f, err := os.CreateTemp("", "abox-seccomp-*.json")
	if err != nil {
		return "", fmt.Errorf("creating seccomp temp file: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(seccompProfile); err != nil {
		return "", fmt.Errorf("writing seccomp profile: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("checking seccomp profile: %w", err)
	}
	if info.Size() == 0 {
		return "", fmt.Errorf("seccomp profile is empty")
	}
	return f.Name(), nil
})

// SeccompProfilePath returns the path to the materialized seccomp profile.
func SeccompProfilePath() (string, error) {
	return seccompPath()
}

// BuildSpec creates a ContainerSpec from profile, session, and config.
func BuildSpec(profile config.EditorProfile, sess *Session, workdir string, cfg *config.Config) (runtime.ContainerSpec, error) {
	seccompProfilePath, err := SeccompProfilePath()
	if err != nil {
		return runtime.ContainerSpec{}, err
	}

	spec := runtime.ContainerSpec{
		Image:       profile.ImageTag,
		Cmd:         buildCommand(profile),
		Env:         buildEnv(profile, cfg.ForwardSSHAgent),
		WorkingDir:  containerWorkDir,
		Tty:         true,
		OpenStdin:   true,
		CapDrop:     []string{"ALL"},
		CapAdd:      []string{"CHOWN", "SETUID", "SETGID"},
		SecurityOpt: []string{"no-new-privileges", "seccomp=" + seccompProfilePath},
		AutoRemove:  true,
	}

	if cfg.MemoryLimit != "" {
		memoryBytes, err := parseMemoryBytes(cfg.MemoryLimit)
		if err != nil {
			return runtime.ContainerSpec{}, err
		}
		spec.Memory = memoryBytes
	}
	if cfg.CPULimit > 0 {
		spec.NanoCPUs = int64(cfg.CPULimit * 1e9)
	}

	spec.Binds = buildBinds(profile, sess, workdir, cfg.ForwardSSHAgent)
	spec.NetworkMode = ResolveNetworkMode(sess, cfg)
	return spec, nil
}

func buildCommand(profile config.EditorProfile) []string {
	if !profile.ConfigIsFile {
		return []string{profile.CmdName}
	}

	configFileName := filepath.Base(profile.ConfigPath)
	containerConfigPath := profile.ConfigFullPath(containerHomeDir)
	cmd := fmt.Sprintf("mkdir -p %s && touch %s/%s && ln -sf %s/%s %s && exec %s \"$@\"",
		filepath.Dir(containerConfigPath),
		fileConfigVolumePath,
		configFileName,
		fileConfigVolumePath,
		configFileName,
		containerConfigPath,
		profile.CmdName)
	return []string{"sh", "-lc", cmd, profile.CmdName}
}

func buildEnv(profile config.EditorProfile, shouldForwardSSHAgent bool) []string {
	env := []string{
		fmt.Sprintf("HOST_UID=%d", os.Getuid()),
		fmt.Sprintf("HOST_GID=%d", os.Getgid()),
	}

	for _, key := range profile.EnvVars {
		if val, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+val)
		}
	}

	if shouldForwardSSHAgent {
		if sshSocket := sshAgentSocket(); sshSocket != "" {
			env = append(env, "SSH_AUTH_SOCK=/tmp/ssh-agent.sock")
		}
	}

	return env
}

func buildBinds(profile config.EditorProfile, sess *Session, workdir string, shouldForwardSSHAgent bool) []string {
	home := config.HomeDir()
	configMountPath := profile.ConfigFullPath(containerHomeDir)
	if profile.ConfigIsFile {
		configMountPath = fileConfigVolumePath
	}

	binds := []string{
		sess.Vol.ConfigVol + ":" + configMountPath,
		sess.Vol.CacheVol + ":" + profile.CachePath(containerHomeDir),
		sess.Vol.StateVol + ":" + profile.StatePath(containerHomeDir),
		sess.Vol.ShareVol + ":" + profile.SharePath(containerHomeDir),
	}

	if profile.LegacyPath != "" {
		binds = append(binds, sess.Vol.ConfigVol+":"+filepath.Join(containerHomeDir, profile.LegacyPath))
	}

	if sess.Vol.WorkspaceVol != "" {
		binds = append(binds, sess.Vol.WorkspaceVol+":"+containerWorkDir)
	} else {
		binds = append(binds, workdir+":"+containerWorkDir)
	}

	if gitConfig := filepath.Join(home, ".gitconfig"); fileExists(gitConfig) {
		binds = append(binds, gitConfig+":"+filepath.Join(containerHomeDir, ".gitconfig")+":"+readOnlyBindFlags)
	}

	if shouldForwardSSHAgent {
		if sock := sshAgentSocket(); sock != "" {
			binds = append(binds, sock+":/tmp/ssh-agent.sock:ro")
		}
	}

	return binds
}

func sshAgentSocket() string {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return ""
	}
	info, err := os.Stat(sock)
	if err != nil {
		return ""
	}
	if info.Mode()&os.ModeSocket == 0 {
		return ""
	}
	return sock
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func parseMemoryBytes(s string) (int64, error) {
	b, err := units.RAMInBytes(s)
	if err != nil {
		return 0, fmt.Errorf("parsing memory limit %q: %w", s, err)
	}
	return b, nil
}
