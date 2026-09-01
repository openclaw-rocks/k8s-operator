# Architecture

This document describes the architecture of the OpenClaw Kubernetes Operator.

## High-Level Overview

The OpenClaw Operator follows the standard Kubernetes [operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/). It is built with [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) (the same framework behind Kubebuilder and Operator SDK) and extends the Kubernetes API with a custom resource definition: `OpenClawInstance`.

The operator watches for `OpenClawInstance` resources and reconciles a set of dependent Kubernetes objects to deploy fully configured AI assistant instances. It runs as a single Deployment in the cluster with leader election support for high availability.

```
                +-----------------------+
                |   Kubernetes API      |
                |   Server              |
                +-----------+-----------+
                            |
              watch/list OpenClawInstance
                            |
                +-----------v-----------+
                |   OpenClaw Operator   |
                |   (Controller)        |
                +-----------+-----------+
                            |
          create/update owned resources
                            |
        +-------------------+-------------------+
        |         |         |         |         |
   ServiceAccount Role  ConfigMap    PVC   Deployment
   RoleBinding    NP     PDB      Service   Ingress
                                           ServiceMonitor
```

## Reconciliation Flow

Each reconciliation cycle follows a deterministic, ordered sequence. The controller processes resources in dependency order so that prerequisites (such as RBAC and storage) exist before the workload starts.

### Step-by-Step Flow

1. **Fetch the OpenClawInstance** -- Retrieve the custom resource from the API server. If it no longer exists (404), stop reconciliation.

2. **Handle deletion** -- If `DeletionTimestamp` is set, transition to the `Terminating` phase, remove the finalizer, and let Kubernetes garbage-collect owned resources via owner references.

3. **Add finalizer** -- If the finalizer `openclaw.rocks/finalizer` is not present, add it and requeue. The finalizer ensures the controller gets a chance to run cleanup logic before the object is removed.

4. **Set initial phase** -- If `status.phase` is empty, set it to `Pending` and requeue. On the next pass, transition from `Pending` to `Provisioning`.

5. **Reconcile resources** -- Create or update all managed resources in the following order:

   | Order | Resource(s)                              | Description                                     |
   |-------|------------------------------------------|-------------------------------------------------|
   | 1     | ServiceAccount, Role, RoleBinding        | RBAC resources for pod identity                  |
   | 2     | NetworkPolicy                            | Default-deny network isolation                   |
   | 3     | ConfigMap                                | OpenClaw configuration (`openclaw.json`)         |
   | 4     | PersistentVolumeClaim                    | Data storage for `~/.openclaw/`                  |
   | 5     | PodDisruptionBudget                      | Disruption protection during node maintenance    |
   | 6     | Deployment                               | The OpenClaw workload (with optional sidecar)    |
   | 7     | Service                                  | ClusterIP/LoadBalancer/NodePort exposure          |
   | 8     | Ingress                                  | External HTTP/HTTPS access (if enabled)          |
   | 9     | ServiceMonitor                           | Prometheus scrape target (if enabled)            |

6. **Update status** -- On success, set phase to `Running`, update conditions, record the `lastReconcileTime`, and emit a Kubernetes event. On failure, set phase to `Failed` and requeue after one minute.

7. **Requeue** -- After a successful reconciliation, the controller requeues after 5 minutes to catch drift.

### Error Handling

When any resource reconciliation step fails, the controller:

- Logs the error with structured context.
- Emits a `ReconcileFailed` warning event on the OpenClawInstance.
- Sets the `Ready` condition to `False` with the error message.
- Transitions the phase to `Failed`.
- Increments the `openclaw_reconcile_total` counter with `result=error`.
- Requeues after 1 minute for retry.

## Resource Ownership

Every resource the operator creates carries an [owner reference](https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/) pointing to the parent `OpenClawInstance`. This means:

- **Cascading deletion**: When an `OpenClawInstance` is deleted, all its owned resources are garbage-collected automatically by the Kubernetes API server.
- **Watch propagation**: The controller watches owned resource types (`Owns(&appsv1.Deployment{})`, etc.). Changes to any owned resource trigger a reconciliation of the parent, enabling self-healing.
- **No orphans**: Resources cannot outlive their parent. If the operator is temporarily unavailable during deletion, the API server still cleans up owned resources.

