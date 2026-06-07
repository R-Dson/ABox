package container

import (
	"context"
	"log/slog"

	"github.com/r-dson/abox/internal/runtime"
)

// Volumes holds the volume and network names for a session.
type Volumes struct {
	ConfigVol    string
	CacheVol     string
	StateVol     string
	ShareVol     string
	WorkspaceVol string
	NetworkID    string
}

// NonEmptyNames returns all non-empty volume names.
func (v *Volumes) NonEmptyNames() []string {
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

// Session holds all ephemeral resources for a single abx run.
type Session struct {
	ID  string
	Vol Volumes
	rt  runtime.ContainerRuntime
}

// NewSession creates a session with the given ID, runtime, and volume configuration.
func NewSession(id string, rt runtime.ContainerRuntime, vols Volumes) *Session {
	return &Session{
		ID:  id,
		rt:  rt,
		Vol: vols,
	}
}

// Cleanup removes all volumes and the strict network created for this session.
// Uses the provided context (typically background) so cleanup runs even if
// the original context was cancelled.
func (s *Session) Cleanup(ctx context.Context) {
	s.CleanupExcept(ctx, nil)
}

// CleanupExcept removes session resources, preserving named volumes for recovery.
func (s *Session) CleanupExcept(ctx context.Context, preserveVolumes map[string]bool) {
	if s.Vol.NetworkID != "" {
		if err := s.rt.NetworkRemove(ctx, s.Vol.NetworkID); err != nil {
			slog.WarnContext(ctx, "cleanup: network remove failed",
				"id", s.Vol.NetworkID, "error", err)
		}
	}
	for _, name := range s.Vol.NonEmptyNames() {
		if preserveVolumes[name] {
			slog.WarnContext(ctx, "cleanup: preserving volume for recovery", "volume", name)
			continue
		}
		if err := s.rt.VolumeRemove(ctx, name, true); err != nil {
			slog.WarnContext(ctx, "cleanup: volume remove failed",
				"volume", name, "error", err)
		}
	}
}
