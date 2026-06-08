package container

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/docker/go-units"
	seccompprofile "github.com/r-dson/abox/config/seccomp"
	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/runtime"
)

const (
	containerHomeDir     = "/home/agent" // Matches the home directory created in Dockerfile
	containerWorkDir     = "/workspace"  // Matches WORKDIR in Dockerfile
	fileConfigVolumePath = "/vol/config" // Mount point for single-file config volumes
	readOnlyBindFlags    = "ro,z"        // Read-only bind with relabeling for SELinux
	defaultPidsLimit     = int64(512)    // Prevent fork bombs
)

// seccompPath materializes the embedded seccomp profile to a temp file.
// The Docker API requires a filesystem path, not inline JSON.
// Validates the JSON before writing to prevent corruption.
var seccompPath = sync.OnceValues(func() (string, error) {
	if !json.Valid(seccompprofile.ABoxDefault) {
		return "", fmt.Errorf("embedded seccomp profile is not valid JSON")
	}

	f, err := os.CreateTemp("", "abox-seccomp-*.json")
	if err != nil {
		return "", fmt.Errorf("creating seccomp temp file: %w", err)
	}

	path := f.Name()
	if _, err := f.Write(seccompprofile.ABoxDefault); err != nil {
		f.Close()
		return "", fmt.Errorf("writing seccomp profile: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("closing seccomp temp file: %w", err)
	}
	return path, nil
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
		Init:        true,
		PidsLimit:   defaultPidsLimit,
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

	spec.Binds = buildBinds(profile, sess, workdir, cfg)
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

func buildBinds(profile config.EditorProfile, sess *Session, workdir string, cfg *config.Config) []string {
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

	if cfg.ForwardSSHAgent {
		if sock := sshAgentSocket(); sock != "" {
			binds = append(binds, sock+":/tmp/ssh-agent.sock:ro")
		}
	}
	if cfg.ForwardGitConfig {
		if path, err := sanitizedGitConfig(); err == nil {
			binds = append(binds, path+":"+filepath.Join(containerHomeDir, ".gitconfig")+":ro")
		}
	}

	return binds
}

func sanitizedGitConfig() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".gitconfig"))
	if err != nil {
		return "", fmt.Errorf("reading host gitconfig: %w", err)
	}

	content := sanitizeGitConfig(data)
	f, err := os.CreateTemp("", "abox-gitconfig-*")
	if err != nil {
		return "", fmt.Errorf("creating sanitized gitconfig: %w", err)
	}
	path := f.Name()
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return "", fmt.Errorf("writing sanitized gitconfig: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("closing sanitized gitconfig: %w", err)
	}
	return path, nil
}

func sanitizeGitConfig(data []byte) string {
	var b strings.Builder
	scanner := bufio.NewScanner(bytes.NewReader(data))
	inUserSection := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inUserSection = trimmed == "[user]"
			if inUserSection {
				b.WriteString("[user]\n")
			}
			continue
		}
		if !inUserSection {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key != "name" && key != "email" {
			continue
		}
		b.WriteString("\t")
		b.WriteString(key)
		b.WriteString(" = ")
		b.WriteString(strings.TrimSpace(value))
		b.WriteString("\n")
	}
	return b.String()
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

func parseMemoryBytes(s string) (int64, error) {
	b, err := units.RAMInBytes(s)
	if err != nil {
		return 0, fmt.Errorf("parsing memory limit %q: %w", s, err)
	}
	return b, nil
}
