# API Reference

## Packages
- [openclaw.rocks/v1alpha1](#openclawrocksv1alpha1)


## openclaw.rocks/v1alpha1

Package v1alpha1 contains API Schema definitions for the openclaw v1alpha1 API group

### Resource Types
- [OpenClawClusterDefaults](#openclawclusterdefaults)
- [OpenClawInstance](#openclawinstance)
- [OpenClawSelfConfig](#openclawselfconfig)



#### AdditionalWorkspace



AdditionalWorkspace defines a named workspace for a secondary agent.
The operator seeds files to ~/.openclaw/workspace-<name>/.



_Appears in:_
- [WorkspaceSpec](#workspacespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name identifies this workspace. The operator seeds files to<br />~/.openclaw/workspace-<name>/. Must match the workspace path<br />configured in spec.config.raw.agents.list[].workspace. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]+(-[a-z0-9]+)*$` <br /> |
| `configMapRef` _[ConfigMapNameSelector](#configmapnameselector)_ | ConfigMapRef references an external ConfigMap whose keys become workspace files. |  | Optional: \{\} <br /> |
| `initialFiles` _object (keys:string, values:string)_ | InitialFiles maps filenames to their content (same as spec.workspace.initialFiles). |  | MaxProperties: 50 <br />Optional: \{\} <br /> |
| `initialDirectories` _string array_ | InitialDirectories is a list of directories to create inside this workspace. |  | MaxItems: 20 <br />Optional: \{\} <br /> |
| `skills` _string array_ | Skills is a list of skills to install scoped to this workspace, using the<br />same reference formats as the top-level spec.skills: a ClawHub skill<br />identifier (installed into workspace-<name>/skills/ via clawhub's<br />--workdir), an npm package prefixed with "npm:" (npm binaries are<br />installed globally and shared across workspaces), or a GitHub-hosted<br />skill pack prefixed with "pack:" (resolved files are seeded into<br />workspace-<name>/ and reconciled per spec.skillPackUpdatePolicy). |  | MaxItems: 20 <br />Optional: \{\} <br /> |


#### AutoScalingSpec



AutoScalingSpec configures horizontal pod auto-scaling via HPA



_Appears in:_
- [AvailabilitySpec](#availabilityspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables HorizontalPodAutoscaler creation | false | Optional: \{\} <br /> |
| `minReplicas` _integer_ | MinReplicas is the lower limit for the number of replicas | 1 | Minimum: 1 <br />Optional: \{\} <br /> |
| `maxReplicas` _integer_ | MaxReplicas is the upper limit for the number of replicas | 5 | Minimum: 1 <br />Optional: \{\} <br /> |
| `targetCPUUtilization` _integer_ | TargetCPUUtilization is the target average CPU utilization (percentage) | 80 | Maximum: 100 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `targetMemoryUtilization` _integer_ | TargetMemoryUtilization is the target average memory utilization (percentage).<br />When not set, only CPU-based scaling is used. |  | Maximum: 100 <br />Minimum: 1 <br />Optional: \{\} <br /> |


#### AutoUpdateSpec



AutoUpdateSpec configures automatic version updates from the OCI registry



_Appears in:_
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables automatic version updates | false | Optional: \{\} <br /> |
| `checkInterval` _string_ | CheckInterval is how often to check for new versions (Go duration, e.g. "24h")<br />Minimum: 1h, Maximum: 168h (7 days) | 24h | Optional: \{\} <br /> |
| `backupBeforeUpdate` _boolean_ | BackupBeforeUpdate creates a backup before applying updates | true | Optional: \{\} <br /> |
| `rollbackOnFailure` _boolean_ | RollbackOnFailure automatically reverts to the previous version if the<br />updated pod fails to become ready within HealthCheckTimeout | true | Optional: \{\} <br /> |
| `healthCheckTimeout` _string_ | HealthCheckTimeout is how long to wait for the updated pod to become ready<br />before triggering a rollback (Go duration, e.g. "10m")<br />Minimum: 2m, Maximum: 30m | 10m | Optional: \{\} <br /> |


#### AvailabilitySpec



AvailabilitySpec defines high availability settings



_Appears in:_
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `podDisruptionBudget` _[PodDisruptionBudgetSpec](#poddisruptionbudgetspec)_ | PodDisruptionBudget configures the PDB |  | Optional: \{\} <br /> |
| `autoScaling` _[AutoScalingSpec](#autoscalingspec)_ | AutoScaling configures horizontal pod auto-scaling |  | Optional: \{\} <br /> |
| `nodeSelector` _object (keys:string, values:string)_ | NodeSelector is a selector which must match a node's labels for the pod to be scheduled |  | Optional: \{\} <br /> |
| `tolerations` _[Toleration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#toleration-v1-core) array_ | Tolerations are tolerations for pod scheduling |  | Optional: \{\} <br /> |
| `affinity` _[Affinity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#affinity-v1-core)_ | Affinity specifies affinity scheduling rules |  | Optional: \{\} <br /> |
| `topologySpreadConstraints` _[TopologySpreadConstraint](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#topologyspreadconstraint-v1-core) array_ | TopologySpreadConstraints describes how pods should spread across topology domains |  | Optional: \{\} <br /> |
| `runtimeClassName` _string_ | RuntimeClassName refers to a RuntimeClass object in the cluster,<br />which should be used to run this pod.<br />If no RuntimeClass resource matches the named class, the pod will not be run.<br />If unset or empty, the default container runtime is used.<br />More info: https://kubernetes.io/docs/concepts/containers/runtime-class/ |  | Optional: \{\} <br /> |


#### BackupSpec



BackupSpec configures periodic scheduled backups to S3-compatible storage.



_Appears in:_
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `schedule` _string_ | Schedule is a cron expression for periodic backups (e.g., "0 2 * * *" for daily at 2 AM).<br />When set, the operator creates a CronJob that runs rclone to sync PVC data to S3.<br />Requires persistence to be enabled and the s3-backup-credentials Secret<br />in the operator namespace. |  | Optional: \{\} <br /> |
| `historyLimit` _integer_ | HistoryLimit is the number of successful CronJob runs to retain. | 3 | Minimum: 0 <br />Optional: \{\} <br /> |
| `failedHistoryLimit` _integer_ | FailedHistoryLimit is the number of failed CronJob runs to retain. | 1 | Minimum: 0 <br />Optional: \{\} <br /> |
| `timeout` _string_ | Timeout is the maximum duration to wait for a pre-delete backup to complete<br />before giving up and proceeding with deletion (Go duration string, e.g. "30m", "1h").<br />Covers all phases: StatefulSet scale-down, pod termination, Job execution, and<br />Job failure retries. When the timeout elapses the operator logs a warning,<br />emits a BackupTimedOut event, and removes the finalizer so deletion can proceed.<br />Minimum: 5m, Maximum: 24h, Default: 30m. |  | Optional: \{\} <br /> |
| `serviceAccountName` _string_ | ServiceAccountName is the name of the ServiceAccount to use for backup and restore Jobs.<br />Use this to assign a cloud-provider workload identity ServiceAccount (e.g., AWS IRSA,<br />GKE Workload Identity, AKS Workload Identity) so backup Jobs can authenticate to the<br />storage backend without static credentials.<br />When set, all backup Jobs (pre-delete, pre-update, periodic, and restore) use this SA. |  | Optional: \{\} <br /> |
| `retentionDays` _integer_ | RetentionDays is the number of days to keep daily snapshots in S3.<br />The periodic backup syncs incrementally to a fixed "latest" path and<br />takes a daily snapshot. Snapshots older than RetentionDays are pruned<br />after each successful backup. | 7 | Maximum: 365 <br />Minimum: 1 <br />Optional: \{\} <br /> |


#### BootstrapSpec



BootstrapSpec controls the operator-managed BOOTSTRAP.md workspace file.



_Appears in:_
- [WorkspaceSpec](#workspacespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled controls whether the operator injects its BOOTSTRAP.md into the<br />default workspace. When true (the default), the init container seeds<br />BOOTSTRAP.md on pod start if the file is not present on the PVC.<br />Set to false if the agent has already completed bootstrap and you don't<br />want the operator to recreate the file on pod restart or config change.<br />OpenClaw deletes BOOTSTRAP.md after applying it, so without this flag<br />every restart would cause the agent to re-run bootstrap. See #463. | true | Optional: \{\} <br /> |


#### CABundleSpec



CABundleSpec configures custom CA certificate injection.



_Appears in:_
- [SecuritySpec](#securityspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `configMapName` _string_ | ConfigMapName is the name of a ConfigMap containing the CA bundle.<br />The ConfigMap should have a key matching the Key field. |  | Optional: \{\} <br /> |
| `secretName` _string_ | SecretName is the name of a Secret containing the CA bundle.<br />The Secret should have a key matching the Key field.<br />Only one of ConfigMapName or SecretName should be set. |  | Optional: \{\} <br /> |
| `key` _string_ | Key is the key in the ConfigMap or Secret containing the CA bundle. | ca-bundle.crt | Optional: \{\} <br /> |


#### ChromiumImageSpec



ChromiumImageSpec defines the Chromium container image



_Appears in:_
- [ChromiumSpec](#chromiumspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `repository` _string_ | Repository is the container image repository | chromedp/headless-shell | Optional: \{\} <br /> |
| `tag` _string_ | Tag is the container image tag | stable | Optional: \{\} <br /> |
| `digest` _string_ | Digest is the container image digest for supply chain security |  | Optional: \{\} <br /> |


#### ChromiumPersistenceSpec



ChromiumPersistenceSpec configures persistent storage for Chromium browser profiles



_Appears in:_
- [ChromiumSpec](#chromiumspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables persistent storage for the Chromium browser profile.<br />When true, a PVC is created (or an existing one is used) and mounted at<br />/chromium-data. The --user-data-dir flag is set automatically so that<br />cookies, localStorage, session tokens, and cached credentials survive<br />pod restarts. | false | Optional: \{\} <br /> |
| `storageClass` _string_ | StorageClass is the name of the StorageClass to use for the PVC.<br />If empty, the cluster default StorageClass is used. |  | Optional: \{\} <br /> |
| `size` _string_ | Size is the requested storage size for the Chromium profile PVC. | 1Gi | Optional: \{\} <br /> |
| `existingClaim` _string_ | ExistingClaim is the name of a pre-existing PVC to use instead of<br />creating a new one. When set, storageClass and size are ignored. |  | Optional: \{\} <br /> |


#### ChromiumSpec



ChromiumSpec defines the Chromium sidecar configuration



_Appears in:_
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables the Chromium sidecar for browser automation | false | Optional: \{\} <br /> |
| `image` _[ChromiumImageSpec](#chromiumimagespec)_ | Image configures the Chromium container image |  | Optional: \{\} <br /> |
| `resources` _[ResourcesSpec](#resourcesspec)_ | Resources specifies compute resources for the Chromium container |  | Optional: \{\} <br /> |
| `persistence` _[ChromiumPersistenceSpec](#chromiumpersistencespec)_ | Persistence configures persistent storage for the Chromium browser profile.<br />When enabled, browser state (cookies, localStorage, session tokens) survives<br />pod restarts. When disabled (default), an emptyDir is used and all browser<br />state is lost on restart. |  | Optional: \{\} <br /> |
| `extraArgs` _string array_ | ExtraArgs specifies additional command-line arguments passed to the<br />Chromium process. These are appended to the default arguments.<br />Example: ["--disable-blink-features=AutomationControlled", "--user-agent=Mozilla/5.0 ..."] |  | Optional: \{\} <br /> |
| `extraEnv` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#envvar-v1-core) array_ | ExtraEnv specifies additional environment variables for the Chromium<br />sidecar container, merged with the operator-managed variables. |  | Optional: \{\} <br /> |


#### ConfigMapKeySelector



ConfigMapKeySelector selects a key from a ConfigMap



_Appears in:_
- [ConfigSpec](#configspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the ConfigMap |  |  |
| `key` _string_ | Key in the ConfigMap to use | openclaw.json | Optional: \{\} <br /> |


#### ConfigMapNameSelector



ConfigMapNameSelector references a ConfigMap by name.
Unlike ConfigMapKeySelector, all keys in the ConfigMap are used.



_Appears in:_
- [AdditionalWorkspace](#additionalworkspace)
- [WorkspaceSpec](#workspacespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the ConfigMap to reference. |  | MinLength: 1 <br /> |


#### ConfigSpec



ConfigSpec defines the OpenClaw configuration



_Appears in:_
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `configMapRef` _[ConfigMapKeySelector](#configmapkeyselector)_ | ConfigMapRef references a ConfigMap containing the openclaw.json configuration |  | Optional: \{\} <br /> |
| `raw` _[RawConfig](#rawconfig)_ | Raw is inline openclaw.json configuration (used if ConfigMapRef is not set) |  | Optional: \{\} <br /> |
| `mergeMode` _string_ | MergeMode controls how operator-managed config is applied to the PVC.<br />"overwrite" replaces the config file on every pod restart.<br />"merge" deep-merges operator config with existing PVC config, preserving runtime changes. | overwrite | Enum: [overwrite merge] <br />Optional: \{\} <br /> |
| `format` _string_ | Format specifies the config file format.<br />"json" (default) expects standard JSON. "json5" accepts JSON5 (comments, trailing commas).<br />JSON5 is converted to standard JSON by the init container using npx json5.<br />JSON5 requires configMapRef (inline raw config must be valid JSON). | json | Enum: [json json5] <br />Optional: \{\} <br /> |
| `forcePaths` _string array_ | ForcePaths is a list of dot-separated config paths (e.g. "gateway",<br />"models.providers", "agents.defaults.sandbox") that the init container<br />force-overwrites from the operator-managed config on every pod restart,<br />even under mergeMode=merge. For each listed path, the subtree is first<br />deleted from the existing PVC config and then re-applied via deep merge<br />from spec.config.raw, so the final value matches the CR exactly.<br />Use case: a managed deployer (operator of a multi-tenant openclaw<br />service) wants tenants to persist user-owned config across restarts<br />(channels.telegram.botToken, settings, etc -- requires mergeMode=merge)<br />but must guarantee that operator-owned paths (gateway auth tokens,<br />allowed model providers, sandbox image) are always rebuilt from the CR.<br />Without forcePaths, a tenant could persist e.g.<br />models.providers.<rogue>.apiKey via the Control UI and route inference<br />through their own third-party key while consuming the deployer's<br />compute -- a verified attack path.<br />Only applies when mergeMode=merge. Ignored (and rejected by the<br />validating webhook) under mergeMode=overwrite, which already overwrites<br />the entire config file on every restart. |  | MaxItems: 20 <br />Optional: \{\} <br /> |


#### ContainerSecurityContextSpec



ContainerSecurityContextSpec defines container-level security context



_Appears in:_
- [SecuritySpec](#securityspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `allowPrivilegeEscalation` _boolean_ | AllowPrivilegeEscalation controls whether a process can gain more privileges | false | Optional: \{\} <br /> |
| `readOnlyRootFilesystem` _boolean_ | ReadOnlyRootFilesystem mounts the container's root filesystem as read-only<br />The PVC at ~/.openclaw/ provides writable home, and a /tmp emptyDir handles temp files | true | Optional: \{\} <br /> |
| `capabilities` _[Capabilities](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#capabilities-v1-core)_ | Capabilities to add/drop |  | Optional: \{\} <br /> |
| `runAsNonRoot` _boolean_ | RunAsNonRoot indicates that the container must run as a non-root user.<br />When not set, inherits from podSecurityContext.runAsNonRoot. |  | Optional: \{\} <br /> |
| `runAsUser` _integer_ | RunAsUser is the UID to run the entrypoint of the container process.<br />When not set, inherits from podSecurityContext.runAsUser. |  | Optional: \{\} <br /> |


#### DiskReadinessSpec



DiskReadinessSpec configures the optional disk-aware readiness guard for
PVC-backed OpenClaw workspaces.



_Appears in:_
- [ProbesSpec](#probesspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled turns on the disk-aware readiness guard. Defaults to false, so<br />existing deployments are unaffected unless they explicitly opt in. | false | Optional: \{\} <br /> |
| `path` _string_ | Path is the workspace mount path checked for writability and free space.<br />Defaults to the OpenClaw workspace data mount (/home/openclaw/.openclaw). |  | Optional: \{\} <br /> |
| `minFree` _string_ | MinFree is the minimum free space the workspace volume must have for the<br />pod to be considered Ready, expressed as a Kubernetes quantity<br />(e.g. "100Mi", "1Gi"). Defaults to "64Mi". |  | Optional: \{\} <br /> |


#### GatewaySpec



GatewaySpec configures the gateway reverse proxy and authentication token



_Appears in:_
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled controls whether the built-in gateway reverse proxy sidecar is<br />injected into the pod. When false, no proxy container is added and health<br />probes target the OpenClaw gateway directly on port 18789.<br />Defaults to true. | true | Optional: \{\} <br /> |
| `existingSecret` _string_ | ExistingSecret is the name of a user-managed Secret containing the gateway token.<br />The Secret must have a key named "token". When set, the operator skips<br />auto-generating a gateway token Secret and uses this Secret instead. |  | Optional: \{\} <br /> |
| `controlUiOrigins` _string array_ | ControlUiOrigins is a list of additional allowed origins for the Control UI.<br />The operator always auto-injects localhost origins (http://localhost:18789,<br />http://127.0.0.1:18789) and derives origins from ingress hosts. Use this<br />field to add extra origins (e.g., custom reverse proxy URLs). |  | MaxItems: 20 <br />Optional: \{\} <br /> |


#### GrafanaDashboardSpec



GrafanaDashboardSpec configures auto-provisioned Grafana dashboard ConfigMaps



_Appears in:_
- [MetricsSpec](#metricsspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables Grafana dashboard ConfigMap creation | false | Optional: \{\} <br /> |
| `labels` _object (keys:string, values:string)_ | Labels to add to the dashboard ConfigMaps (in addition to grafana_dashboard: "1") |  | Optional: \{\} <br /> |
| `folder` _string_ | Folder is the Grafana folder to place the dashboards in | OpenClaw | Optional: \{\} <br /> |


#### HTTPRouteParentRef



HTTPRouteParentRef identifies a Gateway that the HTTPRoute attaches to



_Appears in:_
- [HTTPRouteSpec](#httproutespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the Gateway |  | MinLength: 1 <br /> |
| `namespace` _string_ | Namespace is the namespace of the Gateway.<br />Defaults to the same namespace as the HTTPRoute. |  | Optional: \{\} <br /> |
| `sectionName` _string_ | SectionName is the name of a specific listener on the Gateway to attach to. |  | Optional: \{\} <br /> |


#### HTTPRouteSpec



HTTPRouteSpec defines a Gateway API HTTPRoute configuration



_Appears in:_
- [NetworkingSpec](#networkingspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables HTTPRoute creation | false | Optional: \{\} <br /> |
| `parentRefs` _[HTTPRouteParentRef](#httprouteparentref) array_ | ParentRefs specifies the Gateways this HTTPRoute attaches to.<br />Each ref identifies a Gateway by name (and optionally namespace and sectionName). |  | Optional: \{\} <br /> |
| `hostnames` _string array_ | Hostnames specifies the hostnames matched by this HTTPRoute. |  | Optional: \{\} <br /> |
| `port` _integer_ | Port is the backend Service port number to route traffic to.<br />Defaults to the gateway port (18789) when not set. |  | Maximum: 65535 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `annotations` _object (keys:string, values:string)_ | Annotations to add to the HTTPRoute |  | Optional: \{\} <br /> |


#### ImageSpec



ImageSpec defines the container image configuration



_Appears in:_
- [OpenClawClusterDefaultsSpec](#openclawclusterdefaultsspec)
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `repository` _string_ | Repository is the container image repository | ghcr.io/openclaw/openclaw | Optional: \{\} <br /> |
| `tag` _string_ | Tag is the container image tag | latest | Optional: \{\} <br /> |
| `digest` _string_ | Digest is the container image digest (overrides tag if specified) |  | Optional: \{\} <br /> |
| `pullPolicy` _[PullPolicy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#pullpolicy-v1-core)_ | PullPolicy specifies when to pull the image | IfNotPresent | Enum: [Always IfNotPresent Never] <br />Optional: \{\} <br /> |
| `pullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core) array_ | PullSecrets is a list of secret names for pulling from private registries |  | Optional: \{\} <br /> |


#### IngressBasicAuthSpec



IngressBasicAuthSpec configures HTTP Basic Authentication for the Ingress.



_Appears in:_
- [IngressSecuritySpec](#ingresssecurityspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables basic authentication. | false | Optional: \{\} <br /> |
| `existingSecret` _string_ | ExistingSecret is the name of an existing Secret that already contains<br />htpasswd-formatted content in a key named "auth".<br />When set, the operator uses this Secret instead of generating one. |  | Optional: \{\} <br /> |
| `username` _string_ | Username for the auto-generated htpasswd Secret.<br />Ignored when existingSecret is set. | openclaw | MaxLength: 64 <br />Optional: \{\} <br /> |
| `realm` _string_ | Realm is the authentication realm shown in browser prompts. | OpenClaw | Optional: \{\} <br /> |


#### IngressHost



IngressHost defines a host for the Ingress



_Appears in:_
- [IngressSpec](#ingressspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `host` _string_ | Host is the fully qualified domain name |  |  |
| `paths` _[IngressPath](#ingresspath) array_ | Paths is a list of paths to route |  | Optional: \{\} <br /> |


#### IngressPath



IngressPath defines a path for the Ingress



_Appears in:_
- [IngressHost](#ingresshost)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `path` _string_ | Path is the path to route | / | Optional: \{\} <br /> |
| `pathType` _string_ | PathType determines how the path should be matched | Prefix | Enum: [Prefix Exact ImplementationSpecific] <br />Optional: \{\} <br /> |
| `port` _integer_ | Port is the backend service port number to route traffic to.<br />Defaults to the gateway port (18789) when not set. |  | Maximum: 65535 <br />Minimum: 1 <br />Optional: \{\} <br /> |


#### IngressSecuritySpec



IngressSecuritySpec defines security settings for the Ingress



_Appears in:_
- [IngressSpec](#ingressspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `forceHTTPS` _boolean_ | ForceHTTPS redirects all HTTP traffic to HTTPS | true | Optional: \{\} <br /> |
| `enableHSTS` _boolean_ | EnableHSTS enables HTTP Strict Transport Security | true | Optional: \{\} <br /> |
| `rateLimiting` _[RateLimitingSpec](#ratelimitingspec)_ | RateLimiting configures rate limiting |  | Optional: \{\} <br /> |
| `basicAuth` _[IngressBasicAuthSpec](#ingressbasicauthspec)_ | BasicAuth configures HTTP Basic Authentication for the Ingress.<br />Disabled by default. When enabled without an existingSecret, the operator<br />auto-generates a random password and stores it in a managed Secret. |  | Optional: \{\} <br /> |


#### IngressSpec



IngressSpec defines the Ingress configuration



_Appears in:_
- [NetworkingSpec](#networkingspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables Ingress creation | false | Optional: \{\} <br /> |
| `className` _string_ | ClassName is the name of the IngressClass to use |  | Optional: \{\} <br /> |
| `annotations` _object (keys:string, values:string)_ | Annotations to add to the Ingress |  | Optional: \{\} <br /> |
| `hosts` _[IngressHost](#ingresshost) array_ | Hosts is a list of hosts to route traffic for |  | Optional: \{\} <br /> |
| `tls` _[IngressTLS](#ingresstls) array_ | TLS configuration |  | Optional: \{\} <br /> |
| `security` _[IngressSecuritySpec](#ingresssecurityspec)_ | Security configures ingress security settings |  | Optional: \{\} <br /> |


#### IngressTLS



IngressTLS defines TLS configuration for the Ingress



_Appears in:_
- [IngressSpec](#ingressspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `hosts` _string array_ | Hosts are a list of hosts included in the TLS certificate |  |  |
| `secretName` _string_ | SecretName is the name of the secret containing the TLS certificate |  |  |


#### LoggingSpec



LoggingSpec defines logging configuration



_Appears in:_
- [ObservabilitySpec](#observabilityspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `level` _string_ | Level is the log level | info | Enum: [debug info warn error] <br />Optional: \{\} <br /> |
| `format` _string_ | Format is the log format | json | Enum: [json text] <br />Optional: \{\} <br /> |


#### MetricsSpec



MetricsSpec defines metrics configuration



_Appears in:_
- [ObservabilitySpec](#observabilityspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables metrics endpoint | true | Optional: \{\} <br /> |
| `port` _integer_ | Port is the port to expose metrics on | 9090 | Optional: \{\} <br /> |
| `serviceMonitor` _[ServiceMonitorSpec](#servicemonitorspec)_ | ServiceMonitor configures the Prometheus ServiceMonitor |  | Optional: \{\} <br /> |
| `prometheusRule` _[PrometheusRuleSpec](#prometheusrulespec)_ | PrometheusRule configures auto-provisioned PrometheusRule alerts |  | Optional: \{\} <br /> |
| `grafanaDashboard` _[GrafanaDashboardSpec](#grafanadashboardspec)_ | GrafanaDashboard configures auto-provisioned Grafana dashboard ConfigMaps |  | Optional: \{\} <br /> |


#### NetworkPolicySpec



NetworkPolicySpec configures network isolation for the OpenClaw instance



_Appears in:_
- [SecuritySpec](#securityspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables network policy creation | true | Optional: \{\} <br /> |
| `allowedIngressCIDRs` _string array_ | AllowedIngressCIDRs is a list of CIDRs allowed to access this instance |  | Optional: \{\} <br /> |
| `allowedIngressNamespaces` _string array_ | AllowedIngressNamespaces is a list of namespace names allowed to access this instance |  | Optional: \{\} <br /> |
| `allowedEgressCIDRs` _string array_ | AllowedEgressCIDRs is a list of CIDRs this instance can reach<br />Default allows all egress on port 443 for AI APIs |  | Optional: \{\} <br /> |
| `allowDNS` _boolean_ | AllowDNS allows DNS resolution (port 53) | true | Optional: \{\} <br /> |
| `additionalEgress` _[NetworkPolicyEgressRule](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#networkpolicyegressrule-v1-networking) array_ | AdditionalEgress appends custom egress rules to the default DNS + HTTPS rules.<br />Use this to allow traffic to cluster-internal services on non-standard ports. |  | Optional: \{\} <br /> |


#### NetworkingSpec



NetworkingSpec defines network-related configuration



_Appears in:_
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `service` _[ServiceSpec](#servicespec)_ | Service configures the Kubernetes Service |  | Optional: \{\} <br /> |
| `ingress` _[IngressSpec](#ingressspec)_ | Ingress configures the Kubernetes Ingress |  | Optional: \{\} <br /> |
| `httpRoute` _[HTTPRouteSpec](#httproutespec)_ | HTTPRoute configures a Gateway API HTTPRoute.<br />This is an alternative to Ingress for clusters using the Gateway API<br />(gateway.networking.k8s.io). The Gateway API CRDs must be installed. |  | Optional: \{\} <br /> |


#### ObservabilitySpec



ObservabilitySpec defines observability configuration



_Appears in:_
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metrics` _[MetricsSpec](#metricsspec)_ | Metrics configures Prometheus metrics |  | Optional: \{\} <br /> |
| `logging` _[LoggingSpec](#loggingspec)_ | Logging configures logging |  | Optional: \{\} <br /> |


#### OllamaImageSpec



OllamaImageSpec defines the Ollama container image



_Appears in:_
- [OllamaSpec](#ollamaspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `repository` _string_ | Repository is the container image repository | ollama/ollama | Optional: \{\} <br /> |
| `tag` _string_ | Tag is the container image tag | latest | Optional: \{\} <br /> |
| `digest` _string_ | Digest is the container image digest for supply chain security |  | Optional: \{\} <br /> |


#### OllamaSpec



OllamaSpec defines the Ollama sidecar configuration



_Appears in:_
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables the Ollama sidecar | false | Optional: \{\} <br /> |
| `image` _[OllamaImageSpec](#ollamaimagespec)_ | Image configures the Ollama container image |  | Optional: \{\} <br /> |
| `models` _string array_ | Models is a list of models to pre-pull during pod init (e.g. ["llama3.2", "nomic-embed-text"]) |  | MaxItems: 10 <br />Optional: \{\} <br /> |
| `resources` _[ResourcesSpec](#resourcesspec)_ | Resources specifies compute resources for the Ollama container |  | Optional: \{\} <br /> |
| `storage` _[OllamaStorageSpec](#ollamastoragespec)_ | Storage configures the model cache volume |  | Optional: \{\} <br /> |
| `gpu` _integer_ | GPU is the number of NVIDIA GPUs to allocate (sets nvidia.com/gpu resource limit) |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### OllamaStorageSpec



OllamaStorageSpec configures the Ollama model cache volume



_Appears in:_
- [OllamaSpec](#ollamaspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `sizeLimit` _string_ | SizeLimit is the size limit for the emptyDir model cache (default "20Gi") | 20Gi | Optional: \{\} <br /> |
| `existingClaim` _string_ | ExistingClaim is the name of an existing PVC for persistent model storage |  | Optional: \{\} <br /> |


#### OpenClawClusterDefaults



OpenClawClusterDefaults is a cluster-scoped singleton (name must be "cluster")
that provides default values merged into every OpenClawInstance at reconcile
time. It exists so platform operators managing air-gapped or restricted-network
environments can set a single source of truth for image registry mirrors,
shared environment variables (e.g. NPM_CONFIG_REGISTRY, PIP_INDEX_URL), and
runtime-dep init containers without duplicating the same boilerplate in every
OpenClawInstance manifest.

Precedence: per-instance fields always win over cluster defaults. A default
is only applied when the corresponding instance field is unset.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `openclaw.rocks/v1alpha1` | | |
| `kind` _string_ | `OpenClawClusterDefaults` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[OpenClawClusterDefaultsSpec](#openclawclusterdefaultsspec)_ |  |  |  |


#### OpenClawClusterDefaultsSpec



OpenClawClusterDefaultsSpec defines cluster-wide defaults that the operator
applies to every OpenClawInstance at reconcile time. Per-instance fields
always win: a default is only applied when the instance field is unset.



_Appears in:_
- [OpenClawClusterDefaults](#openclawclusterdefaults)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `registry` _string_ | Registry is the default container image registry override applied to<br />instances where spec.registry is unset. Replaces the registry prefix of<br />all container images (main, sidecars, init containers).<br />Example: "my-registry.example.com". |  | Optional: \{\} <br /> |
| `image` _[ImageSpec](#imagespec)_ | Image is the default container image configuration applied to instances<br />where the corresponding instance fields are unset. Each sub-field is<br />merged independently (e.g. a cluster-default tag still applies even when<br />the instance sets its own repository). |  | Optional: \{\} <br /> |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#envvar-v1-core) array_ | Env is a list of default environment variables merged into every<br />instance's container env. Instance-level env entries with the same Name<br />override the cluster default for that name. Defaults appear first in<br />the resulting env list, followed by instance-only names. |  | Optional: \{\} <br /> |
| `runtimeDeps` _[RuntimeDepsSpec](#runtimedepsspec)_ | RuntimeDeps configures the default set of built-in init containers<br />(pnpm, Python) applied to instances where the corresponding fields are<br />unset. A cluster default of true for a runtime dep is always applied<br />unless the instance explicitly opts out (sets the field to false).<br />NOTE: because RuntimeDepsSpec fields are plain booleans, "unset" and<br />"false" are indistinguishable; cluster defaults are OR-merged here. |  | Optional: \{\} <br /> |


#### OpenClawInstance



OpenClawInstance is the Schema for the openclawinstances API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `openclaw.rocks/v1alpha1` | | |
| `kind` _string_ | `OpenClawInstance` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[OpenClawInstanceSpec](#openclawinstancespec)_ |  |  |  |


#### OpenClawInstanceSpec



OpenClawInstanceSpec defines the desired state of OpenClawInstance



_Appears in:_
- [OpenClawInstance](#openclawinstance)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `registry` _string_ | Registry is the global container image registry override.<br />When set, this registry replaces the registry part of all container images<br />used by the instance (main container, sidecars, init containers).<br />Example: "my-registry.example.com" will change "ghcr.io/openclaw/openclaw:latest"<br />to "my-registry.example.com/openclaw/openclaw:latest". |  | Optional: \{\} <br /> |
| `image` _[ImageSpec](#imagespec)_ | Image configuration for the OpenClaw container |  | Optional: \{\} <br /> |
| `config` _[ConfigSpec](#configspec)_ | Config specifies the OpenClaw configuration |  | Optional: \{\} <br /> |
| `workspace` _[WorkspaceSpec](#workspacespec)_ | Workspace configures initial workspace files seeded into the instance.<br />Files are copied once on first boot and never overwritten, so agent<br />modifications survive pod restarts. |  | Optional: \{\} <br /> |
| `skills` _string array_ | Skills is a list of skills to install via init container.<br />Each entry is either a ClawHub skill identifier (e.g., "@anthropic/mcp-server-fetch")<br />or an npm package prefixed with "npm:" (e.g., "npm:@openclaw/matrix").<br />npm lifecycle scripts are disabled for security (see #91). |  | MaxItems: 20 <br />Optional: \{\} <br /> |
| `skillPackUpdatePolicy` _string_ | SkillPackUpdatePolicy controls how workspace files seeded from "pack:"<br />skill entries are reconciled on pod start.<br />"Replace" (default) converges seeded pack files to the declared pack<br />revision on every pod start: changed files are overwritten and files that<br />were seeded by a previous revision but are no longer part of any declared<br />pack are removed. The operator tracks the seeded file set in a manifest<br />at /data/.skillpack-manifest on the data volume.<br />"CreateOnly" preserves the legacy behavior: pack files are only copied<br />when absent and never overwritten or removed, so updating a pinned pack<br />revision does not refresh already-seeded contents (see #564).<br />Only files at paths declared by pack: entries are affected; files from<br />spec.workspace.initialFiles are always seeded create-only. | Replace | Enum: [Replace CreateOnly] <br />Optional: \{\} <br /> |
| `plugins` _string array_ | Plugins is a list of plugins to install via init container.<br />Each entry is an npm package name (e.g., "@openclaw/matrix" or<br />"@martian-engineering/lossless-claw"). An optional "npm:" prefix is<br />accepted and stripped before installation.<br />Installation goes through the OpenClaw CLI's ClawHub installer<br />("openclaw plugins install clawhub:<pkg>") rather than raw npm install<br />so packages published with workspace:* dependency markers resolve<br />correctly. npm lifecycle scripts are disabled for security. |  | MaxItems: 20 <br />Optional: \{\} <br /> |
| `envFrom` _[EnvFromSource](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#envfromsource-v1-core) array_ | EnvFrom is a list of sources to populate environment variables from<br />Use this for API keys and other secrets (e.g., ANTHROPIC_API_KEY, OPENAI_API_KEY) |  | Optional: \{\} <br /> |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#envvar-v1-core) array_ | Env is a list of environment variables to set in the container |  | Optional: \{\} <br /> |
| `resources` _[ResourcesSpec](#resourcesspec)_ | Resources specifies the compute resources for the OpenClaw container |  | Optional: \{\} <br /> |
| `security` _[SecuritySpec](#securityspec)_ | Security specifies security-related configuration |  | Optional: \{\} <br /> |
| `shareProcessNamespace` _boolean_ | ShareProcessNamespace enables PID namespace sharing between all containers<br />in the pod. When true, the infrastructure (pause) container becomes PID 1<br />and reaps zombie processes, which prevents accumulation of defunct helper<br />processes (git, plugins, QMD memory, shells) under a Node.js gateway that<br />does not call waitpid(). Defaults to true.<br />Security note: enabling this lets every container in the pod see and signal<br />every other container's processes. A compromised sidecar (Tailscale, Ollama,<br />browser, custom) could send signals to the gateway and vice versa. Set to<br />false to keep per-container PID isolation; you are then responsible for<br />reaping zombies (e.g. by baking tini or dumb-init into the image). | true | Optional: \{\} <br /> |
| `storage` _[StorageSpec](#storagespec)_ | Storage specifies persistent storage configuration |  | Optional: \{\} <br /> |
| `chromium` _[ChromiumSpec](#chromiumspec)_ | Chromium enables the Chromium sidecar for browser automation |  | Optional: \{\} <br /> |
| `tailscale` _[TailscaleSpec](#tailscalespec)_ | Tailscale configures Tailscale integration for tailnet access and HTTPS |  | Optional: \{\} <br /> |
| `ollama` _[OllamaSpec](#ollamaspec)_ | Ollama enables the Ollama sidecar for local LLM inference |  | Optional: \{\} <br /> |
| `webTerminal` _[WebTerminalSpec](#webterminalspec)_ | WebTerminal enables a browser-based terminal (ttyd) sidecar for debugging |  | Optional: \{\} <br /> |
| `initContainers` _[Container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#container-v1-core) array_ | InitContainers is a list of additional init containers to run before the main container.<br />They run after the operator-managed init-config and init-skills containers. |  | MaxItems: 10 <br />Optional: \{\} <br /> |
| `sidecars` _[Container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#container-v1-core) array_ | Sidecars is a list of additional sidecar containers to inject into the pod.<br />Use this for custom sidecars like database proxies, log forwarders, or service meshes. |  | Optional: \{\} <br /> |
| `sidecarVolumes` _[Volume](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#volume-v1-core) array_ | SidecarVolumes is a list of additional volumes to make available to sidecar containers. |  | Optional: \{\} <br /> |
| `extraVolumes` _[Volume](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#volume-v1-core) array_ | ExtraVolumes adds additional volumes to the pod.<br />These volumes are available to the main container via ExtraVolumeMounts. |  | MaxItems: 10 <br />Optional: \{\} <br /> |
| `extraVolumeMounts` _[VolumeMount](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#volumemount-v1-core) array_ | ExtraVolumeMounts adds additional volume mounts to the main container.<br />Use with ExtraVolumes to mount ConfigMaps, Secrets, NFS shares, or CSI volumes. |  | MaxItems: 10 <br />Optional: \{\} <br /> |
| `networking` _[NetworkingSpec](#networkingspec)_ | Networking specifies network-related configuration |  | Optional: \{\} <br /> |
| `probes` _[ProbesSpec](#probesspec)_ | Probes configures health probes for the OpenClaw container |  | Optional: \{\} <br /> |
| `observability` _[ObservabilitySpec](#observabilityspec)_ | Observability configures metrics and logging |  | Optional: \{\} <br /> |
| `availability` _[AvailabilitySpec](#availabilityspec)_ | Availability configures high availability settings |  | Optional: \{\} <br /> |
| `suspended` _boolean_ | Suspended scales the workload to zero replicas when true.<br />Non-runtime resources (Service, ConfigMap, RBAC, NetworkPolicy, PVC)<br />remain fully managed. Set to false to resume normal operation. | false | Optional: \{\} <br /> |
| `backup` _[BackupSpec](#backupspec)_ | Backup configures periodic scheduled backups to S3-compatible storage.<br />Requires the s3-backup-credentials Secret in the operator namespace and persistence enabled. |  | Optional: \{\} <br /> |
| `restoreFrom` _string_ | RestoreFrom is the remote backup path to restore data from (e.g. "backups/\{tenantId\}/\{instanceId\}/\{timestamp\}").<br />When set, the operator restores PVC data from this path before creating the StatefulSet.<br />Cleared automatically after successful restore. |  | Optional: \{\} <br /> |
| `runtimeDeps` _[RuntimeDepsSpec](#runtimedepsspec)_ | RuntimeDeps configures built-in init containers that install runtime<br />dependencies (pnpm, Python) for MCP servers and skills. |  | Optional: \{\} <br /> |
| `gateway` _[GatewaySpec](#gatewayspec)_ | Gateway configures the gateway reverse proxy and authentication token |  | Optional: \{\} <br /> |
| `autoUpdate` _[AutoUpdateSpec](#autoupdatespec)_ | AutoUpdate configures automatic version updates from the OCI registry |  | Optional: \{\} <br /> |
| `selfConfigure` _[SelfConfigureSpec](#selfconfigurespec)_ | SelfConfigure enables agents to modify their own instance via OpenClawSelfConfig resources.<br />When enabled, the operator injects RBAC, env vars, and a helper skill into the workspace. |  | Optional: \{\} <br /> |
| `podAnnotations` _object (keys:string, values:string)_ | PodAnnotations are extra annotations merged into the pod template metadata.<br />Operator-managed annotations (e.g. config-hash) take precedence on conflict. |  | Optional: \{\} <br /> |


#### OpenClawSelfConfig



OpenClawSelfConfig is the Schema for the openclawselfconfigs API.
It represents a request from an agent to modify its own OpenClawInstance spec.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `openclaw.rocks/v1alpha1` | | |
| `kind` _string_ | `OpenClawSelfConfig` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[OpenClawSelfConfigSpec](#openclawselfconfigspec)_ |  |  |  |


#### OpenClawSelfConfigSpec



OpenClawSelfConfigSpec defines the desired changes to an OpenClawInstance.



_Appears in:_
- [OpenClawSelfConfig](#openclawselfconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instanceRef` _string_ | InstanceRef is the name of the parent OpenClawInstance in the same namespace. |  | MinLength: 1 <br /> |
| `addSkills` _string array_ | AddSkills is a list of skills to add to the instance. |  | MaxItems: 10 <br />Optional: \{\} <br /> |
| `removeSkills` _string array_ | RemoveSkills is a list of skills to remove from the instance. |  | MaxItems: 10 <br />Optional: \{\} <br /> |
| `configPatch` _[RawConfig](#rawconfig)_ | ConfigPatch is a partial JSON configuration to deep-merge into the instance config. |  | Optional: \{\} <br /> |
| `addWorkspaceFiles` _object (keys:string, values:string)_ | AddWorkspaceFiles maps filenames to content to add to the workspace. |  | MaxProperties: 10 <br />Optional: \{\} <br /> |
| `removeWorkspaceFiles` _string array_ | RemoveWorkspaceFiles is a list of workspace filenames to remove. |  | MaxItems: 10 <br />Optional: \{\} <br /> |
| `addEnvVars` _[SelfConfigEnvVar](#selfconfigenvvar) array_ | AddEnvVars is a list of environment variables to add (plain values only). |  | MaxItems: 10 <br />Optional: \{\} <br /> |
| `removeEnvVars` _string array_ | RemoveEnvVars is a list of environment variable names to remove. |  | MaxItems: 10 <br />Optional: \{\} <br /> |


#### PersistenceSpec



PersistenceSpec defines PVC configuration



_Appears in:_
- [StorageSpec](#storagespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables persistent storage | true | Optional: \{\} <br /> |
| `storageClass` _string_ | StorageClass is the name of the StorageClass to use |  | Optional: \{\} <br /> |
| `size` _string_ | Size is the size of the PVC (e.g., "10Gi") | 10Gi | Optional: \{\} <br /> |
| `accessModes` _[PersistentVolumeAccessMode](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#persistentvolumeaccessmode-v1-core) array_ | AccessModes contains the desired access modes for the PVC | [ReadWriteOnce] | Optional: \{\} <br /> |
| `existingClaim` _string_ | ExistingClaim is the name of an existing PVC to use |  | Optional: \{\} <br /> |
| `orphan` _boolean_ | Orphan controls whether the PVC is retained when the OpenClawInstance is deleted.<br />When true (the default), the operator removes the owner reference from the PVC<br />before deleting the CR so Kubernetes does not garbage-collect it.<br />Set to false if you want the PVC deleted together with the CR. | true | Optional: \{\} <br /> |


#### PodDisruptionBudgetSpec



PodDisruptionBudgetSpec defines PDB configuration



_Appears in:_
- [AvailabilitySpec](#availabilityspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables PDB creation | true | Optional: \{\} <br /> |
| `maxUnavailable` _integer_ | MaxUnavailable is the maximum number of pods that can be unavailable during disruption | 1 | Optional: \{\} <br /> |


#### PodSecurityContextSpec



PodSecurityContextSpec defines pod-level security context



_Appears in:_
- [SecuritySpec](#securityspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `runAsUser` _integer_ | RunAsUser is the UID to run the entrypoint of the container process | 1000 | Optional: \{\} <br /> |
| `runAsGroup` _integer_ | RunAsGroup is the GID to run the entrypoint of the container process | 1000 | Optional: \{\} <br /> |
| `fsGroup` _integer_ | FSGroup is a special supplemental group that applies to all containers | 1000 | Optional: \{\} <br /> |
| `fsGroupChangePolicy` _[PodFSGroupChangePolicy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#podfsgroupchangepolicy-v1-core)_ | FSGroupChangePolicy defines the behavior of changing ownership and permission of the volume.<br />"OnRootMismatch" skips recursive chown when ownership already matches, improving startup<br />time for large PVCs. "Always" recursively chowns on every mount (Kubernetes default). |  | Enum: [OnRootMismatch Always] <br />Optional: \{\} <br /> |
| `runAsNonRoot` _boolean_ | RunAsNonRoot indicates that the container must run as a non-root user | true | Optional: \{\} <br /> |


#### ProbeSpec



ProbeSpec defines a health probe



_Appears in:_
- [ProbesSpec](#probesspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables the probe | true | Optional: \{\} <br /> |
| `initialDelaySeconds` _integer_ | InitialDelaySeconds is the number of seconds after the container starts before the probe is initiated |  | Optional: \{\} <br /> |
| `periodSeconds` _integer_ | PeriodSeconds is how often (in seconds) to perform the probe |  | Optional: \{\} <br /> |
| `timeoutSeconds` _integer_ | TimeoutSeconds is the number of seconds after which the probe times out |  | Optional: \{\} <br /> |
| `failureThreshold` _integer_ | FailureThreshold is the number of times to retry before giving up |  | Optional: \{\} <br /> |


#### ProbesSpec



ProbesSpec defines health probe configuration



_Appears in:_
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `liveness` _[ProbeSpec](#probespec)_ | Liveness probe configuration |  | Optional: \{\} <br /> |
| `readiness` _[ProbeSpec](#probespec)_ | Readiness probe configuration |  | Optional: \{\} <br /> |
| `startup` _[ProbeSpec](#probespec)_ | Startup probe configuration |  | Optional: \{\} <br /> |
| `diskReadiness` _[DiskReadinessSpec](#diskreadinessspec)_ | DiskReadiness configures an optional, opt-in defense-in-depth readiness<br />guard for PVC-backed workspaces. When enabled, the readiness probe is<br />rendered as an exec probe that also verifies the workspace volume is<br />writable and has free space above a threshold, so a full or read-only<br />PVC marks the pod NotReady (draining it from Service endpoints) instead<br />of silently accepting traffic it cannot persist. Liveness and startup<br />probes are unaffected, so a full PVC does not turn into a CrashLoopBackOff.<br />The application-level /readyz endpoint remains the primary readiness<br />signal; this guard is secondary. Defaults to disabled. |  | Optional: \{\} <br /> |


#### PrometheusRuleSpec



PrometheusRuleSpec configures auto-provisioned PrometheusRule alerts



_Appears in:_
- [MetricsSpec](#metricsspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables PrometheusRule creation with operator alerts | false | Optional: \{\} <br /> |
| `labels` _object (keys:string, values:string)_ | Labels to add to the PrometheusRule (e.g., for Prometheus rule selector matching) |  | Optional: \{\} <br /> |
| `runbookBaseURL` _string_ | RunbookBaseURL is the base URL for alert runbook links | https://paperclip.inc/docs/operators/openclaw/runbooks | Optional: \{\} <br /> |


#### RBACRule



RBACRule represents a RBAC rule



_Appears in:_
- [RBACSpec](#rbacspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiGroups` _string array_ | APIGroups is the name of the APIGroup that contains the resources |  |  |
| `resources` _string array_ | Resources is a list of resources this rule applies to |  |  |
| `verbs` _string array_ | Verbs is a list of verbs that apply to the resources |  |  |


#### RBACSpec



RBACSpec configures RBAC for the OpenClaw instance



_Appears in:_
- [SecuritySpec](#securityspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `createServiceAccount` _boolean_ | CreateServiceAccount creates a dedicated ServiceAccount for the instance | true | Optional: \{\} <br /> |
| `serviceAccountName` _string_ | ServiceAccountName is the name of an existing ServiceAccount to use<br />Only used if CreateServiceAccount is false |  | Optional: \{\} <br /> |
| `serviceAccountAnnotations` _object (keys:string, values:string)_ | ServiceAccountAnnotations are annotations to add to the managed ServiceAccount.<br />Use this for cloud provider integrations like AWS IRSA or GCP Workload Identity. |  | Optional: \{\} <br /> |
| `additionalRules` _[RBACRule](#rbacrule) array_ | AdditionalRules adds custom RBAC rules to the generated Role |  | Optional: \{\} <br /> |


#### RateLimitingSpec



RateLimitingSpec defines rate limiting configuration



_Appears in:_
- [IngressSecuritySpec](#ingresssecurityspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables rate limiting | true | Optional: \{\} <br /> |
| `requestsPerSecond` _integer_ | RequestsPerSecond is the maximum requests per second | 10 | Optional: \{\} <br /> |


#### RawConfig



RawConfig holds arbitrary JSON configuration for openclaw.json



_Appears in:_
- [ConfigSpec](#configspec)
- [OpenClawSelfConfigSpec](#openclawselfconfigspec)



#### ResourcesSpec



ResourcesSpec defines compute resource requirements



_Appears in:_
- [ChromiumSpec](#chromiumspec)
- [OllamaSpec](#ollamaspec)
- [OpenClawInstanceSpec](#openclawinstancespec)
- [TailscaleSpec](#tailscalespec)
- [WebTerminalSpec](#webterminalspec)



#### RuntimeDepsSpec



RuntimeDepsSpec configures built-in init containers that install runtime
dependencies to the data PVC for use by MCP servers and skills.



_Appears in:_
- [OpenClawClusterDefaultsSpec](#openclawclusterdefaultsspec)
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `pnpm` _boolean_ | Pnpm installs pnpm via corepack for npm-based MCP servers and skills. |  | Optional: \{\} <br /> |
| `python` _boolean_ | Python installs Python 3.12 and uv for Python-based MCP servers and skills. |  | Optional: \{\} <br /> |


#### SecuritySpec



SecuritySpec defines security-related configuration



_Appears in:_
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `podSecurityContext` _[PodSecurityContextSpec](#podsecuritycontextspec)_ | PodSecurityContext holds pod-level security attributes |  | Optional: \{\} <br /> |
| `containerSecurityContext` _[ContainerSecurityContextSpec](#containersecuritycontextspec)_ | ContainerSecurityContext holds container-level security attributes |  | Optional: \{\} <br /> |
| `networkPolicy` _[NetworkPolicySpec](#networkpolicyspec)_ | NetworkPolicy configures network isolation |  | Optional: \{\} <br /> |
| `rbac` _[RBACSpec](#rbacspec)_ | RBAC configures role-based access control |  | Optional: \{\} <br /> |
| `caBundle` _[CABundleSpec](#cabundlespec)_ | CABundle injects a custom CA certificate bundle into all containers.<br />Use this in environments with TLS-intercepting proxies or private CAs. |  | Optional: \{\} <br /> |


#### SelfConfigAction

_Underlying type:_ _string_

SelfConfigAction represents an action category that can be allowed for self-configuration.

_Validation:_
- Enum: [skills config workspaceFiles envVars]

_Appears in:_
- [SelfConfigureSpec](#selfconfigurespec)

| Field | Description |
| --- | --- |
| `skills` |  |
| `config` |  |
| `workspaceFiles` |  |
| `envVars` |  |


#### SelfConfigEnvVar



SelfConfigEnvVar defines a plain-value environment variable (no secret refs).



_Appears in:_
- [OpenClawSelfConfigSpec](#openclawselfconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the environment variable. |  | MinLength: 1 <br /> |
| `value` _string_ | Value of the environment variable. |  |  |




#### SelfConfigureSpec



SelfConfigureSpec configures whether an agent can modify its own instance.



_Appears in:_
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables self-configuration for this instance.<br />When true, the agent can create OpenClawSelfConfig resources to modify its own spec. | false | Optional: \{\} <br /> |
| `allowedActions` _[SelfConfigAction](#selfconfigaction) array_ | AllowedActions restricts which action categories the agent can perform.<br />If empty and enabled is true, no actions are allowed (fail-safe). |  | Enum: [skills config workspaceFiles envVars] <br />MaxItems: 4 <br />Optional: \{\} <br /> |


#### ServiceMonitorSpec



ServiceMonitorSpec defines the ServiceMonitor configuration



_Appears in:_
- [MetricsSpec](#metricsspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables ServiceMonitor creation | false | Optional: \{\} <br /> |
| `interval` _string_ | Interval is the scrape interval | 30s | Optional: \{\} <br /> |
| `labels` _object (keys:string, values:string)_ | Labels to add to the ServiceMonitor |  | Optional: \{\} <br /> |


#### ServicePortSpec



ServicePortSpec defines a port exposed by the Service



_Appears in:_
- [ServiceSpec](#servicespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the port |  | MinLength: 1 <br /> |
| `port` _integer_ | Port is the port number exposed on the Service |  | Maximum: 65535 <br />Minimum: 1 <br /> |
| `targetPort` _integer_ | TargetPort is the port on the container to route to (defaults to Port) |  | Maximum: 65535 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `protocol` _[Protocol](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#protocol-v1-core)_ | Protocol is the protocol for the port | TCP | Enum: [TCP UDP SCTP] <br />Optional: \{\} <br /> |


#### ServiceSpec



ServiceSpec defines the Service configuration



_Appears in:_
- [NetworkingSpec](#networkingspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[ServiceType](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#servicetype-v1-core)_ | Type is the Kubernetes Service type | ClusterIP | Enum: [ClusterIP LoadBalancer NodePort] <br />Optional: \{\} <br /> |
| `annotations` _object (keys:string, values:string)_ | Annotations to add to the Service |  | Optional: \{\} <br /> |
| `ports` _[ServicePortSpec](#serviceportspec) array_ | Ports defines custom ports exposed on the Service.<br />When set, these replace the default gateway and canvas ports.<br />When empty, the operator creates default gateway (18789) and canvas (18793) ports. |  | MaxItems: 20 <br />Optional: \{\} <br /> |


#### StorageSpec



StorageSpec defines persistent storage configuration



_Appears in:_
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `persistence` _[PersistenceSpec](#persistencespec)_ | Persistence configures the PersistentVolumeClaim |  | Optional: \{\} <br /> |


#### TailscaleImageSpec



TailscaleImageSpec defines the Tailscale sidecar container image



_Appears in:_
- [TailscaleSpec](#tailscalespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `repository` _string_ | Repository is the container image repository | ghcr.io/tailscale/tailscale | Optional: \{\} <br /> |
| `tag` _string_ | Tag is the container image tag | latest | Optional: \{\} <br /> |
| `digest` _string_ | Digest is the container image digest for supply chain security |  | Optional: \{\} <br /> |


#### TailscaleSpec



TailscaleSpec configures Tailscale integration for secure tailnet access.
When enabled, a Tailscale sidecar container runs tailscaled and handles
serve/funnel via TS_SERVE_CONFIG. An init container copies the tailscale
CLI binary to a shared volume so the main container can call
"tailscale whois" for SSO authentication.



_Appears in:_
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables Tailscale integration | false | Optional: \{\} <br /> |
| `mode` _string_ | Mode selects the Tailscale mode.<br />"serve" exposes the instance to tailnet members only (default).<br />"funnel" exposes the instance to the public internet via Tailscale Funnel. | serve | Enum: [serve funnel] <br />Optional: \{\} <br /> |
| `image` _[TailscaleImageSpec](#tailscaleimagespec)_ | Image configures the Tailscale sidecar container image.<br />The same image is used for the sidecar and the init container that<br />copies the tailscale CLI binary. |  | Optional: \{\} <br /> |
| `authKeySecretRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core)_ | AuthKeySecretRef references a Secret containing the Tailscale auth key.<br />The Secret must have a key matching AuthKeySecretKey (default: "authkey").<br />Use ephemeral+reusable keys from the Tailscale admin console. |  | Optional: \{\} <br /> |
| `authKeySecretKey` _string_ | AuthKeySecretKey is the key in the referenced Secret. | authkey | Optional: \{\} <br /> |
| `hostname` _string_ | Hostname sets the Tailscale device name (defaults to instance name). |  | Optional: \{\} <br /> |
| `authSSO` _boolean_ | AuthSSO enables passwordless login for tailnet members.<br />Sets gateway.auth.allowTailscale=true in the OpenClaw config. | false | Optional: \{\} <br /> |
| `resources` _[ResourcesSpec](#resourcesspec)_ | Resources specifies compute resources for the Tailscale sidecar container. |  | Optional: \{\} <br /> |


#### WebTerminalCredentialSpec



WebTerminalCredentialSpec configures basic auth for the web terminal



_Appears in:_
- [WebTerminalSpec](#webterminalspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `secretRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core)_ | SecretRef references a Secret containing "username" and "password" keys |  |  |


#### WebTerminalImageSpec



WebTerminalImageSpec defines the ttyd container image



_Appears in:_
- [WebTerminalSpec](#webterminalspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `repository` _string_ | Repository is the container image repository | tsl0922/ttyd | Optional: \{\} <br /> |
| `tag` _string_ | Tag is the container image tag | latest | Optional: \{\} <br /> |
| `digest` _string_ | Digest is the container image digest for supply chain security |  | Optional: \{\} <br /> |


#### WebTerminalSpec



WebTerminalSpec defines the ttyd web terminal sidecar configuration



_Appears in:_
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled enables the ttyd web terminal sidecar for browser-based shell access | false | Optional: \{\} <br /> |
| `image` _[WebTerminalImageSpec](#webterminalimagespec)_ | Image configures the ttyd container image |  | Optional: \{\} <br /> |
| `resources` _[ResourcesSpec](#resourcesspec)_ | Resources specifies compute resources for the ttyd container |  | Optional: \{\} <br /> |
| `readOnly` _boolean_ | ReadOnly starts ttyd in read-only mode (view-only, no input) | false | Optional: \{\} <br /> |
| `credential` _[WebTerminalCredentialSpec](#webterminalcredentialspec)_ | Credential configures basic auth for the web terminal via a Secret.<br />The Secret must have "username" and "password" keys. |  | Optional: \{\} <br /> |


#### WorkspaceSpec



WorkspaceSpec configures initial workspace files for the instance.
Files listed in InitialFiles are seeded once (only if they don't already
exist on the PVC), so agent modifications survive pod restarts.



_Appears in:_
- [OpenClawInstanceSpec](#openclawinstancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `configMapRef` _[ConfigMapNameSelector](#configmapnameselector)_ | ConfigMapRef references an external ConfigMap whose keys become workspace files.<br />All keys in the referenced ConfigMap are included as workspace files.<br />This is useful for GitOps workflows where workspace files (AGENT.md, SOUL.md, etc.)<br />are managed as standalone files and bundled via Kustomize configMapGenerator or similar.<br />Merge priority (highest wins):<br />1. Operator-injected files (ENVIRONMENT.md, BOOTSTRAP.md, SELFCONFIG.md, selfconfig.sh)<br />2. Inline initialFiles<br />3. External configMapRef entries<br />4. Skill pack files |  | Optional: \{\} <br /> |
| `initialFiles` _object (keys:string, values:string)_ | InitialFiles maps filenames to their content. Each file is written<br />to the workspace directory only if it does not already exist. |  | MaxProperties: 50 <br />Optional: \{\} <br /> |
| `initialDirectories` _string array_ | InitialDirectories is a list of directories to create (mkdir -p)<br />inside the workspace directory. Nested paths like "tools/scripts" are allowed. |  | MaxItems: 20 <br />Optional: \{\} <br /> |
| `additionalWorkspaces` _[AdditionalWorkspace](#additionalworkspace) array_ | AdditionalWorkspaces configures workspace files for secondary agents.<br />Each entry seeds files to ~/.openclaw/workspace-<name>/, matching the<br />workspace path configured in spec.config.raw.agents.list[].workspace. |  | MaxItems: 10 <br />Optional: \{\} <br /> |
| `bootstrap` _[BootstrapSpec](#bootstrapspec)_ | Bootstrap controls the operator-managed BOOTSTRAP.md file injected into<br />the default workspace to guide first-run agent onboarding. |  | Optional: \{\} <br /> |


