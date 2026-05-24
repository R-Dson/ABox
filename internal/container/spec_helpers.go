package container

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/go-units"
	"github.com/r-dson/abox/internal/config"
)

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

	// SSH agent socket forwarding
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if _, err := os.Stat(sock); err == nil {
			env = append(env, "SSH_AUTH_SOCK=/tmp/ssh-agent.sock")
		}
	}

	return env
}

func buildBinds(_ config.EditorProfile, sess *Session, workdir string) []string {
	home := config.HomeDir()
	var binds []string

	// Config volume
	binds = append(binds, sess.ConfigVol()+":/vol/config")
	// Cache volume
	binds = append(binds, sess.CacheVol()+":/vol/cache")
	// State volume
	binds = append(binds, sess.StateVol()+":/vol/state")
	// Share volume
	binds = append(binds, sess.ShareVol()+":/vol/share")

	// Workspace: volume (exclusions active) or direct bind
	if sess.WorkspaceVol() != "" {
		binds = append(binds, sess.WorkspaceVol()+":/workspace")
	} else {
		binds = append(binds, workdir+":/workspace")
	}

	// gitconfig — read-only
	if gc := filepath.Join(home, ".gitconfig"); fileExists(gc) {
		binds = append(binds, gc+":/home/agent/.gitconfig:ro,z")
	}

	// SSH: prefer agent socket forwarding; fall back to .ssh directory mount
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if _, err := os.Stat(sock); err == nil {
			binds = append(binds, sock+":/tmp/ssh-agent.sock:ro")
		}
	} else if sshDir := filepath.Join(home, ".ssh"); dirExists(sshDir) {
		binds = append(binds, sshDir+":/home/agent/.ssh:ro,z")
	}

	return binds
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
	bytes, err := units.RAMInBytes(s)
	if err != nil {
		return 0
	}
	return bytes
}
