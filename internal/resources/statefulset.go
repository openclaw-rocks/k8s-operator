/*
Copyright 2026 Paperclip Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resources

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// BuildStatefulSet creates a StatefulSet for the OpenClawInstance.
// If gatewayTokenSecretName is non-empty and the user hasn't already set
// OPENCLAW_GATEWAY_TOKEN in spec.env, the env var is injected via SecretKeyRef.
// externalWorkspaceFiles are the resolved contents of spec.workspace.configMapRef (may be nil).
// additionalExternalFiles maps workspace name to resolved configMapRef contents (may be nil).
func BuildStatefulSet(instance *openclawv1alpha1.OpenClawInstance, gatewayTokenSecretName string, skillPacks *ResolvedSkillPacks, externalWorkspaceFiles map[string]string, additionalExternalFiles map[string]map[string]string) *appsv1.StatefulSet {
	labels := Labels(instance)
	selectorLabels := SelectorLabels(instance)

	gwSecretName := gatewayTokenSecretName

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      StatefulSetName(instance),
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:             statefulSetReplicas(instance),
			RevisionHistoryLimit: Ptr(int32(10)),
			ServiceName:          ServiceName(instance),
			PodManagementPolicy:  appsv1.ParallelPodManagement,
			PersistentVolumeClaimRetentionPolicy: &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenDeleted: appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
				WhenScaled:  appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: buildPodAnnotations(instance, skillPacks, externalWorkspaceFiles, additionalExternalFiles),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:            ServiceAccountName(instance),
					DeprecatedServiceAccount:      ServiceAccountName(instance),
					AutomountServiceAccountToken:  Ptr(instance.Spec.SelfConfigure.Enabled || instance.Spec.Tailscale.Enabled),
					ShareProcessNamespace:         shareProcessNamespace(instance),
					SecurityContext:               buildPodSecurityContext(instance),
					InitContainers:                buildInitContainers(instance, externalWorkspaceFiles, additionalExternalFiles, skillPacks),
					Containers:                    buildContainers(instance, gwSecretName),
					Volumes:                       buildVolumes(instance, skillPacks),
					NodeSelector:                  instance.Spec.Availability.NodeSelector,
					Tolerations:                   instance.Spec.Availability.Tolerations,
					Affinity:                      instance.Spec.Availability.Affinity,
					TopologySpreadConstraints:     instance.Spec.Availability.TopologySpreadConstraints,
					RuntimeClassName:              instance.Spec.Availability.RuntimeClassName,
					RestartPolicy:                 corev1.RestartPolicyAlways,
					DNSPolicy:                     corev1.DNSClusterFirst,
					SchedulerName:                 corev1.DefaultSchedulerName,
					TerminationGracePeriodSeconds: Ptr(int64(30)),
				},
			},
		},
	}

	// Add image pull secrets
	sts.Spec.Template.Spec.ImagePullSecrets = append(
		sts.Spec.Template.Spec.ImagePullSecrets,
		instance.Spec.Image.PullSecrets...,
	)

	// When persistence is enabled with HPA (multi-replica), use VolumeClaimTemplates
	// so each replica gets its own PVC instead of sharing a single static PVC.
	if IsPersistenceEnabled(instance) && IsHPAEnabled(instance) {
		size := ParseQuantity(instance.Spec.Storage.Persistence.Size, "10Gi")
		accessModes := instance.Spec.Storage.Persistence.AccessModes
		if len(accessModes) == 0 {
			accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		}
		vct := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "data",
				Labels: labels,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: accessModes,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: size,
					},
				},
			},
		}
		if instance.Spec.Storage.Persistence.StorageClass != nil {
			vct.Spec.StorageClassName = instance.Spec.Storage.Persistence.StorageClass
		}
		sts.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{vct}
	}

	return sts
}

// buildPodAnnotations builds the pod annotations for the pod template
func buildPodAnnotations(instance *openclawv1alpha1.OpenClawInstance, skillPacks *ResolvedSkillPacks, externalWorkspaceFiles map[string]string, additionalExternalFiles map[string]map[string]string) map[string]string {
	annotations := make(map[string]string, len(instance.Spec.PodAnnotations)+1)
	for k, v := range instance.Spec.PodAnnotations {
		annotations[k] = v
	}
	annotations["openclaw.rocks/config-hash"] = calculateConfigHash(instance, skillPacks, externalWorkspaceFiles, additionalExternalFiles)
	return annotations
}

// shareProcessNamespace returns the effective ShareProcessNamespace value, defaulting
// to true. The kubebuilder default would otherwise populate this at the API server,
// but explicit handling lets existing instances stored before the field was added
// still get the zombie-reaping behavior on the next reconcile.
func shareProcessNamespace(instance *openclawv1alpha1.OpenClawInstance) *bool {
	if instance.Spec.ShareProcessNamespace != nil {
		return instance.Spec.ShareProcessNamespace
	}
	return Ptr(true)
}

// buildPodSecurityContext creates the pod-level security context
func buildPodSecurityContext(instance *openclawv1alpha1.OpenClawInstance) *corev1.PodSecurityContext {
	psc := &corev1.PodSecurityContext{
		RunAsNonRoot: Ptr(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}

	// Apply user overrides or defaults
	spec := instance.Spec.Security.PodSecurityContext
	if spec != nil {
		if spec.RunAsUser != nil {
			psc.RunAsUser = spec.RunAsUser
		} else {
			psc.RunAsUser = Ptr(int64(1000))
		}
		if spec.RunAsGroup != nil {
			psc.RunAsGroup = spec.RunAsGroup
		} else {
			psc.RunAsGroup = Ptr(int64(1000))
		}
		if spec.FSGroup != nil {
			psc.FSGroup = spec.FSGroup
		} else {
			psc.FSGroup = Ptr(int64(1000))
		}
		if spec.FSGroupChangePolicy != nil {
			psc.FSGroupChangePolicy = spec.FSGroupChangePolicy
		}
		if spec.RunAsNonRoot != nil {
			psc.RunAsNonRoot = spec.RunAsNonRoot
		}
	} else {
		psc.RunAsUser = Ptr(int64(1000))
		psc.RunAsGroup = Ptr(int64(1000))
		psc.FSGroup = Ptr(int64(1000))
	}

	return psc
}

// podRunAsNonRoot returns the effective RunAsNonRoot value from the pod security context.
// Returns true if not explicitly configured (secure default).
func podRunAsNonRoot(instance *openclawv1alpha1.OpenClawInstance) bool {
	if spec := instance.Spec.Security.PodSecurityContext; spec != nil && spec.RunAsNonRoot != nil {
		return *spec.RunAsNonRoot
	}
	return true
}

// buildContainerSecurityContext creates the container-level security context
func buildContainerSecurityContext(instance *openclawv1alpha1.OpenClawInstance) *corev1.SecurityContext {
	nonRoot := podRunAsNonRoot(instance)

	sc := &corev1.SecurityContext{
		AllowPrivilegeEscalation: Ptr(false),
		ReadOnlyRootFilesystem:   Ptr(true), // PVC subpaths at ~/.openclaw/, ~/.local/, ~/.cache/, ~/.config/ + /tmp emptyDir provide writable paths
		RunAsNonRoot:             Ptr(nonRoot),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}

	// Apply user overrides
	spec := instance.Spec.Security.ContainerSecurityContext
	if spec != nil {
		if spec.AllowPrivilegeEscalation != nil {
			sc.AllowPrivilegeEscalation = spec.AllowPrivilegeEscalation
		}
		if spec.ReadOnlyRootFilesystem != nil {
			sc.ReadOnlyRootFilesystem = spec.ReadOnlyRootFilesystem
		}
		if spec.Capabilities != nil {
			sc.Capabilities = spec.Capabilities
		}
		if spec.RunAsNonRoot != nil {
			sc.RunAsNonRoot = spec.RunAsNonRoot
		}
		if spec.RunAsUser != nil {
			sc.RunAsUser = spec.RunAsUser
		}
	}

	return sc
}

// buildContainers creates the container specs
func buildContainers(instance *openclawv1alpha1.OpenClawInstance, gatewayTokenSecretName string) []corev1.Container {
	containers := []corev1.Container{
		buildMainContainer(instance, gatewayTokenSecretName),
	}

	// Add gateway proxy sidecar if enabled (default: true)
	if IsGatewayProxyEnabled(instance) {
		containers = append(containers, buildGatewayProxyContainer(instance))
	}

	// Add Tailscale sidecar if enabled
	if instance.Spec.Tailscale.Enabled {
		containers = append(containers, buildTailscaleContainer(instance))
	}

	// Chromium is now a native sidecar (init container with restartPolicy: Always)
	// to guarantee it starts before the main container. See buildInitContainers.

	// Add Ollama sidecar if enabled
	if instance.Spec.Ollama.Enabled {
		containers = append(containers, buildOllamaContainer(instance))
	}

	// Add web terminal sidecar if enabled
	if instance.Spec.WebTerminal.Enabled {
		containers = append(containers, buildWebTerminalContainer(instance))
	}

	// Add OTel Collector sidecar when metrics are enabled.
	// The collector receives OTLP metrics from OpenClaw and exposes a
	// Prometheus scrape endpoint on the configured metrics port.
	if IsMetricsEnabled(instance) {
		containers = append(containers, buildOTelCollectorContainer(instance))
	}

	// Add custom sidecars
	containers = append(containers, instance.Spec.Sidecars...)

	return containers
}

// buildMainContainerPorts returns the container ports for the main container.
// Always includes gateway and canvas. The metrics port is on the OTel
// Collector sidecar, not the main container.
func buildMainContainerPorts(instance *openclawv1alpha1.OpenClawInstance) []corev1.ContainerPort {
	_ = instance // signature kept for consistency
	return []corev1.ContainerPort{
		{
			Name:          "gateway",
			ContainerPort: GatewayPort,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "canvas",
			ContainerPort: CanvasPort,
			Protocol:      corev1.ProtocolTCP,
		},
	}
}

// buildMainContainer creates the main OpenClaw container
func buildMainContainer(instance *openclawv1alpha1.OpenClawInstance, gatewayTokenSecretName string) corev1.Container {
	container := corev1.Container{
		Name:                     "openclaw",
		Image:                    GetImage(instance),
		ImagePullPolicy:          getPullPolicy(instance),
		SecurityContext:          buildContainerSecurityContext(instance),
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		Ports:                    buildMainContainerPorts(instance),
		Env:                      buildMainEnv(instance, gatewayTokenSecretName),
		EnvFrom:                  instance.Spec.EnvFrom,
		Resources:                buildResourceRequirements(instance),
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "data",
				MountPath: "/home/openclaw/.openclaw",
			},
			{
				Name:      "data",
				MountPath: "/home/openclaw/.local",
				SubPath:   ".local",
			},
			{
				Name:      "data",
				MountPath: "/home/openclaw/.cache",
				SubPath:   ".cache",
			},
			{
				Name:      "data",
				MountPath: "/home/openclaw/.config",
				SubPath:   ".config",
			},
			{
				Name:      "tmp",
				MountPath: "/tmp",
			},
		},
	}

	// Add CA bundle mount and env if configured
	if cab := instance.Spec.Security.CABundle; cab != nil {
		key := cab.Key
		if key == "" {
			key = DefaultCABundleKey
		}
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "ca-bundle",
			MountPath: "/etc/ssl/certs/custom-ca-bundle.crt",
			SubPath:   key,
			ReadOnly:  true,
		})
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "NODE_EXTRA_CA_CERTS",
			Value: "/etc/ssl/certs/custom-ca-bundle.crt",
		})
	}

	// Add Tailscale volume mounts (socket for tailscale whois, bin for CLI binary)
	if instance.Spec.Tailscale.Enabled {
		container.VolumeMounts = append(container.VolumeMounts,
			corev1.VolumeMount{
				Name:      "tailscale-socket",
				MountPath: TailscaleSocketDir,
				ReadOnly:  true,
			},
			corev1.VolumeMount{
				Name:      "tailscale-bin",
				MountPath: TailscaleBinPath,
				ReadOnly:  true,
			},
		)
	}

	// Add extra volume mounts from spec
	container.VolumeMounts = append(container.VolumeMounts, instance.Spec.ExtraVolumeMounts...)

	// Mount PVC-backed skills directory at /app/skills so ClawHub-installed
	// skills persist and are visible to the main container (#313).
	if hasClawHubSkills(FilterNonPackSkills(instance.Spec.Skills)) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "data",
			MountPath: "/app/skills",
			SubPath:   "skills",
		})
	}

	// Mount the config volume read-only so the postStart hook can restore
	// operator-managed config on every container start (init containers only
	// run on pod creation, not on container restarts within the same pod).
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      "config",
		MountPath: "/operator-config",
		ReadOnly:  true,
	})

	// PostStart lifecycle hook: restore the operator-managed config file on
	// every container start. This prevents crashloops when the agent modifies
	// its own config and then crashes -- without this, the broken config
	// persists because init containers don't re-run on container restarts.
	if cmd := buildConfigRestoreCommand(instance); cmd != "" {
		container.Lifecycle = &corev1.Lifecycle{
			PostStart: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"sh", "-c", cmd},
				},
			},
		}
	}

	// Add probes
	container.LivenessProbe = buildLivenessProbe(instance)
	container.ReadinessProbe = buildReadinessProbe(instance)
	container.StartupProbe = buildStartupProbe(instance)

	return container
}

// buildMainEnv creates the environment variables for the main container
func buildMainEnv(instance *openclawv1alpha1.OpenClawInstance, gatewayTokenSecretName string) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{Name: "HOME", Value: "/home/openclaw"},
		// mDNS/Bonjour pairing is unusable in Kubernetes — always disable it
		{Name: "OPENCLAW_DISABLE_BONJOUR", Value: "1"},
		// OpenClaw v2026.3.12 reduced the WebSocket handshake timeout from
		// ~10s to 3s (GHSA-jv4g-m82p-2j93), which is too short for K8s where
		// plugin loading adds container startup overhead. Inject a safe
		// default via env var (config key is not yet supported upstream).
		// See: https://github.com/openclaw/openclaw/issues/46892
		{Name: "OPENCLAW_GATEWAY_HANDSHAKE_TIMEOUT_MS", Value: fmt.Sprintf("%d", DefaultHandshakeTimeoutMs)},
		// npm: redirect global installs to the writable PVC subpath at ~/.local
		{Name: "NPM_CONFIG_PREFIX", Value: "/home/openclaw/.local"},
		// npm/npx: redirect cache to the writable PVC subpath at ~/.cache
		{Name: "NPM_CONFIG_CACHE", Value: "/home/openclaw/.cache/npm"},
		// pip: default to user-level installs so "pip install <pkg>" writes to
		// the writable ~/.local/ PVC subpath instead of read-only system site-packages
		{Name: "PIP_USER", Value: "1"},
	}

	if instance.Spec.Chromium.Enabled {
		// Use the headless CDP Service DNS name to reach the Chromium sidecar.
		// A non-loopback address triggers OpenClaw's remote/attach mode so
		// the browser control service connects to the existing sidecar
		// instead of trying to launch a local browser process.
		// Using DNS instead of pod IP avoids IPv6 URL formatting issues.
		// The headless CDP Service has publishNotReadyAddresses=true so the
		// endpoint resolves before the pod is fully Ready, avoiding a race
		// where OpenClaw checks CDP during startup before the main Service
		// has endpoints. Use localhost because the chromium sidecar shares the
		// pod network. Non-localhost addresses trigger OpenClaw's remote browser
		// pairing flow which requires device identity.
		env = append(env,
			corev1.EnvVar{
				Name:  "OPENCLAW_CHROMIUM_CDP",
				Value: fmt.Sprintf("http://127.0.0.1:%d", ChromiumPort),
			},
		)
	}

	if instance.Spec.Ollama.Enabled {
		env = append(env, corev1.EnvVar{
			Name:  "OLLAMA_HOST",
			Value: fmt.Sprintf("http://localhost:%d", OllamaPort),
		})
	}

	// Inject OPENCLAW_GATEWAY_TOKEN from Secret unless the user already set it in spec.env
	if gatewayTokenSecretName != "" && !hasUserEnv(instance, "OPENCLAW_GATEWAY_TOKEN") {
		env = append(env, corev1.EnvVar{
			Name: "OPENCLAW_GATEWAY_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: gatewayTokenSecretName},
					Key:                  GatewayTokenSecretKey,
				},
			},
		})
	}

	// Tailscale socket path - main container uses this to talk to the sidecar's
	// tailscaled (e.g. "tailscale whois" for SSO auth)
	if instance.Spec.Tailscale.Enabled {
		env = append(env, corev1.EnvVar{
			Name:  "TS_SOCKET",
			Value: TailscaleSocketPath,
		})
	}

	// Self-configure env vars - let the agent know its identity
	if instance.Spec.SelfConfigure.Enabled {
		env = append(env,
			corev1.EnvVar{Name: "OPENCLAW_INSTANCE_NAME", Value: instance.Name},
			corev1.EnvVar{Name: "OPENCLAW_NAMESPACE", Value: instance.Namespace},
		)
	}

	// Plugin discovery - set NODE_PATH so Node.js module resolution finds
	// packages installed by the init-plugins container in the PVC (#424)
	if len(instance.Spec.Plugins) > 0 {
		env = append(env, corev1.EnvVar{
			Name:  "NODE_PATH",
			Value: "/home/openclaw/.openclaw/node_modules",
		})
	}

	// Build custom PATH with optional prefixes for runtime deps, Tailscale CLI,
	// and npm-installed skill binaries (#335)
	hasRuntimeDeps := instance.Spec.RuntimeDeps.Pnpm || instance.Spec.RuntimeDeps.Python
	hasTailscale := instance.Spec.Tailscale.Enabled
	hasNpmBins := hasNpmSkills(instance.Spec.Skills) || hasWorkspaceNpmSkills(instance)
	if hasRuntimeDeps || hasTailscale || hasNpmBins {
		basePath := "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
		var prefixes []string
		if hasTailscale {
			prefixes = append(prefixes, TailscaleBinPath)
		}
		if hasRuntimeDeps || hasNpmBins {
			prefixes = append(prefixes, RuntimeDepsLocalBin)
		}
		env = append(env, corev1.EnvVar{
			Name:  "PATH",
			Value: strings.Join(append(prefixes, basePath), ":"),
		})
	}

	return append(env, instance.Spec.Env...)
}

// hasUserEnv checks whether the user has defined a specific env var in spec.env.
func hasUserEnv(instance *openclawv1alpha1.OpenClawInstance, name string) bool {
	for _, e := range instance.Spec.Env {
		if e.Name == name {
			return true
		}
	}
	return false
}

// buildInitContainers creates init containers that seed config and workspace
// files into the data volume. Config is always overwritten (operator-managed),
// while workspace files use seed-once semantics (only copied if not present).
// Skills are installed via a separate init container using the OpenClaw image.
func buildInitContainers(instance *openclawv1alpha1.OpenClawInstance, externalWorkspaceFiles map[string]string, additionalExternalFiles map[string]map[string]string, skillPacks *ResolvedSkillPacks) []corev1.Container {
	var initContainers []corev1.Container

	// Config/workspace init container (only if there's something to do)
	if script := BuildInitScript(instance, externalWorkspaceFiles, additionalExternalFiles, skillPacks); script != "" {
		mounts := []corev1.VolumeMount{
			{Name: "data", MountPath: "/data"},
		}

		// Config volume mount (only if config exists)
		if configMapKey(instance) != "" {
			mounts = append(mounts, corev1.VolumeMount{Name: "config", MountPath: "/config"})
		}

		// Tmp mount for merge mode (node writes to /tmp/merged.json) or JSON5 mode (npx writes to /tmp/converted.json)
		if instance.Spec.Config.MergeMode == ConfigMergeModeMerge || instance.Spec.Config.Format == ConfigFormatJSON5 {
			mounts = append(mounts, corev1.VolumeMount{Name: "init-tmp", MountPath: "/tmp"})
		}

		// Workspace volume mount (only if workspace files exist)
		if hasWorkspaceFiles(instance, skillPacks) {
			mounts = append(mounts, corev1.VolumeMount{Name: "workspace-init", MountPath: "/workspace-init", ReadOnly: true})
		}

		// Merge and JSON5 modes use the OpenClaw image (has Node.js + sh);
		// overwrite mode uses busybox (lightweight, only needs cp).
		// Note: ghcr.io/jqlang/jq and ghcr.io/astral-sh/uv base tags are
		// distroless (no shell), so we cannot use them with "sh -c".
		initImage := ApplyRegistryOverride("busybox:1.37", instance.Spec.Registry)
		if instance.Spec.Config.MergeMode == ConfigMergeModeMerge || instance.Spec.Config.Format == ConfigFormatJSON5 {
			initImage = GetImage(instance)
		}

		// Merge and JSON5 modes use the OpenClaw image which needs writable rootfs and HOME env
		readOnlyRoot := true
		var initEnv []corev1.EnvVar
		initPullPolicy := corev1.PullIfNotPresent
		if instance.Spec.Config.MergeMode == ConfigMergeModeMerge || instance.Spec.Config.Format == ConfigFormatJSON5 {
			readOnlyRoot = false
			initEnv = []corev1.EnvVar{
				{Name: "HOME", Value: "/tmp"},
				{Name: "NPM_CONFIG_CACHE", Value: "/tmp/.npm"},
			}
			initPullPolicy = getPullPolicy(instance)
		}

		initContainers = append(initContainers, corev1.Container{
			Name:                     "init-config",
			Image:                    initImage,
			Command:                  []string{"sh", "-c", script},
			ImagePullPolicy:          initPullPolicy,
			Env:                      initEnv,
			TerminationMessagePath:   corev1.TerminationMessagePathDefault,
			TerminationMessagePolicy: corev1.TerminationMessageReadFile,
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: Ptr(false),
				ReadOnlyRootFilesystem:   Ptr(readOnlyRoot),
				RunAsNonRoot:             Ptr(podRunAsNonRoot(instance)),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			VolumeMounts: mounts,
		})
	}

	// Tailscale binary init container (copies tailscale CLI to shared volume)
	if instance.Spec.Tailscale.Enabled {
		initContainers = append(initContainers, buildTailscaleBinInitContainer(instance))
	}

	// uv + pip init containers:
	// - init-uv: copies uv binary so agents can "uv tool install" CLI tools
	// - init-pip: bootstraps pip via ensurepip so agents can "pip install <pkg>"
	//   (PIP_USER=1 makes pip write to the writable ~/.local/ PVC subpath)
	// - init-plugin-runtime-deps: points bundled-plugin imports of "openclaw"
	//   at the current container image so version upgrades don't crash loop (#462).
	initContainers = append(
		initContainers,
		buildUvInitContainer(instance),
		buildPipInitContainer(instance),
		buildPluginRuntimeDepsInitContainer(instance),
	)

	// Runtime dependency init containers (run before skills so skills can use pnpm/python)
	if instance.Spec.RuntimeDeps.Pnpm {
		initContainers = append(initContainers, buildPnpmInitContainer(instance))
	}
	if instance.Spec.RuntimeDeps.Python {
		initContainers = append(initContainers, buildPythonInitContainer(instance))
	}

	// Skills init container (only if skills are defined)
	if skillsContainer := buildSkillsInitContainer(instance); skillsContainer != nil {
		initContainers = append(initContainers, *skillsContainer)
	}

	// Plugins init container (only if plugins are defined)
	if pluginsContainer := buildPluginsInitContainer(instance); pluginsContainer != nil {
		initContainers = append(initContainers, *pluginsContainer)
	}

	// Ollama model-pulling init container (only if enabled and models are specified)
	if instance.Spec.Ollama.Enabled && len(instance.Spec.Ollama.Models) > 0 {
		initContainers = append(initContainers, buildOllamaModelPullInitContainer(instance))
	}

	// Chromium native sidecar (K8s 1.28+): starts before main containers and
	// stays running for the pod's lifetime. This guarantees the Chromium CDP
	// endpoint is ready before OpenClaw boots and performs its one-time CDP
	// health check. Without this ordering guarantee, OpenClaw may check the
	// CDP URL before the Service has endpoints and cache "unreachable"
	// permanently (see #270).
	//
	// Chrome runs via run.sh which handles --remote-debugging-port=9222
	// internally (no browserless proxy layer). This avoids session lifecycle
	// issues where browserless kills Chrome when the WebSocket client
	// disconnects between tool calls (see #360).
	if instance.Spec.Chromium.Enabled {
		chromium := buildChromiumContainer(instance)
		chromium.RestartPolicy = Ptr(corev1.ContainerRestartPolicyAlways)
		initContainers = append(initContainers, chromium)
	}

	// Custom init containers (user-defined, run after operator-managed ones)
	initContainers = append(initContainers, instance.Spec.InitContainers...)

	return initContainers
}

// shellQuote escapes a string for safe use inside single-quoted shell arguments.
// Single quotes are escaped as '\” (end quote, escaped quote, start quote).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// forcePathsJSON returns spec.config.forcePaths as a compact JSON array
// string suitable for injection via the __forcepaths env var. Always returns
// a valid JSON array, including "[]" when unset -- the merge script's
// JSON.parse expects valid input either way.
func forcePathsJSON(instance *openclawv1alpha1.OpenClawInstance) string {
	paths := instance.Spec.Config.ForcePaths
	if len(paths) == 0 {
		return "[]"
	}
	// json.Marshal of a []string is guaranteed to produce a JSON array
	// with no characters that need shell escaping beyond what shellQuote
	// already handles.
	b, err := json.Marshal(paths)
	if err != nil {
		// Unreachable: []string is always marshalable. Fall back to an
		// empty list so the init script doesn't fail.
		return "[]"
	}
	return string(b)
}

// BuildInitScript generates the shell script for the init container.
// It handles config copy or merge, directory creation (idempotent),
// workspace file seeding (only if not present), and skill pack file mapping.
// Returns "" if there is nothing to do.
func BuildInitScript(instance *openclawv1alpha1.OpenClawInstance, externalWorkspaceFiles map[string]string, additionalExternalFiles map[string]map[string]string, skillPacks *ResolvedSkillPacks) string {
	var lines []string

	// 1. Config handling — overwrite or merge, with optional JSON5 conversion
	if key := configMapKey(instance); key != "" {
		switch {
		case instance.Spec.Config.MergeMode == ConfigMergeModeMerge:
			// Deep-merge operator config with existing PVC config via Node.js.
			// Uses the OpenClaw image (has Node.js + sh); the jq distroless image
			// cannot be used because it has no shell (#105).
			// The config path is passed via env var to avoid shell/JS quoting issues.
			//
			// When spec.config.forcePaths is non-empty, each listed dot-path is
			// deleted from the PVC config before the deep merge -- so the
			// final value at that path is whatever the CR's raw config supplies,
			// not whatever the agent persisted on disk. This is the
			// tenant-isolation lever for managed multi-tenant deployments: it
			// lets channels.* (user-owned) survive restarts while keeping
			// gateway.*, models.providers.*, etc rebuilt from the CR.
			//
			// dp(o,p) deletes a dot-path from an object. Its typeof check
			// treats arrays as objects (typeof [] === "object") and descends
			// into them; harmless here because the operator-controlled config
			// schema has no arrays at forcePath-eligible depths.
			lines = append(lines, fmt.Sprintf(
				`__cfgpath=/config/%s __forcepaths=%s node -e '`+
					`const fs=require("fs");`+
					`function dm(a,b){const r={...a};for(const k in b){r[k]=b[k]&&typeof b[k]==="object"&&!Array.isArray(b[k])&&r[k]&&typeof r[k]==="object"&&!Array.isArray(r[k])?dm(r[k],b[k]):b[k]}return r}`+
					`function dp(o,p){const k=p.split(".");let c=o;for(let i=0;i<k.length-1;i++){if(!c[k[i]]||typeof c[k[i]]!=="object")return;c=c[k[i]]}delete c[k[k.length-1]]}`+ // dp descends into arrays (typeof []==="object") -- harmless: operator schema has no arrays at forcePath depths
					`const e="/data/openclaw.json",c=process.env.__cfgpath,t="/tmp/merged.json";`+
					`const base=fs.existsSync(e)?JSON.parse(fs.readFileSync(e,"utf8")):{};`+
					`const fp=JSON.parse(process.env.__forcepaths);`+
					`for(const p of fp)dp(base,p);`+
					`const inc=JSON.parse(fs.readFileSync(c,"utf8"));`+
					`fs.writeFileSync(t,JSON.stringify(dm(base,inc),null,2));`+
					`fs.copyFileSync(t,e);`+
					`'`,
				shellQuote(key),
				shellQuote(forcePathsJSON(instance))))
		case instance.Spec.Config.Format == ConfigFormatJSON5:
			// JSON5 overwrite — convert to standard JSON via npx json5
			lines = append(lines, fmt.Sprintf(
				"npx -y json5 /config/%s > /tmp/converted.json && mv /tmp/converted.json /data/openclaw.json",
				shellQuote(key)))
		default:
			// Overwrite (default) — operator-managed config always wins
			lines = append(lines, fmt.Sprintf("cp /config/%s /data/openclaw.json", shellQuote(key)))
		}
	}

	ws := instance.Spec.Workspace

	// 2. Create workspace directories (idempotent)
	if ws != nil {
		// Sort for deterministic output
		dirs := make([]string, len(ws.InitialDirectories))
		copy(dirs, ws.InitialDirectories)
		sort.Strings(dirs)
		for _, dir := range dirs {
			lines = append(lines, fmt.Sprintf("mkdir -p /data/workspace/%s", shellQuote(dir)))
		}
	}

	// Skill pack directories
	if skillPacks != nil {
		for _, dir := range skillPacks.Directories {
			lines = append(lines, fmt.Sprintf("mkdir -p /data/workspace/%s", shellQuote(dir)))
		}
	}

	// 3. Seed workspace files (only if not present)
	// Collect all workspace file names from both user-defined and operator-injected sources
	hasFiles := hasWorkspaceFiles(instance, skillPacks)
	if hasFiles {
		// Flat (single-segment) seeds share source and destination filenames.
		// User initialFiles paths that contain '/' need encoded source keys
		// and explicit mkdir -p of the parent directory; collect them
		// separately and emit them in their own loop below.
		flatFiles := make(map[string]bool)
		var nestedUserPaths []string
		// External configMapRef files (keys are always flat)
		for name := range externalWorkspaceFiles {
			flatFiles[name] = true
		}
		if ws != nil {
			for name := range ws.InitialFiles {
				if strings.Contains(name, "/") {
					nestedUserPaths = append(nestedUserPaths, name)
				} else {
					flatFiles[name] = true
				}
			}
		}
		// Always inject operator files
		flatFiles["ENVIRONMENT.md"] = true
		// BOOTSTRAP.md injection is opt-out (#463).
		if bootstrapEnabled(instance) {
			flatFiles["BOOTSTRAP.md"] = true
		}
		if instance.Spec.SelfConfigure.Enabled {
			flatFiles["SELFCONFIG.md"] = true
			flatFiles["selfconfig.sh"] = true
		}

		// Ensure the workspace directory exists (may not on first run with emptyDir)
		lines = append(lines, "mkdir -p /data/workspace")
		// Sort keys for deterministic output
		sorted := make([]string, 0, len(flatFiles))
		for name := range flatFiles {
			sorted = append(sorted, name)
		}
		sort.Strings(sorted)
		for _, name := range sorted {
			q := shellQuote(name)
			lines = append(lines, fmt.Sprintf("[ -f /data/workspace/%s ] || cp /workspace-init/%s /data/workspace/%s", q, q, q))
		}

		// Nested user initialFiles: encoded ConfigMap key, mkdir parent, cp to original path (#482).
		sort.Strings(nestedUserPaths)
		for _, wsPath := range nestedUserPaths {
			cmKey := SkillPackCMKey(wsPath)
			lines = append(lines,
				fmt.Sprintf("mkdir -p /data/workspace/%s", shellQuote(path.Dir(wsPath))),
				fmt.Sprintf("[ -f /data/workspace/%s ] || cp /workspace-init/%s /data/workspace/%s",
					shellQuote(wsPath), shellQuote(cmKey), shellQuote(wsPath)))
		}

		// Skill pack files use mapped paths (ConfigMap key differs from workspace path).
		// Replace mode (default) converges seeded files to the declared pack
		// revision: overwrite on every start and remove files seeded by a previous
		// revision that are no longer declared, tracked via a manifest on the data
		// volume (#564). CreateOnly preserves the legacy seed-if-absent behavior.
		if instance.Spec.SkillPackUpdatePolicy == SkillPackUpdatePolicyCreateOnly {
			if HasSkillPackFiles(skillPacks) {
				for _, cmKey := range sortedPackKeys(skillPacks) {
					wsPath := skillPacks.PathMapping[cmKey]
					lines = append(lines, fmt.Sprintf("[ -f /data/workspace/%s ] || cp /workspace-init/%s /data/workspace/%s",
						shellQuote(wsPath), shellQuote(cmKey), shellQuote(wsPath)))
				}
			}
		} else {
			lines = append(lines, buildSkillPackSyncLines(skillPacks)...)
		}
	}

	// Additional workspaces - create dirs and seed files for each
	if ws != nil {
		// Sort additional workspaces for deterministic output
		addlWs := make([]openclawv1alpha1.AdditionalWorkspace, len(ws.AdditionalWorkspaces))
		copy(addlWs, ws.AdditionalWorkspaces)
		sort.Slice(addlWs, func(i, j int) bool { return addlWs[i].Name < addlWs[j].Name })

		for _, aw := range addlWs {
			wsDir := fmt.Sprintf("workspace-%s", aw.Name)

			// Create the workspace directory
			lines = append(lines, fmt.Sprintf("mkdir -p /data/%s", shellQuote(wsDir)))

			// Create initialDirectories
			dirs := make([]string, len(aw.InitialDirectories))
			copy(dirs, aw.InitialDirectories)
			sort.Strings(dirs)
			for _, dir := range dirs {
				lines = append(lines, fmt.Sprintf("mkdir -p /data/%s/%s", shellQuote(wsDir), shellQuote(dir)))
			}

			// Collect file names for this workspace. Flat names map source→dest 1:1;
			// nested user initialFiles paths need parent-dir creation and encoded
			// ConfigMap keys (#482).
			flatNames := make(map[string]bool)
			var nestedAWPaths []string

			// External configMapRef files (keys are always flat)
			if extFiles, ok := additionalExternalFiles[aw.Name]; ok {
				for name := range extFiles {
					flatNames[name] = true
				}
			}
			// Inline initialFiles
			for name := range aw.InitialFiles {
				if strings.Contains(name, "/") {
					nestedAWPaths = append(nestedAWPaths, name)
				} else {
					flatNames[name] = true
				}
			}
			// Operator-injected ENVIRONMENT.md
			flatNames["ENVIRONMENT.md"] = true

			// Seed flat files (only if not present)
			sorted := make([]string, 0, len(flatNames))
			for name := range flatNames {
				sorted = append(sorted, name)
			}
			sort.Strings(sorted)
			for _, name := range sorted {
				cmKey := AdditionalWorkspaceCMKey(aw.Name, name)
				lines = append(lines, fmt.Sprintf("[ -f /data/%s/%s ] || cp /workspace-init/%s /data/%s/%s",
					shellQuote(wsDir), shellQuote(name),
					shellQuote(cmKey),
					shellQuote(wsDir), shellQuote(name)))
			}

			// Seed nested user initialFiles (#482).
			sort.Strings(nestedAWPaths)
			for _, wsPath := range nestedAWPaths {
				cmKey := AdditionalWorkspaceCMKey(aw.Name, SkillPackCMKey(wsPath))
				lines = append(lines,
					fmt.Sprintf("mkdir -p /data/%s/%s", shellQuote(wsDir), shellQuote(path.Dir(wsPath))),
					fmt.Sprintf("[ -f /data/%s/%s ] || cp /workspace-init/%s /data/%s/%s",
						shellQuote(wsDir), shellQuote(wsPath),
						shellQuote(cmKey),
						shellQuote(wsDir), shellQuote(wsPath)))
			}

			// Workspace-scoped skill pack files (from additionalWorkspaces[].skills
			// pack: entries). Mirrors the default workspace: directories first,
			// then seed files per the instance-level skillPackUpdatePolicy (#568).
			wsPacks := skillPacks.WorkspacePacks(aw.Name)
			if wsPacks != nil {
				for _, dir := range wsPacks.Directories {
					lines = append(lines, fmt.Sprintf("mkdir -p /data/%s/%s", shellQuote(wsDir), shellQuote(dir)))
				}
			}
			if instance.Spec.SkillPackUpdatePolicy == SkillPackUpdatePolicyCreateOnly {
				if HasSkillPackFiles(wsPacks) {
					for _, cmKey := range sortedPackKeys(wsPacks) {
						wsPath := wsPacks.PathMapping[cmKey]
						lines = append(lines, fmt.Sprintf("[ -f /data/%s/%s ] || cp /workspace-init/%s /data/%s/%s",
							shellQuote(wsDir), shellQuote(wsPath),
							shellQuote(AdditionalWorkspaceCMKey(aw.Name, cmKey)),
							shellQuote(wsDir), shellQuote(wsPath)))
					}
				}
			} else {
				lines = append(lines, buildWorkspaceSkillPackSyncLines(wsPacks, aw.Name)...)
			}
		}
	}

	if len(lines) == 0 {
		return ""
	}

	return strings.Join(lines, "\n")
}

// sortedPackKeys returns the ConfigMap keys of the resolved skill pack files
// in sorted order for deterministic script output.
func sortedPackKeys(skillPacks *ResolvedSkillPacks) []string {
	keys := make([]string, 0, len(skillPacks.PathMapping))
	for cmKey := range skillPacks.PathMapping {
		keys = append(keys, cmKey)
	}
	sort.Strings(keys)
	return keys
}

// skillPackCleanupLoop renders the loop that removes previously seeded pack
// files that are not in the desired staging manifest, pruning directories that
// become empty. Entries that are absolute or contain ".." are skipped so a
// corrupted manifest can never delete anything outside the workspace root.
// Callers must guard it with a check that the manifest exists.
func skillPackCleanupLoop(wsRoot, manifest string) string {
	return `while IFS= read -r f; do
[ -n "$f" ] || continue
case "$f" in /*|*..*) continue ;; esac
if ! grep -Fxq -- "$f" ` + manifest + `.new; then
rm -f "` + wsRoot + `/$f"
d=$(dirname -- "$f")
while [ "$d" != "." ] && [ "$d" != "/" ]; do
rmdir "` + wsRoot + `/$d" 2>/dev/null || break
d=$(dirname -- "$d")
done
fi
done < ` + manifest
}

// buildSkillPackSyncLines emits the Replace-mode skill pack sync for the
// default workspace (#564). See buildSkillPackSyncLinesAt.
func buildSkillPackSyncLines(skillPacks *ResolvedSkillPacks) []string {
	return buildSkillPackSyncLinesAt(skillPacks, "/data/workspace", "/data/.skillpack-manifest",
		func(cmKey string) string { return cmKey })
}

// buildWorkspaceSkillPackSyncLines emits the Replace-mode skill pack sync for
// an additional workspace. Each workspace tracks its seeded file set in its
// own manifest so removal/update semantics are scoped per workspace (#568).
func buildWorkspaceSkillPackSyncLines(wsPacks *ResolvedSkillPacks, wsName string) []string {
	return buildSkillPackSyncLinesAt(wsPacks, "/data/workspace-"+wsName, "/data/.skillpack-manifest-ws-"+wsName,
		func(cmKey string) string { return AdditionalWorkspaceCMKey(wsName, cmKey) })
}

// buildSkillPackSyncLinesAt emits the Replace-mode skill pack sync (#564):
//  1. Write the desired set of pack-seeded workspace paths to a staging
//     manifest on the data volume (the init container rootfs is read-only and
//     /tmp is not mounted in overwrite mode, so /data is the only writable spot).
//  2. Delete files recorded in the previous manifest that are no longer desired.
//  3. Copy every pack file unconditionally so contents converge to the declared
//     pack revision.
//  4. Promote the staging manifest.
//
// srcKey maps a pack's ConfigMap-safe key to the key it is stored under in the
// workspace ConfigMap (additional workspaces namespace their keys).
//
// With no pack files, the sync only runs cleanup when a manifest from a
// previous revision exists, so instances that never used packs get a no-op.
func buildSkillPackSyncLinesAt(skillPacks *ResolvedSkillPacks, wsRoot, manifest string, srcKey func(string) string) []string {
	var lines []string

	if !HasSkillPackFiles(skillPacks) {
		// All packs were removed (or none declared). Clean up anything a
		// previous revision seeded, then drop the manifest. No-op unless a
		// manifest from a previous revision exists.
		lines = append(lines,
			fmt.Sprintf("if [ -f %s ]; then", manifest),
			fmt.Sprintf(": > %s.new", manifest),
			skillPackCleanupLoop(wsRoot, manifest),
			fmt.Sprintf("rm -f %s.new %s", manifest, manifest),
			"fi")
		return lines
	}

	mappedKeys := sortedPackKeys(skillPacks)

	// Desired manifest: one workspace-relative path per line.
	quoted := make([]string, 0, len(mappedKeys))
	for _, cmKey := range mappedKeys {
		quoted = append(quoted, shellQuote(skillPacks.PathMapping[cmKey]))
	}
	lines = append(lines,
		fmt.Sprintf("printf '%%s\\n' %s > %s.new", strings.Join(quoted, " "), manifest),
		fmt.Sprintf("if [ -f %s ]; then", manifest),
		skillPackCleanupLoop(wsRoot, manifest),
		"fi")

	// Unconditional copy so file contents follow the declared revision.
	for _, cmKey := range mappedKeys {
		wsPath := skillPacks.PathMapping[cmKey]
		if dir := path.Dir(wsPath); dir != "." && dir != "/" {
			lines = append(lines, fmt.Sprintf("mkdir -p %s/%s", wsRoot, shellQuote(dir)))
		}
		lines = append(lines, fmt.Sprintf("cp /workspace-init/%s %s/%s",
			shellQuote(srcKey(cmKey)), wsRoot, shellQuote(wsPath)))
	}

	// Promote the manifest only after the seed completed.
	lines = append(lines, fmt.Sprintf("mv %s.new %s", manifest, manifest))
	return lines
}

// clawHubSkillsSetup prepares a PVC-backed skills directory in the init
// container. It seeds built-in skills from the container image on first run
// (cp -rn = no-clobber), then redirects /app/skills to the PVC via symlink
// so clawhub install writes to persistent storage (#313).
const clawHubSkillsSetup = `mkdir -p /home/openclaw/.openclaw/skills
cp -rn /app/skills/. /home/openclaw/.openclaw/skills/ 2>/dev/null || true
rm -rf /app/skills && ln -s /home/openclaw/.openclaw/skills /app/skills`

// skillInstallWrapper is a shell function that wraps `clawhub install` to
// tolerate "Already installed" errors, making the init container idempotent
// when persistent storage is enabled (#258). An optional second argument is
// passed to clawhub as --workdir so skills can be installed into an additional
// workspace's skills/ directory instead of the global one (#568).
const skillInstallWrapper = `_install_skill() {
  local output
  if output=$(npx -y clawhub ${2:+--workdir "$2"} install "$1" 2>&1); then
    echo "$output"
  elif echo "$output" | grep -q 'Already installed'; then
    echo "Skill $1 already installed, skipping"
  else
    echo "$output" >&2
    return 1
  fi
}`

// normalizeClawHubSlug normalizes a ClawHub skill identifier into the form that
// `clawhub install` expects.
//
// Owner-qualified refs must be passed through verbatim as "@owner/slug" so that
// ClawHub can disambiguate slugs that exist under multiple owners (#558); passing
// only the bare slug makes ambiguous installs fail and leaves the pod stuck in
// Init:CrashLoopBackOff. A defensive "owner/slug" without the leading "@" is
// normalized to "@owner/slug" as well. Bare slugs (e.g. "mcp-server-fetch") are
// passed through unchanged, and a stray leading "@" on a bare slug (no owner, no
// "/") is trimmed so it stays a bare slug (#288). npm: and pack: prefixes are not
// ClawHub slugs and are returned as-is.
func normalizeClawHubSlug(entry string) string {
	// npm: and pack: prefixes are not ClawHub slugs
	if strings.HasPrefix(entry, "npm:") || strings.HasPrefix(entry, "pack:") {
		return entry
	}
	// Owner-qualified refs contain a "/". Preserve them verbatim so ClawHub can
	// disambiguate slugs shared across owners, ensuring the leading "@" is
	// present (so a defensive "owner/slug" becomes "@owner/slug").
	if strings.Contains(entry, "/") {
		return "@" + strings.TrimPrefix(entry, "@")
	}
	// Bare slug: drop a stray leading "@" since there is no owner to qualify.
	return strings.TrimPrefix(entry, "@")
}

// parseSkillEntry returns the shell command to install a single skill entry.
// Entries prefixed with "npm:" are installed globally via `npm install -g`
// so that binaries land in ~/.local/bin (via NPM_CONFIG_PREFIX) alongside
// uv, pnpm, and other tools (#335). All other entries use the _install_skill
// wrapper around `npx -y clawhub install`.
func parseSkillEntry(entry string) string {
	if pkg, ok := strings.CutPrefix(entry, "npm:"); ok {
		return fmt.Sprintf("npm install -g %s", shellQuote(pkg))
	}
	return fmt.Sprintf("_install_skill %s", shellQuote(normalizeClawHubSlug(entry)))
}

// hasClawHubSkills returns true if any entry is a ClawHub skill (not npm-prefixed).
func hasClawHubSkills(skills []string) bool {
	for _, s := range skills {
		if !strings.HasPrefix(s, "npm:") {
			return true
		}
	}
	return false
}

// hasNpmSkills returns true if any entry is an npm-prefixed skill.
func hasNpmSkills(skills []string) bool {
	for _, s := range skills {
		if strings.HasPrefix(s, "npm:") {
			return true
		}
	}
	return false
}

// hasWorkspaceNpmSkills returns true if any additional workspace declares an
// npm-prefixed skill.
func hasWorkspaceNpmSkills(instance *openclawv1alpha1.OpenClawInstance) bool {
	if instance.Spec.Workspace == nil {
		return false
	}
	for i := range instance.Spec.Workspace.AdditionalWorkspaces {
		if hasNpmSkills(instance.Spec.Workspace.AdditionalWorkspaces[i].Skills) {
			return true
		}
	}
	return false
}

// hasWorkspaceSkills returns true if any additional workspace declares skills.
func hasWorkspaceSkills(instance *openclawv1alpha1.OpenClawInstance) bool {
	if instance.Spec.Workspace == nil {
		return false
	}
	for i := range instance.Spec.Workspace.AdditionalWorkspaces {
		if len(instance.Spec.Workspace.AdditionalWorkspaces[i].Skills) > 0 {
			return true
		}
	}
	return false
}

// BuildSkillsScript generates the shell script for the skills init container.
// Each entry produces either a `clawhub install` (default) or `npm install`
// (when prefixed with "npm:") command. Entries prefixed with "pack:" are
// handled by workspace seeding and are excluded here.
//
// Additional-workspace entries (spec.workspace.additionalWorkspaces[].skills)
// are emitted after the top-level ones, grouped per workspace in name order.
// ClawHub installs for a workspace pass --workdir so the skill lands in
// workspace-<name>/skills/; npm binaries are global by nature and install the
// same way as top-level entries (#568).
//
// Entries are sorted for determinism. Returns "" if no installable skills are defined.
func BuildSkillsScript(instance *openclawv1alpha1.OpenClawInstance) string {
	// Filter out pack: entries — those are handled by workspace seeding, not npm/clawhub
	skills := FilterNonPackSkills(instance.Spec.Skills)
	sort.Strings(skills)

	type workspaceSkills struct {
		name    string
		entries []string
	}
	var wsInstalls []workspaceSkills
	needWrapper := hasClawHubSkills(skills)
	if instance.Spec.Workspace != nil {
		for i := range instance.Spec.Workspace.AdditionalWorkspaces {
			aw := &instance.Spec.Workspace.AdditionalWorkspaces[i]
			entries := FilterNonPackSkills(aw.Skills)
			if len(entries) == 0 {
				continue
			}
			sort.Strings(entries)
			wsInstalls = append(wsInstalls, workspaceSkills{name: aw.Name, entries: entries})
			needWrapper = needWrapper || hasClawHubSkills(entries)
		}
		sort.Slice(wsInstalls, func(i, j int) bool { return wsInstalls[i].name < wsInstalls[j].name })
	}

	if len(skills) == 0 && len(wsInstalls) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "set -e")
	if hasClawHubSkills(skills) {
		lines = append(lines, clawHubSkillsSetup)
	}
	if needWrapper {
		lines = append(lines, skillInstallWrapper)
	}
	for _, skill := range skills {
		lines = append(lines, parseSkillEntry(skill))
	}
	for _, wi := range wsInstalls {
		wsDir := "/home/openclaw/.openclaw/workspace-" + wi.name
		mkdirEmitted := false
		for _, skill := range wi.entries {
			if pkg, ok := strings.CutPrefix(skill, "npm:"); ok {
				lines = append(lines, fmt.Sprintf("npm install -g %s", shellQuote(pkg)))
				continue
			}
			if !mkdirEmitted {
				lines = append(lines, fmt.Sprintf("mkdir -p %s", shellQuote(wsDir)))
				mkdirEmitted = true
			}
			lines = append(lines, fmt.Sprintf("_install_skill %s %s",
				shellQuote(normalizeClawHubSlug(skill)), shellQuote(wsDir)))
		}
	}
	return strings.Join(lines, "\n")
}

// buildSkillsInitContainer creates the init container that installs skills.
// Supports both ClawHub skills (default) and npm packages (npm: prefix).
// npm lifecycle scripts are disabled globally via NPM_CONFIG_IGNORE_SCRIPTS (#91).
func buildSkillsInitContainer(instance *openclawv1alpha1.OpenClawInstance) *corev1.Container {
	script := BuildSkillsScript(instance)
	if script == "" {
		return nil
	}

	mounts := []corev1.VolumeMount{
		{Name: "data", MountPath: "/home/openclaw/.openclaw"},
		{Name: "skills-tmp", MountPath: "/tmp"},
	}

	env := []corev1.EnvVar{
		{Name: "HOME", Value: "/tmp"},
		{Name: "NPM_CONFIG_CACHE", Value: "/tmp/.npm"},
		// Redirect npm global installs to the PVC so binaries land in
		// ~/.openclaw/.local/bin (same physical dir as ~/.local/bin in the
		// main container via subpath mount). This keeps npm skill binaries
		// alongside uv, pnpm, and other tools (#335).
		{Name: "NPM_CONFIG_PREFIX", Value: "/home/openclaw/.openclaw/.local"},
		// Disable npm lifecycle scripts for all npm operations in this
		// container tree (clawhub install + npm install). This mitigates
		// supply chain attacks via malicious preinstall/postinstall scripts.
		// See #91 and the ClawHavoc incident for context.
		{Name: "NPM_CONFIG_IGNORE_SCRIPTS", Value: "true"},
	}

	// CA bundle for skills install (makes network calls)
	if cab := instance.Spec.Security.CABundle; cab != nil {
		key := cab.Key
		if key == "" {
			key = DefaultCABundleKey
		}
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "ca-bundle",
			MountPath: "/etc/ssl/certs/custom-ca-bundle.crt",
			SubPath:   key,
			ReadOnly:  true,
		})
		env = append(env, corev1.EnvVar{
			Name:  "NODE_EXTRA_CA_CERTS",
			Value: "/etc/ssl/certs/custom-ca-bundle.crt",
		})
	}

	// Append user-supplied env vars after hardcoded defaults so that
	// credentials like CLAWHUB_TOKEN are available during skill installation.
	// Hardcoded vars (HOME, NPM_CONFIG_CACHE, NPM_CONFIG_IGNORE_SCRIPTS)
	// take precedence because they appear first.
	env = append(env, instance.Spec.Env...)

	return &corev1.Container{
		Name:                     "init-skills",
		Image:                    GetImage(instance),
		Command:                  []string{"sh", "-c", script},
		ImagePullPolicy:          getPullPolicy(instance),
		Env:                      env,
		EnvFrom:                  instance.Spec.EnvFrom,
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: Ptr(false),
			ReadOnlyRootFilesystem:   Ptr(false), // npx needs to write to node_modules
			RunAsNonRoot:             Ptr(podRunAsNonRoot(instance)),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		VolumeMounts: mounts,
	}
}

// parsePluginEntry returns the shell command to install a single plugin entry
// via the OpenClaw CLI. Entries without a prefix are routed through ClawHub
// (`clawhub:<spec>`); entries prefixed with "npm:" are routed through the
// CLI's npm resolver (`npm:<spec>`), which installs the package directly
// from the npm registry.
//
// Going through the CLI (rather than raw `npm install`) is required because
// it writes plugins into ~/.openclaw/extensions/<name>/ in the layout the
// gateway's plugin discovery expects (#474). First-party ClawHub plugins
// (e.g. `@openclaw/matrix`) also use `workspace:*` dependency markers that
// raw npm rejects with EUNSUPPORTEDPROTOCOL (#505).
func parsePluginEntry(entry string) string {
	if pkg, ok := strings.CutPrefix(entry, "npm:"); ok {
		return fmt.Sprintf("openclaw plugins install --force %s", shellQuote("npm:"+pkg))
	}
	return fmt.Sprintf("openclaw plugins install --force %s", shellQuote("clawhub:"+entry))
}

// BuildPluginsScript generates the shell script for the plugins init container.
// Entries are sorted for determinism. Returns "" if no plugins are defined.
func BuildPluginsScript(instance *openclawv1alpha1.OpenClawInstance) string {
	plugins := instance.Spec.Plugins
	if len(plugins) == 0 {
		return ""
	}

	sorted := make([]string, len(plugins))
	copy(sorted, plugins)
	sort.Strings(sorted)

	lines := []string{
		"set -e",
		"mkdir -p /home/openclaw/.openclaw/extensions",
	}
	for _, plugin := range sorted {
		lines = append(lines, parsePluginEntry(plugin))
	}
	return strings.Join(lines, "\n")
}

// buildPluginsInitContainer creates the init container that installs plugins.
// npm lifecycle scripts are disabled globally via NPM_CONFIG_IGNORE_SCRIPTS.
func buildPluginsInitContainer(instance *openclawv1alpha1.OpenClawInstance) *corev1.Container {
	script := BuildPluginsScript(instance)
	if script == "" {
		return nil
	}

	// Mirror the main container's PVC subpath layout so the `openclaw plugins
	// install` CLI writes its state to the same persistent locations the main
	// container reads from at runtime (#505). The CLI uses $HOME to locate the
	// OpenClaw data directory and uses NPM_CONFIG_PREFIX / NPM_CONFIG_CACHE for
	// its own npm operations under the hood.
	mounts := []corev1.VolumeMount{
		{Name: "data", MountPath: "/home/openclaw/.openclaw"},
		{Name: "data", MountPath: "/home/openclaw/.local", SubPath: ".local"},
		{Name: "data", MountPath: "/home/openclaw/.cache", SubPath: ".cache"},
		{Name: "plugins-tmp", MountPath: "/tmp"},
	}

	env := []corev1.EnvVar{
		{Name: "HOME", Value: "/home/openclaw"},
		{Name: "NPM_CONFIG_PREFIX", Value: "/home/openclaw/.local"},
		{Name: "NPM_CONFIG_CACHE", Value: "/home/openclaw/.cache/npm"},
		// Disable npm lifecycle scripts for all npm operations in this
		// container. This mitigates supply chain attacks via malicious
		// preinstall/postinstall scripts.
		{Name: "NPM_CONFIG_IGNORE_SCRIPTS", Value: "true"},
	}

	// CA bundle for plugin install (makes network calls)
	if cab := instance.Spec.Security.CABundle; cab != nil {
		key := cab.Key
		if key == "" {
			key = DefaultCABundleKey
		}
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "ca-bundle",
			MountPath: "/etc/ssl/certs/custom-ca-bundle.crt",
			SubPath:   key,
			ReadOnly:  true,
		})
		env = append(env, corev1.EnvVar{
			Name:  "NODE_EXTRA_CA_CERTS",
			Value: "/etc/ssl/certs/custom-ca-bundle.crt",
		})
	}

	// Append user-supplied env vars after hardcoded defaults so that
	// credentials are available during plugin installation.
	// Hardcoded vars (HOME, NPM_CONFIG_PREFIX, NPM_CONFIG_CACHE,
	// NPM_CONFIG_IGNORE_SCRIPTS) take precedence because they appear first.
	env = append(env, instance.Spec.Env...)

	return &corev1.Container{
		Name:                     "init-plugins",
		Image:                    GetImage(instance),
		Command:                  []string{"sh", "-c", script},
		ImagePullPolicy:          getPullPolicy(instance),
		Env:                      env,
		EnvFrom:                  instance.Spec.EnvFrom,
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: Ptr(false),
			ReadOnlyRootFilesystem:   Ptr(false), // npm needs to write to node_modules
			RunAsNonRoot:             Ptr(podRunAsNonRoot(instance)),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
		VolumeMounts: mounts,
	}
}

// buildPnpmInitContainer creates the init container that installs pnpm via corepack.
func buildPnpmInitContainer(instance *openclawv1alpha1.OpenClawInstance) corev1.Container {
	script := `set -e
INSTALL_DIR=/home/openclaw/.openclaw/.local
mkdir -p "$INSTALL_DIR/bin"
if [ -x "$INSTALL_DIR/bin/pnpm" ]; then echo "pnpm already installed"; exit 0; fi
export COREPACK_HOME="$INSTALL_DIR/corepack"
corepack enable pnpm --install-directory "$INSTALL_DIR/bin"
pnpm --version`

	mounts := []corev1.VolumeMount{
		{Name: "data", MountPath: "/home/openclaw/.openclaw"},
		{Name: "pnpm-tmp", MountPath: "/tmp"},
	}

	env := []corev1.EnvVar{
		{Name: "HOME", Value: "/tmp"},
		{Name: "NPM_CONFIG_CACHE", Value: "/tmp/.npm"},
	}

	// CA bundle for pnpm init (may make network calls)
	if cab := instance.Spec.Security.CABundle; cab != nil {
		key := cab.Key
		if key == "" {
			key = DefaultCABundleKey
		}
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "ca-bundle",
			MountPath: "/etc/ssl/certs/custom-ca-bundle.crt",
			SubPath:   key,
			ReadOnly:  true,
		})
		env = append(env, corev1.EnvVar{
			Name:  "NODE_EXTRA_CA_CERTS",
			Value: "/etc/ssl/certs/custom-ca-bundle.crt",
		})
	}

	return corev1.Container{
		Name:                     "init-pnpm",
		Image:                    GetImage(instance),
		Command:                  []string{"sh", "-c", script},
		ImagePullPolicy:          getPullPolicy(instance),
		Env:                      env,
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: Ptr(false),
			ReadOnlyRootFilesystem:   Ptr(false), // corepack writes to node internals
			RunAsNonRoot:             Ptr(podRunAsNonRoot(instance)),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		VolumeMounts: mounts,
	}
}

// buildUvInitContainer creates an init container that copies the uv binary from
// the uv image into the PVC-backed ~/.local/bin/. This gives agents immediate
// access to "uv pip install" for Python packages and "uv tool install" for CLI
// tools without any manual bootstrapping. The check is idempotent - subsequent
// pod starts skip the copy if uv is already present.
//
// The data volume is mounted in full at /data (not via a SubPath) so this
// container also seeds the .local, .cache, .config, and skills subdirectories
// with the pod UID as owner. kubelet would otherwise create missing SubPath
// directories as root:root, which breaks on hostPath-backed PVCs where fsGroup
// ownership is not applied (e.g. Rancher local-path-provisioner on Talos).
// Every downstream container that mounts these paths via SubPath inherits the
// correct ownership from the pre-created directory. See #448.
func buildUvInitContainer(instance *openclawv1alpha1.OpenClawInstance) corev1.Container {
	script := `set -e
mkdir -p /data/.local/bin /data/.cache /data/.config /data/skills
if [ -x /data/.local/bin/uv ]; then echo "uv already installed"; exit 0; fi
cp /usr/local/bin/uv /data/.local/bin/uv
echo "uv $(/data/.local/bin/uv --version) installed"`

	return corev1.Container{
		Name:                     "init-uv",
		Image:                    ApplyRegistryOverride(UvImage, instance.Spec.Registry),
		Command:                  []string{"sh", "-c", script},
		ImagePullPolicy:          corev1.PullIfNotPresent,
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		Resources:                corev1.ResourceRequirements{},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: Ptr(false),
			ReadOnlyRootFilesystem:   Ptr(true),
			RunAsNonRoot:             Ptr(true),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "data", MountPath: "/data"},
		},
	}
}

// buildPipInitContainer creates an init container that bootstraps pip via ensurepip.
// Uses the same image as the main container so pip targets the correct Python version.
// Combined with PIP_USER=1 env var, "pip install <pkg>" writes to the writable
// ~/.local/ PVC subpath. Runs on every pod start (fast, no network needed).
//
// HOME is set to /data (the full data volume mount) so ensurepip --user
// resolves ~/.local to /data/.local. This avoids a SubPath mount that kubelet
// would otherwise create as root:root on hostPath-backed PVCs where fsGroup
// is not applied. The resulting files land in the same PVC location the main
// container reads via its SubPath .local mount. See #448.
func buildPipInitContainer(instance *openclawv1alpha1.OpenClawInstance) corev1.Container {
	script := `python3 -m ensurepip --upgrade --user 2>/dev/null || echo "ensurepip unavailable, skipping"`

	return corev1.Container{
		Name:                     "init-pip",
		Image:                    GetImage(instance),
		Command:                  []string{"sh", "-c", script},
		ImagePullPolicy:          getPullPolicy(instance),
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		Resources:                corev1.ResourceRequirements{},
		Env: []corev1.EnvVar{
			{Name: "HOME", Value: "/data"},
		},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: Ptr(false),
			ReadOnlyRootFilesystem:   Ptr(true),
			RunAsNonRoot:             Ptr(true),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "data", MountPath: "/data"},
			{Name: "tmp", MountPath: "/tmp"},
		},
	}
}

// buildPluginRuntimeDepsInitContainer creates an init container that points the
// bundled-plugin runtime-dep lookup path at the current container image.
//
// OpenClaw ships bundled plugin extensions under
// ~/.openclaw/plugin-runtime-deps/openclaw-<version>-<hash>/dist/extensions/.
// These extensions import "openclaw/..." as bare ESM specifiers. Node's
// resolver walks up the tree looking for `openclaw` in node_modules. Without
// this symlink, the resolver either finds nothing or falls back to a stale
// npm-cached package from an earlier image version on the PVC, causing
// crash loops like "Cannot find package 'openclaw'" or "Package subpath
// '...' is not defined by exports" after an image upgrade (#462).
//
// The symlink ~/.openclaw/plugin-runtime-deps/node_modules/openclaw -> /app
// is version-agnostic: Node's resolver finds it from any
// openclaw-<version>-<hash>/ subdirectory, and the target always resolves
// against /app in the main container (the current image's openclaw package).
//
// ln -sfn is idempotent, so re-running on every pod start is safe. Using
// HOME=/data is unnecessary -- this container only writes under /data.
func buildPluginRuntimeDepsInitContainer(instance *openclawv1alpha1.OpenClawInstance) corev1.Container {
	script := `set -e
mkdir -p /data/plugin-runtime-deps/node_modules
ln -sfn /app /data/plugin-runtime-deps/node_modules/openclaw`

	return corev1.Container{
		Name:                     "init-plugin-runtime-deps",
		Image:                    GetImage(instance),
		Command:                  []string{"sh", "-c", script},
		ImagePullPolicy:          getPullPolicy(instance),
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		Resources:                corev1.ResourceRequirements{},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: Ptr(false),
			ReadOnlyRootFilesystem:   Ptr(true),
			RunAsNonRoot:             Ptr(podRunAsNonRoot(instance)),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "data", MountPath: "/data"},
		},
	}
}

// buildPythonInitContainer creates the init container that installs Python 3.12 and uv.
func buildPythonInitContainer(instance *openclawv1alpha1.OpenClawInstance) corev1.Container {
	script := `set -e
INSTALL_DIR=/home/openclaw/.openclaw/.local
mkdir -p "$INSTALL_DIR/bin"
if [ -x "$INSTALL_DIR/bin/python3" ]; then echo "Python already installed"; exit 0; fi
export UV_PYTHON_INSTALL_DIR="$INSTALL_DIR/python"
uv python install 3.12
ln -sf "$INSTALL_DIR/python/"cpython-3.12*/bin/python3 "$INSTALL_DIR/bin/python3"
ln -sf "$INSTALL_DIR/python/"cpython-3.12*/bin/python3 "$INSTALL_DIR/bin/python"
cp /usr/local/bin/uv "$INSTALL_DIR/bin/uv"
python3 --version
uv --version`

	mounts := []corev1.VolumeMount{
		{Name: "data", MountPath: "/home/openclaw/.openclaw"},
		{Name: "python-tmp", MountPath: "/tmp"},
	}

	env := []corev1.EnvVar{
		{Name: "HOME", Value: "/tmp"},
		{Name: "XDG_CACHE_HOME", Value: "/tmp/.cache"},
	}

	// CA bundle for uv python install (downloads from the internet)
	if cab := instance.Spec.Security.CABundle; cab != nil {
		key := cab.Key
		if key == "" {
			key = DefaultCABundleKey
		}
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "ca-bundle",
			MountPath: "/etc/ssl/certs/custom-ca-bundle.crt",
			SubPath:   key,
			ReadOnly:  true,
		})
		env = append(env, corev1.EnvVar{
			Name:  "SSL_CERT_FILE",
			Value: "/etc/ssl/certs/custom-ca-bundle.crt",
		})
	}

	return corev1.Container{
		Name:                     "init-python",
		Image:                    ApplyRegistryOverride(UvImage, instance.Spec.Registry),
		Command:                  []string{"sh", "-c", script},
		ImagePullPolicy:          corev1.PullIfNotPresent,
		Env:                      env,
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: Ptr(false),
			ReadOnlyRootFilesystem:   Ptr(false), // uv needs writable paths
			RunAsNonRoot:             Ptr(podRunAsNonRoot(instance)),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		VolumeMounts: mounts,
	}
}

// hasWorkspaceFiles returns true if the instance has workspace files to seed.
// Always returns true because the operator always injects ENVIRONMENT.md and BOOTSTRAP.md.
func hasWorkspaceFiles(_ *openclawv1alpha1.OpenClawInstance, _ *ResolvedSkillPacks) bool {
	return true
}

// configMapKey returns the ConfigMap key for the config file.
// Always returns "openclaw.json" because the operator-managed ConfigMap always
// uses this key, regardless of whether the user provided config via raw,
// configMapRef, or none. The controller reads external CMs and writes the
// enriched result into the operator-managed CM under "openclaw.json".
func configMapKey(_ *openclawv1alpha1.OpenClawInstance) string {
	return "openclaw.json"
}

// buildTailscaleContainer creates the Tailscale sidecar that runs tailscaled.
// It handles serve/funnel declaratively via TS_SERVE_CONFIG and exposes a Unix
// socket so the main container can call "tailscale whois" for SSO auth.
func buildTailscaleContainer(instance *openclawv1alpha1.OpenClawInstance) corev1.Container {
	image := GetTailscaleImage(instance)

	hostname := instance.Spec.Tailscale.Hostname
	if hostname == "" {
		hostname = instance.Name
	}

	env := []corev1.EnvVar{
		{Name: "TS_USERSPACE", Value: "true"},
		{Name: "TS_STATE_DIR", Value: TailscaleStatePath},
		{Name: "TS_SOCKET", Value: TailscaleSocketPath},
		{Name: "TS_SERVE_CONFIG", Value: "/etc/tailscale/serve/" + TailscaleServeConfigKey},
		{Name: "TS_HOSTNAME", Value: hostname},
		// Persist Tailscale node identity and TLS certificates to a
		// Kubernetes Secret so state survives pod restarts. This prevents
		// hostname incrementing (device-1, device-2, ...) and Let's Encrypt
		// certificate re-issuance on every restart.
		{Name: "TS_KUBE_SECRET", Value: TailscaleStateSecretName(instance)},
	}

	// Inject TS_AUTHKEY from Secret
	if instance.Spec.Tailscale.AuthKeySecretRef != nil {
		secretKey := instance.Spec.Tailscale.AuthKeySecretKey
		if secretKey == "" {
			secretKey = DefaultTailscaleAuthKeySecretKey
		}
		env = append(env, corev1.EnvVar{
			Name: "TS_AUTHKEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: *instance.Spec.Tailscale.AuthKeySecretRef,
					Key:                  secretKey,
				},
			},
		})
	}

	return corev1.Container{
		Name:            "tailscale",
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Env:             env,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "tailscale-socket",
				MountPath: TailscaleSocketDir,
			},
			{
				Name:      "config",
				MountPath: "/etc/tailscale/serve/" + TailscaleServeConfigKey,
				SubPath:   TailscaleServeConfigKey,
				ReadOnly:  true,
			},
			{
				// State dir (/tmp/tailscale) is created by tailscaled under /tmp.
				Name:      "tailscale-tmp",
				MountPath: "/tmp",
			},
		},
		Resources: buildTailscaleResourceRequirements(instance),
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: Ptr(false),
			ReadOnlyRootFilesystem:   Ptr(true),
			RunAsNonRoot:             Ptr(podRunAsNonRoot(instance)),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
	}
}

// buildTailscaleResourceRequirements creates resource requirements for the Tailscale sidecar
func buildTailscaleResourceRequirements(instance *openclawv1alpha1.OpenClawInstance) corev1.ResourceRequirements {
	req := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}

	req.Requests[corev1.ResourceCPU] = ParseQuantity(instance.Spec.Tailscale.Resources.Requests.CPU, "50m")
	req.Requests[corev1.ResourceMemory] = ParseQuantity(instance.Spec.Tailscale.Resources.Requests.Memory, "64Mi")
	req.Limits[corev1.ResourceCPU] = ParseQuantity(instance.Spec.Tailscale.Resources.Limits.CPU, "200m")
	req.Limits[corev1.ResourceMemory] = ParseQuantity(instance.Spec.Tailscale.Resources.Limits.Memory, "256Mi")

	return req
}

// buildTailscaleBinInitContainer creates the init container that copies the
// tailscale CLI binary from the Tailscale image to a shared emptyDir volume.
// The main container mounts this volume at TailscaleBinPath so OpenClaw can
// find the "tailscale" binary via PATH (e.g. for "tailscale whois").
func buildTailscaleBinInitContainer(instance *openclawv1alpha1.OpenClawInstance) corev1.Container {
	image := GetTailscaleImage(instance)

	return corev1.Container{
		Name:            "init-tailscale-bin",
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"sh", "-c", "cp /usr/local/bin/tailscale " + TailscaleBinPath + "/tailscale"},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "tailscale-bin",
				MountPath: TailscaleBinPath,
			},
		},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: Ptr(false),
			ReadOnlyRootFilesystem:   Ptr(true),
			RunAsNonRoot:             Ptr(podRunAsNonRoot(instance)),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
	}
}

// buildGatewayProxyContainer creates the nginx reverse proxy sidecar that
// exposes the loopback-bound gateway and canvas ports for external access.
func buildGatewayProxyContainer(instance *openclawv1alpha1.OpenClawInstance) corev1.Container {
	return corev1.Container{
		Name:            "gateway-proxy",
		Image:           ApplyRegistryOverride(DefaultGatewayProxyImage, instance.Spec.Registry),
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports: []corev1.ContainerPort{
			{
				Name:          "gw-proxy",
				ContainerPort: GatewayProxyPort,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "canvas-proxy",
				ContainerPort: CanvasProxyPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "config",
				MountPath: "/etc/nginx/nginx.conf",
				SubPath:   NginxConfigKey,
				ReadOnly:  true,
			},
			{
				Name:      "gateway-proxy-tmp",
				MountPath: "/tmp",
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("16Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: Ptr(false),
			ReadOnlyRootFilesystem:   Ptr(true),
			RunAsNonRoot:             Ptr(true),
			RunAsUser:                Ptr(int64(101)), // nginx user in alpine
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
	}
}

// buildChromiumContainer creates the Chromium sidecar container.
// Chrome runs via run.sh which handles --remote-debugging-port=9222
// internally (no browserless proxy layer). This avoids session lifecycle
// issues where browserless kills Chrome when the WebSocket client
// disconnects between tool calls (see #360). Additional launch args
// (anti-bot flags + user ExtraArgs) are passed as container args to run.sh.
func buildChromiumContainer(instance *openclawv1alpha1.OpenClawInstance) corev1.Container {
	repo := instance.Spec.Chromium.Image.Repository
	if repo == "" {
		repo = DefaultChromiumImage
	}

	// Migrate instances created before v0.22.1 that have the old browserless
	// image stored via kubebuilder defaults. The old image no longer exists
	// on GHCR, and even if it did, its entrypoint is incompatible with the
	// Chrome launch flags we pass as container args (#396).
	if repo == DeprecatedChromiumImage {
		rLog.Info("migrating deprecated chromium image to default",
			"old", repo, "new", DefaultChromiumImage)
		repo = DefaultChromiumImage
	}

	tag := instance.Spec.Chromium.Image.Tag
	if tag == "" {
		if repo == DefaultChromiumImage {
			tag = DefaultChromiumTag
		} else {
			tag = DefaultImageTag
		}
	}

	// The old browserless image defaulted to "latest"; normalize to "stable"
	// when migrating to the new image.
	if repo == DefaultChromiumImage && tag == "latest" {
		tag = DefaultChromiumTag
	}

	image := repo + ":" + tag
	if instance.Spec.Chromium.Image.Digest != "" {
		image = repo + "@" + instance.Spec.Chromium.Image.Digest
	}
	image = ApplyRegistryOverride(image, instance.Spec.Registry)

	chromiumMounts := []corev1.VolumeMount{
		{
			Name:      "chromium-tmp",
			MountPath: "/tmp",
		},
		{
			Name:      "chromium-shm",
			MountPath: "/dev/shm",
		},
		{
			Name:      "chromium-data",
			MountPath: "/chromium-data",
		},
	}

	// Set HOME to /tmp (an emptyDir) so fontconfig and other tools that
	// need a writable home directory work correctly. The default nobody
	// user (65534) has home /nonexistent which does not exist.
	chromiumEnv := []corev1.EnvVar{
		{Name: "HOME", Value: "/tmp"},
	}

	// Add CA bundle mount if configured. The certificate file is mounted
	// into the system CA directory so Chrome picks it up automatically.
	if cab := instance.Spec.Security.CABundle; cab != nil {
		key := cab.Key
		if key == "" {
			key = DefaultCABundleKey
		}
		chromiumMounts = append(chromiumMounts, corev1.VolumeMount{
			Name:      "ca-bundle",
			MountPath: "/etc/ssl/certs/custom-ca-bundle.crt",
			SubPath:   key,
			ReadOnly:  true,
		})
	}

	// Append user-supplied extra env vars
	chromiumEnv = append(chromiumEnv, instance.Spec.Chromium.ExtraEnv...)

	// Override entrypoint for the default image to fix unquoted $@ in
	// upstream run.sh that causes word-splitting of args with spaces
	// (e.g. --user-agent), leading to "Multiple targets are not supported".
	// Custom images keep their own entrypoint. See #396.
	var command []string
	if repo == DefaultChromiumImage {
		command = ChromiumEntrypointCommand
	}

	return corev1.Container{
		Name:                     "chromium",
		Image:                    image,
		ImagePullPolicy:          corev1.PullIfNotPresent,
		Command:                  command,
		Args:                     ChromiumArgs(instance),
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: Ptr(false),
			ReadOnlyRootFilesystem:   Ptr(false), // Chromium needs writable dirs for profiles, cache, crash dumps
			RunAsNonRoot:             Ptr(true),
			RunAsUser:                Ptr(int64(65534)), // nobody - headless-shell has no pre-created users
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "cdp",
				ContainerPort: ChromiumPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources:    buildChromiumResourceRequirements(instance),
		Env:          chromiumEnv,
		VolumeMounts: chromiumMounts,
		// Startup probe ensures Chrome is ready to accept CDP connections
		// before the pod is marked Ready.
		StartupProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/json/version",
					Port: intstr.FromInt32(ChromiumPort),
				},
			},
			InitialDelaySeconds: 1,
			PeriodSeconds:       2,
			FailureThreshold:    15,
			SuccessThreshold:    1,
			TimeoutSeconds:      5,
		},
	}
}

// buildOllamaContainer creates the Ollama sidecar container
func buildOllamaContainer(instance *openclawv1alpha1.OpenClawInstance) corev1.Container {
	repo := instance.Spec.Ollama.Image.Repository
	if repo == "" {
		repo = "ollama/ollama"
	}

	tag := instance.Spec.Ollama.Image.Tag
	if tag == "" {
		tag = DefaultImageTag
	}

	image := repo + ":" + tag
	if instance.Spec.Ollama.Image.Digest != "" {
		image = repo + "@" + instance.Spec.Ollama.Image.Digest
	}
	image = ApplyRegistryOverride(image, instance.Spec.Registry)

	container := corev1.Container{
		Name:                     "ollama",
		Image:                    image,
		ImagePullPolicy:          corev1.PullIfNotPresent,
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: Ptr(false),
			ReadOnlyRootFilesystem:   Ptr(false), // Ollama needs writable dirs
			RunAsNonRoot:             Ptr(false), // Ollama requires root
			RunAsUser:                Ptr(int64(0)),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "ollama",
				ContainerPort: OllamaPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: buildOllamaResourceRequirements(instance),
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "ollama-models",
				MountPath: "/root/.ollama",
			},
		},
	}

	return container
}

// buildWebTerminalContainer creates the ttyd web terminal sidecar container
func buildWebTerminalContainer(instance *openclawv1alpha1.OpenClawInstance) corev1.Container {
	repo := instance.Spec.WebTerminal.Image.Repository
	if repo == "" {
		repo = "tsl0922/ttyd"
	}

	tag := instance.Spec.WebTerminal.Image.Tag
	if tag == "" {
		tag = DefaultImageTag
	}

	image := repo + ":" + tag
	if instance.Spec.WebTerminal.Image.Digest != "" {
		image = repo + "@" + instance.Spec.WebTerminal.Image.Digest
	}
	image = ApplyRegistryOverride(image, instance.Spec.Registry)

	// Build ttyd command flags
	var flags []string
	if instance.Spec.WebTerminal.ReadOnly {
		flags = append(flags, "-R")
	} else {
		// ttyd defaults to read-only when --writable/-W is not passed.
		// Explicitly pass -W so that ReadOnly: false results in an interactive terminal.
		flags = append(flags, "-W")
	}
	if instance.Spec.WebTerminal.Credential != nil {
		flags = append(flags, `-c "${TTYD_USERNAME}:${TTYD_PASSWORD}"`)
	}

	// Always use sh -c to support env var expansion for credentials
	var flagStr string
	if len(flags) > 0 {
		flagStr = strings.Join(flags, " ") + " "
	}
	command := []string{"sh", "-c", "exec ttyd " + flagStr + "sh"}

	// Volume mounts
	dataReadOnly := instance.Spec.WebTerminal.ReadOnly
	mounts := []corev1.VolumeMount{
		{
			Name:      "data",
			MountPath: "/home/openclaw/.openclaw",
			ReadOnly:  dataReadOnly,
		},
		{
			Name:      "web-terminal-tmp",
			MountPath: "/tmp",
		},
	}

	// Environment variables (credentials from Secret)
	var env []corev1.EnvVar
	if instance.Spec.WebTerminal.Credential != nil {
		secretName := instance.Spec.WebTerminal.Credential.SecretRef.Name
		env = append(env,
			corev1.EnvVar{
				Name: "TTYD_USERNAME",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
						Key:                  "username",
					},
				},
			},
			corev1.EnvVar{
				Name: "TTYD_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
						Key:                  "password",
					},
				},
			},
		)
	}

	return corev1.Container{
		Name:                     "web-terminal",
		Image:                    image,
		Command:                  command,
		ImagePullPolicy:          corev1.PullIfNotPresent,
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: Ptr(false),
			ReadOnlyRootFilesystem:   Ptr(false), // ttyd needs writable rootfs
			RunAsNonRoot:             Ptr(podRunAsNonRoot(instance)),
			RunAsUser:                Ptr(int64(1000)), // same as main container
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "web-terminal",
				ContainerPort: WebTerminalPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources:    buildWebTerminalResourceRequirements(instance),
		Env:          env,
		VolumeMounts: mounts,
	}
}

// buildWebTerminalResourceRequirements creates resource requirements for the web terminal container
func buildWebTerminalResourceRequirements(instance *openclawv1alpha1.OpenClawInstance) corev1.ResourceRequirements {
	req := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}

	// Requests
	req.Requests[corev1.ResourceCPU] = ParseQuantity(instance.Spec.WebTerminal.Resources.Requests.CPU, "50m")
	req.Requests[corev1.ResourceMemory] = ParseQuantity(instance.Spec.WebTerminal.Resources.Requests.Memory, "64Mi")

	// Limits
	req.Limits[corev1.ResourceCPU] = ParseQuantity(instance.Spec.WebTerminal.Resources.Limits.CPU, "200m")
	req.Limits[corev1.ResourceMemory] = ParseQuantity(instance.Spec.WebTerminal.Resources.Limits.Memory, "128Mi")

	return req
}

// buildOTelCollectorContainer creates the OpenTelemetry Collector sidecar.
// It receives OTLP metrics from OpenClaw and exposes a Prometheus scrape
// endpoint on the configured metrics port.
func buildOTelCollectorContainer(instance *openclawv1alpha1.OpenClawInstance) corev1.Container {
	image := DefaultOTelCollectorImage + ":" + DefaultOTelCollectorTag
	image = ApplyRegistryOverride(image, instance.Spec.Registry)

	return corev1.Container{
		Name:                     "otel-collector",
		Image:                    image,
		ImagePullPolicy:          corev1.PullIfNotPresent,
		Args:                     []string{"--config=/etc/otel-collector/" + OTelCollectorConfigKey},
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: Ptr(false),
			ReadOnlyRootFilesystem:   Ptr(true),
			RunAsNonRoot:             Ptr(true),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "metrics",
				ContainerPort: MetricsPort(instance),
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    ParseQuantity("", "25m"),
				corev1.ResourceMemory: ParseQuantity("", "32Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    ParseQuantity("", "100m"),
				corev1.ResourceMemory: ParseQuantity("", "128Mi"),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "otel-collector-config",
				MountPath: "/etc/otel-collector",
				ReadOnly:  true,
			},
		},
	}
}

// buildOllamaResourceRequirements creates resource requirements for the Ollama container
func buildOllamaResourceRequirements(instance *openclawv1alpha1.OpenClawInstance) corev1.ResourceRequirements {
	req := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}

	// Requests
	req.Requests[corev1.ResourceCPU] = ParseQuantity(instance.Spec.Ollama.Resources.Requests.CPU, "500m")
	req.Requests[corev1.ResourceMemory] = ParseQuantity(instance.Spec.Ollama.Resources.Requests.Memory, "1Gi")

	// Limits
	req.Limits[corev1.ResourceCPU] = ParseQuantity(instance.Spec.Ollama.Resources.Limits.CPU, "2000m")
	req.Limits[corev1.ResourceMemory] = ParseQuantity(instance.Spec.Ollama.Resources.Limits.Memory, "4Gi")

	// GPU support
	if instance.Spec.Ollama.GPU != nil && *instance.Spec.Ollama.GPU > 0 {
		gpuQty := ParseQuantity(fmt.Sprintf("%d", *instance.Spec.Ollama.GPU), "0")
		req.Requests[corev1.ResourceName("nvidia.com/gpu")] = gpuQty
		req.Limits[corev1.ResourceName("nvidia.com/gpu")] = gpuQty
	}

	return req
}

// buildOllamaModelPullInitContainer creates the init container that pre-pulls Ollama models.
func buildOllamaModelPullInitContainer(instance *openclawv1alpha1.OpenClawInstance) corev1.Container {
	// Build the pull command: start server, pull each model, then stop server
	var pullCmds []string
	for _, model := range instance.Spec.Ollama.Models {
		pullCmds = append(pullCmds, fmt.Sprintf("ollama pull %s", shellQuote(model)))
	}
	script := fmt.Sprintf("ollama serve & sleep 2 && %s; kill %%1 2>/dev/null; exit 0", strings.Join(pullCmds, " && "))

	repo := instance.Spec.Ollama.Image.Repository
	if repo == "" {
		repo = "ollama/ollama"
	}
	tag := instance.Spec.Ollama.Image.Tag
	if tag == "" {
		tag = DefaultImageTag
	}
	image := repo + ":" + tag
	if instance.Spec.Ollama.Image.Digest != "" {
		image = repo + "@" + instance.Spec.Ollama.Image.Digest
	}
	image = ApplyRegistryOverride(image, instance.Spec.Registry)

	return corev1.Container{
		Name:                     "init-ollama",
		Image:                    image,
		Command:                  []string{"sh", "-c", script},
		ImagePullPolicy:          corev1.PullIfNotPresent,
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: Ptr(false),
			ReadOnlyRootFilesystem:   Ptr(false), // Ollama needs writable dirs
			RunAsNonRoot:             Ptr(false), // Ollama requires root
			RunAsUser:                Ptr(int64(0)),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		Resources: buildOllamaResourceRequirements(instance),
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "ollama-models",
				MountPath: "/root/.ollama",
			},
		},
	}
}

// buildVolumes creates the volume specs
func buildVolumes(instance *openclawv1alpha1.OpenClawInstance, skillPacks *ResolvedSkillPacks) []corev1.Volume {
	volumes := []corev1.Volume{}

	// Data volume (PVC or emptyDir)
	switch {
	case IsPersistenceEnabled(instance) && IsHPAEnabled(instance):
		// VolumeClaimTemplates handle per-replica PVCs - the StatefulSet
		// controller auto-creates a volume named "data" for each pod.
	case IsPersistenceEnabled(instance):
		pvcName := PVCName(instance)
		if instance.Spec.Storage.Persistence.ExistingClaim != "" {
			pvcName = instance.Spec.Storage.Persistence.ExistingClaim
		}
		volumes = append(volumes, corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		})
	default:
		volumes = append(volumes, corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	// Config volume - always mount the operator-managed ConfigMap.
	// The controller enriches all config sources (raw, configMapRef, or
	// empty default) and writes the result into this ConfigMap.
	defaultMode := int32(0o644)
	volumes = append(volumes, corev1.Volume{
		Name: "config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: ConfigMapName(instance),
				},
				DefaultMode: &defaultMode,
			},
		},
	})

	// OTel Collector config - directory mount (no subPath) so the kubelet
	// uses an atomic symlink that tolerates ConfigMap/pod creation races.
	if IsMetricsEnabled(instance) {
		volumes = append(volumes, corev1.Volume{
			Name: "otel-collector-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: ConfigMapName(instance),
					},
					DefaultMode: &defaultMode,
					Items: []corev1.KeyToPath{
						{
							Key:  OTelCollectorConfigKey,
							Path: OTelCollectorConfigKey,
						},
					},
				},
			},
		})
	}

	// Workspace init volume (ConfigMap with seed files)
	if hasWorkspaceFiles(instance, skillPacks) {
		volumes = append(volumes, corev1.Volume{
			Name: "workspace-init",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: WorkspaceConfigMapName(instance),
					},
					DefaultMode: &defaultMode,
				},
			},
		})
	}

	// Skills-tmp volume for skills init container
	if len(instance.Spec.Skills) > 0 || hasWorkspaceSkills(instance) {
		volumes = append(volumes, corev1.Volume{
			Name: "skills-tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	// Plugins-tmp volume for plugins init container
	if len(instance.Spec.Plugins) > 0 {
		volumes = append(volumes, corev1.Volume{
			Name: "plugins-tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	// Runtime dep tmp volumes
	if instance.Spec.RuntimeDeps.Pnpm {
		volumes = append(volumes, corev1.Volume{
			Name: "pnpm-tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}
	if instance.Spec.RuntimeDeps.Python {
		volumes = append(volumes, corev1.Volume{
			Name: "python-tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	// Init-tmp volume for merge mode (node writes to /tmp/merged.json) or JSON5 mode (npx writes to /tmp/converted.json)
	if instance.Spec.Config.MergeMode == ConfigMergeModeMerge || instance.Spec.Config.Format == ConfigFormatJSON5 {
		volumes = append(volumes, corev1.Volume{
			Name: "init-tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	// Tmp volume: main container (read-only rootfs needs writable /tmp)
	volumes = append(volumes,
		corev1.Volume{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	)

	// Gateway proxy tmp volume (nginx pid file) — only when proxy is enabled
	if IsGatewayProxyEnabled(instance) {
		volumes = append(volumes, corev1.Volume{
			Name: "gateway-proxy-tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	// Tailscale volumes (state lives under /tmp so no separate state volume)
	if instance.Spec.Tailscale.Enabled {
		volumes = append(volumes,
			corev1.Volume{
				Name: "tailscale-socket",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			corev1.Volume{
				Name: "tailscale-bin",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			corev1.Volume{
				Name: "tailscale-tmp",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		)
	}

	// Chromium volumes
	if instance.Spec.Chromium.Enabled {
		volumes = append(volumes,
			corev1.Volume{
				Name: "chromium-tmp",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			corev1.Volume{
				Name: "chromium-shm",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{
						Medium:    corev1.StorageMediumMemory,
						SizeLimit: resource.NewQuantity(1024*1024*1024, resource.BinarySI), // 1Gi
					},
				},
			},
		)

		// Chromium browser profile data volume - persistent PVC or ephemeral emptyDir
		if instance.Spec.Chromium.Persistence.Enabled {
			claimName := ChromiumPVCName(instance)
			if instance.Spec.Chromium.Persistence.ExistingClaim != "" {
				claimName = instance.Spec.Chromium.Persistence.ExistingClaim
			}
			volumes = append(volumes, corev1.Volume{
				Name: "chromium-data",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: claimName,
					},
				},
			})
		} else {
			volumes = append(volumes, corev1.Volume{
				Name: "chromium-data",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			})
		}
	}

	// Ollama model cache volume
	if instance.Spec.Ollama.Enabled {
		if instance.Spec.Ollama.Storage.ExistingClaim != "" {
			volumes = append(volumes, corev1.Volume{
				Name: "ollama-models",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: instance.Spec.Ollama.Storage.ExistingClaim,
					},
				},
			})
		} else {
			sizeLimit := instance.Spec.Ollama.Storage.SizeLimit
			qty := ParseQuantity(sizeLimit, "20Gi")
			volumes = append(volumes, corev1.Volume{
				Name: "ollama-models",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{
						SizeLimit: &qty,
					},
				},
			})
		}
	}

	// Web terminal tmp volume
	if instance.Spec.WebTerminal.Enabled {
		volumes = append(volumes, corev1.Volume{
			Name: "web-terminal-tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	// CA bundle volume
	if cab := instance.Spec.Security.CABundle; cab != nil {
		if cab.ConfigMapName != "" {
			volumes = append(volumes, corev1.Volume{
				Name: "ca-bundle",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cab.ConfigMapName,
						},
						DefaultMode: &defaultMode,
					},
				},
			})
		} else if cab.SecretName != "" {
			volumes = append(volumes, corev1.Volume{
				Name: "ca-bundle",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  cab.SecretName,
						DefaultMode: &defaultMode,
					},
				},
			})
		}
	}

	// Custom sidecar volumes
	volumes = append(volumes, instance.Spec.SidecarVolumes...)

	// Extra volumes (available to main container via ExtraVolumeMounts)
	volumes = append(volumes, instance.Spec.ExtraVolumes...)

	return volumes
}

// buildResourceRequirements creates resource requirements for the main container
func buildResourceRequirements(instance *openclawv1alpha1.OpenClawInstance) corev1.ResourceRequirements {
	req := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}

	// Requests
	req.Requests[corev1.ResourceCPU] = ParseQuantity(instance.Spec.Resources.Requests.CPU, "500m")
	req.Requests[corev1.ResourceMemory] = ParseQuantity(instance.Spec.Resources.Requests.Memory, "1Gi")

	// Limits
	req.Limits[corev1.ResourceCPU] = ParseQuantity(instance.Spec.Resources.Limits.CPU, "2000m")
	req.Limits[corev1.ResourceMemory] = ParseQuantity(instance.Spec.Resources.Limits.Memory, "4Gi")

	return req
}

// buildChromiumResourceRequirements creates resource requirements for the Chromium container
func buildChromiumResourceRequirements(instance *openclawv1alpha1.OpenClawInstance) corev1.ResourceRequirements {
	req := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}

	// Requests
	req.Requests[corev1.ResourceCPU] = ParseQuantity(instance.Spec.Chromium.Resources.Requests.CPU, "250m")
	req.Requests[corev1.ResourceMemory] = ParseQuantity(instance.Spec.Chromium.Resources.Requests.Memory, "512Mi")

	// Limits
	req.Limits[corev1.ResourceCPU] = ParseQuantity(instance.Spec.Chromium.Resources.Limits.CPU, "1000m")
	req.Limits[corev1.ResourceMemory] = ParseQuantity(instance.Spec.Chromium.Resources.Limits.Memory, "2Gi")

	return req
}

// buildHTTPProbeHandler returns an HTTP GET probe handler. When the gateway
// proxy sidecar is enabled, probes target the proxy port (18790) which
// forwards to the gateway on loopback. When disabled, probes hit the
// gateway directly on port 18789.
func buildHTTPProbeHandler(probePath string, instance *openclawv1alpha1.OpenClawInstance) corev1.ProbeHandler {
	return corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Path:   probePath,
			Port:   intstr.FromInt32(probeTargetPort(instance)),
			Scheme: corev1.URISchemeHTTP,
		},
	}
}

// probeTargetPort returns the loopback port that readiness checks should hit.
// When the gateway proxy sidecar is enabled, traffic goes through the proxy
// port; otherwise it hits the gateway directly.
func probeTargetPort(instance *openclawv1alpha1.OpenClawInstance) int32 {
	if IsGatewayProxyEnabled(instance) {
		return GatewayProxyPort
	}
	return int32(GatewayPort)
}

// buildDiskReadinessHandler renders the exec handler for the optional
// disk-aware readiness guard. The script first verifies the workspace mount is
// writable and has free space above the configured threshold, then defers to
// the gateway /readyz endpoint as the primary readiness signal. The HTTP check
// gracefully degrades (is skipped) only if neither curl nor wget is present in
// the image, so a missing HTTP client never makes a healthy pod permanently
// NotReady. A full or read-only PVC always fails the probe.
func buildDiskReadinessHandler(spec *openclawv1alpha1.DiskReadinessSpec, instance *openclawv1alpha1.OpenClawInstance) corev1.ProbeHandler {
	mountPath := spec.Path
	if mountPath == "" {
		mountPath = WorkspaceDataMountPath
	}
	minFree := ParseQuantity(spec.MinFree, DefaultDiskReadinessMinFree)
	// Compare in KiB, not bytes: df -Pk already reports integer 1K blocks, and
	// any awk arithmetic on the column ($4 * 1024) can render the result in
	// scientific notation (OFMT %.6g, e.g. 1.2642e+10 under busybox awk), which
	// POSIX [ -ge ] rejects with "Illegal number" (#567). Round the threshold
	// up so a fractional KiB still requires the full requested free space.
	minFreeKiB := (minFree.Value() + 1023) / 1024
	port := probeTargetPort(instance)

	// POSIX sh: the available column from df -Pk is passed through verbatim as
	// an integer string. Quoting keeps paths with spaces safe. set -e
	// propagates the first failing check as a non-zero exit (=> NotReady).
	script := fmt.Sprintf(`set -e