The only exception is `ServiceMonitor`, which uses an unstructured client (because the `monitoring.coreos.com/v1` types may not be installed). Owner references are set manually for this resource.

## Status Management

### Phases

The `status.phase` field represents the high-level lifecycle state of the instance:

| Phase          | Meaning                                                              |
|----------------|----------------------------------------------------------------------|
| `Pending`      | The resource has been created but reconciliation has not started.     |
| `Provisioning` | The controller is actively creating or updating managed resources.   |
| `Running`      | All resources are reconciled successfully.                           |
| `Degraded`     | Instance is running but with reduced functionality (e.g., skill packs unavailable). |
| `Failed`       | A reconciliation error occurred. The controller will retry.          |
| `Terminating`  | The instance is being deleted. Finalizer cleanup is in progress.     |

Phase transitions follow this flow:

```
Pending --> Provisioning --> Running
                |               |
                v               v
             Failed         Degraded (e.g., skill packs unavailable)
                |               |
                v               v
           (retry)          (retry --> Running when resolved)

Deletion from any phase:
  * --> Terminating --> (removed)
```

### Conditions

The controller maintains fine-grained conditions using the standard `metav1.Condition` type:

| Condition Type       | Meaning                                          |
|----------------------|--------------------------------------------------|
| `Ready`              | Overall readiness of the instance.                |
| `ConfigValid`        | The configuration is valid and loaded.            |
| `DeploymentReady`    | The Deployment has at least one ready replica.    |
| `ServiceReady`       | The Service has been created.                     |
| `NetworkPolicyReady` | The NetworkPolicy has been applied.               |
| `RBACReady`          | ServiceAccount, Role, and RoleBinding exist.      |
| `StorageReady`       | The PVC has been created (or an existing one set).|
| `BackupComplete`     | The backup job completed successfully.            |
| `RestoreComplete`    | The restore job completed successfully.           |
| `ScheduledBackupReady`| The periodic backup CronJob is configured.       |
| `AutoUpdateAvailable`| A newer version is available in the OCI registry. |
| `SecretsReady`       | All referenced Secrets exist and are accessible.  |
| `SkillPacksReady`    | Skill packs resolved from GitHub. `False` when GitHub is unreachable - instance runs without skill packs. |

### Endpoints

The status includes computed endpoints for direct access:

- `status.gatewayEndpoint` -- `<name>.<namespace>.svc:18789` (WebSocket gateway)
- `status.canvasEndpoint` -- `<name>.<namespace>.svc:18793` (Canvas HTTP server)

### Managed Resources

The `status.managedResources` section tracks the names of all created resources, useful for debugging and inventory.

## Security Model

Security is a first-class concern. The operator enforces multiple layers of defense.

### Pod Security

Every managed pod runs with a hardened security context:

- **Non-root execution**: `runAsUser: 1000`, `runAsGroup: 1000`, `runAsNonRoot: true`.
- **Dropped capabilities**: All Linux capabilities are dropped (`drop: ["ALL"]`).
- **No privilege escalation**: `allowPrivilegeEscalation: false`.
- **Seccomp profile**: `RuntimeDefault` seccomp profile on both pod and container levels.
- **Read-only root filesystem**: `false` by default because OpenClaw writes to `~/.openclaw/`, but configurable.
- **FSGroup**: Set to `1000` for consistent volume ownership.

The Chromium sidecar (if enabled) runs as UID `1001` with `readOnlyRootFilesystem: true`, using emptyDir volumes for `/tmp` and a memory-backed emptyDir for `/dev/shm`.

### Network Isolation

When `security.networkPolicy.enabled` is `true` (the default), the operator creates a NetworkPolicy that implements a default-deny posture with selective allowlisting:

**Ingress rules:**
- Allow traffic from the same namespace on ports 18789 (gateway) and 18793 (canvas) by default. Set `security.networkPolicy.allowSameNamespaceIngress: false` when explicit ingress rules own the application boundary.
- Allow traffic from explicitly listed namespaces (`allowedIngressNamespaces`).
- Allow traffic from explicitly listed CIDRs (`allowedIngressCIDRs`).

