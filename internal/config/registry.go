package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"sync"

	"github.com/r-dson/abox/internal/osutil"
)

//go:embed editors.json
var editorsJSON []byte

// EditorProfile is the single typed representation of one row in editors.json.
type EditorProfile struct {
	Version      string   `json:"version"`
	InstallCmd   string   `json:"install_cmd"`
	CmdName      string   `json:"cmd_name"`
	ImageTag     string   `json:"image_tag"`
	ConfigPath   string   `json:"config_path"`
	ConfigIsFile bool     `json:"config_is_file,omitzero"`
	EnvVars      []string `json:"env_vars"`
	LegacyPath   string   `json:"legacy_path,omitzero"`
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

var loadEditorRegistry = sync.OnceValues(loadEmbeddedEditorRegistry)

// LoadEditorRegistry parses the embedded editors.json and returns a registry.
func LoadEditorRegistry() (*EditorRegistry, error) {
	return loadEditorRegistry()
}

func loadEmbeddedEditorRegistry() (*EditorRegistry, error) {
	var f editorsFile
	if err := json.Unmarshal(editorsJSON, &f); err != nil {
		return nil, fmt.Errorf("parsing embedded editors.json: %w", err)
	}
	if len(f.Editors) == 0 {
		return nil, fmt.Errorf("embedded editors.json has no editors")
	}
	profiles := make(map[string]EditorProfile, len(f.Editors))
	for name, profile := range f.Editors {
		if profile.ImageTag == "" || profile.CmdName == "" || profile.ConfigPath == "" || profile.EnvVars == nil {
			return nil, fmt.Errorf("editor %q missing required fields", name)
		}
		profiles[name] = cloneEditorProfile(profile)
	}
	return &EditorRegistry{profiles: profiles}, nil
}

// Get returns the EditorProfile for the named editor.
func (r *EditorRegistry) Get(name string) (EditorProfile, error) {
	p, ok := r.profiles[name]
	if !ok {
		return EditorProfile{}, fmt.Errorf("unknown editor %q (available: %v)", name, r.Names())
	}
	return cloneEditorProfile(p), nil
}

func cloneEditorProfile(p EditorProfile) EditorProfile {
	if p.EnvVars != nil {
		envVars := make([]string, len(p.EnvVars))
		copy(envVars, p.EnvVars)
		p.EnvVars = envVars
	}
	return p
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
	return osutil.HomeDir()
}
