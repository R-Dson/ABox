package runtime

import (
	"testing"

	dockercontainer "github.com/docker/docker/api/types/container"
	dockermount "github.com/docker/docker/api/types/mount"
)

func TestDockerMapping_ExplicitMountType(t *testing.T) {
	_, hostConfig, err := dockerCreateConfigs(ContainerSpec{
		Image: "image:tag",
		Mounts: []Mount{
			{Type: MountTypeVolume, Source: "/path-shaped-volume", Target: "/data", NoCopy: true},
			{Type: MountTypeBind, Source: "relative-bind-name", Target: "/bind", ReadOnly: true},
		},
	}, true)
	if err != nil {
		t.Fatalf("dockerCreateConfigs() error = %v", err)
	}
	if hostConfig.Mounts[0].Type != dockermount.TypeVolume {
		t.Fatalf("first mount type = %q, want volume", hostConfig.Mounts[0].Type)
	}
	if hostConfig.Mounts[0].VolumeOptions == nil || !hostConfig.Mounts[0].VolumeOptions.NoCopy {
		t.Fatalf("first mount volume options = %+v, want nocopy", hostConfig.Mounts[0].VolumeOptions)
	}
	if hostConfig.Mounts[1].Type != dockermount.TypeBind {
		t.Fatalf("second mount type = %q, want bind", hostConfig.Mounts[1].Type)
	}
	if !hostConfig.Mounts[1].ReadOnly {
		t.Fatal("second mount ReadOnly = false, want true")
	}
}

func TestDockerMapping_PropagatesNetworkEnvResourcesSecurity(t *testing.T) {
	containerConfig, hostConfig, err := dockerCreateConfigs(ContainerSpec{
		Name:        "test",
		Image:       "image:tag",
		Cmd:         []string{"echo", "ok"},
		Env:         []string{"A=B"},
		User:        "1000:1000",
		WorkingDir:  "/workspace",
		Tty:         true,
		OpenStdin:   false,
		Binds:       []string{"/host/work:/workspace:ro,z", "named-volume:/data"},
		CapDrop:     []string{"ALL"},
		CapAdd:      []string{"CHOWN"},
		SecurityOpt: []string{"no-new-privileges", `seccomp={"defaultAction":"SCMP_ACT_ALLOW"}`},
		NetworkMode: "none",
		AutoRemove:  true,
		Init:        true,
		PidsLimit:   128,
		Memory:      256,
		NanoCPUs:    500,
	}, true)
	if err != nil {
		t.Fatalf("dockerCreateConfigs() error = %v", err)
	}

	if containerConfig.Image != "image:tag" || containerConfig.Env[0] != "A=B" {
		t.Fatalf("container config = %+v", containerConfig)
	}
	if containerConfig.AttachStdin {
		t.Fatal("AttachStdin = true, want false when OpenStdin is false")
	}
	if hostConfig.NetworkMode != dockercontainer.NetworkMode("none") {
		t.Fatalf("NetworkMode = %q, want none", hostConfig.NetworkMode)
	}
	if hostConfig.Memory != 256 || hostConfig.NanoCPUs != 500 {
		t.Fatalf("resources = memory %d nanoCPUs %d", hostConfig.Memory, hostConfig.NanoCPUs)
	}
	if hostConfig.Init == nil || !*hostConfig.Init {
		t.Fatal("Init not enabled")
	}
	if hostConfig.PidsLimit == nil || *hostConfig.PidsLimit != 128 {
		t.Fatalf("PidsLimit = %v, want 128", hostConfig.PidsLimit)
	}
	if len(hostConfig.SecurityOpt) != 2 || hostConfig.SecurityOpt[0] != "no-new-privileges" {
		t.Fatalf("SecurityOpt = %v", hostConfig.SecurityOpt)
	}
	if len(hostConfig.Binds) != 2 {
		t.Fatalf("Binds = %v, want 2", hostConfig.Binds)
	}
	if hostConfig.Binds[0] != "/host/work:/workspace:ro,z" {
		t.Fatalf("first bind = %q, want /host/work:/workspace:ro,z", hostConfig.Binds[0])
	}
	if hostConfig.Binds[1] != "named-volume:/data" {
		t.Fatalf("second bind = %q, want named-volume:/data", hostConfig.Binds[1])
	}
}
