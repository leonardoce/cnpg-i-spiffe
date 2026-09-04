/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

//nolint:testpackage // needs access to unexported helpers (injectWorkloadAPIVolume and friends)
package lifecycle

import (
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/leonardoce/cnpg-i-spiffe/internal/config"
)

func newPodSpecWithPostgres() *corev1.PodSpec {
	return &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: postgresContainerName},
		},
	}
}

func TestInjectWorkloadAPIVolume(t *testing.T) {
	t.Parallel()

	spec := newPodSpecWithPostgres()

	injectWorkloadAPIVolume(spec, "/run/spire/agent-sockets/spire-agent.sock")
	injectWorkloadAPIVolume(spec, "/run/spire/agent-sockets/spire-agent.sock")

	if len(spec.Volumes) != 1 {
		t.Fatalf("expected exactly one volume after calling twice, got %d", len(spec.Volumes))
	}

	volume := spec.Volumes[0]
	if volume.Name != workloadAPIVolumeName {
		t.Errorf("expected volume name %q, got %q", workloadAPIVolumeName, volume.Name)
	}
	if volume.HostPath == nil {
		t.Fatal("expected a hostPath volume source")
	}
	if volume.HostPath.Path != "/run/spire/agent-sockets" {
		t.Errorf("expected hostPath %q, got %q", "/run/spire/agent-sockets", volume.HostPath.Path)
	}
	if volume.HostPath.Type == nil || *volume.HostPath.Type != corev1.HostPathDirectory {
		t.Errorf("expected hostPath type %q", corev1.HostPathDirectory)
	}
}

func TestInjectCertsVolume(t *testing.T) {
	t.Parallel()

	spec := newPodSpecWithPostgres()

	injectCertsVolume(spec, "/spiffe-certs", corev1.StorageMediumMemory)
	injectCertsVolume(spec, "/spiffe-certs", corev1.StorageMediumMemory)

	if len(spec.Volumes) != 1 {
		t.Fatalf("expected exactly one volume after calling twice, got %d", len(spec.Volumes))
	}

	volume := spec.Volumes[0]
	if volume.Name != certsVolumeName {
		t.Errorf("expected volume name %q, got %q", certsVolumeName, volume.Name)
	}
	if volume.EmptyDir == nil {
		t.Fatal("expected an emptyDir volume source")
	}
	if volume.EmptyDir.Medium != corev1.StorageMediumMemory {
		t.Errorf("expected medium %q, got %q", corev1.StorageMediumMemory, volume.EmptyDir.Medium)
	}

	postgres := spec.Containers[0]
	if len(postgres.VolumeMounts) != 1 {
		t.Fatalf("expected exactly one volume mount on the postgres container after calling twice, got %d",
			len(postgres.VolumeMounts))
	}

	mount := postgres.VolumeMounts[0]
	if mount.Name != certsVolumeName {
		t.Errorf("expected mount name %q, got %q", certsVolumeName, mount.Name)
	}
	if mount.MountPath != "/spiffe-certs" {
		t.Errorf("expected mount path %q, got %q", "/spiffe-certs", mount.MountPath)
	}
	if !mount.ReadOnly {
		t.Error("expected the postgres container's certs mount to be read-only")
	}
}

func TestCertsVolumeMedium(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value    string
		expected corev1.StorageMedium
	}{
		{"Memory", corev1.StorageMediumMemory},
		{"Disk", corev1.StorageMediumDefault},
		{"", corev1.StorageMediumDefault},
	}

	for _, tt := range tests {
		if got := certsVolumeMedium(tt.value); got != tt.expected {
			t.Errorf("certsVolumeMedium(%q) = %q, expected %q", tt.value, got, tt.expected)
		}
	}
}

