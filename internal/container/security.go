package container

import (
	"fmt"

	"github.com/r-dson/abox/internal/runtime"
)

const (
	helperMemoryBytes = 256 * 1024 * 1024
	helperNanoCPUs    = 500_000_000
)

// ApplyHelperSecurityDefaults applies the shared hardening profile for short-lived helper containers.
func ApplyHelperSecurityDefaults(spec *runtime.ContainerSpec) error {
	seccompPath, err := SeccompProfilePath()
	if err != nil {
		return fmt.Errorf("materializing helper seccomp profile: %w", err)
	}
	spec.NetworkMode = "none"
	spec.CapDrop = []string{"ALL"}
	spec.CapAdd = []string{"CHOWN"}
	spec.SecurityOpt = []string{"no-new-privileges", "seccomp=" + seccompPath}
	spec.Memory = helperMemoryBytes
	spec.NanoCPUs = helperNanoCPUs
	return nil
}
