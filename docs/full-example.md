---
title: Full Example
description: An end-to-end OpenClawInstance YAML demonstrating most spec fields.
---

# Full Example

The following is an end-to-end `OpenClawInstance` YAML demonstrating most available spec fields. See the [API Reference](api-reference.md) for detailed field documentation.

```yaml
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: my-assistant
  namespace: openclaw
spec:
  image:
    repository: ghcr.io/openclaw/openclaw
    tag: "0.5.0"
    pullPolicy: IfNotPresent
    pullSecrets:
      - name: ghcr-secret

  config:
    mergeMode: merge
    raw:
      mcpServers:
        filesystem:
          command: npx
          args: ["-y", "@modelcontextprotocol/server-filesystem", "/data"]

  workspace:
    initialDirectories:
      - tools/scripts
    initialFiles:
      CLAUDE.md: |
        # Project Instructions
        Use the filesystem MCP server for file operations.

  skills:
    - "mcp-server-fetch"
    - "npm:@openclaw/matrix"

  envFrom:
    - secretRef:
        name: openclaw-api-keys

  selfConfigure:
    enabled: true
    allowedActions:
      - skills
      - config

  resources:
    requests:
      cpu: "1"
      memory: 2Gi
    limits:
      cpu: "4"
      memory: 8Gi

  security:
    podSecurityContext:
      runAsUser: 1000
      runAsGroup: 1000
      fsGroup: 1000
      fsGroupChangePolicy: OnRootMismatch
      runAsNonRoot: true
    containerSecurityContext:
      allowPrivilegeEscalation: false
    networkPolicy:
      enabled: true
      allowedIngressNamespaces:
        - monitoring
      allowDNS: true
      additionalEgress:
        - to:
            - namespaceSelector:
                matchLabels:
                  app: postgres
          ports:
            - protocol: TCP
              port: 5432
    rbac:
      createServiceAccount: true
      serviceAccountAnnotations:
        eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/openclaw-role
    caBundle:
      configMapName: corporate-ca
      key: ca-bundle.crt

  storage:
    persistence:
      enabled: true
      storageClass: gp3
      size: 50Gi
      accessModes:
        - ReadWriteOnce

  chromium:
    enabled: true
    image:
      repository: chromedp/headless-shell
      tag: "stable"
    resources:
      requests:
        cpu: 500m
        memory: 1Gi
      limits:
        cpu: "2"
        memory: 4Gi
    persistence:
      enabled: true
      size: 2Gi

  ollama:
    enabled: true
    models:
      - llama3.2
      - nomic-embed-text
    resources:
      requests:
        cpu: "2"
        memory: 4Gi
      limits:
        cpu: "8"
        memory: 16Gi
    storage:
      sizeLimit: 40Gi
    gpu: 1

  networking:
    service:
      type: ClusterIP
    ingress:
      enabled: true
      className: nginx
      hosts:
        - host: openclaw.example.com
          paths:
            - path: /
              pathType: Prefix
      tls:
        - hosts:
            - openclaw.example.com
          secretName: openclaw-tls
      security:
        forceHTTPS: true
        enableHSTS: true
        rateLimiting:
          enabled: true
          requestsPerSecond: 20

  probes:
    liveness:
      enabled: true
      initialDelaySeconds: 60
      periodSeconds: 15
    readiness:
      enabled: true
      initialDelaySeconds: 10
    startup:
      enabled: true
      failureThreshold: 60

  observability:
    metrics:
      enabled: true
      serviceMonitor:
        enabled: true
        interval: 15s
        labels:
          release: prometheus
    logging:
      level: info
      format: json

  availability:
    podDisruptionBudget:
      enabled: true
      maxUnavailable: 1
    nodeSelector:
      node-type: compute
    tolerations:
      - key: dedicated
        operator: Equal
        value: openclaw
        effect: NoSchedule
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              topologyKey: kubernetes.io/hostname
              labelSelector:
                matchLabels:
                  app.kubernetes.io/name: openclaw

  runtimeDeps:
    pnpm: true
    python: true
    uvImage:
      repository: ghcr.io/astral-sh/uv
      tag: 0.6-bookworm-slim
      # digest: sha256:...

  gateway:
    existingSecret: my-gateway-token

  autoUpdate:
    enabled: true
    checkInterval: 12h
    backupBeforeUpdate: true
    rollbackOnFailure: true
    healthCheckTimeout: 15m
```
