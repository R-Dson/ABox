package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

//go:embed editors.json
var editorsJSON []byte

// EditorProfile is the single typed representation of one row in editors.json.
type EditorProfile struct {
	Version    string   `json:"version"`
	InstallCmd string   `json:"install_cmd"`
	CmdName    string   `json:"cmd_name"`
	ImageTag   string   `json:"image_tag"`
	ConfigPath string   `json:"config_path"`
	EnvVars    []string `json:"env_vars"`
	LegacyPath string   `json:"legacy_path,omitzero"`
}

// CachePath returns the derived cache directory for this editor.
func (p EditorProfile) CachePath(home string) string {
	return filepath.Join(home, ".cache", p.CmdName)
}

// StatePath returns the derived state directory for this editor.
func (p EditorProfile) StatePath(home string) string {
	return filepath.Join(home, ".local", "state", p.CmdName)
}

// SharePath returns the derived share directory for this editor.
func (p EditorProfile) SharePath(home string) string {
	return filepath.Join(home, ".local", "share", p.CmdName)
}

// ConfigFullPath returns the absolute config path for this editor.
func (p EditorProfile) ConfigFullPath(home string) string {
	return filepath.Join(home, p.ConfigPath)
}

type editorsFile struct {
	Editors map[string]EditorProfile `json:"editors"`
}

// EditorRegistry holds all editor profiles loaded from the embedded editors.json.
type EditorRegistry struct {
	profiles map[string]EditorProfile
}

// LoadEditorRegistry parses the embedded editors.json and returns a registry.
func LoadEditorRegistry() (*EditorRegistry, error) {
	var f editorsFile
	if err := json.Unmarshal(editorsJSON, &f); err != nil {
		return nil, fmt.Errorf("parsing embedded editors.json: %w", err)
	}
	return &EditorRegistry{profiles: f.Editors}, nil
}

// Get returns the EditorProfile for the named editor.
// Falls back to "opencode" for unknown names (matching Bash behavior).
func (r *EditorRegistry) Get(name string) (EditorProfile, error) {
	p, ok := r.profiles[name]
	if !ok {
		fallback, fok := r.profiles["opencode"]
		if !fok {
			return EditorProfile{}, fmt.Errorf(
				"unknown editor %q and no opencode fallback — check editors.json", name)
		}
		return fallback, nil
	}
	return p, nil
}

// Names returns all editor names sorted alphabetically.
func (r *EditorRegistry) Names() []string {
	names := make([]string, 0, len(r.profiles))
	for name := range r.profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Has returns true if the editor exists in the registry.
func (r *EditorRegistry) Has(name string) bool {
	_, ok := r.profiles[name]
	return ok
}

// HomeDir returns the user's home directory.
func HomeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	home, _ := os.UserHomeDir()
	return home
}
