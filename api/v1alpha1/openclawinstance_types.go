package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// OpenClawInstanceSpec defines the desired state of OpenClawInstance
type OpenClawInstanceSpec struct {
	// Registry is the global container image registry override.
	// When set, this registry replaces the registry part of all container images
	// used by the instance (main container, sidecars, init containers).
	// Example: "my-registry.example.com" will change "ghcr.io/openclaw/openclaw:latest"
	// to "my-registry.example.com/openclaw/openclaw:latest".
	// +optional
	Registry string `json:"registry,omitempty"`

	// Image configuration for the OpenClaw container
	// +optional
	Image ImageSpec `json:"image,omitempty"`

	// Config specifies the OpenClaw configuration
	// +optional
	Config ConfigSpec `json:"config,omitempty"`

	// Workspace configures initial workspace files seeded into the instance.
	// Files are copied once on first boot and never overwritten, so agent
	// modifications survive pod restarts.
	// +optional
	Workspace *WorkspaceSpec `json:"workspace,omitempty"`

	// Skills is a list of skills to install via init container.
	// Each entry is either a ClawHub skill identifier (e.g., "@anthropic/mcp-server-fetch")
	// or an npm package prefixed with "npm:" (e.g., "npm:@openclaw/matrix").
	// npm lifecycle scripts are disabled for security (see #91).
	// +kubebuilder:validation:MaxItems=20
	// +listType=set
	// +optional
	Skills []string `json:"skills,omitempty"`

	// SkillPackUpdatePolicy controls how workspace files seeded from "pack:"
	// skill entries are reconciled on pod start.
	//
	// "Replace" (default) converges seeded pack files to the declared pack
	// revision on every pod start: changed files are overwritten and files that
	// were seeded by a previous revision but are no longer part of any declared
	// pack are removed. The operator tracks the seeded file set in a manifest
	// at /data/.skillpack-manifest on the data volume.
	//
	// "CreateOnly" preserves the legacy behavior: pack files are only copied
	// when absent and never overwritten or removed, so updating a pinned pack
	// revision does not refresh already-seeded contents (see #564).
	//
	// Only files at paths declared by pack: entries are affected; files from
	// spec.workspace.initialFiles are always seeded create-only.
	// +kubebuilder:validation:Enum=Replace;CreateOnly
	// +kubebuilder:default=Replace
	// +optional
	SkillPackUpdatePolicy string `json:"skillPackUpdatePolicy,omitempty"`

	// Plugins is a list of plugins to install via init container.
	// Each entry is an npm package name (e.g., "@openclaw/matrix" or
	// "@martian-engineering/lossless-claw"). An optional "npm:" prefix is
	// accepted and stripped before installation.
	// Installation goes through the OpenClaw CLI's ClawHub installer
	// ("openclaw plugins install clawhub:<pkg>") rather than raw npm install
	// so packages published with workspace:* dependency markers resolve
	// correctly. npm lifecycle scripts are disabled for security.
	// +kubebuilder:validation:MaxItems=20
	// +optional
	Plugins []string `json:"plugins,omitempty"`

	// EnvFrom is a list of sources to populate environment variables from
	// Use this for API keys and other secrets (e.g., ANTHROPIC_API_KEY, OPENAI_API_KEY)
	// +optional
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

	// Env is a list of environment variables to set in the container
	// +listType=map
	// +listMapKey=name
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Resources specifies the compute resources for the OpenClaw container
	// +optional
	Resources ResourcesSpec `json:"resources,omitempty"`

	// Security specifies security-related configuration
	// +optional
	Security SecuritySpec `json:"security,omitempty"`

	// ShareProcessNamespace enables PID namespace sharing between all containers
	// in the pod. When true, the infrastructure (pause) container becomes PID 1
	// and reaps zombie processes, which prevents accumulation of defunct helper
	// processes (git, plugins, QMD memory, shells) under a Node.js gateway that
	// does not call waitpid(). Defaults to true.
	//
	// Security note: enabling this lets every container in the pod see and signal
	// every other container's processes. A compromised sidecar (Tailscale, Ollama,
	// browser, custom) could send signals to the gateway and vice versa. Set to
	// false to keep per-container PID isolation; you are then responsible for
	// reaping zombies (e.g. by baking tini or dumb-init into the image).
	// +kubebuilder:default=true
	// +optional
	ShareProcessNamespace *bool `json:"shareProcessNamespace,omitempty"`

	// Storage specifies persistent storage configuration
	// +optional
	Storage StorageSpec `json:"storage,omitempty"`

	// Chromium enables the Chromium sidecar for browser automation
	// +optional
	Chromium ChromiumSpec `json:"chromium,omitempty"`

	// Tailscale configures Tailscale integration for tailnet access and HTTPS
	// +optional
	Tailscale TailscaleSpec `json:"tailscale,omitempty"`

	// Ollama enables the Ollama sidecar for local LLM inference
	// +optional
	Ollama OllamaSpec `json:"ollama,omitempty"`

	// WebTerminal enables a browser-based terminal (ttyd) sidecar for debugging
	// +optional
	WebTerminal WebTerminalSpec `json:"webTerminal,omitempty"`

	// InitContainers is a list of additional init containers to run before the main container.
	// They run after the operator-managed init-config and init-skills containers.
	// +kubebuilder:validation:MaxItems=10
	// +optional
	InitContainers []corev1.Container `json:"initContainers,omitempty"`

	// Sidecars is a list of additional sidecar containers to inject into the pod.
	// Use this for custom sidecars like database proxies, log forwarders, or service meshes.
	// +optional
	Sidecars []corev1.Container `json:"sidecars,omitempty"`

	// SidecarVolumes is a list of additional volumes to make available to sidecar containers.
	// +optional
	SidecarVolumes []corev1.Volume `json:"sidecarVolumes,omitempty"`

	// ExtraVolumes adds additional volumes to the pod.
	// These volumes are available to the main container via ExtraVolumeMounts.
	// +kubebuilder:validation:MaxItems=10
	// +optional
	ExtraVolumes []corev1.Volume `json:"extraVolumes,omitempty"`

	// ExtraVolumeMounts adds additional volume mounts to the main container.
	// Use with ExtraVolumes to mount ConfigMaps, Secrets, NFS shares, or CSI volumes.
	// +kubebuilder:validation:MaxItems=10
	// +optional
	ExtraVolumeMounts []corev1.VolumeMount `json:"extraVolumeMounts,omitempty"`

	// Networking specifies network-related configuration
	// +optional
	Networking NetworkingSpec `json:"networking,omitempty"`

	// Probes configures health probes for the OpenClaw container
	// +optional
	// +nullable
	Probes *ProbesSpec `json:"probes,omitempty"`

	// Observability configures metrics and logging
	// +optional
	Observability ObservabilitySpec `json:"observability,omitempty"`

	// Availability configures high availability settings
	// +optional
	Availability AvailabilitySpec `json:"availability,omitempty"`

	// Suspended scales the workload to zero replicas when true.
	// Non-runtime resources (Service, ConfigMap, RBAC, NetworkPolicy, PVC)
	// remain fully managed. Set to false to resume normal operation.
	// +kubebuilder:default=false
	// +optional
	Suspended bool `json:"suspended,omitempty"`

	// Backup configures periodic scheduled backups to S3-compatible storage.
	// Requires the s3-backup-credentials Secret in the operator namespace and persistence enabled.
	// +optional
	Backup BackupSpec `json:"backup,omitempty"`

	// RestoreFrom is the remote backup path to restore data from (e.g. "backups/{tenantId}/{instanceId}/{timestamp}").
	// When set, the operator restores PVC data from this path before creating the StatefulSet.
	// Cleared automatically after successful restore.
	// +optional
	RestoreFrom string `json:"restoreFrom,omitempty"`

	// RuntimeDeps configures built-in init containers that install runtime
	// dependencies (pnpm, Python) for MCP servers and skills.
	// +optional
	RuntimeDeps RuntimeDepsSpec `json:"runtimeDeps,omitempty"`

	// Gateway configures the gateway reverse proxy and authentication token
	// +optional
	Gateway GatewaySpec `json:"gateway,omitempty"`

	// AutoUpdate configures automatic version updates from the OCI registry
	// +optional
	AutoUpdate AutoUpdateSpec `json:"autoUpdate,omitempty"`

	// SelfConfigure enables agents to modify their own instance via OpenClawSelfConfig resources.
	// When enabled, the operator injects RBAC, env vars, and a helper skill into the workspace.
	// +optional
	SelfConfigure SelfConfigureSpec `json:"selfConfigure,omitempty"`

	// PodAnnotations are extra annotations merged into the pod template metadata.
	// Operator-managed annotations (e.g. config-hash) take precedence on conflict.
	// +optional
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`
}

// ImageSpec defines the container image configuration
// +kubebuilder:validation:XValidation:rule="(has(self.tag) && size(self.tag) > 0) || (has(self.digest) && size(self.digest) > 0)",message="spec.image: one of tag or digest must be set (pinning to an empty tag is not supported, pick a specific upstream release tag or a digest)"
type ImageSpec struct {
	// Repository is the container image repository
	// +kubebuilder:default="ghcr.io/openclaw/openclaw"
	// +optional
	Repository string `json:"repository,omitempty"`

	// Tag is the container image tag
	// +kubebuilder:default="latest"
	// +optional
	Tag string `json:"tag,omitempty"`

	// Digest is the container image digest (overrides tag if specified)
	// +optional
	Digest string `json:"digest,omitempty"`

	// PullPolicy specifies when to pull the image
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	// +kubebuilder:default="IfNotPresent"
	// +optional
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`

	// PullSecrets is a list of secret names for pulling from private registries
	// +optional
	PullSecrets []corev1.LocalObjectReference `json:"pullSecrets,omitempty"`
}

