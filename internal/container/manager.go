package container

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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
// If hasWorkspaceVol is true, a 5th workspace volume is created for exclusion-filtered sync.
func (m *Manager) CreateSession(ctx context.Context, profile config.EditorProfile, cfg *config.Config, hasWorkspaceVol bool) (*Session, error) {
	id := strconv.FormatInt(time.Now().UnixNano(), 10)
	vols := SessionVolumes{
		ConfigVol: "abox-config-" + id,
		CacheVol:  "abox-cache-" + id,
		StateVol:  "abox-state-" + id,
		ShareVol:  "abox-share-" + id,
	}

	if hasWorkspaceVol {
		vols.WorkspaceVol = "abox-workspace-" + id
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
		sess := NewSession(id, m.rt, vols)
		sess.Cleanup(ctx)
		return nil, fmt.Errorf("creating volumes: %w", err)
	}

	sess := NewSession(id, m.rt, vols)

	// Bootstrap volume ownership using sync image
	if err := m.bootstrapOwnership(ctx, sess); err != nil {
		sess.Cleanup(ctx)
		return nil, fmt.Errorf("bootstrapping ownership: %w", err)
	}

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

// bootstrapOwnership runs a short-lived container as root with the sync image
// to chown all volume mount paths to the host user's UID:GID.
func (m *Manager) bootstrapOwnership(ctx context.Context, sess *Session) error {
	uid, gid := os.Getuid(), os.Getgid()

	// Build bind mounts for all volumes
	type volMount struct {
		name   string
		target string
	}
	mounts := []volMount{
		{sess.ConfigVol(), "/vol/config"},
		{sess.CacheVol(), "/vol/cache"},
		{sess.StateVol(), "/vol/state"},
		{sess.ShareVol(), "/vol/share"},
	}

	var chownTargets []string
	var bindMounts []string
	for _, m := range mounts {
		bindMounts = append(bindMounts, m.name+":"+m.target)
		chownTargets = append(chownTargets, m.target)
	}
	if sess.WorkspaceVol() != "" {
		bindMounts = append(bindMounts, sess.WorkspaceVol()+":/vol/workspace")
		chownTargets = append(chownTargets, "/vol/workspace")
	}

	// Join targets for the chown command
	targetStr := ""
	for _, t := range chownTargets {
		targetStr += t + " "
	}

	spec := runtime.ContainerSpec{
		Image:      runtime.SyncImage,
		Cmd:        []string{"sh", "-c", fmt.Sprintf("chown -R %d:%d %s", uid, gid, targetStr)},
		User:       "0:0",
		Binds:      bindMounts,
		AutoRemove: true,
		CapDrop:    []string{"ALL"},
		CapAdd:     []string{"CHOWN"},
	}

	return m.runEphemeral(ctx, spec, "bootstrap")
}

// runEphemeral creates, starts, waits, and removes a container.
func (m *Manager) runEphemeral(ctx context.Context, spec runtime.ContainerSpec, purpose string) error {
	id, err := m.rt.ContainerCreate(ctx, spec)
	if err != nil {
		return fmt.Errorf("%s create: %w", purpose, err)
	}
	defer func() { _ = m.rt.ContainerRemove(context.Background(), id, true) }()

	if err := m.rt.ContainerStart(ctx, id); err != nil {
		return fmt.Errorf("%s start: %w", purpose, err)
	}

	code, err := m.rt.ContainerWait(ctx, id)
	if err != nil {
		return fmt.Errorf("%s wait: %w", purpose, err)
	}
	if code != 0 {
		return fmt.Errorf("%s exited with code %d", purpose, code)
	}
	return nil
}

// Run creates, starts, and waits for a container to complete.
// Returns the container's exit code. Cleans up the container on any error.
func (m *Manager) Run(ctx context.Context, spec Spec) (int, error) {
	id, err := m.rt.ContainerCreate(ctx, runtime.ContainerSpec(spec))
	if err != nil {
		return -1, fmt.Errorf("creating container: %w", err)
	}

	// Ensure cleanup on any failure path
	defer func() {
		_ = m.rt.ContainerRemove(context.Background(), id, true)
	}()

	if err := m.rt.ContainerStart(ctx, id); err != nil {
		return -1, fmt.Errorf("starting container: %w", err)
	}

	code, err := m.rt.ContainerWait(ctx, id)
	if err != nil {
		return -1, fmt.Errorf("waiting for container: %w", err)
	}

	return int(code), nil
}
