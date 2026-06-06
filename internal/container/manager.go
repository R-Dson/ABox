package container

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/r-dson/abox/internal/config"
	"github.com/r-dson/abox/internal/runtime"
	"golang.org/x/sync/errgroup"
	"golang.org/x/term"
)

// CreateSession creates volumes, bootstraps ownership, and optionally sets up
// strict networking. Returns a session that the caller must clean up.
// If hasWorkspaceVol is true, a 5th workspace volume is created for exclusion-filtered sync.
func CreateSession(ctx context.Context, rt runtime.ContainerRuntime, profile config.EditorProfile, cfg *config.Config, hasWorkspaceVol bool) (*Session, error) {
	id := strconv.FormatInt(time.Now().UnixNano(), 10)
	vols := Volumes{
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
	for _, name := range vols.NonEmptyNames() {
		name := name
		g.Go(func() error {
			return rt.VolumeCreate(gctx, name, labels)
		})
	}
	if err := g.Wait(); err != nil {
		sess := NewSession(id, rt, vols)
		sess.Cleanup(ctx)
		return nil, fmt.Errorf("creating volumes: %w", err)
	}

	sess := NewSession(id, rt, vols)

	// Bootstrap volume ownership using sync image
	if err := bootstrapOwnership(ctx, rt, sess); err != nil {
		sess.Cleanup(ctx)
		return nil, fmt.Errorf("bootstrapping ownership: %w", err)
	}

	// Create strict network if requested
	if cfg.StrictNetwork {
		netID, err := rt.NetworkCreate(ctx, "abox-strict-"+id, true)
		if err != nil {
			sess.Cleanup(ctx)
			return nil, fmt.Errorf("creating strict network: %w", err)
		}
		sess.Vol.NetworkID = netID
		slog.DebugContext(ctx, "created strict network", "id", netID)
	}

	return sess, nil
}

// bootstrapOwnership runs a short-lived container as root with the sync image
// to chown all volume mount paths to the host user's UID:GID.
func bootstrapOwnership(ctx context.Context, rt runtime.ContainerRuntime, sess *Session) error {
	uid, gid := os.Getuid(), os.Getgid()

	type volMount struct {
		name   string
		target string
	}
	mounts := []volMount{
		{sess.Vol.ConfigVol, "/vol/config"},
		{sess.Vol.CacheVol, "/vol/cache"},
		{sess.Vol.StateVol, "/vol/state"},
		{sess.Vol.ShareVol, "/vol/share"},
	}

	var chownTargets []string
	var bindMounts []string
	for _, m := range mounts {
		bindMounts = append(bindMounts, m.name+":"+m.target)
		chownTargets = append(chownTargets, m.target)
	}
	if sess.Vol.WorkspaceVol != "" {
		bindMounts = append(bindMounts, sess.Vol.WorkspaceVol+":/vol/workspace")
		chownTargets = append(chownTargets, "/vol/workspace")
	}

	chownCommand := fmt.Sprintf("chown -R %d:%d %s", uid, gid, strings.Join(chownTargets, " "))

	spec := runtime.ContainerSpec{
		Image:      runtime.SyncImage,
		Cmd:        []string{"sh", "-c", chownCommand},
		User:       "0:0",
		Binds:      bindMounts,
		AutoRemove: true,
		CapDrop:    []string{"ALL"},
		CapAdd:     []string{"CHOWN"},
	}

	return runEphemeral(ctx, rt, spec, "bootstrap")
}

// runEphemeral creates, starts, waits, and removes a container.
func runEphemeral(ctx context.Context, rt runtime.ContainerRuntime, spec runtime.ContainerSpec, purpose string) error {
	id, err := rt.ContainerCreate(ctx, spec)
	if err != nil {
		return fmt.Errorf("%s create: %w", purpose, err)
	}
	defer func() { _ = rt.ContainerRemove(context.Background(), id, true) }()

	if err := rt.ContainerStart(ctx, id); err != nil {
		return fmt.Errorf("%s start: %w", purpose, err)
	}

	code, err := rt.ContainerWait(ctx, id)
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
func Run(ctx context.Context, rt runtime.ContainerRuntime, spec runtime.ContainerSpec) (int, error) {
	id, err := rt.ContainerCreate(ctx, spec)
	if err != nil {
		return -1, fmt.Errorf("creating container: %w", err)
	}

	// Ensure cleanup on any failure path
	defer func() {
		_ = rt.ContainerRemove(context.Background(), id, true)
	}()

	attached, err := rt.ContainerAttach(ctx, id)
	if err != nil {
		return -1, fmt.Errorf("attaching to container: %w", err)
	}
	defer attached.Close()

	isInputTerminal := spec.OpenStdin && isTerminalFile(os.Stdin)
	rawState, err := prepareRawTerminal(isInputTerminal, makeStdinRaw)
	if err != nil {
		return -1, err
	}
	defer rawState.restore()

	streamDone := streamContainerIO(attached, os.Stdin, os.Stdout, os.Stderr, isInputTerminal, spec.Tty)

	if err := rt.ContainerStart(ctx, id); err != nil {
		_ = attached.Close()
		<-streamDone
		return -1, fmt.Errorf("starting container: %w", err)
	}

	if spec.Tty {
		if err := resizeContainerTTY(ctx, rt, id, stdinTerminalSize); err != nil {
			slog.DebugContext(ctx, "initial container tty resize failed", "error", err)
		}
		stopResize := watchTerminalResize(ctx, rt, id, stdinTerminalSize)
		defer stopResize()
	}
	forwardedSignals := make(chan os.Signal, 1)
	signal.Notify(forwardedSignals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	stopSignals := forwardContainerSignals(ctx, rt, id, forwardedSignals)
	defer stopForwardedSignals(func() {
		signal.Stop(forwardedSignals)
	}, stopSignals)

	code, err := rt.ContainerWait(ctx, id)
	_ = attached.Close()
	if streamErr := <-streamDone; streamErr != nil {
		slog.DebugContext(ctx, "container attach stream ended with error", "error", streamErr)
	}
	if err != nil {
		return -1, fmt.Errorf("waiting for container: %w", err)
	}

	return int(code), nil
}

type rawTerminalState struct {
	restore func()
}

func prepareRawTerminal(isTTY bool, makeRaw func() (func() error, error)) (*rawTerminalState, error) {
	if !isTTY {
		return &rawTerminalState{restore: func() {}}, nil
	}

	restore, err := makeRaw()
	if err != nil {
		return nil, fmt.Errorf("setting terminal raw mode: %w", err)
	}

	return &rawTerminalState{restore: func() {
		if err := restore(); err != nil {
			slog.Debug("restoring terminal mode failed", "error", err)
		}
	}}, nil
}

func makeStdinRaw() (func() error, error) {
	fd := int(os.Stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("make stdin raw: %w", err)
	}
	return func() error {
		return term.Restore(fd, state)
	}, nil
}

func streamContainerIO(attached io.ReadWriteCloser, stdin io.Reader, stdout, stderr io.Writer, forwardInput, tty bool) <-chan error {
	done := make(chan error, 1)
	if forwardInput && stdin != nil {
		go func() {
			if _, err := io.Copy(attached, stdin); err != nil {
				slog.Debug("container stdin stream ended with error", "error", err)
			}
		}()
	}

	go func() {
		var err error
		if tty {
			_, err = io.Copy(stdout, attached)
		} else {
			_, err = stdcopy.StdCopy(stdout, stderr, attached)
		}
		if err != nil {
			done <- fmt.Errorf("streaming container output: %w", err)
			return
		}
		done <- nil
	}()
	return done
}

func resizeContainerTTY(ctx context.Context, rt runtime.ContainerRuntime, id string, getSize func() (uint, uint, bool)) error {
	height, width, ok := getSize()
	if !ok {
		return nil
	}
	if err := rt.ContainerResize(ctx, id, height, width); err != nil {
		return fmt.Errorf("resizing container tty: %w", err)
	}
	return nil
}

func stopForwardedSignals(stopOSSignals, stopForwarder func()) {
	stopOSSignals()
	stopForwarder()
}

func forwardContainerSignals(ctx context.Context, rt runtime.ContainerRuntime, id string, signals <-chan os.Signal) func() {
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case sig := <-signals:
				name, ok := signalName(sig)
				if !ok {
					continue
				}
				if err := rt.ContainerSignal(ctx, id, name); err != nil {
					slog.DebugContext(ctx, "container signal forward failed", "signal", name, "error", err)
				}
			}
		}
	}()

	return func() {
		close(done)
	}
}

func signalName(sig os.Signal) (string, bool) {
	switch sig {
	case syscall.SIGINT:
		return "SIGINT", true
	case syscall.SIGTERM:
		return "SIGTERM", true
	case syscall.SIGHUP:
		return "SIGHUP", true
	default:
		return "", false
	}
}

func watchTerminalResize(ctx context.Context, rt runtime.ContainerRuntime, id string, getSize func() (uint, uint, bool)) func() {
	signals := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(signals, syscall.SIGWINCH)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-signals:
				if err := resizeContainerTTY(ctx, rt, id, getSize); err != nil {
					slog.DebugContext(ctx, "container tty resize failed", "error", err)
				}
			}
		}
	}()

	return func() {
		signal.Stop(signals)
		close(done)
	}
}

func stdinTerminalSize() (uint, uint, bool) {
	width, height, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return 0, 0, false
	}
	return uint(height), uint(width), true
}

func isTerminalFile(file *os.File) bool {
	return term.IsTerminal(int(file.Fd()))
}