// ConfigSpec defines the OpenClaw configuration
type ConfigSpec struct {
	// ConfigMapRef references a ConfigMap containing the openclaw.json configuration
	// +optional
	ConfigMapRef *ConfigMapKeySelector `json:"configMapRef,omitempty"`

	// Raw is inline openclaw.json configuration (used if ConfigMapRef is not set)
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Raw *RawConfig `json:"raw,omitempty"`

	// MergeMode controls how operator-managed config is applied to the PVC.
	// "overwrite" replaces the config file on every pod restart.
	// "merge" deep-merges operator config with existing PVC config, preserving runtime changes.
	// +kubebuilder:validation:Enum=overwrite;merge
	// +kubebuilder:default="overwrite"
	// +optional
	MergeMode string `json:"mergeMode,omitempty"`

	// Format specifies the config file format.
	// "json" (default) expects standard JSON. "json5" accepts JSON5 (comments, trailing commas).
	// JSON5 is converted to standard JSON by the init container using npx json5.
	// JSON5 requires configMapRef (inline raw config must be valid JSON).
	// +kubebuilder:validation:Enum=json;json5
	// +kubebuilder:default="json"
	// +optional
	Format string `json:"format,omitempty"`

	// ForcePaths is a list of dot-separated config paths (e.g. "gateway",
	// "models.providers", "agents.defaults.sandbox") that the init container
	// force-overwrites from the operator-managed config on every pod restart,
	// even under mergeMode=merge. For each listed path, the subtree is first
	// deleted from the existing PVC config and then re-applied via deep merge
	// from spec.config.raw, so the final value matches the CR exactly.
	//
	// Use case: a managed deployer (operator of a multi-tenant openclaw
	// service) wants tenants to persist user-owned config across restarts
	// (channels.telegram.botToken, settings, etc -- requires mergeMode=merge)
	// but must guarantee that operator-owned paths (gateway auth tokens,
	// allowed model providers, sandbox image) are always rebuilt from the CR.
	// Without forcePaths, a tenant could persist e.g.
	// models.providers.<rogue>.apiKey via the Control UI and route inference
	// through their own third-party key while consuming the deployer's
	// compute -- a verified attack path.
	//
	// Only applies when mergeMode=merge. Ignored (and rejected by the
	// validating webhook) under mergeMode=overwrite, which already overwrites
	// the entire config file on every restart.
	//
	// +kubebuilder:validation:MaxItems=20
	// +listType=set
	// +optional
	ForcePaths []string `json:"forcePaths,omitempty"`
}

// ConfigMapKeySelector selects a key from a ConfigMap
type ConfigMapKeySelector struct {
	// Name of the ConfigMap
	Name string `json:"name"`

	// Key in the ConfigMap to use
	// +kubebuilder:default="openclaw.json"
	// +optional
	Key string `json:"key,omitempty"`
}

// RawConfig holds arbitrary JSON configuration for openclaw.json
// +kubebuilder:pruning:PreserveUnknownFields
type RawConfig struct {
	runtime.RawExtension `json:",inline"`
}

// WorkspaceSpec configures initial workspace files for the instance.
// Files listed in InitialFiles are seeded once (only if they don't already
// exist on the PVC), so agent modifications survive pod restarts.
type WorkspaceSpec struct {
	// ConfigMapRef references an external ConfigMap whose keys become workspace files.
	// All keys in the referenced ConfigMap are included as workspace files.
	// This is useful for GitOps workflows where workspace files (AGENT.md, SOUL.md, etc.)
	// are managed as standalone files and bundled via Kustomize configMapGenerator or similar.
	//
	// Merge priority (highest wins):
	// 1. Operator-injected files (ENVIRONMENT.md, BOOTSTRAP.md, SELFCONFIG.md, selfconfig.sh)
	// 2. Inline initialFiles
	// 3. External configMapRef entries
	// 4. Skill pack files
	// +optional
	ConfigMapRef *ConfigMapNameSelector `json:"configMapRef,omitempty"`

	// InitialFiles maps filenames to their content. Each file is written
	// to the workspace directory only if it does not already exist.
	// +kubebuilder:validation:MaxProperties=50
	// +optional
	InitialFiles map[string]string `json:"initialFiles,omitempty"`

	// InitialDirectories is a list of directories to create (mkdir -p)
	// inside the workspace directory. Nested paths like "tools/scripts" are allowed.
	// +kubebuilder:validation:MaxItems=20
	// +optional
	InitialDirectories []string `json:"initialDirectories,omitempty"`

	// AdditionalWorkspaces configures workspace files for secondary agents.
	// Each entry seeds files to ~/.openclaw/workspace-<name>/, matching the
	// workspace path configured in spec.config.raw.agents.list[].workspace.
	// +kubebuilder:validation:MaxItems=10
	// +optional
	AdditionalWorkspaces []AdditionalWorkspace `json:"additionalWorkspaces,omitempty"`

	// Bootstrap controls the operator-managed BOOTSTRAP.md file injected into
	// the default workspace to guide first-run agent onboarding.
	// +optional
	Bootstrap BootstrapSpec `json:"bootstrap,omitempty"`
}