func testSpiffeAgentConfiguration() *config.Configuration {
	return &config.Configuration{
		SidecarImage:         "ghcr.io/leonardoce/cnpg-i-spiffe:latest",
		SpireAgentSocketPath: "/run/spire/agent-sockets/spire-agent.sock",
		CertsMountPath:       "/spiffe-certs",
		SVIDFileName:         "svid.pem",
		SVIDKeyFileName:      "svid_key.pem",
		SVIDBundleFileName:   "svid_bundle.pem",
		PostgresSocketDir:    "/controller/run",
	}
}

func assertVolumeMount(t *testing.T, mount corev1.VolumeMount, name, mountPath string, readOnly bool) {
	t.Helper()

	if mount.Name != name || mount.MountPath != mountPath || mount.ReadOnly != readOnly {
		t.Errorf("unexpected volume mount: %+v (want name=%q mountPath=%q readOnly=%v)",
			mount, name, mountPath, readOnly)
	}
}

func TestBuildSpiffeAgentContainer(t *testing.T) {
	t.Parallel()

	configuration := testSpiffeAgentConfiguration()
	container := buildSpiffeAgentContainer(configuration, 26, 26)

	if container.Name != spiffeAgentContainerName {
		t.Errorf("expected container name %q, got %q", spiffeAgentContainerName, container.Name)
	}
	if container.Image != configuration.SidecarImage {
		t.Errorf("expected image %q, got %q", configuration.SidecarImage, container.Image)
	}

	wantArgs := []string{
		"agent",
		"--spire-agent-socket-path=/run/spire/agent-sockets/spire-agent.sock",
		"--certs-dir=/spiffe-certs",
		"--svid-file-name=svid.pem",
		"--svid-key-file-name=svid_key.pem",
		"--svid-bundle-file-name=svid_bundle.pem",
		"--postgres-socket-dir=/controller/run",
		"--plugin-path=/plugins",
	}
	if !slices.Equal(container.Args, wantArgs) {
		t.Errorf("expected args %v, got %v", wantArgs, container.Args)
	}

	if len(container.VolumeMounts) != 3 {
		t.Fatalf("expected exactly three volume mounts, got %d", len(container.VolumeMounts))
	}
	assertVolumeMount(t, container.VolumeMounts[0], workloadAPIVolumeName, "/run/spire/agent-sockets", true)
	assertVolumeMount(t, container.VolumeMounts[1], certsVolumeName, configuration.CertsMountPath, false)
	assertVolumeMount(t, container.VolumeMounts[2], pluginVolumeName, pluginMountPath, false)

	if container.SecurityContext == nil ||
		container.SecurityContext.RunAsUser == nil || *container.SecurityContext.RunAsUser != 26 ||
		container.SecurityContext.RunAsGroup == nil || *container.SecurityContext.RunAsGroup != 26 {
		t.Errorf("expected RunAsUser/RunAsGroup 26/26, got %+v", container.SecurityContext)
	}

	assertHealthCheckProbe(t, container.LivenessProbe, healthCheckLivenessPath)
	assertHealthCheckProbe(t, container.ReadinessProbe, healthCheckReadinessPath)
	assertHealthCheckProbe(t, container.StartupProbe, healthCheckReadinessPath)
}

func assertHealthCheckProbe(t *testing.T, probe *corev1.Probe, path string) {
	t.Helper()

	if probe == nil || probe.HTTPGet == nil {
		t.Fatalf("expected an HTTPGet probe, got %+v", probe)
	}
	if probe.HTTPGet.Path != path {
		t.Errorf("expected probe path %q, got %q", path, probe.HTTPGet.Path)
	}
	if probe.HTTPGet.Port.IntValue() != healthCheckPort {
		t.Errorf("expected probe port %d, got %v", healthCheckPort, probe.HTTPGet.Port)
	}
}

func TestInjectPostgresSocketVolumeMount(t *testing.T) {
	t.Parallel()

	spec := newPodSpecWithPostgres()
	spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{Name: scratchDataVolumeName, MountPath: "/controller/run"},
	}

	container := &corev1.Container{}
	if err := injectPostgresSocketVolumeMount(spec, container, "/controller/run"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(container.VolumeMounts) != 1 {
		t.Fatalf("expected exactly one volume mount, got %d", len(container.VolumeMounts))
	}
	assertVolumeMount(t, container.VolumeMounts[0], scratchDataVolumeName, "/controller/run", true)
}