p='%s'
[ -w "$p" ] || exit 1
avail_kib=$(df -Pk "$p" 2>/dev/null | awk 'NR==2 {print $4}')
[ -n "$avail_kib" ] || exit 1
[ "$avail_kib" -ge %d ] || exit 1
if command -v curl >/dev/null 2>&1; then
  curl -fsS -o /dev/null --max-time 3 "http://127.0.0.1:%d/readyz"
elif command -v wget >/dev/null 2>&1; then
  wget -q -O /dev/null -T 3 "http://127.0.0.1:%d/readyz"
fi
`, mountPath, minFreeKiB, port, port)

	return corev1.ProbeHandler{
		Exec: &corev1.ExecAction{Command: []string{"sh", "-c", script}},
	}
}

// buildLivenessProbe creates the liveness probe
func buildLivenessProbe(instance *openclawv1alpha1.OpenClawInstance) *corev1.Probe {
	var spec *openclawv1alpha1.ProbeSpec
	if instance.Spec.Probes != nil {
		spec = instance.Spec.Probes.Liveness
	}
	if spec != nil && spec.Enabled != nil && !*spec.Enabled {
		return nil
	}

	probe := &corev1.Probe{
		ProbeHandler:        buildHTTPProbeHandler("/healthz", instance),
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
		TimeoutSeconds:      5,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}

	if spec != nil {
		if spec.InitialDelaySeconds != nil {
			probe.InitialDelaySeconds = *spec.InitialDelaySeconds
		}
		if spec.PeriodSeconds != nil {
			probe.PeriodSeconds = *spec.PeriodSeconds
		}
		if spec.TimeoutSeconds != nil {
			probe.TimeoutSeconds = *spec.TimeoutSeconds
		}
		if spec.FailureThreshold != nil {
			probe.FailureThreshold = *spec.FailureThreshold
		}
	}

	return probe
}

// buildReadinessProbe creates the readiness probe
func buildReadinessProbe(instance *openclawv1alpha1.OpenClawInstance) *corev1.Probe {
	var spec *openclawv1alpha1.ProbeSpec
	if instance.Spec.Probes != nil {
		spec = instance.Spec.Probes.Readiness
	}
	if spec != nil && spec.Enabled != nil && !*spec.Enabled {
		return nil
	}

	// Defense-in-depth: when the opt-in disk-aware guard is enabled, render the
	// readiness probe as an exec check that combines workspace disk health with
	// the gateway /readyz signal. Liveness/startup stay HTTP-only.
	handler := buildHTTPProbeHandler("/readyz", instance)
	if instance.Spec.Probes != nil && instance.Spec.Probes.DiskReadiness != nil {
		dr := instance.Spec.Probes.DiskReadiness
		if dr.Enabled != nil && *dr.Enabled {
			handler = buildDiskReadinessHandler(dr, instance)
		}
	}

	probe := &corev1.Probe{
		ProbeHandler:        handler,
		InitialDelaySeconds: 5,
		PeriodSeconds:       5,
		TimeoutSeconds:      3,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}

	if spec != nil {
		if spec.InitialDelaySeconds != nil {
			probe.InitialDelaySeconds = *spec.InitialDelaySeconds
		}
		if spec.PeriodSeconds != nil {
			probe.PeriodSeconds = *spec.PeriodSeconds
		}
		if spec.TimeoutSeconds != nil {
			probe.TimeoutSeconds = *spec.TimeoutSeconds
		}
		if spec.FailureThreshold != nil {
			probe.FailureThreshold = *spec.FailureThreshold
		}
	}

	return probe
}

// buildStartupProbe creates the startup probe
func buildStartupProbe(instance *openclawv1alpha1.OpenClawInstance) *corev1.Probe {
	var spec *openclawv1alpha1.ProbeSpec
	if instance.Spec.Probes != nil {
		spec = instance.Spec.Probes.Startup
	}
	if spec != nil && spec.Enabled != nil && !*spec.Enabled {
		return nil
	}

	probe := &corev1.Probe{
		ProbeHandler:        buildHTTPProbeHandler("/healthz", instance),
		InitialDelaySeconds: 5,
		PeriodSeconds:       5,
		TimeoutSeconds:      3,
		SuccessThreshold:    1,
		FailureThreshold:    60, // 60 * 5s = 300s startup time
	}

	if spec != nil {
		if spec.InitialDelaySeconds != nil {
			probe.InitialDelaySeconds = *spec.InitialDelaySeconds
		}
		if spec.PeriodSeconds != nil {
			probe.PeriodSeconds = *spec.PeriodSeconds
		}
		if spec.TimeoutSeconds != nil {
			probe.TimeoutSeconds = *spec.TimeoutSeconds
		}
		if spec.FailureThreshold != nil {
			probe.FailureThreshold = *spec.FailureThreshold
		}
	}

	return probe
}

// buildConfigRestoreCommand returns the shell command for the main container's
// postStart lifecycle hook. It copies the operator-managed config from the
// ConfigMap volume to the PVC on every container start, ensuring the config is
// restored even after a container restart (where init containers don't re-run).
// Returns "" for JSON5 format (requires npx, too slow for postStart).
func buildConfigRestoreCommand(instance *openclawv1alpha1.OpenClawInstance) string {
	key := configMapKey(instance)
	if key == "" {
		return ""
	}

	src := "/operator-config/" + key
	dst := "/home/openclaw/.openclaw/openclaw.json"

	switch {
	case instance.Spec.Config.MergeMode == ConfigMergeModeMerge:
		// Deep-merge operator config into existing PVC config via Node.js.
		// Same logic as the init container merge, but with main container paths.
		// forcePaths handling must match the init script -- otherwise an
		// attacker could persist a rogue subtree across a container restart
		// (which does not re-run init containers) and bypass tenant isolation.
		//
		// dp(o,p) treats arrays as objects (typeof [] === "object"); harmless
		// given the operator-controlled schema has no arrays at these depths.
		return fmt.Sprintf(
			`__forcepaths=%s node -e '`+
				`const fs=require("fs");`+
				`function dm(a,b){const r={...a};for(const k in b){r[k]=b[k]&&typeof b[k]==="object"&&!Array.isArray(b[k])&&r[k]&&typeof r[k]==="object"&&!Array.isArray(r[k])?dm(r[k],b[k]):b[k]}return r}`+
				`function dp(o,p){const k=p.split(".");let c=o;for(let i=0;i<k.length-1;i++){if(!c[k[i]]||typeof c[k[i]]!=="object")return;c=c[k[i]]}delete c[k[k.length-1]]}`+ // dp descends into arrays (typeof []==="object") -- harmless: operator schema has no arrays at forcePath depths
				`const e="%s",c="%s",t="/tmp/merged.json";`+
				`const base=fs.existsSync(e)?JSON.parse(fs.readFileSync(e,"utf8")):{};`+
				`const fp=JSON.parse(process.env.__forcepaths);`+
				`for(const p of fp)dp(base,p);`+
				`const inc=JSON.parse(fs.readFileSync(c,"utf8"));`+
				`fs.writeFileSync(t,JSON.stringify(dm(base,inc),null,2));`+
				`fs.copyFileSync(t,e);`+
				`'`,
			shellQuote(forcePathsJSON(instance)),
			dst, src)
	case instance.Spec.Config.Format == ConfigFormatJSON5:
		// JSON5 conversion requires npx which is too slow for a postStart hook.
		// Config is only restored on pod recreation (init container).
		return ""
	default:
		// Overwrite (default) - operator-managed config always wins
		return fmt.Sprintf("cp %s %s", src, dst)
	}
}