// BootstrapSpec controls the operator-managed BOOTSTRAP.md workspace file.
type BootstrapSpec struct {
	// Enabled controls whether the operator injects its BOOTSTRAP.md into the
	// default workspace. When true (the default), the init container seeds
	// BOOTSTRAP.md on pod start if the file is not present on the PVC.
	//
	// Set to false if the agent has already completed bootstrap and you don't
	// want the operator to recreate the file on pod restart or config change.
	// OpenClaw deletes BOOTSTRAP.md after applying it, so without this flag
	// every restart would cause the agent to re-run bootstrap. See #463.
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// AdditionalWorkspace defines a named workspace for a secondary agent.
// The operator seeds files to ~/.openclaw/workspace-<name>/.
type AdditionalWorkspace struct {
	// Name identifies this workspace. The operator seeds files to
	// ~/.openclaw/workspace-<name>/. Must match the workspace path
	// configured in spec.config.raw.agents.list[].workspace.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]+(-[a-z0-9]+)*$`
	Name string `json:"name"`

	// ConfigMapRef references an external ConfigMap whose keys become workspace files.
	// +optional
	ConfigMapRef *ConfigMapNameSelector `json:"configMapRef,omitempty"`

	// InitialFiles maps filenames to their content (same as spec.workspace.initialFiles).
	// +kubebuilder:validation:MaxProperties=50
	// +optional
	InitialFiles map[string]string `json:"initialFiles,omitempty"`

	// InitialDirectories is a list of directories to create inside this workspace.
	// +kubebuilder:validation:MaxItems=20
	// +optional
	InitialDirectories []string `json:"initialDirectories,omitempty"`

	// Skills is a list of skills to install scoped to this workspace, using the
	// same reference formats as the top-level spec.skills: a ClawHub skill
	// identifier (installed into workspace-<name>/skills/ via clawhub's
	// --workdir), an npm package prefixed with "npm:" (npm binaries are
	// installed globally and shared across workspaces), or a GitHub-hosted
	// skill pack prefixed with "pack:" (resolved files are seeded into
	// workspace-<name>/ and reconciled per spec.skillPackUpdatePolicy).
	// +kubebuilder:validation:MaxItems=20
	// +listType=set
	// +optional
	Skills []string `json:"skills,omitempty"`
}

// ConfigMapNameSelector references a ConfigMap by name.
// Unlike ConfigMapKeySelector, all keys in the ConfigMap are used.
type ConfigMapNameSelector struct {
	// Name is the name of the ConfigMap to reference.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// ResourcesSpec defines compute resource requirements
type ResourcesSpec struct {
	// Requests describes the minimum amount of compute resources required
	// +optional
	Requests ResourceList `json:"requests,omitempty"`

	// Limits describes the maximum amount of compute resources allowed
	// +optional
	Limits ResourceList `json:"limits,omitempty"`
}

// ResourceList defines CPU and memory resources
type ResourceList struct {
	// CPU resource (e.g., "500m", "2")
	// +optional
	CPU string `json:"cpu,omitempty"`

	// Memory resource (e.g., "512Mi", "2Gi")
	// +optional
	Memory string `json:"memory,omitempty"`
}

// SecuritySpec defines security-related configuration
type SecuritySpec struct {
	// PodSecurityContext holds pod-level security attributes
	// +optional
	PodSecurityContext *PodSecurityContextSpec `json:"podSecurityContext,omitempty"`

	// ContainerSecurityContext holds container-level security attributes
	// +optional
	ContainerSecurityContext *ContainerSecurityContextSpec `json:"containerSecurityContext,omitempty"`

	// NetworkPolicy configures network isolation
	// +optional
	NetworkPolicy NetworkPolicySpec `json:"networkPolicy,omitempty"`

	// RBAC configures role-based access control
	// +optional
	RBAC RBACSpec `json:"rbac,omitempty"`

	// CABundle injects a custom CA certificate bundle into all containers.
	// Use this in environments with TLS-intercepting proxies or private CAs.
	// +optional
	CABundle *CABundleSpec `json:"caBundle,omitempty"`
}

// CABundleSpec configures custom CA certificate injection.
type CABundleSpec struct {
	// ConfigMapName is the name of a ConfigMap containing the CA bundle.
	// The ConfigMap should have a key matching the Key field.
	// +optional
	ConfigMapName string `json:"configMapName,omitempty"`

	// SecretName is the name of a Secret containing the CA bundle.
	// The Secret should have a key matching the Key field.
	// Only one of ConfigMapName or SecretName should be set.
	// +optional
	SecretName string `json:"secretName,omitempty"`

	// Key is the key in the ConfigMap or Secret containing the CA bundle.
	// +kubebuilder:default="ca-bundle.crt"
	// +optional
	Key string `json:"key,omitempty"`
}

// PodSecurityContextSpec defines pod-level security context
type PodSecurityContextSpec struct {
	// RunAsUser is the UID to run the entrypoint of the container process
	// +kubebuilder:default=1000
	// +optional
	RunAsUser *int64 `json:"runAsUser,omitempty"`

	// RunAsGroup is the GID to run the entrypoint of the container process
	// +kubebuilder:default=1000
	// +optional
	RunAsGroup *int64 `json:"runAsGroup,omitempty"`

	// FSGroup is a special supplemental group that applies to all containers
	// +kubebuilder:default=1000
	// +optional
	FSGroup *int64 `json:"fsGroup,omitempty"`

	// FSGroupChangePolicy defines the behavior of changing ownership and permission of the volume.
	// "OnRootMismatch" skips recursive chown when ownership already matches, improving startup
	// time for large PVCs. "Always" recursively chowns on every mount (Kubernetes default).
	// +kubebuilder:validation:Enum=OnRootMismatch;Always
	// +optional
	FSGroupChangePolicy *corev1.PodFSGroupChangePolicy `json:"fsGroupChangePolicy,omitempty"`

	// RunAsNonRoot indicates that the container must run as a non-root user
	// +kubebuilder:default=true
	// +optional
	RunAsNonRoot *bool `json:"runAsNonRoot,omitempty"`
}

// ContainerSecurityContextSpec defines container-level security context
type ContainerSecurityContextSpec struct {
	// AllowPrivilegeEscalation controls whether a process can gain more privileges
	// +kubebuilder:default=false
	// +optional
	AllowPrivilegeEscalation *bool `json:"allowPrivilegeEscalation,omitempty"`

	// ReadOnlyRootFilesystem mounts the container's root filesystem as read-only
	// The PVC at ~/.openclaw/ provides writable home, and a /tmp emptyDir handles temp files
	// +kubebuilder:default=true
	// +optional
	ReadOnlyRootFilesystem *bool `json:"readOnlyRootFilesystem,omitempty"`

	// Capabilities to add/drop
	// +optional
	Capabilities *corev1.Capabilities `json:"capabilities,omitempty"`

	// RunAsNonRoot indicates that the container must run as a non-root user.
	// When not set, inherits from podSecurityContext.runAsNonRoot.
	// +optional
	RunAsNonRoot *bool `json:"runAsNonRoot,omitempty"`

	// RunAsUser is the UID to run the entrypoint of the container process.
	// When not set, inherits from podSecurityContext.runAsUser.
	// +optional
	RunAsUser *int64 `json:"runAsUser,omitempty"`
}

// NetworkPolicySpec configures network isolation for the OpenClaw instance
type NetworkPolicySpec struct {
	// Enabled enables network policy creation
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// AllowedIngressCIDRs is a list of CIDRs allowed to access this instance
	// +optional
	AllowedIngressCIDRs []string `json:"allowedIngressCIDRs,omitempty"`

	// AllowedIngressNamespaces is a list of namespace names allowed to access this instance
	// +optional
	AllowedIngressNamespaces []string `json:"allowedIngressNamespaces,omitempty"`

	// AllowedEgressCIDRs is a list of CIDRs this instance can reach
	// Default allows all egress on port 443 for AI APIs
	// +optional
	AllowedEgressCIDRs []string `json:"allowedEgressCIDRs,omitempty"`

	// AllowDNS allows DNS resolution (port 53)
	// +kubebuilder:default=true
	// +optional
	AllowDNS *bool `json:"allowDNS,omitempty"`

	// AdditionalEgress appends custom egress rules to the default DNS + HTTPS rules.
	// Use this to allow traffic to cluster-internal services on non-standard ports.
	// +optional
	AdditionalEgress []networkingv1.NetworkPolicyEgressRule `json:"additionalEgress,omitempty"`
}

// RBACSpec configures RBAC for the OpenClaw instance
type RBACSpec struct {
	// CreateServiceAccount creates a dedicated ServiceAccount for the instance
	// +kubebuilder:default=true
	// +optional
	CreateServiceAccount *bool `json:"createServiceAccount,omitempty"`

	// ServiceAccountName is the name of an existing ServiceAccount to use
	// Only used if CreateServiceAccount is false
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// ServiceAccountAnnotations are annotations to add to the managed ServiceAccount.
	// Use this for cloud provider integrations like AWS IRSA or GCP Workload Identity.
	// +optional
	ServiceAccountAnnotations map[string]string `json:"serviceAccountAnnotations,omitempty"`

	// AdditionalRules adds custom RBAC rules to the generated Role
	// +optional
	AdditionalRules []RBACRule `json:"additionalRules,omitempty"`
}

// RBACRule represents a RBAC rule
type RBACRule struct {
	// APIGroups is the name of the APIGroup that contains the resources
	APIGroups []string `json:"apiGroups"`
	// Resources is a list of resources this rule applies to
	Resources []string `json:"resources"`
	// Verbs is a list of verbs that apply to the resources
	Verbs []string `json:"verbs"`
}

// StorageSpec defines persistent storage configuration
type StorageSpec struct {
	// Persistence configures the PersistentVolumeClaim
	// +optional
	Persistence PersistenceSpec `json:"persistence,omitempty"`
}

// PersistenceSpec defines PVC configuration
type PersistenceSpec struct {
	// Enabled enables persistent storage
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// StorageClass is the name of the StorageClass to use
	// +optional
	StorageClass *string `json:"storageClass,omitempty"`

	// Size is the size of the PVC (e.g., "10Gi")
	// +kubebuilder:default="10Gi"
	// +optional
	Size string `json:"size,omitempty"`

	// AccessModes contains the desired access modes for the PVC
	// +kubebuilder:default={"ReadWriteOnce"}
	// +optional
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`

	// ExistingClaim is the name of an existing PVC to use
	// +optional
	ExistingClaim string `json:"existingClaim,omitempty"`

	// Orphan controls whether the PVC is retained when the OpenClawInstance is deleted.
	// When true (the default), the operator removes the owner reference from the PVC
	// before deleting the CR so Kubernetes does not garbage-collect it.
	// Set to false if you want the PVC deleted together with the CR.
	// +kubebuilder:default=true
	// +optional
	Orphan *bool `json:"orphan,omitempty"`
}

// BackupSpec configures periodic scheduled backups to S3-compatible storage.
type BackupSpec struct {
	// Schedule is a cron expression for periodic backups (e.g., "0 2 * * *" for daily at 2 AM).
	// When set, the operator creates a CronJob that runs rclone to sync PVC data to S3.
	// Requires persistence to be enabled and the s3-backup-credentials Secret
	// in the operator namespace.
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// HistoryLimit is the number of successful CronJob runs to retain.
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=0
	// +optional
	HistoryLimit *int32 `json:"historyLimit,omitempty"`

	// FailedHistoryLimit is the number of failed CronJob runs to retain.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +optional
	FailedHistoryLimit *int32 `json:"failedHistoryLimit,omitempty"`

	// Timeout is the maximum duration to wait for a pre-delete backup to complete
	// before giving up and proceeding with deletion (Go duration string, e.g. "30m", "1h").
	// Covers all phases: StatefulSet scale-down, pod termination, Job execution, and
	// Job failure retries. When the timeout elapses the operator logs a warning,
	// emits a BackupTimedOut event, and removes the finalizer so deletion can proceed.
	// Minimum: 5m, Maximum: 24h, Default: 30m.
	// +optional
	Timeout string `json:"timeout,omitempty"`

	// ServiceAccountName is the name of the ServiceAccount to use for backup and restore Jobs.
	// Use this to assign a cloud-provider workload identity ServiceAccount (e.g., AWS IRSA,
	// GKE Workload Identity, AKS Workload Identity) so backup Jobs can authenticate to the
	// storage backend without static credentials.
	// When set, all backup Jobs (pre-delete, pre-update, periodic, and restore) use this SA.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// RetentionDays is the number of days to keep daily snapshots in S3.
	// The periodic backup syncs incrementally to a fixed "latest" path and
	// takes a daily snapshot. Snapshots older than RetentionDays are pruned
	// after each successful backup.
	// +kubebuilder:default=7
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=365
	// +optional
	RetentionDays *int32 `json:"retentionDays,omitempty"`
}

// ChromiumSpec defines the Chromium sidecar configuration
type ChromiumSpec struct {
	// Enabled enables the Chromium sidecar for browser automation
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Image configures the Chromium container image
	// +optional
	Image ChromiumImageSpec `json:"image,omitempty"`

	// Resources specifies compute resources for the Chromium container
	// +optional
	Resources ResourcesSpec `json:"resources,omitempty"`

	// Persistence configures persistent storage for the Chromium browser profile.
	// When enabled, browser state (cookies, localStorage, session tokens) survives
	// pod restarts. When disabled (default), an emptyDir is used and all browser
	// state is lost on restart.
	// +optional
	Persistence ChromiumPersistenceSpec `json:"persistence,omitempty"`

	// ExtraArgs specifies additional command-line arguments passed to the
	// Chromium process. These are appended to the default arguments.
	// Example: ["--disable-blink-features=AutomationControlled", "--user-agent=Mozilla/5.0 ..."]
	// +optional
	ExtraArgs []string `json:"extraArgs,omitempty"`

	// ExtraEnv specifies additional environment variables for the Chromium
	// sidecar container, merged with the operator-managed variables.
	// +optional
	ExtraEnv []corev1.EnvVar `json:"extraEnv,omitempty"`
}

// ChromiumPersistenceSpec configures persistent storage for Chromium browser profiles
type ChromiumPersistenceSpec struct {
	// Enabled enables persistent storage for the Chromium browser profile.
	// When true, a PVC is created (or an existing one is used) and mounted at
	// /chromium-data. The --user-data-dir flag is set automatically so that
	// cookies, localStorage, session tokens, and cached credentials survive
	// pod restarts.
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// StorageClass is the name of the StorageClass to use for the PVC.
	// If empty, the cluster default StorageClass is used.
	// +optional
	StorageClass *string `json:"storageClass,omitempty"`

	// Size is the requested storage size for the Chromium profile PVC.
	// +kubebuilder:default="1Gi"
	// +optional
	Size string `json:"size,omitempty"`

	// ExistingClaim is the name of a pre-existing PVC to use instead of
	// creating a new one. When set, storageClass and size are ignored.
	// +optional
	ExistingClaim string `json:"existingClaim,omitempty"`
}

// ChromiumImageSpec defines the Chromium container image
type ChromiumImageSpec struct {
	// Repository is the container image repository
	// +kubebuilder:default="docker.io/chromedp/headless-shell"
	// +optional
	Repository string `json:"repository,omitempty"`

	// Tag is the container image tag
	// +kubebuilder:default="stable"
	// +optional
	Tag string `json:"tag,omitempty"`

	// Digest is the container image digest for supply chain security
	// +optional
	Digest string `json:"digest,omitempty"`
}

// TailscaleSpec configures Tailscale integration for secure tailnet access.
// When enabled, a Tailscale sidecar container runs tailscaled and handles
// serve/funnel via TS_SERVE_CONFIG. An init container copies the tailscale
// CLI binary to a shared volume so the main container can call
// "tailscale whois" for SSO authentication.
type TailscaleSpec struct {
	// Enabled enables Tailscale integration
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Mode selects the Tailscale mode.
	// "serve" exposes the instance to tailnet members only (default).
	// "funnel" exposes the instance to the public internet via Tailscale Funnel.
	// +kubebuilder:validation:Enum=serve;funnel
	// +kubebuilder:default="serve"
	// +optional
	Mode string `json:"mode,omitempty"`

	// Image configures the Tailscale sidecar container image.
	// The same image is used for the sidecar and the init container that
	// copies the tailscale CLI binary.
	// +optional
	Image TailscaleImageSpec `json:"image,omitempty"`

	// AuthKeySecretRef references a Secret containing the Tailscale auth key.
	// The Secret must have a key matching AuthKeySecretKey (default: "authkey").
	// Use ephemeral+reusable keys from the Tailscale admin console.
	// +optional
	AuthKeySecretRef *corev1.LocalObjectReference `json:"authKeySecretRef,omitempty"`

	// AuthKeySecretKey is the key in the referenced Secret.
	// +kubebuilder:default="authkey"
	// +optional
	AuthKeySecretKey string `json:"authKeySecretKey,omitempty"`

	// Hostname sets the Tailscale device name (defaults to instance name).
	// +optional
	Hostname string `json:"hostname,omitempty"`

	// AuthSSO enables passwordless login for tailnet members.
	// Sets gateway.auth.allowTailscale=true in the OpenClaw config.
	// +kubebuilder:default=false
	// +optional
	AuthSSO bool `json:"authSSO,omitempty"`

	// Resources specifies compute resources for the Tailscale sidecar container.
	// +optional
	Resources ResourcesSpec `json:"resources,omitempty"`
}

// TailscaleImageSpec defines the Tailscale sidecar container image
type TailscaleImageSpec struct {
	// Repository is the container image repository
	// +kubebuilder:default="ghcr.io/tailscale/tailscale"
	// +optional
	Repository string `json:"repository,omitempty"`

	// Tag is the container image tag
	// +kubebuilder:default="latest"
	// +optional
	Tag string `json:"tag,omitempty"`

	// Digest is the container image digest for supply chain security
	// +optional
	Digest string `json:"digest,omitempty"`
}

// OllamaSpec defines the Ollama sidecar configuration
type OllamaSpec struct {
	// Enabled enables the Ollama sidecar
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Image configures the Ollama container image
	// +optional
	Image OllamaImageSpec `json:"image,omitempty"`

	// Models is a list of models to pre-pull during pod init (e.g. ["llama3.2", "nomic-embed-text"])
	// +kubebuilder:validation:MaxItems=10
	// +optional
	Models []string `json:"models,omitempty"`

	// Resources specifies compute resources for the Ollama container
	// +optional
	Resources ResourcesSpec `json:"resources,omitempty"`

	// Storage configures the model cache volume
	// +optional
	Storage OllamaStorageSpec `json:"storage,omitempty"`

	// GPU is the number of NVIDIA GPUs to allocate (sets nvidia.com/gpu resource limit)
	// +kubebuilder:validation:Minimum=0
	// +optional
	GPU *int32 `json:"gpu,omitempty"`
}

// OllamaImageSpec defines the Ollama container image
type OllamaImageSpec struct {
	// Repository is the container image repository
	// +kubebuilder:default="docker.io/ollama/ollama"
	// +optional
	Repository string `json:"repository,omitempty"`

	// Tag is the container image tag
	// +kubebuilder:default="latest"
	// +optional
	Tag string `json:"tag,omitempty"`

	// Digest is the container image digest for supply chain security
	// +optional
	Digest string `json:"digest,omitempty"`
}

// OllamaStorageSpec configures the Ollama model cache volume
type OllamaStorageSpec struct {
	// SizeLimit is the size limit for the emptyDir model cache (default "20Gi")
	// +kubebuilder:default="20Gi"
	// +optional
	SizeLimit string `json:"sizeLimit,omitempty"`

	// ExistingClaim is the name of an existing PVC for persistent model storage
	// +optional
	ExistingClaim string `json:"existingClaim,omitempty"`
}

// WebTerminalSpec defines the ttyd web terminal sidecar configuration
type WebTerminalSpec struct {
	// Enabled enables the ttyd web terminal sidecar for browser-based shell access
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Image configures the ttyd container image
	// +optional
	Image WebTerminalImageSpec `json:"image,omitempty"`

	// Resources specifies compute resources for the ttyd container
	// +optional
	Resources ResourcesSpec `json:"resources,omitempty"`

	// ReadOnly starts ttyd in read-only mode (view-only, no input)
	// +kubebuilder:default=false
	// +optional
	ReadOnly bool `json:"readOnly,omitempty"`

	// Credential configures basic auth for the web terminal via a Secret.
	// The Secret must have "username" and "password" keys.
	// +optional
	Credential *WebTerminalCredentialSpec `json:"credential,omitempty"`
}

// WebTerminalImageSpec defines the ttyd container image
type WebTerminalImageSpec struct {
	// Repository is the container image repository
	// +kubebuilder:default="tsl0922/ttyd"
	// +optional
	Repository string `json:"repository,omitempty"`

	// Tag is the container image tag
	// +kubebuilder:default="latest"
	// +optional
	Tag string `json:"tag,omitempty"`

	// Digest is the container image digest for supply chain security
	// +optional
	Digest string `json:"digest,omitempty"`
}

// WebTerminalCredentialSpec configures basic auth for the web terminal
type WebTerminalCredentialSpec struct {
	// SecretRef references a Secret containing "username" and "password" keys
	SecretRef corev1.LocalObjectReference `json:"secretRef"`
}

// NetworkingSpec defines network-related configuration
type NetworkingSpec struct {
	// Service configures the Kubernetes Service
	// +optional
	Service ServiceSpec `json:"service,omitempty"`

	// Ingress configures the Kubernetes Ingress
	// +optional
	Ingress IngressSpec `json:"ingress,omitempty"`

	// HTTPRoute configures a Gateway API HTTPRoute.
	// This is an alternative to Ingress for clusters using the Gateway API
	// (gateway.networking.k8s.io). The Gateway API CRDs must be installed.
	// +optional
	HTTPRoute *HTTPRouteSpec `json:"httpRoute,omitempty"`
}

// HTTPRouteSpec defines a Gateway API HTTPRoute configuration
type HTTPRouteSpec struct {
	// Enabled enables HTTPRoute creation
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// ParentRefs specifies the Gateways this HTTPRoute attaches to.
	// Each ref identifies a Gateway by name (and optionally namespace and sectionName).
	// +optional
	ParentRefs []HTTPRouteParentRef `json:"parentRefs,omitempty"`

	// Hostnames specifies the hostnames matched by this HTTPRoute.
	// +optional
	Hostnames []string `json:"hostnames,omitempty"`

	// Port is the backend Service port number to route traffic to.
	// Defaults to the gateway port (18789) when not set.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port *int32 `json:"port,omitempty"`

	// Annotations to add to the HTTPRoute
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// HTTPRouteParentRef identifies a Gateway that the HTTPRoute attaches to
type HTTPRouteParentRef struct {
	// Name is the name of the Gateway
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace is the namespace of the Gateway.
	// Defaults to the same namespace as the HTTPRoute.
	// +optional
	Namespace *string `json:"namespace,omitempty"`

	// SectionName is the name of a specific listener on the Gateway to attach to.
	// +optional
	SectionName *string `json:"sectionName,omitempty"`
}

// ServiceSpec defines the Service configuration
type ServiceSpec struct {
	// Type is the Kubernetes Service type
	// +kubebuilder:validation:Enum=ClusterIP;LoadBalancer;NodePort
	// +kubebuilder:default="ClusterIP"
	// +optional
	Type corev1.ServiceType `json:"type,omitempty"`

	// Annotations to add to the Service
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Ports defines custom ports exposed on the Service.
	// When set, these replace the default gateway and canvas ports.
	// When empty, the operator creates default gateway (18789) and canvas (18793) ports.
	// +kubebuilder:validation:MaxItems=20
	// +optional
	Ports []ServicePortSpec `json:"ports,omitempty"`
}

// ServicePortSpec defines a port exposed by the Service
type ServicePortSpec struct {
	// Name is the name of the port
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Port is the port number exposed on the Service
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// TargetPort is the port on the container to route to (defaults to Port)
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	TargetPort *int32 `json:"targetPort,omitempty"`

	// Protocol is the protocol for the port
	// +kubebuilder:validation:Enum=TCP;UDP;SCTP
	// +kubebuilder:default="TCP"
	// +optional
	Protocol corev1.Protocol `json:"protocol,omitempty"`
}

// IngressSpec defines the Ingress configuration
type IngressSpec struct {
	// Enabled enables Ingress creation
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// ClassName is the name of the IngressClass to use
	// +optional
	ClassName *string `json:"className,omitempty"`

	// Annotations to add to the Ingress
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Hosts is a list of hosts to route traffic for
	// +optional
	Hosts []IngressHost `json:"hosts,omitempty"`

	// TLS configuration
	// +optional
	TLS []IngressTLS `json:"tls,omitempty"`

	// Security configures ingress security settings
	// +optional
	Security IngressSecuritySpec `json:"security,omitempty"`
}

// IngressHost defines a host for the Ingress
type IngressHost struct {
	// Host is the fully qualified domain name
	Host string `json:"host"`

	// Paths is a list of paths to route
	// +optional
	Paths []IngressPath `json:"paths,omitempty"`
}

// IngressPath defines a path for the Ingress
type IngressPath struct {
	// Path is the path to route
	// +kubebuilder:default="/"
	// +optional
	Path string `json:"path,omitempty"`

	// PathType determines how the path should be matched
	// +kubebuilder:validation:Enum=Prefix;Exact;ImplementationSpecific
	// +kubebuilder:default="Prefix"
	// +optional
	PathType string `json:"pathType,omitempty"`

	// Port is the backend service port number to route traffic to.
	// Defaults to the gateway port (18789) when not set.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port *int32 `json:"port,omitempty"`
}

// IngressTLS defines TLS configuration for the Ingress
type IngressTLS struct {
	// Hosts are a list of hosts included in the TLS certificate
	Hosts []string `json:"hosts,omitempty"`

	// SecretName is the name of the secret containing the TLS certificate
	SecretName string `json:"secretName,omitempty"`
}

// IngressSecuritySpec defines security settings for the Ingress
type IngressSecuritySpec struct {
	// ForceHTTPS redirects all HTTP traffic to HTTPS
	// +kubebuilder:default=true
	// +optional
	ForceHTTPS *bool `json:"forceHTTPS,omitempty"`

	// EnableHSTS enables HTTP Strict Transport Security
	// +kubebuilder:default=true
	// +optional
	EnableHSTS *bool `json:"enableHSTS,omitempty"`

	// RateLimiting configures rate limiting
	// +optional
	RateLimiting *RateLimitingSpec `json:"rateLimiting,omitempty"`

	// BasicAuth configures HTTP Basic Authentication for the Ingress.
	// Disabled by default. When enabled without an existingSecret, the operator
	// auto-generates a random password and stores it in a managed Secret.
	// +optional
	BasicAuth *IngressBasicAuthSpec `json:"basicAuth,omitempty"`
}

// IngressBasicAuthSpec configures HTTP Basic Authentication for the Ingress.
type IngressBasicAuthSpec struct {
	// Enabled enables basic authentication.
	// +kubebuilder:default=false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// ExistingSecret is the name of an existing Secret that already contains
	// htpasswd-formatted content in a key named "auth".
	// When set, the operator uses this Secret instead of generating one.
	// +optional
	ExistingSecret string `json:"existingSecret,omitempty"`

	// Username for the auto-generated htpasswd Secret.
	// Ignored when existingSecret is set.
	// +kubebuilder:default="openclaw"
	// +kubebuilder:validation:MaxLength=64
	// +optional
	Username string `json:"username,omitempty"`

	// Realm is the authentication realm shown in browser prompts.
	// +kubebuilder:default="OpenClaw"
	// +optional
	Realm string `json:"realm,omitempty"`
}

// RateLimitingSpec defines rate limiting configuration
type RateLimitingSpec struct {
	// Enabled enables rate limiting
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// RequestsPerSecond is the maximum requests per second
	// +kubebuilder:default=10
	// +optional
	RequestsPerSecond *int32 `json:"requestsPerSecond,omitempty"`
}

// ProbesSpec defines health probe configuration
type ProbesSpec struct {
	// Liveness probe configuration
	// +optional
	Liveness *ProbeSpec `json:"liveness,omitempty"`

	// Readiness probe configuration
	// +optional
	Readiness *ProbeSpec `json:"readiness,omitempty"`

	// Startup probe configuration
	// +optional
	Startup *ProbeSpec `json:"startup,omitempty"`

	// DiskReadiness configures an optional, opt-in defense-in-depth readiness
	// guard for PVC-backed workspaces. When enabled, the readiness probe is
	// rendered as an exec probe that also verifies the workspace volume is
	// writable and has free space above a threshold, so a full or read-only
	// PVC marks the pod NotReady (draining it from Service endpoints) instead
	// of silently accepting traffic it cannot persist. Liveness and startup
	// probes are unaffected, so a full PVC does not turn into a CrashLoopBackOff.
	// The application-level /readyz endpoint remains the primary readiness
	// signal; this guard is secondary. Defaults to disabled.
	// +optional
	DiskReadiness *DiskReadinessSpec `json:"diskReadiness,omitempty"`
}

// DiskReadinessSpec configures the optional disk-aware readiness guard for
// PVC-backed OpenClaw workspaces.
type DiskReadinessSpec struct {
	// Enabled turns on the disk-aware readiness guard. Defaults to false, so
	// existing deployments are unaffected unless they explicitly opt in.
	// +kubebuilder:default=false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Path is the workspace mount path checked for writability and free space.
	// Defaults to the OpenClaw workspace data mount (/home/openclaw/.openclaw).
	// +optional
	Path string `json:"path,omitempty"`

	// MinFree is the minimum free space the workspace volume must have for the
	// pod to be considered Ready, expressed as a Kubernetes quantity
	// (e.g. "100Mi", "1Gi"). Defaults to "64Mi".
	// +optional
	MinFree string `json:"minFree,omitempty"`
}

// ProbeSpec defines a health probe
type ProbeSpec struct {
	// Enabled enables the probe
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// InitialDelaySeconds is the number of seconds after the container starts before the probe is initiated
	// +optional
	InitialDelaySeconds *int32 `json:"initialDelaySeconds,omitempty"`

	// PeriodSeconds is how often (in seconds) to perform the probe
	// +optional
	PeriodSeconds *int32 `json:"periodSeconds,omitempty"`

	// TimeoutSeconds is the number of seconds after which the probe times out
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	// FailureThreshold is the number of times to retry before giving up
	// +optional
	FailureThreshold *int32 `json:"failureThreshold,omitempty"`
}

// ObservabilitySpec defines observability configuration
type ObservabilitySpec struct {
	// Metrics configures Prometheus metrics
	// +optional
	Metrics MetricsSpec `json:"metrics,omitempty"`

	// Logging configures logging
	// +optional
	Logging LoggingSpec `json:"logging,omitempty"`
}

// MetricsSpec defines metrics configuration
type MetricsSpec struct {
	// Enabled enables metrics endpoint
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Port is the port to expose metrics on
	// +kubebuilder:default=9090
	// +optional
	Port *int32 `json:"port,omitempty"`

	// ServiceMonitor configures the Prometheus ServiceMonitor
	// +optional
	ServiceMonitor *ServiceMonitorSpec `json:"serviceMonitor,omitempty"`

	// PrometheusRule configures auto-provisioned PrometheusRule alerts
	// +optional
	PrometheusRule *PrometheusRuleSpec `json:"prometheusRule,omitempty"`

	// GrafanaDashboard configures auto-provisioned Grafana dashboard ConfigMaps
	// +optional
	GrafanaDashboard *GrafanaDashboardSpec `json:"grafanaDashboard,omitempty"`
}

// ServiceMonitorSpec defines the ServiceMonitor configuration
type ServiceMonitorSpec struct {
	// Enabled enables ServiceMonitor creation
	// +kubebuilder:default=false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Interval is the scrape interval
	// +kubebuilder:default="30s"
	// +optional
	Interval string `json:"interval,omitempty"`

	// Labels to add to the ServiceMonitor
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// PrometheusRuleSpec configures auto-provisioned PrometheusRule alerts
type PrometheusRuleSpec struct {
	// Enabled enables PrometheusRule creation with operator alerts
	// +kubebuilder:default=false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Labels to add to the PrometheusRule (e.g., for Prometheus rule selector matching)
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// RunbookBaseURL is the base URL for alert runbook links
	// +kubebuilder:default="https://paperclip.inc/docs/operators/openclaw/runbooks"
	// +optional
	RunbookBaseURL string `json:"runbookBaseURL,omitempty"`
}

// GrafanaDashboardSpec configures auto-provisioned Grafana dashboard ConfigMaps
type GrafanaDashboardSpec struct {
	// Enabled enables Grafana dashboard ConfigMap creation
	// +kubebuilder:default=false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Labels to add to the dashboard ConfigMaps (in addition to grafana_dashboard: "1")
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Folder is the Grafana folder to place the dashboards in
	// +kubebuilder:default="OpenClaw"
	// +optional
	Folder string `json:"folder,omitempty"`
}

// LoggingSpec defines logging configuration
type LoggingSpec struct {
	// Level is the log level
	// +kubebuilder:validation:Enum=debug;info;warn;error
	// +kubebuilder:default="info"
	// +optional
	Level string `json:"level,omitempty"`

	// Format is the log format
	// +kubebuilder:validation:Enum=json;text
	// +kubebuilder:default="json"
	// +optional
	Format string `json:"format,omitempty"`
}

// AvailabilitySpec defines high availability settings
type AvailabilitySpec struct {
	// PodDisruptionBudget configures the PDB
	// +optional
	PodDisruptionBudget *PodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`

	// AutoScaling configures horizontal pod auto-scaling
	// +optional
	AutoScaling *AutoScalingSpec `json:"autoScaling,omitempty"`

	// NodeSelector is a selector which must match a node's labels for the pod to be scheduled
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations are tolerations for pod scheduling
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity specifies affinity scheduling rules
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// TopologySpreadConstraints describes how pods should spread across topology domains
	// +optional
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`

	// RuntimeClassName refers to a RuntimeClass object in the cluster,
	// which should be used to run this pod.
	// If no RuntimeClass resource matches the named class, the pod will not be run.
	// If unset or empty, the default container runtime is used.
	// More info: https://kubernetes.io/docs/concepts/containers/runtime-class/
	// +optional
	RuntimeClassName *string `json:"runtimeClassName,omitempty"`
}

// AutoScalingSpec configures horizontal pod auto-scaling via HPA
type AutoScalingSpec struct {
	// Enabled enables HorizontalPodAutoscaler creation
	// +kubebuilder:default=false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// MinReplicas is the lower limit for the number of replicas
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas is the upper limit for the number of replicas
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`

	// TargetCPUUtilization is the target average CPU utilization (percentage)
	// +kubebuilder:default=80
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	TargetCPUUtilization *int32 `json:"targetCPUUtilization,omitempty"`

	// TargetMemoryUtilization is the target average memory utilization (percentage).
	// When not set, only CPU-based scaling is used.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	TargetMemoryUtilization *int32 `json:"targetMemoryUtilization,omitempty"`
}

// PodDisruptionBudgetSpec defines PDB configuration
type PodDisruptionBudgetSpec struct {
	// Enabled enables PDB creation
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// MaxUnavailable is the maximum number of pods that can be unavailable during disruption
	// +kubebuilder:default=1
	// +optional
	MaxUnavailable *int32 `json:"maxUnavailable,omitempty"`
}

// AutoUpdateSpec configures automatic version updates from the OCI registry
type AutoUpdateSpec struct {
	// Enabled enables automatic version updates
	// +kubebuilder:default=false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// CheckInterval is how often to check for new versions (Go duration, e.g. "24h")
	// Minimum: 1h, Maximum: 168h (7 days)
	// +kubebuilder:default="24h"
	// +optional
	CheckInterval string `json:"checkInterval,omitempty"`

	// BackupBeforeUpdate creates a backup before applying updates
	// +kubebuilder:default=true
	// +optional
	BackupBeforeUpdate *bool `json:"backupBeforeUpdate,omitempty"`

	// RollbackOnFailure automatically reverts to the previous version if the
	// updated pod fails to become ready within HealthCheckTimeout
	// +kubebuilder:default=true
	// +optional
	RollbackOnFailure *bool `json:"rollbackOnFailure,omitempty"`

	// HealthCheckTimeout is how long to wait for the updated pod to become ready
	// before triggering a rollback (Go duration, e.g. "10m")
	// Minimum: 2m, Maximum: 30m
	// +kubebuilder:default="10m"
	// +optional
	HealthCheckTimeout string `json:"healthCheckTimeout,omitempty"`
}

// RuntimeDepsSpec configures built-in init containers that install runtime
// dependencies to the data PVC for use by MCP servers and skills.
type RuntimeDepsSpec struct {
	// Pnpm installs pnpm via corepack for npm-based MCP servers and skills.
	// +optional
	Pnpm bool `json:"pnpm,omitempty"`

	// Python installs Python 3.12 and uv for Python-based MCP servers and skills.
	// +optional
	Python bool `json:"python,omitempty"`
}

// GatewaySpec configures the gateway reverse proxy and authentication token
type GatewaySpec struct {
	// Enabled controls whether the built-in gateway reverse proxy sidecar is
	// injected into the pod. When false, no proxy container is added and health
	// probes target the OpenClaw gateway directly on port 18789.
	// Defaults to true.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`

	// ExistingSecret is the name of a user-managed Secret containing the gateway token.
	// The Secret must have a key named "token". When set, the operator skips
	// auto-generating a gateway token Secret and uses this Secret instead.
	// +optional
	ExistingSecret string `json:"existingSecret,omitempty"`

	// ControlUiOrigins is a list of additional allowed origins for the Control UI.
	// The operator always auto-injects localhost origins (http://localhost:18789,
	// http://127.0.0.1:18789) and derives origins from ingress hosts. Use this
	// field to add extra origins (e.g., custom reverse proxy URLs).
	// +kubebuilder:validation:MaxItems=20
	// +optional
	ControlUIOrigins []string `json:"controlUiOrigins,omitempty"`
}

// AutoUpdateStatus tracks the state of automatic version updates
type AutoUpdateStatus struct {
	// LastCheckTime is when the registry was last checked for new versions
	// +optional
	LastCheckTime *metav1.Time `json:"lastCheckTime,omitempty"`

	// LatestVersion is the latest version available in the registry
	// +optional
	LatestVersion string `json:"latestVersion,omitempty"`

	// CurrentVersion is the version currently running
	// +optional
	CurrentVersion string `json:"currentVersion,omitempty"`

	// PendingVersion is set during an in-flight update
	// +optional
	PendingVersion string `json:"pendingVersion,omitempty"`

	// UpdatePhase tracks progress of an in-flight update
	// +kubebuilder:validation:Enum="";BackingUp;ApplyingUpdate;HealthCheck;RollingBack
	// +optional
	UpdatePhase string `json:"updatePhase,omitempty"`

	// LastUpdateTime is when the last successful update was applied
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`

	// LastUpdateError records the error from the last failed update attempt
	// +optional
	LastUpdateError string `json:"lastUpdateError,omitempty"`

	// PreviousVersion is the version before the last update (used for rollback)
	// +optional
	PreviousVersion string `json:"previousVersion,omitempty"`

	// PreUpdateBackupPath is the S3 path of the pre-update backup (used for rollback restore)
	// +optional
	PreUpdateBackupPath string `json:"preUpdateBackupPath,omitempty"`

	// FailedVersion is a version that failed health checks and will be skipped in future checks
	// Cleared when a newer version becomes available
	// +optional
	FailedVersion string `json:"failedVersion,omitempty"`

	// RollbackCount tracks consecutive rollbacks; auto-update pauses after 3
	// Reset to 0 on any successful update
	// +optional
	RollbackCount int32 `json:"rollbackCount,omitempty"`
}

// OpenClawInstanceStatus defines the observed state of OpenClawInstance
type OpenClawInstanceStatus struct {
	// Phase represents the current lifecycle phase of the instance
	// +kubebuilder:validation:Enum=Pending;Provisioning;Running;Degraded;Failed;Terminating;BackingUp;Restoring;Updating;Suspended
	// +optional
	Phase string `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the instance's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// GatewayEndpoint is the endpoint for the OpenClaw gateway
	// +optional
	GatewayEndpoint string `json:"gatewayEndpoint,omitempty"`

	// CanvasEndpoint is the endpoint for the OpenClaw canvas
	// +optional
	CanvasEndpoint string `json:"canvasEndpoint,omitempty"`

	// LastReconcileTime is the timestamp of the last reconciliation
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// ManagedResources tracks the resources created by the operator
	// +optional
	ManagedResources ManagedResourcesStatus `json:"managedResources,omitempty"`

	// BackingUpSince is the timestamp when the instance entered the BackingUp phase.
	// Used to enforce spec.backup.timeout. Set once when the phase transitions to BackingUp
	// and cleared when the phase changes.
	// +optional
	BackingUpSince *metav1.Time `json:"backingUpSince,omitempty"`

	// BackupJobName is the name of the active backup Job
	// +optional
	BackupJobName string `json:"backupJobName,omitempty"`

	// RestoreJobName is the name of the active restore Job
	// +optional
	RestoreJobName string `json:"restoreJobName,omitempty"`

	// LastBackupPath is the S3 path of the last successful backup
	// +optional
	LastBackupPath string `json:"lastBackupPath,omitempty"`

	// LastBackupTime is the timestamp of the last successful backup
	// +optional
	LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`

	// RestoredFrom is the S3 path this instance was restored from
	// +optional
	RestoredFrom string `json:"restoredFrom,omitempty"`

	// AutoUpdate tracks the state of automatic version updates
	// +optional
	AutoUpdate AutoUpdateStatus `json:"autoUpdate,omitempty"`
}

// ManagedResourcesStatus tracks resources created by the operator
type ManagedResourcesStatus struct {
	// StatefulSet is the name of the managed StatefulSet
	// +optional
	StatefulSet string `json:"statefulSet,omitempty"`

	// Deployment is the name of the legacy Deployment (deprecated, used during migration)
	// +optional
	Deployment string `json:"deployment,omitempty"`

	// Service is the name of the managed Service
	// +optional
	Service string `json:"service,omitempty"`

	// ConfigMap is the name of the managed ConfigMap
	// +optional
	ConfigMap string `json:"configMap,omitempty"`

	// PVC is the name of the managed PersistentVolumeClaim
	// +optional
	PVC string `json:"pvc,omitempty"`

	// ChromiumPVC is the name of the managed Chromium browser profile PVC
	// +optional
	ChromiumPVC string `json:"chromiumPVC,omitempty"`

	// NetworkPolicy is the name of the managed NetworkPolicy
	// +optional
	NetworkPolicy string `json:"networkPolicy,omitempty"`

	// PodDisruptionBudget is the name of the managed PDB
	// +optional
	PodDisruptionBudget string `json:"podDisruptionBudget,omitempty"`

	// ServiceAccount is the name of the managed ServiceAccount
	// +optional
	ServiceAccount string `json:"serviceAccount,omitempty"`

	// Role is the name of the managed Role
	// +optional
	Role string `json:"role,omitempty"`

	// RoleBinding is the name of the managed RoleBinding
	// +optional
	RoleBinding string `json:"roleBinding,omitempty"`

	// GatewayTokenSecret is the name of the auto-generated gateway token Secret
	// +optional
	GatewayTokenSecret string `json:"gatewayTokenSecret,omitempty"`

	// PrometheusRule is the name of the managed PrometheusRule
	// +optional
	PrometheusRule string `json:"prometheusRule,omitempty"`

	// GrafanaDashboardOperator is the name of the operator overview dashboard ConfigMap
	// +optional
	GrafanaDashboardOperator string `json:"grafanaDashboardOperator,omitempty"`

	// GrafanaDashboardInstance is the name of the instance detail dashboard ConfigMap
	// +optional
	GrafanaDashboardInstance string `json:"grafanaDashboardInstance,omitempty"`

	// HorizontalPodAutoscaler is the name of the managed HPA
	// +optional
	HorizontalPodAutoscaler string `json:"horizontalPodAutoscaler,omitempty"`

	// BasicAuthSecret is the name of the auto-generated Ingress Basic Auth htpasswd Secret
	// +optional
	BasicAuthSecret string `json:"basicAuthSecret,omitempty"`

	// HTTPRoute is the name of the managed Gateway API HTTPRoute
	// +optional
	HTTPRoute string `json:"httpRoute,omitempty"`

	// BackupCronJob is the name of the managed periodic backup CronJob
	// +optional
	BackupCronJob string `json:"backupCronJob,omitempty"`

	// TailscaleStateSecret is the name of the Secret used to persist Tailscale
	// node identity and TLS certificate state across pod restarts
	// +optional
	TailscaleStateSecret string `json:"tailscaleStateSecret,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=='Ready')].status`
// +kubebuilder:printcolumn:name="Gateway",type=string,JSONPath=`.status.gatewayEndpoint`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// OpenClawInstance is the Schema for the openclawinstances API
type OpenClawInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenClawInstanceSpec   `json:"spec,omitempty"`
	Status OpenClawInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenClawInstanceList contains a list of OpenClawInstance
type OpenClawInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenClawInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenClawInstance{}, &OpenClawInstanceList{})
}

// Condition types for OpenClawInstance
const (
	// ConditionTypeReady indicates the overall readiness of the instance
	ConditionTypeReady = "Ready"

	// ConditionTypeConfigValid indicates the configuration is valid
	ConditionTypeConfigValid = "ConfigValid"

	// ConditionTypeStatefulSetReady indicates the StatefulSet is ready
	ConditionTypeStatefulSetReady = "StatefulSetReady"

	// ConditionTypeDeploymentReady indicates the Deployment is ready (deprecated)
	ConditionTypeDeploymentReady = "DeploymentReady"

	// ConditionTypeServiceReady indicates the Service is ready
	ConditionTypeServiceReady = "ServiceReady"

	// ConditionTypeNetworkPolicyReady indicates the NetworkPolicy is ready
	ConditionTypeNetworkPolicyReady = "NetworkPolicyReady"

	// ConditionTypeRBACReady indicates RBAC resources are ready
	ConditionTypeRBACReady = "RBACReady"

	// ConditionTypeStorageReady indicates the PVC is bound
	ConditionTypeStorageReady = "StorageReady"

	// ConditionTypeBackupComplete indicates the backup completed successfully
	ConditionTypeBackupComplete = "BackupComplete"

	// ConditionTypeRestoreComplete indicates the restore completed successfully
	ConditionTypeRestoreComplete = "RestoreComplete"

	// ConditionTypeAutoUpdateAvailable indicates a newer version is available
	ConditionTypeAutoUpdateAvailable = "AutoUpdateAvailable"

	// ConditionTypeScheduledBackupReady indicates the periodic backup CronJob is configured
	ConditionTypeScheduledBackupReady = "ScheduledBackupReady"

	// ConditionTypeSecretsReady indicates all referenced secrets exist
	ConditionTypeSecretsReady = "SecretsReady"

	// ConditionTypeSkillPacksReady indicates skill packs were resolved successfully
	ConditionTypeSkillPacksReady = "SkillPacksReady"

	// ConditionTypeWorkspaceReady indicates the workspace configuration is valid
	// and any external ConfigMap referenced by spec.workspace.configMapRef exists
	ConditionTypeWorkspaceReady = "WorkspaceReady"

	// ConditionTypeHTTPRouteReady indicates the Gateway API HTTPRoute is reconciled
	ConditionTypeHTTPRouteReady = "HTTPRouteReady"
)

// Phase constants
const (
	PhasePending      = "Pending"
	PhaseProvisioning = "Provisioning"
	PhaseRunning      = "Running"
	PhaseDegraded     = "Degraded"
	PhaseFailed       = "Failed"
	PhaseTerminating  = "Terminating"
	PhaseBackingUp    = "BackingUp"
	PhaseRestoring    = "Restoring"
	PhaseUpdating     = "Updating"
	PhaseSuspended    = "Suspended"
)