func TestInjectPostgresSocketVolumeMountAncestorDirectory(t *testing.T) {
	t.Parallel()

	// The socket directory is typically not a mount point itself, but a
	// subdirectory created at runtime inside a broader mount (this mirrors
	// CNPG's own "scratch-data" volume, mounted at both "/run" and
	// "/controller", with PGHOST set to "/controller/run").
	spec := newPodSpecWithPostgres()
	spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{Name: scratchDataVolumeName, MountPath: "/run"},
		{Name: scratchDataVolumeName, MountPath: "/controller"},
	}

	container := &corev1.Container{}
	if err := injectPostgresSocketVolumeMount(spec, container, "/controller/run"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(container.VolumeMounts) != 1 {
		t.Fatalf("expected exactly one volume mount, got %d", len(container.VolumeMounts))
	}
	assertVolumeMount(t, container.VolumeMounts[0], scratchDataVolumeName, "/controller", true)
}

func TestInjectPostgresSocketVolumeMountNotFound(t *testing.T) {
	t.Parallel()

	spec := newPodSpecWithPostgres()
	container := &corev1.Container{}

	if err := injectPostgresSocketVolumeMount(spec, container, "/controller/run"); err == nil {
		t.Fatal("expected an error when no matching volume mount exists")
	}
}

func TestPostgresUIDAndGID(t *testing.T) {
	t.Parallel()

	uid100, gid200 := int64(100), int64(200)
	uid300, gid400 := int64(300), int64(400)

	tests := []struct {
		name    string
		spec    *corev1.PodSpec
		wantUID int64
		wantGID int64
	}{
		{
			name:    "defaults",
			spec:    newPodSpecWithPostgres(),
			wantUID: 26,
			wantGID: 26,
		},
		{
			name: "pod-level security context",
			spec: func() *corev1.PodSpec {
				spec := newPodSpecWithPostgres()
				spec.SecurityContext = &corev1.PodSecurityContext{RunAsUser: &uid100, RunAsGroup: &gid200}

				return spec
			}(),
			wantUID: 100,
			wantGID: 200,
		},
		{
			name: "postgres container security context takes precedence",
			spec: func() *corev1.PodSpec {
				spec := newPodSpecWithPostgres()
				spec.SecurityContext = &corev1.PodSecurityContext{RunAsUser: &uid100, RunAsGroup: &gid200}
				spec.Containers[0].SecurityContext = &corev1.SecurityContext{RunAsUser: &uid300, RunAsGroup: &gid400}

				return spec
			}(),
			wantUID: 300,
			wantGID: 400,
		},
	}

	for _, tt := range tests {
		uid, gid := postgresUIDAndGID(tt.spec)
		if uid != tt.wantUID || gid != tt.wantGID {
			t.Errorf("%s: postgresUIDAndGID() = (%d, %d), expected (%d, %d)", tt.name, uid, gid, tt.wantUID, tt.wantGID)
		}
	}
}

func TestAppendVolumeMountIfMissing(t *testing.T) {
	t.Parallel()

	mounts := []corev1.VolumeMount{{Name: "existing", MountPath: "/existing"}}

	mounts = appendVolumeMountIfMissing(mounts, corev1.VolumeMount{Name: "existing", MountPath: "/other-path"})
	if len(mounts) != 1 || mounts[0].MountPath != "/existing" {
		t.Fatalf("expected the existing mount to be left untouched, got %+v", mounts)
	}

	mounts = appendVolumeMountIfMissing(mounts, corev1.VolumeMount{Name: "new", MountPath: "/new"})
	if len(mounts) != 2 || mounts[1].Name != "new" {
		t.Fatalf("expected a new mount to be appended, got %+v", mounts)
	}
}
