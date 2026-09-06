# Troubleshooting

This guide covers common issues and how to diagnose them.

## Checking Operator Logs

The operator logs are the first place to look for any issue.

```bash
# Find the operator pod
kubectl get pods -n openclaw-system -l app.kubernetes.io/name=openclaw-operator

# Stream logs
kubectl logs -n openclaw-system -l app.kubernetes.io/name=openclaw-operator -f

# Show logs with increased verbosity
kubectl logs -n openclaw-system -l app.kubernetes.io/name=openclaw-operator --all-containers
```

## Checking Events

Kubernetes events provide a timeline of what happened to your resources.

```bash
# Events for a specific instance
kubectl describe openclawinstance my-assistant -n openclaw

# All events in the namespace (sorted by time)
kubectl get events -n openclaw --sort-by='.lastTimestamp'

# Watch events in real time
kubectl get events -n openclaw --watch
```

## Checking Instance Status

```bash
# Quick status overview
kubectl get openclawinstance -n openclaw

# Detailed status with conditions
kubectl get openclawinstance my-assistant -n openclaw -o yaml | grep -A 50 'status:'

# Check specific condition
kubectl get openclawinstance my-assistant -n openclaw \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")]}'
```

---

## Common Issues

### Instance Stuck in Pending

**Symptoms**: The instance stays in `Pending` phase and never transitions to `Provisioning`.

**Possible causes and solutions**:

1. **Operator is not running**:
   ```bash
   kubectl get pods -n openclaw-system
   ```
   Verify the operator pod is `Running` and ready. If it is in `CrashLoopBackOff`, check its logs.

2. **CRD not installed or outdated**:
   ```bash
   kubectl get crd openclawinstances.openclaw.rocks
   ```
   If the CRD is missing, install it:
   ```bash
   kubectl apply -f config/crd/bases/
   ```
   If you upgraded the operator but new fields (e.g. `selfConfigure`) are
   rejected as "field not declared in schema", the CRD is outdated. Upgrade
   the Helm chart or apply CRDs manually:
   ```bash
   kubectl apply --server-side -f config/crd/bases/
   ```

3. **RBAC issues with the operator**:
   ```bash
   kubectl auth can-i get openclawinstances --as=system:serviceaccount:openclaw-system:openclaw-operator -n openclaw
   ```
   Ensure the operator's ClusterRole has the required permissions. Reinstall the Helm chart if RBAC is missing.