// getPullPolicy returns the image pull policy with defaults
func getPullPolicy(instance *openclawv1alpha1.OpenClawInstance) corev1.PullPolicy {
	if instance.Spec.Image.PullPolicy != "" {
		return instance.Spec.Image.PullPolicy
	}
	return corev1.PullIfNotPresent
}

// calculateConfigHash computes a hash of the config, skills, plugins, and
// runtime settings for rollout detection. Changes to any of these trigger a
// pod restart.
//
// The hash covers the RENDERED ConfigMap data (the same openclaw.json/nginx/
// tailscale/otel bytes produced by BuildConfigMap), not just selected spec
// fields. This way every spec section the enrichment pipeline reads (e.g.
// spec.gateway.controlUiOrigins, spec.networking.ingress hosts/TLS) rolls the
// pod when it changes the config the pod actually consumes. Hashing only
// hand-picked spec fields previously let the ConfigMap re-render while the
// StatefulSet never rolled, so pods kept serving the stale config.
//
// External workspace files (spec.workspace.configMapRef and per-workspace
// configMapRefs) are hashed too: the workspace-init container copies them into
// the workspace at pod start, so content changes only take effect after a
// restart. Inline spec.workspace.initialFiles remain excluded (unchanged
// behavior; they are delivered via a projected ConfigMap volume).
//
// The gateway token is deliberately NOT part of the hash: BuildConfigMap is
// invoked with an empty token here, so token rotation does not roll the pod
// (same behavior as before).
func calculateConfigHash(instance *openclawv1alpha1.OpenClawInstance, skillPacks *ResolvedSkillPacks, externalWorkspaceFiles map[string]string, additionalExternalFiles map[string]map[string]string) string {
	h := sha256.New()
	configData, _ := json.Marshal(instance.Spec.Config)
	h.Write(configData)
	// Rendered ConfigMap data: openclaw.json after the full enrichment
	// pipeline plus the nginx/tailscale-serve/otel-collector companion keys.
	// json.Marshal sorts map keys, so this is deterministic across reconciles.
	renderedData, _ := json.Marshal(BuildConfigMap(instance, "", skillPacks).Data)
	h.Write(renderedData)
	if len(externalWorkspaceFiles) > 0 {
		extData, _ := json.Marshal(externalWorkspaceFiles)
		h.Write(extData)
	}
	if len(additionalExternalFiles) > 0 {
		addExtData, _ := json.Marshal(additionalExternalFiles)
		h.Write(addExtData)
	}
	if len(instance.Spec.Skills) > 0 {
		skillsData, _ := json.Marshal(instance.Spec.Skills)
		h.Write(skillsData)
	}
	// Workspace-scoped skills, keyed by workspace name so moving an identical
	// skill between workspaces still changes the hash and triggers a rollout (#568).
	if instance.Spec.Workspace != nil {
		for i := range instance.Spec.Workspace.AdditionalWorkspaces {
			aw := &instance.Spec.Workspace.AdditionalWorkspaces[i]
			if len(aw.Skills) > 0 {
				wsSkillsData, _ := json.Marshal(map[string][]string{aw.Name: aw.Skills})
				h.Write(wsSkillsData)
			}
		}
	}
	if len(instance.Spec.Plugins) > 0 {
		pluginsData, _ := json.Marshal(instance.Spec.Plugins)
		h.Write(pluginsData)
	}
	if len(instance.Spec.InitContainers) > 0 {
		icData, _ := json.Marshal(instance.Spec.InitContainers)
		h.Write(icData)
	}
	if instance.Spec.RuntimeDeps.Pnpm || instance.Spec.RuntimeDeps.Python {
		rdData, _ := json.Marshal(instance.Spec.RuntimeDeps)
		h.Write(rdData)
	}
	if instance.Spec.Tailscale.Enabled {
		tsData, _ := json.Marshal(instance.Spec.Tailscale)
		h.Write(tsData)
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// NormalizeStatefulSet applies the same defaults that the Kubernetes API server
// admission controller would apply. This prevents CreateOrUpdate from detecting
// spurious diffs between the desired spec (built by the operator) and the
// existing spec (read from the API server with defaults applied).
//
// Without this, the operator issues an Update on every reconcile. K8s re-applies
// defaults, so the stored spec doesn't actually change, but the unnecessary
// Update calls waste API server resources and can interfere with rolling updates.
func NormalizeStatefulSet(sts *appsv1.StatefulSet) {
	spec := &sts.Spec.Template.Spec

	// K8s defaults DeprecatedServiceAccount from ServiceAccountName
	if spec.ServiceAccountName != "" && spec.DeprecatedServiceAccount == "" {
		spec.DeprecatedServiceAccount = spec.ServiceAccountName
	}

	// Normalize all containers (init + regular)
	for i := range spec.InitContainers {
		normalizeContainer(&spec.InitContainers[i])
	}
	for i := range spec.Containers {
		normalizeContainer(&spec.Containers[i])
	}

	// K8s defaults UpdateStrategy.RollingUpdate to &{} when Type == RollingUpdate.
	// Without this, CreateOrUpdate sees nil vs &{} on every reconcile, issues an
	// unnecessary Update, increments the StatefulSet resourceVersion, fires a watch
	// event, and causes a continuous reconcile loop.
	// Mirrors: k8s.io/kubernetes/pkg/apis/apps/v1/defaults.go SetDefaults_StatefulSetSpec
	if sts.Spec.UpdateStrategy.Type == appsv1.RollingUpdateStatefulSetStrategyType && sts.Spec.UpdateStrategy.RollingUpdate == nil {
		sts.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{}
	}

	// K8s defaults VolumeMode to Filesystem on VolumeClaimTemplates
	filesystemMode := corev1.PersistentVolumeFilesystem
	for i := range sts.Spec.VolumeClaimTemplates {
		if sts.Spec.VolumeClaimTemplates[i].Spec.VolumeMode == nil {
			sts.Spec.VolumeClaimTemplates[i].Spec.VolumeMode = &filesystemMode
		}
	}
}

// normalizeContainer applies K8s admission defaults to a single container.
func normalizeContainer(c *corev1.Container) {
	// K8s defaults FieldRef.APIVersion to "v1" in env var sources
	for i := range c.Env {
		if c.Env[i].ValueFrom != nil && c.Env[i].ValueFrom.FieldRef != nil {
			if c.Env[i].ValueFrom.FieldRef.APIVersion == "" {
				c.Env[i].ValueFrom.FieldRef.APIVersion = "v1"
			}
		}
	}

	// K8s defaults TerminationMessagePath and TerminationMessagePolicy
	if c.TerminationMessagePath == "" {
		c.TerminationMessagePath = corev1.TerminationMessagePathDefault
	}
	if c.TerminationMessagePolicy == "" {
		c.TerminationMessagePolicy = corev1.TerminationMessageReadFile
	}

	// K8s defaults ImagePullPolicy based on image tag
	if c.ImagePullPolicy == "" {
		if strings.HasSuffix(c.Image, ":latest") || !strings.Contains(c.Image, ":") {
			c.ImagePullPolicy = corev1.PullAlways
		} else {
			c.ImagePullPolicy = corev1.PullIfNotPresent
		}
	}

	// K8s defaults probe fields when probes are non-nil
	normalizeProbe(c.LivenessProbe)
	normalizeProbe(c.ReadinessProbe)
	normalizeProbe(c.StartupProbe)
}

// normalizeProbe applies K8s admission defaults to probe fields.
func normalizeProbe(p *corev1.Probe) {
	if p == nil {
		return
	}
	if p.TimeoutSeconds == 0 {
		p.TimeoutSeconds = 1
	}
	if p.PeriodSeconds == 0 {
		p.PeriodSeconds = 10
	}
	if p.SuccessThreshold == 0 {
		p.SuccessThreshold = 1
	}
	if p.FailureThreshold == 0 {
		p.FailureThreshold = 3
	}
	if p.HTTPGet != nil && p.HTTPGet.Scheme == "" {
		p.HTTPGet.Scheme = corev1.URISchemeHTTP
	}
}

// statefulSetReplicas returns the replica count for the StatefulSet.
// When suspended, replicas is explicitly set to 0.
// When HPA is enabled, replicas is set to nil so the HPA manages scaling.
// Otherwise defaults to 1 (single-instance).
func statefulSetReplicas(instance *openclawv1alpha1.OpenClawInstance) *int32 {
	if instance.Spec.Suspended {
		return Ptr(int32(0))
	}
	if IsHPAEnabled(instance) {
		return nil
	}
	return Ptr(int32(1))
}

// VolumeClaimTemplatesEqual compares two VolumeClaimTemplate slices by name
// and spec. Both name and spec are immutable on existing StatefulSets, so any
// change requires a delete+recreate. The caller must normalize the desired VCTs
// (e.g. via NormalizeStatefulSet) before comparing so that API server defaults
// like VolumeMode don't cause false negatives.
func VolumeClaimTemplatesEqual(a, b []corev1.PersistentVolumeClaim) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
		if !apiequality.Semantic.DeepEqual(a[i].Spec, b[i].Spec) {
			return false
		}
	}
	return true
}
