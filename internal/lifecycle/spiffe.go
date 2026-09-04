package lifecycle

import (
	"fmt"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/leonardoce/cnpg-i-spiffe/internal/config"
)

const (
	workloadAPIVolumeName    = "spiffe-workload-api"
	certsVolumeName          = "spiffe-certs"
	spiffeAgentContainerName = "spiffe-agent"
	postgresContainerName    = "postgres"

	// pluginVolumeName and pluginMountPath mirror the unexported constants of
	// the same name in cnpg-i-machinery's pkg/pluginhelper/object: the
	// "plugins" emptyDir volume it adds (and mounts into the postgres
	// container) whenever object.InjectPluginInitContainerSidecarSpec is
	// called, below. That call doesn't mount it into the sidecar itself, so
	// it's mounted here explicitly to let the agent serve its CNPG-i Postgres
	// service on a socket under it.
	pluginVolumeName = "plugins"
	pluginMountPath  = "/plugins"

	// defaultPostgresUID and defaultPostgresGID mirror apiv1.ClusterSpec's
	// own kubebuilder defaults for the `postgres` user/group inside the
	// image, used when the Cluster doesn't override them.
	defaultPostgresUID = 26
	defaultPostgresGID = 26

	// healthCheckPort, healthCheckLivenessPath and healthCheckReadinessPath
	// match the health check HTTP server the agent serves itself (see
	// internal/agent/health.go), and are used here to wire up the sidecar's
	// probes against it.
	healthCheckPort          = 8081
	healthCheckLivenessPath  = "/live"
	healthCheckReadinessPath = "/ready"

	// startupProbePeriodSeconds and startupProbeFailureThreshold bound how
	// long the kubelet will wait, at Pod startup, for the sidecar's startup
	// probe to succeed. Native sidecars (restartPolicy: Always init
	// containers) only block the start of the next container - here, the
	// postgres container itself - on their startup probe, not on their
	// readiness probe: with no startup probe, the kubelet would consider the
	// sidecar "started" as soon as its process is running, well before the
	// first SVID is fetched from the Workload API. Reusing the readiness
	// path here makes that same "first SVID written" condition gate
	// postgres's start too. The ten-minute budget accounts for a SPIRE
	// Server/Agent that isn't up yet when the Pod is scheduled.
	startupProbePeriodSeconds    = 2
	startupProbeFailureThreshold = 300
)

// injectWorkloadAPIVolume adds a hostPath volume exposing the SPIRE Agent's
// Workload API socket to the Pod spec, if not already present. The volume is
// not mounted into any container here: buildSpiffeAgentContainer mounts it
// into the sidecar only.
func injectWorkloadAPIVolume(spec *corev1.PodSpec, socketPath string) {
	for i := range spec.Volumes {
		if spec.Volumes[i].Name == workloadAPIVolumeName {
			return
		}
	}

	hostPathType := corev1.HostPathDirectory
	spec.Volumes = append(spec.Volumes, corev1.Volume{
		Name: workloadAPIVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: filepath.Dir(socketPath),
				Type: &hostPathType,
			},
		},
	})
}

// injectCertsVolume adds an emptyDir volume, used to store the SVID/bundle
// material fetched by the sidecar, to the Pod spec and mounts it read-only
// into the postgres container, if not already present. The sidecar's own
// (read-write) mount is set up in buildSpiffeAgentContainer.
func injectCertsVolume(spec *corev1.PodSpec, mountPath string, medium corev1.StorageMedium) {
	foundVolume := false
	for i := range spec.Volumes {
		if spec.Volumes[i].Name == certsVolumeName {
			foundVolume = true
		}
	}

	if !foundVolume {
		spec.Volumes = append(spec.Volumes, corev1.Volume{
			Name: certsVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium: medium,
				},
			},
		})
	}

	for i := range spec.Containers {
		if spec.Containers[i].Name != postgresContainerName {
			continue
		}

		spec.Containers[i].VolumeMounts = appendVolumeMountIfMissing(
			spec.Containers[i].VolumeMounts,
			corev1.VolumeMount{
				Name:      certsVolumeName,
				MountPath: mountPath,
				ReadOnly:  true,
			},
		)
	}
}

// certsVolumeMedium maps the plugin's "certsVolumeMedium" parameter value to
// the corev1.StorageMedium expected by an emptyDir volume.
func certsVolumeMedium(value string) corev1.StorageMedium {
	if value == "Memory" {
		return corev1.StorageMediumMemory
	}

	return corev1.StorageMediumDefault
}

