<p align="center">
  <img src="docs/images/banner.svg" alt="OpenClaw Kubernetes Operator — OpenClaws sailing the Kubernetes seas" width="100%">
</p>

# OpenClaw Kubernetes Operator

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/paperclipinc/openclaw-operator)](https://goreportcard.com/report/github.com/paperclipinc/openclaw-operator)
[![CI](https://github.com/paperclipinc/openclaw-operator/actions/workflows/ci.yaml/badge.svg)](https://github.com/paperclipinc/openclaw-operator/actions/workflows/ci.yaml)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.28%2B-326CE5?logo=kubernetes&logoColor=white)](https://kubernetes.io)
[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)](https://go.dev)

**Self-host [OpenClaw](https://openclaw.ai) AI agents on Kubernetes with production-grade security, observability, and lifecycle management.**

OpenClaw is an AI agent platform that acts on your behalf across Telegram, Discord, WhatsApp, and Signal. It manages your inbox, calendar, smart home, and more through 50+ integrations. While [Paperclip Inc.](https://paperclip.inc) offers fully managed hosting, this operator lets you run OpenClaw on your own infrastructure with the same operational rigor.

---

## Why an Operator?

Deploying AI agents to Kubernetes involves more than a Deployment and a Service. You need network isolation, secret management, persistent storage, health monitoring, optional browser automation, and config rollouts, all wired correctly. This operator encodes those concerns into a single `OpenClawInstance` custom resource so you can go from zero to production in minutes:

```yaml
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: my-agent
spec:
  envFrom:
    - secretRef:
        name: openclaw-api-keys
  storage:
    persistence:
      enabled: true
      size: 10Gi
```

The operator reconciles this into a fully managed stack of 9+ Kubernetes resources: secured, monitored, and self-healing.

## Agents That Adapt Themselves

Agents can autonomously install skills, patch their config, add environment variables, and seed workspace files - all through the Kubernetes API, validated by the operator on every request.

```yaml
# 1. Enable self-configure on the instance
spec:
  selfConfigure:
    enabled: true
    allowedActions: [skills, config, envVars, workspaceFiles]
```

```yaml
# 2. The agent creates this to install a skill at runtime
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawSelfConfig
metadata:
  name: add-fetch-skill
spec:
  instanceRef: my-agent
  addSkills:
    - "@anthropic/mcp-server-fetch"
```

Every request is validated against the instance's allowlist policy. Protected config keys cannot be overwritten, and denied requests are logged with a reason. See [Self-configure](#self-configure) for details.

> **Note:** Without `selfConfigure` enabled, config or skill changes made by the agent inside the container won't trigger a pod restart. You'll need to restart the pod manually (e.g. `kubectl delete pod <pod-name>`) for changes to take effect.

## Features

| | Feature | Details |
|---|---|---|
| **Declarative** | Single CRD | One resource defines the entire stack: StatefulSet, Service, RBAC, NetworkPolicy, PVC, PDB, Ingress, and more |
| **Adaptive** | Agent self-configure | Agents autonomously install skills, patch config, and adapt their environment via the K8s API - every change validated against an allowlist policy |
| **Secure** | Hardened by default | Non-root (UID 1000), read-only root filesystem, all capabilities dropped, seccomp RuntimeDefault, default-deny NetworkPolicy, validating webhook |
| **Observable** | Built-in metrics | Prometheus metrics, ServiceMonitor integration, structured JSON logging, Kubernetes events |
| **Flexible** | Provider-agnostic config | Use any AI provider (Anthropic, OpenAI, or others) via environment variables and inline or external config |
| **Config Modes** | Merge or overwrite | `overwrite` replaces config on restart; `merge` deep-merges with PVC config, preserving runtime changes. Config is restored on every container restart via init container. |
| **Force Paths** | Operator-owned paths under merge | `config.forcePaths` lists dot-paths the init container rebuilds from the CR on every restart even under `mergeMode: merge` -- lets managed deployers keep operator-owned config (auth, allowed providers, sandbox image) immune to tenant edits while user-owned config persists |
| **Skills** | Declarative install | Install ClawHub skills, npm packages, or GitHub-hosted skill packs via `spec.skills` - supports `npm:` and `pack:` prefixes. Additional workspaces can declare their own workspace-scoped skills via `additionalWorkspaces[].skills` |
| **Plugins** | Declarative install | Install OpenClaw plugins via `spec.plugins` - resolved through the OpenClaw CLI ClawHub installer in a secure init container |
| **Runtime Deps** | pnpm & Python/uv | Built-in init containers install pnpm (via corepack) or Python 3.12 + uv for MCP servers and skills |
| **Auto-Update** | OCI registry polling | Opt-in version tracking: checks the registry for new semver releases, backs up first, rolls out, and auto-rolls back if the new version fails health checks |
| **Scalable** | Auto-scaling | HPA integration with CPU and memory metrics, min/max replica bounds, automatic StatefulSet replica management |
| **Operational** | Instance suspension | Scale to zero with `spec.suspended: true` - all non-runtime resources remain managed, resume instantly with `false` |
| **Resilient** | Self-healing lifecycle | PodDisruptionBudgets, health probes, automatic config rollouts via content hashing, 5-minute drift detection |
| **Disk-Aware Readiness** | Opt-in ENOSPC guard | `spec.probes.diskReadiness` renders the readiness probe as an exec check that ANDs the gateway `/readyz` signal with a workspace writability + free-space check, so a full or read-only PVC drains the pod from Service endpoints instead of silently accepting writes it cannot persist. Liveness/startup stay HTTP so a full disk never turns into a CrashLoopBackOff. Defaulted off. |
| **Backup/Restore** | S3-backed snapshots | Automatic backup to S3-compatible storage on deletion, pre-update, and on a cron schedule; restore into a new instance from any snapshot |
| **Workspace Seeding** | Initial files & dirs | Pre-populate the workspace with files and directories before the agent starts; reference an external ConfigMap for GitOps workflows |
| **Gateway Auth** | Auto-generated tokens | Automatic gateway token Secret per instance, bypassing mDNS pairing (unusable in k8s) |
| **Tailscale** | Tailnet access | Expose via Tailscale Serve or Funnel with SSO auth - no Ingress needed |
| **Extensible** | Sidecars & init containers | Chromium for browser automation, Ollama for local LLMs, Tailscale for tailnet access, plus custom init containers and sidecars |
| **Cloud Native** | SA annotations & CA bundles | AWS IRSA / GCP Workload Identity via ServiceAccount annotations; CA bundle injection for corporate proxies |
| **Cluster Defaults** | Singleton CR | `OpenClawClusterDefaults` (name `cluster`) fills in unset instance fields - ideal for air-gapped / China regions where every instance would otherwise duplicate the same registry + mirror env boilerplate. Per-instance fields always win. |
| **Zombie Reaping** | Shared PID namespace | `spec.shareProcessNamespace` defaults to `true` so the pause container becomes PID 1 and reaps defunct helper processes from QMD, git, plugins, and shells - no custom init image needed |


## Architecture

```
+-----------------------------------------------------------------+
|  OpenClawInstance CR          OpenClawSelfConfig CR              |
|  (your declarative config)   (agent self-modification requests) |
+---------------+-------------------------------------------------+
                | watch
                v
+-----------------------------------------------------------------+
|  OpenClaw Operator                                              |
|  +-----------+  +-------------+  +----------------------------+ |
|  | Reconciler|  |   Webhooks  |  |   Prometheus Metrics       | |
|  |           |  |  (validate  |  |  (reconcile count,         | |
|  |  creates ->  |   & default)|  |   duration, phases)        | |
|  +-----------+  +-------------+  +----------------------------+ |
+---------------+-------------------------------------------------+
                | manages
                v
+-----------------------------------------------------------------+
|  Managed Resources (per instance)                               |
|                                                                 |
|  ServiceAccount -> Role -> RoleBinding    NetworkPolicy         |
|  ConfigMap        PVC      PDB            ServiceMonitor        |
|  GatewayToken Secret                                            |
|                                                                 |
|  StatefulSet                                                    |
|  +-----------------------------------------------------------+ |
|  | Init: config -> pnpm* -> python* -> skills* -> custom      | |
|  |                                        (* = opt-in)        | |
|  +------------------------------------------------------------+ |
|  | OpenClaw Container  Gateway Proxy (nginx)                  | |
|  |                     Chromium (opt) / Ollama (opt)          | |
|  |                     Tailscale (opt) + custom sidecars      | |
|  +------------------------------------------------------------+ |
|                                                                 |
|  Service (default: 18789, 18793 or custom) -> Ingress (opt)     |
+-----------------------------------------------------------------+
```

## Quick Start

### Prerequisites

- Kubernetes 1.28+
- Helm 3

### 1. Install the operator

```bash
helm install openclaw-operator \
  oci://ghcr.io/paperclipinc/charts/openclaw-operator \
  --namespace openclaw-operator-system \
  --create-namespace
```

<details>
<summary>Alternative: install with Kustomize</summary>

```bash
# Install CRDs
make install

# Deploy the operator
make deploy IMG=ghcr.io/paperclipinc/openclaw-operator:latest
```

</details>

<details>
<summary>Restrict the operator to specific namespaces</summary>

To run the operator with namespaced RBAC instead of cluster-wide permissions,
list the namespaces it should watch. The chart switches the namespace-scoped
permissions from a `ClusterRole`/`ClusterRoleBinding` to per-namespace
`Role`/`RoleBinding`, and passes `--watch-namespaces` to the operator so its
informer cache is scoped to that list (plus the operator's own namespace, for
backup credentials). A small `ClusterRole`/`ClusterRoleBinding` is still created
for the cluster-scoped `OpenClawClusterDefaults` resource, which the operator
watches regardless of namespace scoping -- a namespaced `Role` cannot grant
access to a cluster-scoped resource.

```bash
helm install openclaw-operator \
  oci://ghcr.io/paperclipinc/charts/openclaw-operator \
  --namespace openclaw-operator-system \
  --create-namespace \
  --set 'watchNamespaces={team-a,team-b}'
```

Each listed namespace must already exist; the chart does not create them.

To bring your own RBAC entirely (e.g. managed by a separate controller or
SecurityCenter policy), disable chart-managed RBAC:

```bash
helm install openclaw-operator \
  oci://ghcr.io/paperclipinc/charts/openclaw-operator \
  --namespace openclaw-operator-system \
  --create-namespace \
  --set rbac.create=false
```

The kubebuilder markers in `internal/controller/` and the manager rules helper
at `charts/openclaw-operator/templates/_helpers.tpl` document the minimum
permission set the operator requires.

</details>

### 2. Create a secret with your API keys

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: openclaw-api-keys
type: Opaque
stringData:
  ANTHROPIC_API_KEY: "sk-ant-..."
```

### 3. Deploy an OpenClaw instance

```yaml
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: my-agent
spec:
  envFrom:
    - secretRef:
        name: openclaw-api-keys
  storage:
    persistence:
      enabled: true
      size: 10Gi
```

```bash
kubectl apply -f secret.yaml -f openclawinstance.yaml
```

### 4. Verify

```bash
kubectl get openclawinstances
# NAME       PHASE     AGE
# my-agent   Running   2m

kubectl get pods
# NAME         READY   STATUS    AGE
# my-agent-0   1/1     Running   2m
```

## Configuration

### Inline config (openclaw.json)

```yaml
spec:
  config:
    raw:
      agents:
        defaults:
          model:
            primary: "anthropic/claude-sonnet-4-20250514"
          sandbox: true
      session:
        scope: "per-sender"
```

### External ConfigMap reference

```yaml
spec:
  config:
    configMapRef:
      name: my-openclaw-config
      key: openclaw.json
```

Config changes are detected via SHA-256 hashing and automatically trigger a rolling update. No manual restart needed.

### Gateway proxy

By default, each pod includes an nginx reverse proxy sidecar that forwards traffic to the OpenClaw gateway on loopback. Set `spec.gateway.enabled: false` to disable it:

- Health probes and Service ports target the gateway directly on port 18789
- `gateway.bind` is set to `0.0.0.0` instead of loopback
- The `gateway-proxy` container and its tmp volume are omitted from the pod
- To replace the built-in proxy with your own (e.g., Envoy, a signing proxy), disable it and add your proxy via `spec.sidecars`
- **Warning:** Do not set `gateway.bind: loopback` in your config JSON when the proxy is disabled - the gateway will only listen on `127.0.0.1` with nothing forwarding external traffic, making the pod unreachable. The operator emits a `GatewayBindConflict` warning event if this misconfiguration is detected.
- **TLS:** When the proxy is disabled, the gateway serves plaintext `ws://` on `0.0.0.0`. Ensure your replacement proxy or Ingress handles TLS termination to avoid exposing unencrypted WebSocket traffic (CWE-319).

### Disk-aware readiness

By default the readiness probe is an HTTP `GET /readyz` against the gateway. For PVC-backed instances, `/readyz` can stay green while the workspace volume is full or read-only (ENOSPC), so the pod keeps receiving traffic while workspace writes fail. Enable the opt-in disk-aware readiness guard to turn the readiness probe into an exec check that combines the gateway `/readyz` signal with a workspace writability and free-space check:

```yaml
spec:
  probes:
    diskReadiness:
      enabled: true            # default: false (existing deployments are unchanged when unset)
      path: /home/openclaw/.openclaw   # optional; defaults to the workspace data mount
      minFree: 128Mi           # optional; minimum free space, a Kubernetes quantity (default 64Mi)
```

- When enabled, the readiness probe becomes `sh -c` exec script that (a) verifies `path` is writable (`test -w`), (b) checks free space with `df` against `minFree`, then (c) defers to the gateway `/readyz` on the same loopback port the HTTP probe would use. The pod is Ready only if all checks pass; the script fails closed (non-zero exit) on any failure.
- **Liveness and startup stay HTTP-only** (`GET /healthz`), so a full PVC yields `NotReady` (draining the pod from Service endpoints) rather than a restart loop / CrashLoopBackOff.
- The exec script uses only POSIX `sh`, `test`, `df`, and `awk`. The `/readyz` HTTP call uses `curl` or `wget` if present and is skipped gracefully if neither is in the image, so a missing HTTP client never makes a healthy pod permanently `NotReady` (disk checks still run).
- This is a secondary, defense-in-depth guard; the application-level `/readyz` endpoint remains the primary readiness signal.

### Gateway authentication

The operator automatically generates a gateway token Secret for each instance and injects it into both the config JSON (`gateway.auth.mode: token`) and the `OPENCLAW_GATEWAY_TOKEN` env var. This bypasses Bonjour/mDNS pairing, which is unusable in Kubernetes.

- The token is generated once and never overwritten - rotate it by editing the Secret directly
- If you set `gateway.auth.token` in your config or `OPENCLAW_GATEWAY_TOKEN` in `spec.env`, your value takes precedence
- To bring your own token Secret, set `spec.gateway.existingSecret` - the operator will use it instead of auto-generating one (the Secret must have a key named `token`)
- The operator automatically sets `gateway.controlUi.dangerouslyDisableDeviceAuth: true` - device pairing is incompatible with Kubernetes (users cannot approve pairing from inside a container, connections are always proxied, and mDNS is unavailable)
- **Do not set `gateway.mode: local`** in your config - this mode is for desktop installs and enforces device identity checks that cannot work behind a reverse proxy in Kubernetes
- When connecting to the Control UI through an Ingress, pass the gateway token in the URL fragment: `https://openclaw.example.com/#token=<your-token>`
- Since v2026.2.24, OpenClaw restricts `gateway.allowedOrigins` to same-origin by default - if accessing via a non-default hostname (e.g. Ingress), set `gateway.allowedOrigins: ["*"]` in your config

### Control UI allowed origins

The operator auto-injects `gateway.controlUi.allowedOrigins` so the Control UI works through reverse proxies without CORS errors. Origins are derived from:

- **Localhost** (always): `http://localhost:18789`, `http://127.0.0.1:18789` for port-forwarding
- **Ingress hosts**: scheme determined from TLS config (`https://` if TLS, `http://` otherwise)
- **Explicit extras**: `spec.gateway.controlUiOrigins` for custom proxy URLs

If you set `gateway.controlUi.allowedOrigins` directly in your config JSON, the operator will not override it.

### Chromium sidecar

Enable headless browser automation for web scraping, screenshots, and browser-based integrations:

```yaml
spec:
  chromium:
    enabled: true
    image:
      repository: chromedp/headless-shell  # default
      tag: "stable"
    resources:
      requests:
        cpu: "250m"
        memory: "512Mi"
      limits:
        cpu: "1000m"
        memory: "2Gi"
    # Pass extra flags to the Chromium process (appended to built-in anti-bot defaults)
    extraArgs:
      - "--user-agent=Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
    # Inject extra environment variables into the sidecar
    extraEnv:
      - name: DISPLAY
        value: ":99"
```

When enabled, the operator automatically:
- Injects a `CHROMIUM_URL` environment variable into the main container
- Configures browser profiles in the OpenClaw config - both `"default"` and `"chrome"` profiles are set to point at the sidecar's CDP endpoint, so browser tool calls work regardless of which profile name the LLM passes
- Sets up shared memory, security contexts, and health probes for the sidecar
- Applies anti-bot-detection flags by default (`--disable-blink-features=AutomationControlled`, `--disable-features=AutomationControlled`, `--no-first-run`)

#### Persistent browser profiles

By default, all browser state (cookies, localStorage, session tokens) is lost on pod restart. Enable persistence to retain browser profiles across restarts:

```yaml
spec:
  chromium:
    enabled: true
    persistence:
      enabled: true          # default: false
      storageClass: ""        # optional - uses cluster default if empty
      size: "1Gi"             # default: 1Gi
      existingClaim: ""       # optional - use a pre-existing PVC
```

When persistence is enabled, the operator creates a dedicated PVC and passes `--user-data-dir=/chromium-data` to Chrome so that cookies, localStorage, IndexedDB, cached credentials, and session tokens survive pod restarts. This is useful for authenticated browser automation, MFA-protected services, and long-running browser workflows.

**Security note:** Persistent browser profiles contain sensitive session tokens. The PVC has the same security posture as other instance volumes. Ensure your StorageClass supports encryption at rest for sensitive workloads.

### Ollama sidecar

Run local LLMs alongside your agent for private, low-latency inference without external API calls:

```yaml
spec:
  ollama:
    enabled: true
    models:
      - llama3.2
      - nomic-embed-text
    gpu: 1
    storage:
      sizeLimit: 30Gi
    resources:
      requests:
        cpu: "1"
        memory: "4Gi"
      limits:
        cpu: "4"
        memory: "16Gi"
```

When enabled, the operator:
- Injects an `OLLAMA_HOST` environment variable into the main container
- Pre-pulls specified models via an init container before the agent starts
- Configures GPU resource limits when `gpu` is set (`nvidia.com/gpu`)
- Mounts a model cache volume (emptyDir by default, or an existing PVC via `storage.existingClaim`)

See [Custom AI Providers](docs/custom-providers.md) for configuring OpenClaw to use Ollama models via environment variables.

### Web terminal sidecar

Provide browser-based shell access to running instances for debugging and inspection without requiring `kubectl exec`:

```yaml
spec:
  webTerminal:
    enabled: true
    readOnly: false
    credential:
      secretRef:
        name: my-terminal-creds
    resources:
      requests:
        cpu: "50m"
        memory: "64Mi"
      limits:
        cpu: "200m"
        memory: "128Mi"
```

When enabled, the operator:
- Injects a [ttyd](https://github.com/tsl0922/ttyd) sidecar container on port 7681
- Mounts the instance data volume at `/home/openclaw/.openclaw` so you can inspect config, logs, and data files
- Adds the web terminal port to the Service and NetworkPolicy for external access
- Supports basic auth via a Secret with `username` and `password` keys
- Supports read-only mode (`readOnly: true`) for production environments where shell input should be disabled

### Tailscale integration

Expose your instance via [Tailscale](https://tailscale.com) Serve (tailnet-only) or Funnel (public internet) - no Ingress or LoadBalancer needed:

```yaml
spec:
  tailscale:
    enabled: true
    mode: serve          # "serve" (tailnet only) or "funnel" (public internet)
    authKeySecretRef:
      name: tailscale-auth
    authSSO: true        # allow passwordless login for tailnet members
    hostname: my-agent   # defaults to instance name
    image:
      repository: ghcr.io/tailscale/tailscale  # default
      tag: latest
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
      limits:
        cpu: 200m
        memory: 256Mi
```

When enabled, the operator runs a **Tailscale sidecar** (`tailscaled`) that handles serve/funnel declaratively via `TS_SERVE_CONFIG`. An **init container** copies the `tailscale` CLI binary to a shared volume so the main container can call `tailscale whois` for SSO authentication. The sidecar runs in userspace mode (`TS_USERSPACE=true`) - no `NET_ADMIN` capability needed.

**State persistence:** Tailscale node identity and TLS certificates are automatically persisted to a Kubernetes Secret (`<instance>-ts-state`) via `TS_KUBE_SECRET`. This prevents hostname incrementing (device-1, device-2, ...) and Let's Encrypt certificate re-issuance across pod restarts. The operator pre-creates the state Secret, grants the pod's ServiceAccount `get/update/patch` access to it, and mounts the SA token automatically.

Use ephemeral+reusable auth keys from the [Tailscale admin console](https://login.tailscale.com/admin/settings/keys). When `authSSO` is enabled, tailnet members can authenticate without a gateway token.

### Config merge mode

By default, the operator overwrites the config file on every pod restart. Set `mergeMode: merge` to deep-merge operator config with existing PVC config, preserving runtime changes made by the agent:

```yaml
spec:
  config:
    mergeMode: merge
    raw:
      agents:
        defaults:
          model:
            primary: "anthropic/claude-sonnet-4-20250514"
```

**Caveat:** In merge mode, removing a key from the CR does not remove it from the PVC config - the old value persists because deep-merge only adds or updates keys. If you need to remove stale config keys (e.g., after removing `gateway.mode: local`), temporarily switch to `mergeMode: overwrite`, apply, wait for the pod to restart, then switch back to `merge`.

### Partial overwrite under merge mode (`forcePaths`)

Under `mergeMode: merge` the operator preserves runtime changes the agent (or a tenant via the Control UI) wrote into the config file. For managed multi-tenant deployments this is a problem: a tenant can persist arbitrary values into operator-owned subtrees -- for example `models.providers.<rogue>.apiKey` -- and route inference through their own third-party key while consuming the deployer's compute.

`spec.config.forcePaths` is the partial-overwrite escape hatch. For each listed dot-path the init container deletes that subtree from the PVC config and re-applies it from `spec.config.raw` on every pod restart, so listed paths always match the CR while everything else still persists.

```yaml
spec:
  config:
    mergeMode: merge
    forcePaths:
      - gateway
      - models.providers
      - agents.defaults.sandbox
    raw:
      gateway:
        auth:
          mode: token
      models:
        providers:
          openai:
            baseUrl: "https://api.openai.com"
      agents:
        defaults:
          sandbox: true
```

With the above, `channels.*`, `settings.*`, and any other user-owned path the agent writes via the Control UI persists across pod restarts; `gateway.*`, `models.providers.*`, and `agents.defaults.sandbox` are rebuilt from the CR on every reconcile.

`forcePaths` is only valid under `mergeMode: merge`. The validating webhook rejects it under `overwrite` (where the whole file is already rebuilt every restart) and rejects malformed paths (empty segments, leading or trailing dot, characters outside `[a-zA-Z0-9._-]`). The same logic runs in both the init container (on pod restart) and the `postStart` lifecycle hook (on container restart without pod recreation), so an attacker cannot bypass the contract by triggering one form of restart over another.

### Skill installation

Install skills declaratively. The operator runs an init container that fetches each skill before the agent starts. Entries use ClawHub by default, or prefix with `npm:` to install from npmjs.com. ClawHub installs are idempotent - if a skill is already installed (e.g., when using persistent storage), it is skipped rather than failing:

```yaml
spec:
  skills:
    - "@anthropic/mcp-server-fetch"       # ClawHub (default)
    - "npm:@openclaw/matrix"              # npm package from npmjs.com
```

npm lifecycle scripts are disabled globally on the init container (`NPM_CONFIG_IGNORE_SCRIPTS=true`) to mitigate supply chain attacks.

### Skill packs

Skill packs bundle multiple files (SKILL.md, scripts, config) into a single installable unit hosted on GitHub. Use the `pack:` prefix with `owner/repo/path` format:

```yaml
spec:
  skills:
    - "pack:paperclipinc/skills/image-gen"            # latest from default branch
    - "pack:paperclipinc/skills/image-gen@v1.0.0"     # pinned to tag
    - "pack:myorg/private-skills/custom-tool@main"       # private repo (requires GITHUB_TOKEN)
```

Packs are resolved in one of two modes:

**1. Manifest mode** (explicit) -- the pack path contains a `skillpack.json` describing which files to seed and where:

```json
{
  "files": {
    "skills/image-gen/SKILL.md": "SKILL.md",
    "skills/image-gen/scripts/generate.py": "scripts/generate.py"
  },
  "directories": ["skills/image-gen/scripts"],
  "config": {
    "image-gen": {"enabled": true}
  }
}
```

**2. Raw-repo mode** (autodiscovery) -- when no `skillpack.json` is present and the pack path contains a `SKILL.md`, the operator installs the entire directory verbatim into `skills/<basename>/` in the workspace. This is useful for multi-skill repositories like [fluxcd/agent-skills](https://github.com/fluxcd/agent-skills) that follow a conventional `skills/<name>/SKILL.md` layout without per-skill manifests:

```yaml
spec:
  skills:
    - "pack:fluxcd/agent-skills/skills/gitops-repo-audit@main"
    # installs every file under skills/gitops-repo-audit/ into the workspace
    # at skills/gitops-repo-audit/ (including nested assets, schemas, etc.)
```

Raw mode does not inject config entries into `config.raw.skills.entries` -- use manifest mode if you need that. The operator refuses to install if GitHub truncates the tree response for very large repositories (add a `skillpack.json` manifest in that case).

The operator resolves packs via the GitHub Contents + Git Trees APIs (cached for 5 minutes), seeds files into the workspace via the init container, and (in manifest mode) injects config entries into `config.raw.skills.entries` with user overrides taking precedence. Set `GITHUB_TOKEN` on the operator deployment for private repo access.

**Updating pack contents.** By default (`spec.skillPackUpdatePolicy: Replace`), pack-seeded files converge to the declared pack revision on every pod start: changing a pinned `@tag`/`@commit` (or pushing to a tracked branch) overwrites the seeded files, and files that are no longer part of any declared pack are removed. The operator tracks what it seeded in a manifest at `/data/.skillpack-manifest` on the data volume, so user-created workspace files are never touched. Files at pack-declared paths are operator-managed -- local edits to them are reverted on restart. Set `spec.skillPackUpdatePolicy: CreateOnly` to opt out and keep the legacy seed-once behavior (files are never overwritten or removed after first seeding; updating a pinned revision then has no effect on already-seeded files).

**Workspace-scoped skills (multi-agent).** Additional workspaces can declare their own skills with the same reference formats as `spec.skills`:

```yaml
spec:
  workspace:
    additionalWorkspaces:
      - name: secondary
        skills:
          - "pack:example-org/openclaw-skills/skills/example-skill@example-skill-v1.2.0"
          - "@acme/browser-use"          # ClawHub, installed into workspace-secondary/skills/
          - "npm:@acme/cli-tool"         # npm binaries are global (~/.local/bin), shared by all agents
```

`pack:` entries resolve exactly like top-level packs (private repos via `GITHUB_TOKEN`, pinned tags/commits) but seed into `~/.openclaw/workspace-<name>/` and are tracked in a per-workspace manifest (`/data/.skillpack-manifest-ws-<name>`), so `skillPackUpdatePolicy` applies per workspace. ClawHub entries are installed with `clawhub --workdir` so the skill lands in that workspace's `skills/` directory instead of the shared `/app/skills`. The same skill may be listed in multiple workspaces (paths are scoped); duplicates within one workspace's list are rejected. Changing a workspace's skills triggers a pod rollout, and the rollout hash is keyed by workspace name, so moving a skill between workspaces rolls out too.

### Plugin installation

Install plugins declaratively. The operator runs a dedicated init container that installs each plugin into `~/.openclaw/extensions/<name>/` before the agent starts, where `<name>` is the unscoped npm package basename (so `@openclaw/brave-plugin` becomes `~/.openclaw/extensions/brave-plugin/`):

```yaml
spec:
  plugins:
    - "@martian-engineering/lossless-claw"
    - "some-other-plugin"
```

Plugin entries are resolved through the OpenClaw CLI's ClawHub installer, not raw `npm install`. An optional `npm:` prefix is accepted for compatibility and stripped before installation, so `npm:@scope/plugin` and `@scope/plugin` both run as `openclaw plugins install clawhub:@scope/plugin`. Use `spec.skills` when you need npm package source selection for skills.

This is the layout the OpenClaw gateway's plugin discovery expects - it scans direct subdirectories of `~/.openclaw/extensions/` for plugin manifests and skips `node_modules/` entirely. The init container shells out to `openclaw plugins install clawhub:<pkg>` so plugins published with `workspace:*` dependency markers, such as the first-party `@openclaw/matrix`, resolve correctly. Raw `npm install` rejects those with `EUNSUPPORTEDPROTOCOL`.

npm lifecycle scripts are disabled globally on the init container (`NPM_CONFIG_IGNORE_SCRIPTS=true`) to mitigate supply chain attacks. The PVC backs `~/.openclaw/`, so installs persist across pod restarts.

> If you previously worked around the install-path bug by adding `plugins.load.paths` entries to your gateway config (pointing at `~/.openclaw/node_modules/<pkg>`), that workaround is no longer needed and can be removed - plugins now land in the documented location and are auto-discovered.

### Workspace seeding

Pre-populate the agent workspace with files and directories before the agent starts. Files can be provided inline or referenced from an external ConfigMap -- ideal for GitOps workflows where workspace content is managed alongside your manifests.

**Inline files:**

```yaml
spec:
  workspace:
    initialDirectories:
      - tools/scripts
    initialFiles:
      README.md: |
        # My Workspace
        This workspace is managed by OpenClaw.
      agents/AGENT.md: |              # nested paths are supported
        # Agent
      skills/redmine/SKILL.md: |
        # Skill
```

Keys may contain `/` for nested files; the operator encodes them for ConfigMap storage and recreates the directory layout when seeding the workspace. The same path safety rules as `initialDirectories` apply (no leading `/`, no `..`, no segment starting with `.`).

**External ConfigMap reference:**

```yaml
spec:
  workspace:
    configMapRef:
      name: my-workspace-files      # all keys become workspace files
    initialFiles:                    # inline files (override configMapRef)
      EXTRA.md: "additional content"
```

All keys in the referenced ConfigMap are written as files into the workspace directory. When both `configMapRef` and `initialFiles` are specified, inline files take precedence over ConfigMap entries with the same filename.

**Merge priority** (highest wins): operator-injected files > inline `initialFiles` > external `configMapRef` > skill packs.

**Disable operator-managed `BOOTSTRAP.md`:**

`BOOTSTRAP.md` is seeded on first boot to guide first-run agent onboarding (identity, user preferences, persona). OpenClaw deletes the file after applying it, so on every pod restart or config change the init container would re-copy it and the agent would re-run bootstrap. Opt out once bootstrap is done:

```yaml
spec:
  workspace:
    bootstrap:
      enabled: false
```

Defaults to `true`. `ENVIRONMENT.md`, self-configure files, and skill-pack files are not affected.

The operator sets a `WorkspaceReady` status condition to `False` when the referenced ConfigMap is missing or contains invalid filenames, and `True` once workspace files are seeded successfully. The controller watches external ConfigMaps for changes and re-reconciles automatically.

**How it works:** Workspace files are seeded once via an init container. The init container copies files from a read-only ConfigMap volume to the PVC. The main container only sees the PVC (writable), so agents can modify their workspace files and changes persist across pod restarts. ConfigMaps are never mounted directly on the main container.

**GitOps example with Kustomize:**

```yaml
# kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: my-namespace              # must match the instance namespace

generatorOptions:
  disableNameSuffixHash: true        # required - operator looks up by exact name

configMapGenerator:
  - name: my-workspace-files
    files:
      - workspace/SOUL.md
      - workspace/AGENT.md
```

> **Important:** Two kustomize settings are required when using `configMapGenerator` with `configMapRef`:
> - **`disableNameSuffixHash: true`** -- The operator looks up ConfigMaps by exact name. Kustomize's default hash suffix (e.g. `-57k7g4dthc`) would cause a `ConfigMapNotFound` error.
> - **`namespace`** -- Generated ConfigMaps must be in the same namespace as the instance. Without this, kustomize creates them in the `default` namespace.

**Additional workspaces (multi-agent):**

When running multiple agents with isolated workspaces, use `additionalWorkspaces` to seed files for each agent. Each entry seeds to `~/.openclaw/workspace-<name>/` -- set matching paths in `spec.config.raw.agents.list[].workspace`.

```yaml
spec:
  workspace:
    configMapRef:
      name: main-agent-workspace
    additionalWorkspaces:
      - name: scheduler
        configMapRef:
          name: scheduler-workspace
        initialFiles:
          SOUL.md: "I am the scheduler agent"
        initialDirectories:
          - tools
  config:
    raw:
      agents:
        list:
          - id: main
            name: "Main Agent"
          - id: scheduler
            name: "Scheduler Agent"
      bindings:
        - agentId: scheduler
          match:
            channel: discord
            peer:
              kind: channel
              id: "123456789"        # bind to a specific channel
```

Each additional workspace supports the same `configMapRef`, `initialFiles`, and `initialDirectories` as the default workspace, plus a `skills` list for workspace-scoped skill installation (see [Skill packs](#skill-packs)). Operator-injected `ENVIRONMENT.md` is included; `BOOTSTRAP.md` is not (only the default agent runs onboarding). Max 10 additional workspaces.

> **Seed-once behavior:** Workspace files (both default and additional) are only written on first boot when they don't already exist on the PVC. If an agent modifies its own SOUL.md or AGENT.md at runtime, those changes persist across pod restarts and are never overwritten by the ConfigMap content. To re-seed a file, delete it from the PVC first.

**Full GitOps example with multiple agents:**

```yaml
# kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: my-namespace

generatorOptions:
  disableNameSuffixHash: true

resources:
  - instance.yaml

configMapGenerator:
  - name: main-agent-workspace
    files:
      - agents/main/SOUL.md
      - agents/main/AGENT.md
  - name: scheduler-workspace
    files:
      - agents/scheduler/SOUL.md
      - agents/scheduler/TOOLS.md
```

### Self-configure

Allow agents to modify their own configuration by creating `OpenClawSelfConfig` resources via the K8s API. The operator validates each request against the instance's `allowedActions` policy before applying changes:

```yaml
spec:
  selfConfigure:
    enabled: true
    allowedActions:
      - skills        # add/remove skills
      - config        # patch openclaw.json
      - workspaceFiles # add/remove workspace files
      - envVars       # add/remove environment variables
```

When enabled, the operator:
- Grants the instance's ServiceAccount RBAC permissions to read its own CRD and create `OpenClawSelfConfig` resources
- Enables SA token automounting so the agent can authenticate with the K8s API
- Injects a `SELFCONFIG.md` skill file and `selfconfig.sh` helper script into the workspace
- Opens port 6443 egress in the NetworkPolicy for K8s API access

The agent creates a request like:

```yaml
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawSelfConfig
metadata:
  name: add-fetch-skill
spec:
  instanceRef: my-agent
  addSkills:
    - "@anthropic/mcp-server-fetch"
```

The operator validates the request, applies it to the parent `OpenClawInstance`, and sets the request's status to `Applied`, `Denied`, or `Failed`. Terminal requests are auto-deleted after 1 hour.

#### GitOps Coexistence

SelfConfig uses Kubernetes Server-Side Apply (SSA) with the field manager name `openclaw-selfconfig`. This enables safe coexistence with GitOps controllers (FluxCD, ArgoCD, etc.) that manage the same `OpenClawInstance` resource:

- **Per-item ownership** -- Skills (set items), env vars (map items by name), and workspace files (map fields) are tracked individually. A SelfConfig can add or remove only the items it owns without conflicting with items managed by other controllers.
- **Atomic ownership** -- The `config.raw` field is owned atomically. If a GitOps controller also manages `config.raw`, `ForceOwnership` transfers ownership to the SelfConfig field manager on apply.
- **Removal safety** -- When a SelfConfig attempts to remove an item owned by another field manager, the operator emits a `Warning` / `SelfConfigSkippedRemoval` event identifying the owning manager and includes the warning in the status message.
- **Non-SSA users are unaffected** -- If you do not use `selfConfigure`, no SSA field managers are created and existing workflows remain unchanged.

See the [API reference](docs/api-reference.md) for the full `OpenClawSelfConfig` CRD spec and `spec.selfConfigure` fields.

### Persistent storage

By default the operator creates a 10Gi PVC and retains it when the CR is deleted (orphan behavior). Override size, storage class, or retention:

```yaml
spec:
  storage:
    persistence:
      size: 20Gi
      storageClass: fast-ssd
      orphan: true   # default -- PVC is RETAINED when the CR is deleted
      # orphan: false  -- PVC is deleted with the CR (garbage collected)
```

To reuse an existing PVC (e.g., after restoring from a backup):

```yaml
spec:
  storage:
    persistence:
      existingClaim: my-agent-data
```

> **Retention is stateful data protection.** Because agent workspaces contain irreplaceable data such as memory, notebooks, and conversation history, the default is `orphan: true`. To re-attach a retained PVC to a new instance, set `existingClaim` to its name.

### Cluster-wide defaults (air-gapped / restricted networks)

For deployments where every instance needs the same registry mirror or the same package-mirror env vars (China regions, air-gapped clusters, private registries), set defaults once on a singleton `OpenClawClusterDefaults` resource and the operator will merge them into every `OpenClawInstance` at reconcile time:

```yaml
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawClusterDefaults
metadata:
  name: cluster  # name MUST be "cluster" - other names are ignored
spec:
  registry: "<account>.dkr.ecr.<region>.amazonaws.com.cn"
  env:
    - name: NPM_CONFIG_REGISTRY
      value: https://registry.npmmirror.com
    - name: PIP_INDEX_URL
      value: https://mirrors.aliyun.com/pypi/simple/
  runtimeDeps:
    python: true
```

**Precedence:** per-instance fields always win. A cluster default is only applied when the corresponding instance field is unset. For `spec.env`, cluster-default entries appear first and any instance entry with the same `name` overrides in place.

Changes to the singleton are watched and automatically re-reconcile every existing instance. Only the CR named `cluster` takes effect; other names are ignored so typos do not silently churn the fleet.

### Runtime dependencies

Enable built-in init containers that install pnpm or Python/uv to the data PVC for MCP servers and skills:

```yaml
spec:
  runtimeDeps:
    pnpm: true    # Installs pnpm via corepack
    python: true  # Installs Python 3.12 + uv
```

### Custom init containers and sidecars

Add custom init containers (run after operator-managed ones) and sidecar containers:

```yaml
spec:
  initContainers:
    - name: fetch-models
      image: curlimages/curl:8.5.0
      command: ["sh", "-c", "curl -o /data/model.bin https://..."]
      volumeMounts:
        - name: data
          mountPath: /data
  sidecars:
    - name: cloud-sql-proxy
      image: gcr.io/cloud-sql-connectors/cloud-sql-proxy:2.14.3
      args: ["--structured-logs", "my-project:us-central1:my-db"]
      ports:
        - containerPort: 5432
  sidecarVolumes:
    - name: proxy-creds
      secret:
        secretName: cloud-sql-proxy-sa
```

Reserved init container names (`init-config`, `init-pnpm`, `init-python`, `init-skills`, `init-ollama`) are rejected by the webhook. If your sidecar replaces the built-in gateway proxy, set `spec.gateway.enabled: false` to avoid running both.

### Extra volumes and mounts

Mount additional ConfigMaps, Secrets, or CSI volumes into the main container:

```yaml
spec:
  extraVolumes:
    - name: shared-data
      persistentVolumeClaim:
        claimName: shared-pvc
  extraVolumeMounts:
    - name: shared-data
      mountPath: /shared
```

### Ingress Basic Auth

Add HTTP Basic Authentication to the Ingress. The operator auto-generates a random password and stores it in a managed Secret:

```yaml
spec:
  networking:
    ingress:
      enabled: true
      className: nginx
      hosts:
        - host: my-agent.example.com
      security:
        basicAuth:
          enabled: true
          username: admin          # default: "openclaw"
          realm: "My Agent"        # default: "OpenClaw"
```

The generated Secret is named `<name>-basic-auth` and contains three keys: `auth` (htpasswd format for ingress controllers), `username`, and `password` (plaintext, for retrieving the auto-generated credentials). It is tracked in `status.managedResources.basicAuthSecret`. To use your own credentials, provide a pre-formatted htpasswd Secret:

```yaml
spec:
  networking:
    ingress:
      security:
        basicAuth:
          enabled: true
          existingSecret: my-htpasswd-secret  # must contain key "auth"
```

For Traefik ingress, a `Middleware` CRD resource is created automatically (requires Traefik CRDs installed).

### Gateway API HTTPRoute

As an alternative to Ingress, on clusters that use the [Gateway API](https://gateway-api.sigs.k8s.io/) you can expose the instance through an HTTPRoute (`gateway.networking.k8s.io/v1`). Set `spec.networking.httpRoute` to attach the OpenClaw gateway Service to one or more Gateways:

```yaml
spec:
  networking:
    httpRoute:
      enabled: true
      parentRefs:
        - name: external          # name of an existing Gateway
          namespace: gateway-system # optional, defaults to the instance namespace
          sectionName: https        # optional, a specific Gateway listener
      hostnames:
        - my-agent.example.com
      port: 18789                  # optional, defaults to the gateway port (18789)
      annotations:
        example.com/foo: bar       # optional
```

The operator builds a single `PathPrefix: /` rule routing to the instance Service. The HTTPRoute is named `<name>` and tracked in `status.managedResources.httpRoute`, with an `HTTPRouteReady` condition reflecting reconcile status. The Gateway API CRDs must be installed; if they are not, reconciliation is skipped and `HTTPRouteReady` reports `GatewayAPINotInstalled`. Set `enabled: false` (or remove the block) to delete a previously created HTTPRoute. See the [API reference](docs/api-reference.md#httproutespec) for all fields.

### Custom service ports

By default the operator creates a Service with the gateway (18789) and canvas (18793) ports. To expose custom ports instead (e.g., for a non-default application), set `spec.networking.service.ports`:

```yaml
spec:
  networking:
    service:
      type: ClusterIP
      ports:
        - name: http
          port: 3978
          targetPort: 3978
```

When `ports` is set, it fully replaces the default ports -- including the Chromium port if the sidecar is enabled. To keep the defaults alongside custom ports, include them explicitly. If `targetPort` is omitted it defaults to `port`. See the [API reference](docs/api-reference.md#specnetworkingservice) for all fields.

### CA bundle injection

Inject a custom CA certificate bundle for environments with TLS-intercepting proxies or private CAs:

```yaml
spec:
  security:
    caBundle:
      configMapName: corporate-ca-bundle  # or secretName
      key: ca-bundle.crt                  # default key name
```

The bundle is mounted into all containers and the `SSL_CERT_FILE` / `NODE_EXTRA_CA_CERTS` environment variables are set automatically.

### ServiceAccount annotations

Add annotations to the managed ServiceAccount for cloud provider integrations:

```yaml
spec:
  security:
    rbac:
      serviceAccountAnnotations:
        # AWS IRSA
        eks.amazonaws.com/role-arn: "arn:aws:iam::123456789:role/openclaw"
        # GCP Workload Identity
        # iam.gke.io/gcp-service-account: "openclaw@project.iam.gserviceaccount.com"
```

### Auto-update

Opt into automatic version tracking so the operator detects new releases and rolls them out without manual intervention:

```yaml
spec:
  autoUpdate:
    enabled: true
    checkInterval: "24h"         # how often to poll the registry (1h-168h)
    backupBeforeUpdate: true     # back up the PVC before applying an update
    rollbackOnFailure: true      # auto-rollback if the new version fails health checks
    healthCheckTimeout: "10m"    # how long to wait for the pod to become ready (2m-30m)
```

When enabled, the operator resolves `latest` to the highest stable semver tag on creation, then polls for newer versions on each `checkInterval`. Before updating, it optionally runs an S3 backup, then patches the image tag and monitors the rollout. If the pod fails to become ready within `healthCheckTimeout`, it reverts the image tag and (optionally) restores the PVC from the pre-update snapshot.

Safety mechanisms include failed-version tracking (skips versions that failed health checks), a circuit breaker (pauses after 3 consecutive rollbacks), and full data restore when `backupBeforeUpdate` is enabled. Auto-update is a no-op for digest-pinned images (`spec.image.digest`).

See `status.autoUpdate` for update progress: `kubectl get openclawinstance my-agent -o jsonpath='{.status.autoUpdate}'`

### Backup and restore

The operator uses [rclone](https://rclone.org/) to back up and restore PVC data to/from S3-compatible storage. All backup operations require a Secret named `s3-backup-credentials` in the **operator namespace**:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: s3-backup-credentials
  namespace: openclaw-operator-system
stringData:
  S3_ENDPOINT: "https://s3.us-east-1.amazonaws.com"
  S3_BUCKET: "my-openclaw-backups"
  S3_ACCESS_KEY_ID: "<key-id>"            # optional - omit for workload identity
  S3_SECRET_ACCESS_KEY: "<secret-key>"    # optional - omit for workload identity
  # S3_PROVIDER: "Other"    # optional - set to "AWS", "GCS", etc. for native credential chains
  # S3_REGION: "us-east-1"  # optional - needed for MinIO or providers with custom regions
```

Compatible with AWS S3, Backblaze B2, Cloudflare R2, MinIO, Wasabi, and any S3-compatible API.

**Cloud workload identity:** Omit `S3_ACCESS_KEY_ID` and `S3_SECRET_ACCESS_KEY` and set `S3_PROVIDER` (e.g., `AWS`, `GCS`) to use the provider's native credential chain. Set `spec.backup.serviceAccountName` to a workload identity-enabled ServiceAccount (IRSA, GKE Workload Identity, AKS Workload Identity) so backup Jobs inherit the cloud IAM role. See the [Workload Identity section](docs/api-reference.md#workload-identity-cloud-native-auth) in the API reference for a full example.

**When backups run automatically:**

- **On delete** - the operator backs up the PVC before removing any resources. Subject to `spec.backup.timeout` (default: 30m) - if the backup does not complete in time, it is skipped automatically. Add `openclaw.rocks/skip-backup: "true"` to skip immediately.
- **Before auto-update** - when `spec.autoUpdate.backupBeforeUpdate: true` (the default).
- **On a schedule** - when `spec.backup.schedule` is set (cron expression).

If the Secret does not exist, backups are silently skipped and operations proceed normally.

**Periodic scheduled backups:**

```yaml
spec:
  backup:
    schedule: "0 2 * * *"   # Daily at 2 AM UTC
    retentionDays: 7         # Keep 7 days of daily snapshots (default)
    historyLimit: 3          # Successful job runs to retain (default: 3)
    failedHistoryLimit: 1    # Failed job runs to retain (default: 1)
    timeout: "30m"           # Max time for pre-delete backup (default: 30m, min: 5m, max: 24h)
    serviceAccountName: ""   # Optional: IRSA/Pod Identity SA for backup Jobs
```

The operator creates a Kubernetes CronJob that runs rclone to sync PVC data to S3. The CronJob uses pod affinity to co-locate on the same node as the StatefulSet pod (required for RWO PVCs). Backups use an incremental sync strategy: data is synced to a fixed `latest` path (only changed files uploaded), a daily snapshot is taken, and snapshots older than `retentionDays` are automatically pruned.

**Restoring from backup:**

```yaml
spec:
  # Path recorded in status.lastBackupPath of the source instance
  restoreFrom: "backups/my-tenant/my-agent/2026-01-15T10:30:00Z"
```

The operator runs a restore job to populate the PVC before starting the StatefulSet, then clears `restoreFrom` automatically. Backup paths follow the format `backups/<tenantId>/<instanceName>/<timestamp>`.

**Clone / migrate an instance:** `restoreFrom` works on both existing and brand-new instances. To clone an instance across namespaces, create a new `OpenClawInstance` with `spec.restoreFrom` pointing to the source's backup path - the operator creates the PVC, runs the restore Job, then starts the StatefulSet. The new instance gets a fresh gateway token; the source is unaffected. The restore Job uses `spec.backup.serviceAccountName` when set, so workload identity (IRSA/Pod Identity) works for cross-namespace clones. For ArgoCD users, add `spec.restoreFrom` to `ignoreDifferences` since the operator auto-clears it after restore.

For full details see the [Backup and Restore section](docs/api-reference.md#backup-and-restore) in the API reference.

### What the operator manages automatically

These behaviors are always applied - no configuration needed:

| Behavior | Details |
|----------|---------|
| `gateway.bind` | When the gateway proxy sidecar is enabled (default), binds to loopback and an nginx reverse proxy handles external access. When disabled (`spec.gateway.enabled: false`), binds to `0.0.0.0` so the gateway is reachable directly. |
| Gateway auth token | Auto-generated Secret per instance; injected into config and env |
| Control UI origins | `gateway.controlUi.allowedOrigins` auto-injected from localhost + ingress hosts + `spec.gateway.controlUiOrigins` |
| `OPENCLAW_GATEWAY_HANDSHAKE_TIMEOUT_MS` | `10000` (10s) to work around upstream timeout regression in v2026.3.12 ([#46892](https://github.com/openclaw/openclaw/issues/46892)) |
| `OPENCLAW_DISABLE_BONJOUR=1` | Always set (mDNS does not work in Kubernetes) |
| Browser profiles | When Chromium is enabled, `"default"` and `"chrome"` profiles are auto-configured with the sidecar's CDP endpoint |
| Tailscale serve config | When Tailscale is enabled, a `tailscale-serve.json` key is added to the ConfigMap for the sidecar's `TS_SERVE_CONFIG` |
| Tailscale state persistence | When Tailscale is enabled, node identity and TLS certs are persisted to a `<instance>-ts-state` Secret via `TS_KUBE_SECRET` |
| Config hash rollouts | Config changes trigger rolling updates via SHA-256 hash annotation |
| Config restoration | The init container restores config on every pod restart (overwrite or merge mode) |

For the full list of configuration options, see the [API reference](docs/api-reference.md) and the [full sample YAML](config/samples/openclaw_v1alpha1_openclawinstance_full.yaml).

## Security

The operator follows a **secure-by-default** philosophy. Every instance ships with hardened settings out of the box, with no extra configuration needed.

### Defaults

- **Non-root execution**: containers run as UID 1000; root (UID 0) is blocked by the validating webhook (exception: Ollama sidecar requires root per the official image)
- **Read-only root filesystem**: enabled by default for the main container and the Chromium sidecar; the PVC at `~/.openclaw/` provides writable home, and a `/tmp` emptyDir handles temp files
- **All capabilities dropped**: no ambient Linux capabilities
- **Seccomp RuntimeDefault**: syscall filtering enabled
- **Default-deny NetworkPolicy**: only DNS (53) and HTTPS (443) egress allowed; ingress limited to same namespace
- **Minimal RBAC**: each instance gets its own ServiceAccount with read-only access to its own ConfigMap; operator can create/update Secrets only for operator-managed gateway tokens
- **No automatic token mounting**: `automountServiceAccountToken: false` on both ServiceAccounts and pod specs (enabled only when `selfConfigure` is active)
- **Secret validation**: the operator checks that all referenced Secrets exist and sets a `SecretsReady` condition
- **Security context propagation**: when `podSecurityContext.runAsNonRoot` is set to `false`, the operator propagates this to init containers and applicable sidecars (tailscale, web terminal) so there is no contradiction between pod-level and container-level settings. Self-consistent sidecars (gateway-proxy, chromium, ollama) retain their own security contexts. The `containerSecurityContext.runAsNonRoot` and `containerSecurityContext.runAsUser` fields allow granular control over the main container independently of the pod level.

### Validating webhook

| Check | Severity | Behavior |
|-------|----------|----------|
| `runAsUser: 0` | Error | Blocked: root execution not allowed |
| Reserved init container name | Error | `init-config`, `init-pnpm`, `init-python`, `init-skills`, `init-ollama` are reserved |
| Invalid skill name | Error | Only alphanumeric, `-`, `_`, `/`, `.`, `@` allowed (max 128 chars). `npm:` prefix for npm packages, `pack:` prefix for skill packs; bare `npm:` or `pack:` is rejected |
| Invalid CA bundle config | Error | Exactly one of `configMapName` or `secretName` must be set |
| JSON5 with inline raw config | Error | JSON5 requires `configMapRef` (inline must be valid JSON) |
| JSON5 with merge mode | Error | JSON5 is not compatible with `mergeMode: merge` |
| Invalid `checkInterval` | Error | Must be a valid Go duration between 1h and 168h |
| Invalid `healthCheckTimeout` | Error | Must be a valid Go duration between 2m and 30m |

<details>
<summary>Warning-level checks (deployment proceeds with a warning)</summary>

| Check | Behavior |
|-------|----------|
| NetworkPolicy disabled | Deployment proceeds with a warning |
| Ingress without TLS | Deployment proceeds with a warning |
| Chromium without digest pinning | Deployment proceeds with a warning |
| Ollama without digest pinning | Deployment proceeds with a warning |
| Web terminal without digest pinning | Deployment proceeds with a warning |
| Ollama runs as root | Required by official image; informational |
| Auto-update with digest pin | Digest overrides auto-update; updates won't apply |
| `readOnlyRootFilesystem` disabled | Proceeds with a security recommendation |
| No AI provider keys detected | Scans `env`/`envFrom` for known provider env vars |
| Unknown config keys | Warns on unrecognized top-level keys in `spec.config.raw` |

</details>

## Observability

### Prometheus metrics

| Metric | Type | Description |
|--------|------|-------------|
| `openclaw_reconcile_total` | Counter | Reconciliations by result (success/error) |
| `openclaw_reconcile_duration_seconds` | Histogram | Reconciliation latency |
| `openclaw_instance_phase` | Gauge | Current phase per instance |
| `openclaw_instance_info` | Gauge | Instance metadata for PromQL joins (always 1) |
| `openclaw_instance_ready` | Gauge | Whether instance pod is ready (1/0) |
| `openclaw_managed_instances` | Gauge | Total number of managed instances |
| `openclaw_resource_creation_failures_total` | Counter | Resource creation failures |
| `openclaw_autoupdate_checks_total` | Counter | Auto-update version checks by result |
| `openclaw_autoupdate_applied_total` | Counter | Successful auto-updates applied |
| `openclaw_autoupdate_rollbacks_total` | Counter | Auto-update rollbacks triggered |

When `metrics.enabled: true` (the default), the operator automatically configures a full metrics pipeline: it injects `diagnostics.otel` config into OpenClaw to push OTLP metrics to a lightweight OTel Collector sidecar (`otel/opentelemetry-collector`), which exposes a Prometheus scrape endpoint on the configured port (default 9090). No manual OpenClaw configuration is needed. If you already set `diagnostics.otel` in your instance config, the operator preserves your settings.

### ServiceMonitor

```yaml
spec:
  observability:
    metrics:
      enabled: true
      serviceMonitor:
        enabled: true
        interval: 15s
        labels:
          release: prometheus
```

### OTLP metrics export (operator)

The operator can push its own metrics (reconciliation counters, workqueue stats, client latencies, etc.) to any OTLP-compatible backend via gRPC. This bridges all Prometheus metrics to OpenTelemetry, running alongside the existing Prometheus scrape endpoint.

```yaml
# values.yaml
otlp:
  enabled: true
  endpoint: "otel-collector.observability.svc:4317"
  insecure: true  # set to false for TLS
```

The endpoint can also be configured via the `OTEL_EXPORTER_OTLP_ENDPOINT` environment variable. Metrics are pushed every 30 seconds. If the OTLP endpoint is unreachable, the operator logs a warning and continues operating normally.

### PrometheusRule (alerts)

Auto-provisions a PrometheusRule with 7 alerts including runbook URLs:

```yaml
spec:
  observability:
    metrics:
      prometheusRule:
        enabled: true
        labels:
          release: kube-prometheus-stack  # must match Prometheus ruleSelector
        runbookBaseURL: https://paperclip.inc/docs/operators/openclaw/runbooks  # default
```

Alerts: `OpenClawReconcileErrors`, `OpenClawInstanceDegraded`, `OpenClawSlowReconciliation`, `OpenClawPodCrashLooping`, `OpenClawPodOOMKilled`, `OpenClawPVCNearlyFull`, `OpenClawAutoUpdateRollback`

### Grafana dashboards

Auto-provisions two Grafana dashboard ConfigMaps (discovered via the `grafana_dashboard: "1"` label):

```yaml
spec:
  observability:
    metrics:
      grafanaDashboard:
        enabled: true
        folder: OpenClaw  # Grafana folder (default)
        labels:
          grafana_dashboard_instance: my-grafana  # optional extra labels
```

Dashboards:
- **OpenClaw Operator** - fleet overview with reconciliation metrics, instance table, workqueue, and auto-update panels
- **OpenClaw Instance** - per-instance detail with CPU, memory, storage, network, and pod health panels

### Auto-Scaling (HPA)

Enable horizontal pod auto-scaling to automatically adjust the number of replicas based on CPU and memory utilization:

```yaml
spec:
  availability:
    autoScaling:
      enabled: true
      minReplicas: 1
      maxReplicas: 10
      targetCPUUtilization: 80
      targetMemoryUtilization: 70  # optional
```

When enabled, the operator creates a `HorizontalPodAutoscaler` targeting the StatefulSet and sets the StatefulSet's replica count to nil so the HPA manages scaling. The HPA is deleted when auto-scaling is disabled.

When auto-scaling is combined with persistent storage:

- Each replica gets its own PVC via StatefulSet `VolumeClaimTemplates` (named `data-<instance>-<ordinal>`)
- PVCs inherit `size`, `storageClass`, and `accessModes` from `spec.storage.persistence`
- Retention policy is `Retain` for both scale-down and deletion -- data is preserved
- If auto-scaling is later disabled, per-replica PVCs become orphaned and must be cleaned up manually

### Instance Suspension

Temporarily scale an instance to zero replicas without deleting it:

```yaml
spec:
  suspended: true
```

When suspended:

- The StatefulSet scales to 0 replicas (pods terminate)
- All non-runtime resources (Service, ConfigMap, RBAC, NetworkPolicy, PVC) remain fully managed
- Phase becomes `Suspended`, Ready condition becomes `False`
- Auto-updates are paused until the instance is resumed
- `openclaw_instance_ready` metric reports `0`

Resume by setting `spec.suspended: false`. The instance returns to `Running` phase through the normal startup lifecycle.

> **Note:** `spec.suspended` and `spec.availability.autoScaling.enabled` are mutually exclusive. Disable auto-scaling before suspending.

### Topology Spread Constraints

Spread pods across topology domains (zones, nodes) for improved availability:

```yaml
spec:
  availability:
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: DoNotSchedule
        labelSelector:
          matchLabels:
            app.kubernetes.io/instance: my-instance
```

### Runtime Class

Schedule pods on alternative container runtimes (Kata Containers, gVisor, etc.) for VM-level isolation or security hardening:

```yaml
spec:
  availability:
    runtimeClassName: kata-fc
```

A matching `RuntimeClass` resource must exist in the cluster. If unset, the default container runtime is used.

### Pod Annotations

Merge extra annotations into the StatefulSet pod template. Operator-managed keys (`openclaw.rocks/config-hash`, `openclaw.rocks/secret-hash`) always take precedence and cannot be overridden.

Useful for cloud-provider hints, such as preventing GKE Autopilot from evicting long-running agent pods:

```yaml
spec:
  podAnnotations:
    cluster-autoscaler.kubernetes.io/safe-to-evict: "false"
```

### Shared Process Namespace

The OpenClaw gateway runs as PID 1 in its container, and the Node.js process does not call `waitpid()` on orphaned children. Long-running pods accumulate defunct processes from QMD memory indexing, git, plugin activity, and shells. With `shareProcessNamespace: true` (the default), all containers in the pod share a PID namespace, the Kubernetes `pause` container becomes PID 1, and zombies are reaped automatically.

Opt out per instance only if you need strict per-container PID isolation (and have a tini/dumb-init wrapper in your image):

```yaml
spec:
  shareProcessNamespace: false
```

Trade-off: with sharing enabled, every container in the pod can see and signal every other container's processes. A compromised sidecar (Tailscale, Ollama, browser, custom) could send signals to the gateway and vice versa. For most deployments the zombie-reaping benefit outweighs this, but if your threat model assumes mutually distrusting sidecars, set this to `false` and reap inside your image instead.

Phases: `Pending` -> `Restoring` -> `Provisioning` -> `Running` | `Updating` | `BackingUp` | `Degraded` | `Failed` | `Terminating`

## Deployment Guides

Platform-specific deployment guides are available for:

- [AWS EKS](docs/deployment.md#aws-eks)
- [Google GKE](docs/deployment.md#google-gke)
- [Azure AKS](docs/deployment.md#azure-aks)
- [Kind (local development)](docs/deployment.md#kind)

## Development

```bash
# Clone and set up
git clone https://github.com/paperclipinc/openclaw-operator.git
cd openclaw-operator
go mod download

# Generate code and manifests
make generate manifests

# Run tests
make test

# Run linter
make lint

# Run locally against a Kind cluster
kind create cluster
make install
make run
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full development guide.

## Roadmap

- **v1.0.0**: API graduation to `v1`, conformance test suite, semver constraints for auto-update, HPA integration, cert-manager integration, multi-cluster support

See the full [roadmap](ROADMAP.md) for details.

## Don't Want to Self-Host?

[Paperclip Inc.](https://paperclip.inc) offers fully managed hosting starting at **EUR 19/mo**. No Kubernetes cluster required. Setup, updates, and 24/7 uptime handled for you.

## Contributing

Contributions are welcome. Please open an issue to discuss significant changes before submitting a PR. See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Disclaimer: AI-Assisted Development

This repository is developed and maintained collaboratively by a human and [Claude Code](https://claude.ai/claude-code). This includes writing code, reviewing and commenting on issues, triaging bugs, and merging pull requests. The human reads everything and acts as the final guard, but Claude does the heavy lifting - from diagnosis to implementation to CI.

In the future, this repo may be fully autonomously operated, whether we humans like that or not.

## License

Apache License 2.0, the same license used by Kubernetes, Prometheus, and most CNCF projects. See [LICENSE](LICENSE) for details.