**Egress rules:**
- Allow DNS (UDP/TCP port 53) when `allowDNS` is `true` (default).
- Allow HTTPS (TCP port 443) to any destination -- required for AI provider API calls.
- Allow additional CIDRs specified in `allowedEgressCIDRs`.

### RBAC (Least Privilege)

Each instance gets its own ServiceAccount, Role, and RoleBinding. The Role grants only:

- `get` and `watch` on the instance's own ConfigMap (by `resourceNames` restriction).

Users can extend this with `additionalRules` in the spec if the workload needs broader permissions. The operator itself requires broader RBAC, but each managed workload follows least privilege.

### Operator Security

The operator pod itself runs with:

- `runAsNonRoot: true`, UID `65532` (nonroot distroless user).
- Read-only root filesystem.
- All capabilities dropped.
- Seccomp `RuntimeDefault`.
- HTTP/2 disabled by default to mitigate CVE-2023-44487 (Rapid Reset).

## Webhook Validation

The operator includes a validating and defaulting admission webhook.

### Validating Webhook

The validator blocks or warns on insecure configurations:

| Check                                    | Severity | Behavior                                              |
|------------------------------------------|----------|-------------------------------------------------------|
| `runAsUser: 0` (root)                    | Error    | Rejects the resource.                                |
| `runAsNonRoot: false`                    | Warning  | Admits with a warning.                                |
| NetworkPolicy disabled                   | Warning  | Admits with a warning.                                |
| Ingress without TLS                      | Warning  | Admits with a warning.                                |
| Ingress with `forceHTTPS: false`         | Warning  | Admits with a warning.                                |
| Chromium without image digest            | Warning  | Admits with a warning about supply chain risk.        |
| No `env` or `envFrom` configured         | Warning  | Warns that API keys are likely missing.               |
| `allowPrivilegeEscalation: true`         | Warning  | Admits with a warning.                                |
| Missing CPU or memory limits             | Warning  | Recommends setting both limits.                       |
| StorageClass changed after creation      | Error    | Rejects the update (immutable field).                 |

### Defaulting Webhook

The defaulter sets sensible values for unspecified fields:

| Field                      | Default Value                |
|----------------------------|------------------------------|
| `image.repository`         | `ghcr.io/openclaw/openclaw`  |
| `image.tag`                | `latest`                     |
| `image.pullPolicy`         | `IfNotPresent`               |
| `security.podSecurityContext` | UID/GID 1000, nonroot     |
| `security.containerSecurityContext` | No privilege escalation |
| `resources.requests.cpu`   | `500m`                       |
| `resources.requests.memory`| `1Gi`                        |
| `resources.limits.cpu`     | `2000m`                      |
| `resources.limits.memory`  | `4Gi`                        |
| `storage.persistence.enabled` | `true`                    |
| `storage.persistence.size` | `10Gi`                       |
| `networking.service.type`  | `ClusterIP`                  |

## Configuration Management

OpenClaw reads its settings from a JSON configuration file (`openclaw.json`). The operator supports two modes for providing this configuration.

### External ConfigMap Reference

Use `spec.config.configMapRef` to point to a pre-existing ConfigMap. The operator mounts the specified key (default `openclaw.json`) into the container at `/home/openclaw/.openclaw/openclaw.json`. In this mode, the operator does not create or manage the ConfigMap.

```yaml
spec:
  config:
    configMapRef:
      name: my-openclaw-config
      key: openclaw.json
```

### Inline Raw JSON

Use `spec.config.raw` to embed configuration directly in the CR. The operator creates a managed ConfigMap named `<instance>-config` containing the JSON, and mounts it into the container.

```yaml
spec:
  config:
    raw:
      mcpServers:
        my-server:
          command: npx
          args: ["-y", "@my/mcp-server"]
```

### Config Hash for Rollout

The operator computes a SHA-256 hash of the configuration and stores it as the annotation `openclaw.rocks/config-hash` on the pod template. When the configuration changes, the hash changes, which triggers a rolling update of the Deployment -- even though the Deployment spec itself has not changed. This ensures configuration changes are always picked up without manual restarts.
