package container

import (
	"context"
	"log/slog"

	"github.com/r-dson/abox/internal/runtime"
)

// SessionVolumes holds the volume and network names for a session.
type SessionVolumes struct {
	ConfigVol    string
	CacheVol     string
	StateVol     string
	ShareVol     string
	WorkspaceVol string
	NetworkID    string
}

// Session holds all ephemeral resources for a single abx run.
type Session struct {
	id      string
	rt      runtime.ContainerRuntime
	volumes SessionVolumes
}

// NewSession creates a session with the given ID, runtime, and volume configuration.
func NewSession(id string, rt runtime.ContainerRuntime, vols SessionVolumes) *Session {
	return &Session{
		id:      id,
		rt:      rt,
		volumes: vols,
	}
}

// ConfigVol returns the config volume name.
func (s *Session) ConfigVol() string { return s.volumes.ConfigVol }

// CacheVol returns the cache volume name.
func (s *Session) CacheVol() string { return s.volumes.CacheVol }

// StateVol returns the state volume name.
func (s *Session) StateVol() string { return s.volumes.StateVol }

// ShareVol returns the share volume name.
func (s *Session) ShareVol() string { return s.volumes.ShareVol }

// WorkspaceVol returns the workspace volume name (empty if exclusions not active).
func (s *Session) WorkspaceVol() string { return s.volumes.WorkspaceVol }

// NetworkID returns the strict network ID (empty if not using strict networking).
func (s *Session) NetworkID() string { return s.volumes.NetworkID }

// VolumeNames returns all non-empty volume names for this session.
func (v *SessionVolumes) nonEmptyNames() []string {
	var names []string
	for _, name := range []string{
		v.ConfigVol,
		v.CacheVol,
		v.StateVol,
		v.ShareVol,
		v.WorkspaceVol,
	} {
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// VolumeNames returns all non-empty volume names (exported for tests).
func (s *Session) VolumeNames() []string {
	return s.volumes.nonEmptyNames()
}

// ID returns the session ID.
func (s *Session) ID() string { return s.id }

// Cleanup removes all volumes and the strict network created for this session.
// Uses the provided context (typically background) so cleanup runs even if
// the original context was cancelled.
func (s *Session) Cleanup(ctx context.Context) {
	if s.volumes.NetworkID != "" {
		if err := s.rt.NetworkRemove(ctx, s.volumes.NetworkID); err != nil {
			slog.WarnContext(ctx, "cleanup: network remove failed",
				"id", s.volumes.NetworkID, "error", err)
		}
	}
	for _, name := range s.VolumeNames() {
		if err := s.rt.VolumeRemove(ctx, name, true); err != nil {
			slog.WarnContext(ctx, "cleanup: volume remove failed",
				"volume", name, "error", err)
		}
	}
}
