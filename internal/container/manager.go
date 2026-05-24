package container

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/runtime"
	"golang.org/x/sync/errgroup"
)

// Manager orchestrates container sessions.
type Manager struct {
	rt runtime.ContainerRuntime
}

// NewManager creates a new container Manager.
func NewManager(rt runtime.ContainerRuntime) *Manager {
	return &Manager{rt: rt}
}

// CreateSession creates volumes, bootstraps ownership, and optionally sets up
// strict networking. Returns a session that the caller must clean up.
func (m *Manager) CreateSession(ctx context.Context, profile config.EditorProfile, cfg *config.Config) (*Session, error) {
	id := strconv.FormatInt(time.Now().UnixNano(), 10)
	vols := SessionVolumes{
		ConfigVol: "abox-config-" + id,
		CacheVol:  "abox-cache-" + id,
		StateVol:  "abox-state-" + id,
		ShareVol:  "abox-share-" + id,
	}

	labels := map[string]string{
		"app": "abox", "editor": profile.CmdName, "session": id,
	}

	// Create volumes in parallel
	g, gctx := errgroup.WithContext(ctx)
	for _, name := range vols.nonEmptyNames() {
		name := name
		g.Go(func() error {
			return m.rt.VolumeCreate(gctx, name, labels)
		})
	}
	if err := g.Wait(); err != nil {
		// Attempt cleanup of any volumes that were created
		sess := NewSession(id, m.rt, vols)
		sess.Cleanup(ctx)
		return nil, fmt.Errorf("creating volumes: %w", err)
	}

	sess := NewSession(id, m.rt, vols)

	// Create strict network if requested
	if cfg.StrictNetwork {
		netID, err := m.rt.NetworkCreate(ctx, "abox-strict-"+id, true)
		if err != nil {
			sess.Cleanup(ctx)
			return nil, fmt.Errorf("creating strict network: %w", err)
		}
		sess.volumes.NetworkID = netID
		slog.DebugContext(ctx, "created strict network", "id", netID)
	}

	return sess, nil
}

// Run creates, starts, and waits for a container to complete.
// Returns the container's exit code.
func (m *Manager) Run(ctx context.Context, spec Spec) (int, error) {
	id, err := m.rt.ContainerCreate(ctx, runtime.ContainerSpec(spec))
	if err != nil {
		return -1, fmt.Errorf("creating container: %w", err)
	}

	if err := m.rt.ContainerStart(ctx, id); err != nil {
		return -1, fmt.Errorf("starting container: %w", err)
	}

	code, err := m.rt.ContainerWait(ctx, id)
	if err != nil {
		return -1, fmt.Errorf("waiting for container: %w", err)
	}

	return int(code), nil
}