// buildSpiffeAgentContainer builds the agent sidecar container: our own
// `cnpg-i-spiffe agent` subcommand, wired to read SVIDs from the SPIRE
// Agent's Workload API socket, write them into the shared certs volume, and
// reload PostgreSQL on every rotation.
//
// postgresUID and postgresGID must match the `postgres` user/group inside
// the postgres container's image, so that the local Unix socket connection
// the agent uses to reload PostgreSQL is accepted by peer authentication.
func buildSpiffeAgentContainer(configuration *config.Configuration, postgresUID, postgresGID int64) *corev1.Container {
	socketDir := filepath.Dir(configuration.SpireAgentSocketPath)

	return &corev1.Container{
		Name:  spiffeAgentContainerName,
		Image: configuration.SidecarImage,
		Args: []string{
			"agent",
			"--spire-agent-socket-path=" + configuration.SpireAgentSocketPath,
			"--certs-dir=" + configuration.CertsMountPath,
			"--svid-file-name=" + configuration.SVIDFileName,
			"--svid-key-file-name=" + configuration.SVIDKeyFileName,
			"--svid-bundle-file-name=" + configuration.SVIDBundleFileName,
			"--postgres-socket-dir=" + configuration.PostgresSocketDir,
			"--plugin-path=" + pluginMountPath,
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      workloadAPIVolumeName,
				MountPath: socketDir,
				ReadOnly:  true,
			},
			{
				Name:      certsVolumeName,
				MountPath: configuration.CertsMountPath,
			},
			{
				Name:      pluginVolumeName,
				MountPath: pluginMountPath,
			},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:  &postgresUID,
			RunAsGroup: &postgresGID,
		},
		StartupProbe:   startupProbe(),
		LivenessProbe:  healthCheckProbe(healthCheckLivenessPath),
		ReadinessProbe: healthCheckProbe(healthCheckReadinessPath),
	}
}

// healthCheckProbe builds a probe querying the agent's own HTTP health
// server on the given path.
func healthCheckProbe(path string) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: path,
				Port: intstr.FromInt32(healthCheckPort),
			},
		},
	}
}

// startupProbe builds the sidecar's startup probe. It polls the same
// endpoint as the readiness probe (only healthy once the first SVID has
// been written to disk), but on its own schedule: as a native sidecar, this
// container's startup probe - not its readiness probe - is what the
// kubelet waits on before starting the postgres container, so this is the
// probe that actually withholds postgres's start until the first SVID is
// available. See the startupProbePeriodSeconds/startupProbeFailureThreshold
// docs above for the timing budget.
func startupProbe() *corev1.Probe {
	probe := healthCheckProbe(healthCheckReadinessPath)
	probe.PeriodSeconds = startupProbePeriodSeconds
	probe.FailureThreshold = startupProbeFailureThreshold
	return probe
}

// scratchDataVolumeName is the name of the operator-managed volume backing
// PostgreSQL's PGDATA-adjacent scratch space, including its Unix socket
// directory (see injectPostgresSocketVolumeMount).
const scratchDataVolumeName = "scratch-data"

// injectPostgresSocketVolumeMount mounts CNPG's "scratch-data" volume into
// container, reusing whichever mount of it the postgres container is an
// ancestor of mountPath (PostgreSQL's Unix socket directory, e.g.
// "/controller/run", is typically not a mount point itself but a
// subdirectory created at runtime inside a broader mount, e.g.
// "/controller").
func injectPostgresSocketVolumeMount(spec *corev1.PodSpec, container *corev1.Container, mountPath string) error {
	for i := range spec.Containers {
		if spec.Containers[i].Name != postgresContainerName {
			continue
		}

		for _, mount := range spec.Containers[i].VolumeMounts {
			if mount.Name != scratchDataVolumeName {
				continue
			}
			if mount.MountPath != mountPath && !strings.HasPrefix(mountPath, mount.MountPath+"/") {
				continue
			}

			container.VolumeMounts = appendVolumeMountIfMissing(container.VolumeMounts, corev1.VolumeMount{
				Name:      mount.Name,
				MountPath: mount.MountPath,
				ReadOnly:  true,
			})

			return nil
		}
	}

	return fmt.Errorf("no %q volume mount above %q was found in the %q container",
		scratchDataVolumeName, mountPath, postgresContainerName)
}

// postgresUIDAndGID returns the UID/GID the postgres container actually
// runs as. It's read directly from the Pod (falling back to
// defaultPostgresUID/defaultPostgresGID when unset) rather than from the
// Cluster spec, since a webhook could have mutated the Pod's effective
// UID/GID independently of it; container-level settings on the postgres
// container itself take precedence over the Pod-level ones, mirroring
// Kubernetes' own precedence.
func postgresUIDAndGID(spec *corev1.PodSpec) (int64, int64) {
	uid, gid := int64(defaultPostgresUID), int64(defaultPostgresGID)

	if spec.SecurityContext != nil {
		if spec.SecurityContext.RunAsUser != nil {
			uid = *spec.SecurityContext.RunAsUser
		}
		if spec.SecurityContext.RunAsGroup != nil {
			gid = *spec.SecurityContext.RunAsGroup
		}
	}

	for i := range spec.Containers {
		if spec.Containers[i].Name != postgresContainerName || spec.Containers[i].SecurityContext == nil {
			continue
		}

		sc := spec.Containers[i].SecurityContext
		if sc.RunAsUser != nil {
			uid = *sc.RunAsUser
		}
		if sc.RunAsGroup != nil {
			gid = *sc.RunAsGroup
		}
	}

	return uid, gid
}

// appendVolumeMountIfMissing appends mount to mounts unless a mount with the
// same name is already present.
func appendVolumeMountIfMissing(mounts []corev1.VolumeMount, mount corev1.VolumeMount) []corev1.VolumeMount {
	for i := range mounts {
		if mounts[i].Name == mount.Name {
			return mounts
		}
	}

	return append(mounts, mount)
}