4. **Webhook blocking the request**:
   Check for admission webhook errors in the API server logs or the operator logs. See the [Webhook Errors](#webhook-errors) section.

### Instance Stuck in Provisioning

**Symptoms**: The instance transitions to `Provisioning` but never reaches `Running`.

**Possible causes and solutions**:

1. **Resource creation failing silently**: Check operator logs for errors:
   ```bash
   kubectl logs -n openclaw-system deploy/openclaw-operator | grep -i error
   ```

2. **Resource quota exceeded**:
   ```bash
   kubectl describe resourcequota -n openclaw
   ```
   If quotas are preventing resource creation, either increase quotas or reduce the instance's resource requests.

3. **Deployment not becoming ready**: The reconciler waits for the Deployment to have ready replicas. Check the pod:
   ```bash
   kubectl get pods -n openclaw -l app.kubernetes.io/instance=my-assistant
   kubectl describe pod -n openclaw -l app.kubernetes.io/instance=my-assistant
   ```

### Instance in Failed State

**Symptoms**: The instance phase is `Failed`. The `Ready` condition shows `status: "False"` with a reason.

**Diagnosis**:

```bash
# Check the failure reason
kubectl get openclawinstance my-assistant -n openclaw \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")].message}'

# Check events
kubectl describe openclawinstance my-assistant -n openclaw
```

**Common failure reasons**:

1. **Image pull errors**:
   ```bash
   kubectl get pods -n openclaw -l app.kubernetes.io/instance=my-assistant -o wide
   kubectl describe pod <pod-name> -n openclaw
   ```
   Look for `ImagePullBackOff` or `ErrImagePull`. Verify:
   - The image repository and tag are correct.
   - Pull secrets are configured if using a private registry.
   - Network connectivity to the registry.

2. **Insufficient resources**:
   ```bash
   kubectl describe pod <pod-name> -n openclaw | grep -A 5 Events
   ```
   Look for `FailedScheduling` events. The cluster may not have nodes with enough CPU/memory. Reduce the resource requests or add capacity.

3. **ConfigMap or Secret not found**:
   If using `configMapRef`, verify the referenced ConfigMap exists:
   ```bash
   kubectl get configmap <name> -n openclaw
   ```
   If using `envFrom` with a Secret, verify the Secret exists:
   ```bash
   kubectl get secret <name> -n openclaw
   ```

### Instance in Degraded State (Skill Packs Unavailable)

**Symptoms**: The instance phase is `Degraded`. The `SkillPacksReady` condition shows `status: "False"` with reason `ResolutionFailed`. The instance is running but without skill packs.

**Diagnosis**:

```bash
# Check the SkillPacksReady condition
kubectl get openclawinstance my-assistant -n openclaw \
  -o jsonpath='{.status.conditions[?(@.type=="SkillPacksReady")]}'

# Check events for details
kubectl describe openclawinstance my-assistant -n openclaw | grep SkillPack
```

**Common causes**:

1. **GitHub API unreachable**: The operator fetches skill packs from GitHub. If GitHub is down or the cluster has no egress access, resolution fails. The instance provisions without skill packs and retries on the next reconcile (30s).

2. **Invalid pack reference**: Verify the `pack:` skill references are valid `owner/repo/path[@ref]` format:
   ```bash
   kubectl get openclawinstance my-assistant -n openclaw -o jsonpath='{.spec.skills}'
   ```

3. **Missing GITHUB_TOKEN**: Private skill pack repositories require a GitHub token. Verify the operator has the `GITHUB_TOKEN` environment variable set.

**Resolution**: The operator automatically retries skill pack resolution on every reconcile. Once GitHub is reachable again, the instance transitions from `Degraded` to `Running`. The operator also uses stale cache - if a previous successful resolution exists, it will use that data even after the cache TTL expires.

### NetworkPolicy Blocking Traffic

**Symptoms**: The instance is `Running` but cannot reach external APIs or other pods cannot reach the instance.

**Diagnosis**:

1. **Verify the NetworkPolicy exists**:
   ```bash
   kubectl get networkpolicy -n openclaw
   kubectl describe networkpolicy my-assistant -n openclaw
   ```

2. **Instance cannot reach AI APIs**: The default NetworkPolicy allows egress to port 443 (HTTPS) and port 53 (DNS). If the AI provider uses a non-standard port, add it to `allowedEgressCIDRs` or disable the NetworkPolicy temporarily to confirm:
   ```yaml
   spec:
     security:
       networkPolicy:
         allowedEgressCIDRs:
           - "0.0.0.0/0"
   ```

3. **DNS resolution failing**: If `allowDNS` was set to `false`, pods cannot resolve hostnames:
   ```yaml
   spec:
     security:
       networkPolicy:
         allowDNS: true
   ```

4. **Other pods cannot reach the instance**: By default, only pods in the same namespace can reach the instance. To allow cross-namespace traffic:
   ```yaml
   spec:
     security:
       networkPolicy:
         allowedIngressNamespaces:
           - ingress-nginx
           - monitoring
   ```

5. **Verify with a test pod**:
   ```bash
   kubectl run -n openclaw test-curl --rm -it --image=curlimages/curl -- \
     curl -v http://my-assistant:18789
   ```

### Instance Stuck in BackingUp Phase

**Symptoms**: After deleting an instance, it remains in `BackingUp` phase and is not deleted.

**Diagnosis**:

```bash
# Check the instance status
kubectl get openclawinstance my-agent -o jsonpath='{.status.phase}'
kubectl get openclawinstance my-agent -o jsonpath='{.status.backingUpSince}'

# Check if a backup Job exists and its status
kubectl get jobs -l openclaw.rocks/instance=my-agent
kubectl describe job my-agent-backup -n <namespace>

# Check events for timeout or failure
kubectl describe openclawinstance my-agent | grep -A5 Events
```

**Possible causes and solutions**:

1. **Backup timeout will resolve it automatically**: By default, the operator waits up to 30 minutes (`spec.backup.timeout`) before giving up and proceeding with deletion. Check `status.backingUpSince` to see when the phase started and how much time remains.

2. **Backup Job failed**: The Job may have failed due to S3 connectivity issues, incorrect credentials, or insufficient permissions. The operator retries until the timeout elapses. Check the Job logs:
   ```bash
   kubectl logs job/my-agent-backup -n <namespace>
   ```

3. **Pods stuck terminating**: The StatefulSet was scaled to 0 but pods are stuck. Check for finalizers or PodDisruptionBudgets:
   ```bash
   kubectl get pods -l openclaw.rocks/instance=my-agent -o yaml | grep finalizers
   ```

4. **Skip backup immediately**: To bypass the backup and delete immediately:
   ```bash
   kubectl annotate openclawinstance my-agent openclaw.rocks/skip-backup=true
   ```

5. **Increase or decrease the timeout**: Adjust `spec.backup.timeout` (min: 5m, max: 24h):
   ```yaml
   spec:
     backup:
       timeout: "1h"
   ```

### PVC Not Binding

**Symptoms**: The pod is stuck in `Pending` with `FailedScheduling` or the PVC shows `Pending`.

**Diagnosis**:

```bash
kubectl get pvc -n openclaw
kubectl describe pvc my-assistant-data -n openclaw
```

**Possible causes**:

1. **StorageClass does not exist**:
   ```bash
   kubectl get storageclass
   ```
   Verify the `storageClass` specified in the spec exists. If omitted, the cluster's default StorageClass is used.

2. **No capacity available**: The storage backend may be out of capacity. Check provisioner logs.

3. **Access mode incompatibility**: Some storage backends do not support `ReadWriteMany`. Use `ReadWriteOnce` (the default).

4. **Zone mismatch**: In multi-zone clusters, PVs may be zone-locked. Ensure nodes and storage are in the same zone, or use a StorageClass that supports multi-zone provisioning.

### Webhook Errors

**Symptoms**: Creating or updating an `OpenClawInstance` fails with a webhook error, such as `failed calling webhook` or `connection refused`.

**Diagnosis**:

1. **Webhook not enabled**: The webhook is optional. Verify it is configured:
   ```bash
   kubectl get validatingwebhookconfigurations | grep openclaw
   kubectl get mutatingwebhookconfigurations | grep openclaw
   ```

2. **cert-manager not installed or certificate not ready**: The webhook requires TLS certificates. If using cert-manager:
   ```bash
   kubectl get certificate -n openclaw-system
   kubectl describe certificate -n openclaw-system <cert-name>
   ```

3. **Webhook Service not reachable**: Verify the webhook Service and its endpoints:
   ```bash
   kubectl get svc -n openclaw-system | grep webhook
   kubectl get endpoints -n openclaw-system | grep webhook
   ```

4. **Bypass the webhook temporarily**: If the webhook is misconfigured and blocking all operations, delete the webhook configuration:
   ```bash
   kubectl delete validatingwebhookconfiguration openclaw-operator-validating-webhook
   kubectl delete mutatingwebhookconfiguration openclaw-operator-mutating-webhook
   ```
   Then fix the underlying issue and redeploy.

### Ingress Not Working

**Symptoms**: The Ingress is created but traffic does not reach the instance.

**Diagnosis**:

1. **IngressClass not found**:
   ```bash
   kubectl get ingressclass
   ```
   Verify the `className` in the spec matches an installed IngressClass.

2. **Ingress controller not installed**: An Ingress resource does nothing without a controller (nginx-ingress, Traefik, etc.):
   ```bash
   kubectl get pods -n ingress-nginx
   ```

3. **TLS Secret missing**: If TLS is configured, the referenced Secret must exist:
   ```bash
   kubectl get secret <secretName> -n openclaw
   ```

4. **DNS not pointing to the Ingress**: Verify DNS resolution for the configured host:
   ```bash
   nslookup openclaw.example.com
   ```
   The DNS should point to the Ingress controller's external IP or load balancer.

5. **NetworkPolicy blocking the Ingress controller**: If NetworkPolicy is enabled, the Ingress controller's namespace must be in `allowedIngressNamespaces`:
   ```yaml
   spec:
     security:
       networkPolicy:
         allowedIngressNamespaces:
           - ingress-nginx
   ```

6. **WebSocket connectivity**: The operator automatically adds WebSocket-related nginx annotations. If using a different Ingress controller, you may need to add controller-specific annotations for WebSocket support.

### Control UI Shows "device identity required"

**Symptoms**: The first Control UI connection through an Ingress fails with `code=1008 reason=device identity required`, and the UI says that device pairing is required.

This is the expected one-time browser pairing flow. Gateway token or trusted-proxy authentication admits the connection, while the browser device identity grants its scopes.

1. Reload the Control UI once so it creates a pending request.
2. List and approve the request from an administrative workstation:
   ```bash
   kubectl exec -n <namespace> <instance>-0 -c openclaw -- openclaw devices list
   kubectl exec -n <namespace> <instance>-0 -c openclaw -- openclaw devices approve <request-id>
   ```
3. Reconnect after approval completes.

Do not add `gateway.controlUi.dangerouslyDisableDeviceAuth`; current OpenClaw releases treat that setting as retired and inert. Supplying the gateway token does not replace device pairing.

If no pending request appears, verify that the reverse proxy preserves the WebSocket upgrade and any trusted identity headers, check `gateway.controlUi.allowedOrigins`, and inspect the OpenClaw container logs.

### Gateway Proxy "Connection Refused" on Startup

**Symptoms**: The gateway-proxy (nginx) container logs show `connect() failed (111: Connection refused)` immediately after pod startup.

**This is expected and harmless.** The nginx proxy sidecar starts before the OpenClaw gateway is fully listening. The connection refused errors resolve within a few seconds once the gateway binds to its port. No action is needed - subsequent connections will succeed.

### Chromium Sidecar Issues

**Symptoms**: The Chromium sidecar is not starting, crashing, or browser automation fails.

**Diagnosis**:

1. **Check sidecar status**:
   ```bash
   kubectl get pods -n openclaw -l app.kubernetes.io/instance=my-assistant -o json | \
     jq '.items[0].status.containerStatuses[] | select(.name=="chromium")'
   ```

2. **Check sidecar logs**:
   ```bash
   kubectl logs -n openclaw <pod-name> -c chromium
   ```

3. **Insufficient shared memory (`/dev/shm`)**: Chromium requires shared memory. The operator mounts a 256Mi memory-backed emptyDir at `/dev/shm`. If Chromium crashes with memory errors, increase the sidecar's memory limit:
   ```yaml
   spec:
     chromium:
       resources:
         limits:
           memory: 4Gi
   ```

4. **Insufficient resources**: Chromium is resource-intensive. The defaults (250m CPU, 512Mi memory request) may not be enough for heavy workloads. Increase the limits:
   ```yaml
   spec:
     chromium:
       resources:
         requests:
           cpu: "1"
           memory: 2Gi
         limits:
           cpu: "2"
           memory: 4Gi
   ```

5. **Security context restrictions**: The Chromium sidecar runs as UID 1001 with a read-only root filesystem and all capabilities dropped. Some Kubernetes environments (e.g., OpenShift) may impose additional restrictions. Check for SecurityContextConstraint (SCC) violations:
   ```bash
   kubectl describe pod <pod-name> -n openclaw | grep -i security
   ```

### Operator CrashLoopBackOff

**Symptoms**: The operator pod itself is restarting repeatedly.

**Diagnosis**:

```bash
kubectl logs -n openclaw-system deploy/openclaw-operator --previous
kubectl describe pod -n openclaw-system -l app.kubernetes.io/name=openclaw-operator
```

**Common causes**:

1. **Leader election failure**: If another instance holds the leader lock, check for stale leases:
   ```bash
   kubectl get lease -n openclaw-system
   ```

2. **Missing CRD**: If the CRD is not installed, the controller fails to start:
   ```bash
   kubectl get crd openclawinstances.openclaw.rocks
   ```

3. **Insufficient RBAC**: The operator needs cluster-wide permissions for certain resources. Verify the ClusterRole and ClusterRoleBinding are in place.

4. **Webhook certificate issues**: If the webhook is enabled but certificates are not provisioned, the server fails to start.

---

## Useful Commands Reference

```bash
# List all OpenClaw instances across namespaces
kubectl get openclawinstance -A

# Get managed resources for an instance
kubectl get openclawinstance my-assistant -n openclaw \
  -o jsonpath='{.status.managedResources}' | jq .

# Check if the operator can reach the API server
kubectl logs -n openclaw-system deploy/openclaw-operator | head -20

# Force a reconciliation by adding an annotation
kubectl annotate openclawinstance my-assistant -n openclaw \
  force-reconcile=$(date +%s) --overwrite

# Check Prometheus metrics from the operator
kubectl port-forward -n openclaw-system svc/openclaw-operator-metrics 8443:8443
# Then: curl -k https://localhost:8443/metrics

# Dump full instance status
kubectl get openclawinstance my-assistant -n openclaw -o yaml
```
