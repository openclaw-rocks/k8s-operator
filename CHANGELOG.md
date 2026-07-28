# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.38.1](https://github.com/paperclipinc/openclaw-operator/compare/v0.38.0...v0.38.1) (2026-07-20)


### Bug Fixes

* gate PhaseRunning on StatefulSetReady so consumers don't route to a not-ready instance ([#572](https://github.com/paperclipinc/openclaw-operator/issues/572)) ([2a1820c](https://github.com/paperclipinc/openclaw-operator/commit/2a1820c6daed065132819b4120ad76c679e373ad))

## [0.38.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.37.0...v0.38.0) (2026-07-19)


### Features

* **skills:** support declarative skills for additional workspaces ([#570](https://github.com/paperclipinc/openclaw-operator/issues/570)) ([efb202c](https://github.com/paperclipinc/openclaw-operator/commit/efb202ca6d1b30a9daebdf0463a7615973387ca9))

## [0.37.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.36.5...v0.37.0) (2026-07-18)


### Features

* **probes:** opt-in disk-aware readiness guard for PVC-backed workspaces ([#557](https://github.com/paperclipinc/openclaw-operator/issues/557)) ([fb98a92](https://github.com/paperclipinc/openclaw-operator/commit/fb98a92df51dbbdd9da45a0a699dab800d0baa20))


### Bug Fixes

* **probes:** compare disk readiness free space as integer KiB ([#569](https://github.com/paperclipinc/openclaw-operator/issues/569)) ([3f5dd83](https://github.com/paperclipinc/openclaw-operator/commit/3f5dd83d9a3d6415cc5db5626ddbfd389d7dd719))
* **skills:** converge PVC-seeded skill pack files to the declared revision ([#566](https://github.com/paperclipinc/openclaw-operator/issues/566)) ([35d2987](https://github.com/paperclipinc/openclaw-operator/commit/35d2987727ead34400ecf0d0417d170f1ee5aff7))
* **skills:** preserve owner-qualified ClawHub refs for disambiguation ([#559](https://github.com/paperclipinc/openclaw-operator/issues/559)) ([bb2f3aa](https://github.com/paperclipinc/openclaw-operator/commit/bb2f3aaeb8078d602bf38feb47d375aa045f13d1))

## [0.36.5](https://github.com/paperclipinc/openclaw-operator/compare/v0.36.4...v0.36.5) (2026-06-20)


### Bug Fixes

* **controller:** treat optimistic-lock conflicts as retryable, not failed ([#553](https://github.com/paperclipinc/openclaw-operator/issues/553)) ([#554](https://github.com/paperclipinc/openclaw-operator/issues/554)) ([07a040b](https://github.com/paperclipinc/openclaw-operator/commit/07a040b42fd43e20ac088b7e5c44adedf9f654c3))

## [0.36.4](https://github.com/paperclipinc/openclaw-operator/compare/v0.36.3...v0.36.4) (2026-06-16)


### Bug Fixes

* **operatorhub:** ship ci.yaml with updateGraph semver-mode ([#551](https://github.com/paperclipinc/openclaw-operator/issues/551)) ([ab5f444](https://github.com/paperclipinc/openclaw-operator/commit/ab5f444b646bb3a74f7b6cb66c5a85221e7e191e))

## [0.36.3](https://github.com/paperclipinc/openclaw-operator/compare/v0.36.2...v0.36.3) (2026-06-16)


### Bug Fixes

* **bundle:** declare com.redhat.openshift.versions for OpenShift cert ([#549](https://github.com/paperclipinc/openclaw-operator/issues/549)) ([9a66a7f](https://github.com/paperclipinc/openclaw-operator/commit/9a66a7f09ab3cc30a27afb5fcd66be9b11dca801))
* **ci:** sync OperatorHub fork to avoid workflow-scope push rejection ([#547](https://github.com/paperclipinc/openclaw-operator/issues/547)) ([4adde0e](https://github.com/paperclipinc/openclaw-operator/commit/4adde0edd49dcb4950aca332907c0cd6e03e6d59))

## [0.36.2](https://github.com/paperclipinc/openclaw-operator/compare/v0.36.1...v0.36.2) (2026-06-15)


### Bug Fixes

* **chart:** refresh changelog for signed-chart release (0.36.2) ([#545](https://github.com/paperclipinc/openclaw-operator/issues/545)) ([f99b35a](https://github.com/paperclipinc/openclaw-operator/commit/f99b35a42fcf57f4c91cb7c5f99f2ee636c53966))

## [0.36.1](https://github.com/paperclipinc/openclaw-operator/compare/v0.36.0...v0.36.1) (2026-06-08)


### Bug Fixes

* keep otel-collector metrics alive (metric_expiration 8760h + directory-mount) ([#538](https://github.com/paperclipinc/openclaw-operator/issues/538)) ([9e6cbe8](https://github.com/paperclipinc/openclaw-operator/commit/9e6cbe8d975abd0b904532e92a6ec9c65df2eb46))

## [0.36.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.35.0...v0.36.0) (2026-06-03)


### Features

* add optional Gateway API HTTPRoute support ([#535](https://github.com/paperclipinc/openclaw-operator/issues/535)) ([e56d4bb](https://github.com/paperclipinc/openclaw-operator/commit/e56d4bb322f1d4184ae72d0653d962532dea8935))
* tier-1/2 parity (operatorhub digest pin, verify-signing, image CEL, conformance) ([#533](https://github.com/paperclipinc/openclaw-operator/issues/533)) ([3787dd0](https://github.com/paperclipinc/openclaw-operator/commit/3787dd0153ae62ead33e2871b3998dbdc69604cc))


### Bug Fixes

* **helm:** grant cluster-scoped OpenClawClusterDefaults RBAC under watchNamespaces ([#531](https://github.com/paperclipinc/openclaw-operator/issues/531)) ([7c176eb](https://github.com/paperclipinc/openclaw-operator/commit/7c176eb8b542d67560452919dcd8a4bf8d6f7ae6))
* idempotent plugin install in init-plugins container ([#528](https://github.com/paperclipinc/openclaw-operator/issues/528)) ([58280c8](https://github.com/paperclipinc/openclaw-operator/commit/58280c8bea4d329b86d7a3cfc18a996700de74da))

## [0.35.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.34.8...v0.35.0) (2026-06-02)


### Features

* **api:** spec.config.forcePaths for partial-overwrite under mergeMode=merge ([#500](https://github.com/paperclipinc/openclaw-operator/issues/500)) ([e59951c](https://github.com/paperclipinc/openclaw-operator/commit/e59951cbaf020d0e384854cfd537aad93b81aa32))


### Bug Fixes

* check skip-backup annotation before setting BackingUp phase ([#524](https://github.com/paperclipinc/openclaw-operator/issues/524)) ([b24b403](https://github.com/paperclipinc/openclaw-operator/commit/b24b40324d384e0d0809b5c31d49b940003e9c11))
* parsePluginEntry drops npm: prefix, all plugins route through clawhub ([#518](https://github.com/paperclipinc/openclaw-operator/issues/518)) ([50e75a9](https://github.com/paperclipinc/openclaw-operator/commit/50e75a9498b033c06dd2f34a2ce9c338173921fc))

## [0.34.8](https://github.com/paperclipinc/openclaw-operator/compare/v0.34.7...v0.34.8) (2026-06-02)


### Bug Fixes

* replace operator logo sunglasses with open eyes ([eb61231](https://github.com/paperclipinc/openclaw-operator/commit/eb61231c14851b3f5c4bf66463742028c126b690))

## [0.34.7](https://github.com/paperclipinc/openclaw-operator/compare/v0.34.6...v0.34.7) (2026-06-01)


### Bug Fixes

* **olm:** ship all three CRDs in the OperatorHub bundle ([842f736](https://github.com/paperclipinc/openclaw-operator/commit/842f73674e7475c8168ce557c2ff138233a687e8))


### Refactoring

* complete openclaw-rocks -&gt; paperclipinc migration ([60d9f9e](https://github.com/paperclipinc/openclaw-operator/commit/60d9f9e14146c9f594fd5eef8008393a6a72ba1b))

## [0.34.6](https://github.com/paperclipinc/openclaw-operator/compare/v0.34.5...v0.34.6) (2026-05-26)


### Bug Fixes

* **ci:** operatorhub workflow gh repo fork CLI syntax ([#515](https://github.com/paperclipinc/openclaw-operator/issues/515)) ([6be0b90](https://github.com/paperclipinc/openclaw-operator/commit/6be0b909322cf9a8a715fdf5b586345301c2141c))

## [0.34.5](https://github.com/paperclipinc/openclaw-operator/compare/v0.34.4...v0.34.5) (2026-05-26)


### Bug Fixes

* rename openclaw-rocks → paperclipinc across chart, CI, docs, Go module ([2de5f6b](https://github.com/paperclipinc/openclaw-operator/commit/2de5f6b68be3f6b05dcc00c004ca283d2f83db98))

## [0.34.4](https://github.com/paperclipinc/openclaw-operator/compare/v0.34.3...v0.34.4) (2026-05-21)


### Bug Fixes

* **plugins:** install via openclaw CLI to handle workspace:* deps ([#506](https://github.com/paperclipinc/openclaw-operator/issues/506)) ([00d08d9](https://github.com/paperclipinc/openclaw-operator/commit/00d08d98818e8d533a6de0533ca139aedd930f9f))

## [0.34.3](https://github.com/paperclipinc/openclaw-operator/compare/v0.34.2...v0.34.3) (2026-05-20)


### Bug Fixes

* **environment-skill:** document openclaw CLI availability ([#499](https://github.com/paperclipinc/openclaw-operator/issues/499)) ([5067192](https://github.com/paperclipinc/openclaw-operator/commit/5067192e9d94f9d58dcef2f3e7d38d04fedda582))

## [0.34.2](https://github.com/paperclipinc/openclaw-operator/compare/v0.34.1...v0.34.2) (2026-05-20)


### Bug Fixes

* **config:** inject gateway.mode=local for openclaw &gt;= v2026.5.18 ([#502](https://github.com/paperclipinc/openclaw-operator/issues/502)) ([e114ce7](https://github.com/paperclipinc/openclaw-operator/commit/e114ce7291d4798e444e2a0ac18bbf30e50d5cae))

## [0.34.1](https://github.com/paperclipinc/openclaw-operator/compare/v0.34.0...v0.34.1) (2026-05-12)


### Bug Fixes

* **docs-site:** correctly detect new CNAME file in gh-pages step ([#490](https://github.com/paperclipinc/openclaw-operator/issues/490)) ([deb6b8c](https://github.com/paperclipinc/openclaw-operator/commit/deb6b8c2079329ba180d955fcbc135b9fa8c8a03))

## [0.34.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.33.0...v0.34.0) (2026-05-12)


### Features

* docs site at openclaw.rocks/docs/operator/ ([#488](https://github.com/paperclipinc/openclaw-operator/issues/488)) ([0a6aa43](https://github.com/paperclipinc/openclaw-operator/commit/0a6aa43f2c4c88367cadbc0de2e11905f69b272b))

## [0.33.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.32.1...v0.33.0) (2026-05-12)


### Features

* shareProcessNamespace (default true) to reap zombies ([#485](https://github.com/paperclipinc/openclaw-operator/issues/485)) ([4cbfd4d](https://github.com/paperclipinc/openclaw-operator/commit/4cbfd4da821b1b6610cf3f73db7d3401e7087aeb))

## [0.32.1](https://github.com/paperclipinc/openclaw-operator/compare/v0.32.0...v0.32.1) (2026-05-11)


### Bug Fixes

* **workspace:** support nested paths in spec.workspace.initialFiles ([#483](https://github.com/paperclipinc/openclaw-operator/issues/483)) ([efca189](https://github.com/paperclipinc/openclaw-operator/commit/efca189304f89a0005d2c3a1b4c2db55d8af82bf))

## [0.32.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.31.1...v0.32.0) (2026-05-10)


### Features

* **chart:** aggregate OpenClaw CRD permissions into admin/edit/view roles ([#480](https://github.com/paperclipinc/openclaw-operator/issues/480)) ([8d06f81](https://github.com/paperclipinc/openclaw-operator/commit/8d06f81f1e828efa3b53cd180f3d09c8b3265181))

## [0.31.1](https://github.com/paperclipinc/openclaw-operator/compare/v0.31.0...v0.31.1) (2026-05-08)


### Bug Fixes

* **plugins:** install plugins under ~/.openclaw/extensions for auto-discovery ([#477](https://github.com/paperclipinc/openclaw-operator/issues/477)) ([de8c1c4](https://github.com/paperclipinc/openclaw-operator/commit/de8c1c477d27d978432efc8451b38d8a30e9a8f4))

## [0.31.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.30.0...v0.31.0) (2026-05-05)


### Features

* **chart:** allow disabling chart RBAC and scoping operator to namespaces ([#469](https://github.com/paperclipinc/openclaw-operator/issues/469)) ([46abfa4](https://github.com/paperclipinc/openclaw-operator/commit/46abfa4756b6fcc82cd9b7b9e8a5a85fc4afb7a1))

## [0.30.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.29.0...v0.30.0) (2026-04-24)


### Features

* **workspace:** add spec.workspace.bootstrap.enabled disable flag ([#465](https://github.com/paperclipinc/openclaw-operator/issues/465)) ([67bbf71](https://github.com/paperclipinc/openclaw-operator/commit/67bbf7125d6df23ab62ea6fcb59ce4472f25df66))


### Bug Fixes

* **statefulset:** symlink plugin-runtime-deps/node_modules/openclaw to /app ([#464](https://github.com/paperclipinc/openclaw-operator/issues/464)) ([c4c5f50](https://github.com/paperclipinc/openclaw-operator/commit/c4c5f50f44a12dfc3676923a11a97fdcb3280d01))

## [0.29.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.28.1...v0.29.0) (2026-04-23)


### Features

* **cluster-defaults:** add OpenClawClusterDefaults singleton CRD ([#460](https://github.com/paperclipinc/openclaw-operator/issues/460)) ([0e3f6bc](https://github.com/paperclipinc/openclaw-operator/commit/0e3f6bc9b82a78855d1b6c896342ec21e0ae181b))

## [0.28.1](https://github.com/paperclipinc/openclaw-operator/compare/v0.28.0...v0.28.1) (2026-04-21)


### Bug Fixes

* **statefulset:** mount ~/.config as writable PVC subPath ([#458](https://github.com/paperclipinc/openclaw-operator/issues/458)) ([5ab8c08](https://github.com/paperclipinc/openclaw-operator/commit/5ab8c081bb1ce98e7668bbf39b3a4772e13aec9f))

## [0.28.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.27.1...v0.28.0) (2026-04-17)


### Features

* **skillpacks:** install packs from plain git repos without skillpack.json ([#454](https://github.com/paperclipinc/openclaw-operator/issues/454)) ([78b92d7](https://github.com/paperclipinc/openclaw-operator/commit/78b92d73d7495d481121961bfc0591717009b008))

## [0.27.1](https://github.com/paperclipinc/openclaw-operator/compare/v0.27.0...v0.27.1) (2026-04-17)


### Bug Fixes

* **init:** mount full data volume in init-uv/init-pip for hostPath PVCs ([#450](https://github.com/paperclipinc/openclaw-operator/issues/450)) ([2eaaea9](https://github.com/paperclipinc/openclaw-operator/commit/2eaaea9dc1f468f3cc1806a2fc4466e297bc4df2))

## [0.27.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.26.2...v0.27.0) (2026-04-14)


### Features

* **helm:** add logLevel value to control operator log verbosity ([#445](https://github.com/paperclipinc/openclaw-operator/issues/445)) ([4cab3f2](https://github.com/paperclipinc/openclaw-operator/commit/4cab3f274a24e9c4c89e0d19156c56e1e63f0b77))


### Bug Fixes

* preserve third-party annotations and labels when reconciling managed resources ([#447](https://github.com/paperclipinc/openclaw-operator/issues/447)) ([3711bce](https://github.com/paperclipinc/openclaw-operator/commit/3711bce50b6de870d9f36c651bceacf0f044f5f9))

## [0.26.2](https://github.com/paperclipinc/openclaw-operator/compare/v0.26.1...v0.26.2) (2026-04-09)


### Bug Fixes

* **backup:** use --skip-links instead of --copy-links in rclone ([#430](https://github.com/paperclipinc/openclaw-operator/issues/430)) ([fd7b65e](https://github.com/paperclipinc/openclaw-operator/commit/fd7b65ea8f28c2b0cadcedda397be9acef844b1d))

## [0.26.1](https://github.com/paperclipinc/openclaw-operator/compare/v0.26.0...v0.26.1) (2026-04-09)


### Bug Fixes

* **helm:** always pass --metrics-secure flag to operator binary ([#440](https://github.com/paperclipinc/openclaw-operator/issues/440)) ([b202def](https://github.com/paperclipinc/openclaw-operator/commit/b202def1245cd9b5a15732cb61b00eb18fea6d3a))

## [0.26.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.25.2...v0.26.0) (2026-04-09)


### Features

* exclude workspace from config-hash to skip unnecessary restarts ([#432](https://github.com/paperclipinc/openclaw-operator/issues/432)) ([586e11f](https://github.com/paperclipinc/openclaw-operator/commit/586e11fca48cac8edb8a732edcf65a9910dcf64a))


### Bug Fixes

* remove env token injection in trusted-proxy mode ([#434](https://github.com/paperclipinc/openclaw-operator/issues/434)) ([6daae12](https://github.com/paperclipinc/openclaw-operator/commit/6daae12a204d6db1ba50a0c2e1660af5e53a281d))

## [0.25.2](https://github.com/paperclipinc/openclaw-operator/compare/v0.25.1...v0.25.2) (2026-04-06)


### Bug Fixes

* remove token injection in trusted-proxy mode ([#428](https://github.com/paperclipinc/openclaw-operator/issues/428)) ([9486685](https://github.com/paperclipinc/openclaw-operator/commit/948668545fed95ac010820c2176b680e693689c0))

## [0.25.1](https://github.com/paperclipinc/openclaw-operator/compare/v0.25.0...v0.25.1) (2026-04-01)


### Bug Fixes

* **plugins:** set NODE_PATH so OpenClaw discovers installed plugins ([#425](https://github.com/paperclipinc/openclaw-operator/issues/425)) ([c43ee12](https://github.com/paperclipinc/openclaw-operator/commit/c43ee124839ff49d67ba00441b8074fbac09774e))

## [0.25.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.24.2...v0.25.0) (2026-03-31)


### Features

* expose internal scale-to-zero as public spec.suspended API ([#420](https://github.com/paperclipinc/openclaw-operator/issues/420)) ([3933d97](https://github.com/paperclipinc/openclaw-operator/commit/3933d9794aadae70f67f764195c4cfc8a91db435))

## [0.24.2](https://github.com/paperclipinc/openclaw-operator/compare/v0.24.1...v0.24.2) (2026-03-30)


### Bug Fixes

* remove accidental worktree gitlinks breaking kustomize remote refs ([#417](https://github.com/paperclipinc/openclaw-operator/issues/417)) ([9a2b5aa](https://github.com/paperclipinc/openclaw-operator/commit/9a2b5aa7b32adf4cd73c27dee1dd15f9c2e0f9d0))

## [0.24.1](https://github.com/paperclipinc/openclaw-operator/compare/v0.24.0...v0.24.1) (2026-03-26)


### Bug Fixes

* **chromium:** suppress container permission warnings ([#415](https://github.com/paperclipinc/openclaw-operator/issues/415)) ([43741dc](https://github.com/paperclipinc/openclaw-operator/commit/43741dc48930bb2711f38e3add76fb44e78e4f0e))

## [0.24.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.23.2...v0.24.0) (2026-03-25)


### Features

* add native OTLP metrics export for the operator ([#410](https://github.com/paperclipinc/openclaw-operator/issues/410)) ([94b2a04](https://github.com/paperclipinc/openclaw-operator/commit/94b2a04d333ecfaf4a7a008be1f05583088e5abf))

## [0.23.2](https://github.com/paperclipinc/openclaw-operator/compare/v0.23.1...v0.23.2) (2026-03-25)


### Bug Fixes

* **security:** harden pod security contexts and enforce resource limits ([#408](https://github.com/paperclipinc/openclaw-operator/issues/408)) ([3b328a2](https://github.com/paperclipinc/openclaw-operator/commit/3b328a2c5b8460d1eb4cc2b644902ee1376a1bf9))

## [0.23.1](https://github.com/paperclipinc/openclaw-operator/compare/v0.23.0...v0.23.1) (2026-03-25)


### Bug Fixes

* **chromium:** override entrypoint to fix unquoted $@ in upstream run.sh ([#407](https://github.com/paperclipinc/openclaw-operator/issues/407)) ([21b03e5](https://github.com/paperclipinc/openclaw-operator/commit/21b03e5dba087f96bf97777ebc365d21d6c0500d))

## [0.23.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.22.2...v0.23.0) (2026-03-24)


### Features

* add spec.workspace.configMapRef and additionalWorkspaces for multi-agent support ([#395](https://github.com/paperclipinc/openclaw-operator/issues/395)) ([8fe107a](https://github.com/paperclipinc/openclaw-operator/commit/8fe107a8ec6808d0a64580c77e7109187a5cc248))


### Bug Fixes

* **backup:** add --copy-links to rclone to include symlinks ([#400](https://github.com/paperclipinc/openclaw-operator/issues/400)) ([8ffb8e8](https://github.com/paperclipinc/openclaw-operator/commit/8ffb8e8668a178facaffdc215c8029b81c5b35f4))
* **chromium:** use run.sh entrypoint and migrate deprecated image ([#397](https://github.com/paperclipinc/openclaw-operator/issues/397)) ([e74dffd](https://github.com/paperclipinc/openclaw-operator/commit/e74dffd6a80a52c7acf6a1b988c4f95e17ccfd65))
* **controller:** normalize UpdateStrategy.RollingUpdate to stop continuous reconcile loop ([#403](https://github.com/paperclipinc/openclaw-operator/issues/403)) ([0df4bcb](https://github.com/paperclipinc/openclaw-operator/commit/0df4bcb1e9b410b76ab950d5dc492d62e866cd73))
* **e2e:** stabilize flaky CDP integration screenshot test ([#405](https://github.com/paperclipinc/openclaw-operator/issues/405)) ([2b441af](https://github.com/paperclipinc/openclaw-operator/commit/2b441af81304a284ba068b17fe7315b2b85a950d))
* preserve user-set gateway auth mode in config enrichment ([#404](https://github.com/paperclipinc/openclaw-operator/issues/404)) ([337eb72](https://github.com/paperclipinc/openclaw-operator/commit/337eb72a3fc889e3ccf38c0247ab2eb34f632772))

## [0.11.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.30...v0.11.0) (2026-03-10)


### Features

* add chromium CDP proxy to inject anti-bot Chrome launch args ([1e30d22](https://github.com/paperclipinc/openclaw-operator/commit/1e30d2243fc0efc01de9fbd7281cae3cc3e1bd8f)), closes [#270](https://github.com/paperclipinc/openclaw-operator/issues/270)
* chromium CDP proxy for anti-bot launch args ([e7d9d86](https://github.com/paperclipinc/openclaw-operator/commit/e7d9d86d97035cb8bcd4e4331a24f9b9c3c23951))
* **resources:** add logging and validation for resource quantities ([0dc3508](https://github.com/paperclipinc/openclaw-operator/commit/0dc35087c96bebb78c720b9dd803e77e8e64340a))
* validate existing PVC and improve resource parsing ([351c87a](https://github.com/paperclipinc/openclaw-operator/commit/351c87a7b07ebe86338631d7018347ccfee10b2d))


### Bug Fixes

* handle merge commits in release tag creation step ([370edf6](https://github.com/paperclipinc/openclaw-operator/commit/370edf6e2459147e06d6cf69d5e4f225f0e98da0))
* handle merge commits in release tag creation step ([07b45bf](https://github.com/paperclipinc/openclaw-operator/commit/07b45bfb2d1e2ca321e760921862542861c8974e))
* resolve chromium sidecar startup race and NetworkPolicy gaps ([8579946](https://github.com/paperclipinc/openclaw-operator/commit/857994691db3206fd904732f03d6fa677a809253))
* resolve chromium sidecar startup race and NetworkPolicy gaps ([8e7fc99](https://github.com/paperclipinc/openclaw-operator/commit/8e7fc99f6dc4cbedf264587547c07411534eefa5)), closes [#270](https://github.com/paperclipinc/openclaw-operator/issues/270)

## [0.10.30](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.29...v0.10.30) (2026-03-10)


### Features

* idempotent ClawHub skill installs for persistent storage ([b02c5f2](https://github.com/paperclipinc/openclaw-operator/commit/b02c5f244315759afb0dfd55f2fd78754af7cd5b))


### Bug Fixes

* add activeDeadlineSeconds and startingDeadlineSeconds to backup CronJob ([d5a5a0a](https://github.com/paperclipinc/openclaw-operator/commit/d5a5a0ad27a6928dce9a5118a89b785df64cfe17)), closes [#286](https://github.com/paperclipinc/openclaw-operator/issues/286)
* add deadline safeguards to backup CronJob ([5f3715d](https://github.com/paperclipinc/openclaw-operator/commit/5f3715d77ecdef97f998b18a1ecabb4d15d9a680))
* add K8s API port 6443 egress when tailscale is enabled ([5968cc1](https://github.com/paperclipinc/openclaw-operator/commit/5968cc1cd28706cc811ed695cdeefacd5c6e2784))
* inject POD_NAMESPACE env via Downward API ([b793c0f](https://github.com/paperclipinc/openclaw-operator/commit/b793c0ff352abd1182a7e7d1847f3105a02aad94))
* inject POD_NAMESPACE env via Downward API in operator deployment ([8b5ae53](https://github.com/paperclipinc/openclaw-operator/commit/8b5ae53605cc1a79981ddeb9d95973bc3b5c92cd)), closes [#281](https://github.com/paperclipinc/openclaw-operator/issues/281)

## [0.10.29](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.28...v0.10.29) (2026-03-09)


### Bug Fixes

* increase chromium startup probe timeout from 2s to 5s ([#279](https://github.com/paperclipinc/openclaw-operator/issues/279)) ([2ccf3ec](https://github.com/paperclipinc/openclaw-operator/commit/2ccf3ec252daefa846e9c7c027dd40f870b996a6)), closes [#270](https://github.com/paperclipinc/openclaw-operator/issues/270)

## [0.10.28](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.27...v0.10.28) (2026-03-09)


### Bug Fixes

* auto-inject 127.0.0.0/8 into gateway.trustedProxies ([#276](https://github.com/paperclipinc/openclaw-operator/issues/276)) ([e7ecc5c](https://github.com/paperclipinc/openclaw-operator/commit/e7ecc5c6f1c50b1b6f26621b9e99d22266dadd34)), closes [#274](https://github.com/paperclipinc/openclaw-operator/issues/274)
* handle OCI pagination in registry tag resolver ([#275](https://github.com/paperclipinc/openclaw-operator/issues/275)) ([2fcf3dd](https://github.com/paperclipinc/openclaw-operator/commit/2fcf3dd41e2e3c24a440fed155019ae664b36255))

## [0.10.27](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.26...v0.10.27) (2026-03-09)


### Features

* support persistent Chromium browser profiles via PVC ([#271](https://github.com/paperclipinc/openclaw-operator/issues/271)) ([9d80414](https://github.com/paperclipinc/openclaw-operator/commit/9d804148da28ad4ab1640d1da15d7b100ade4347)), closes [#267](https://github.com/paperclipinc/openclaw-operator/issues/267)

## [0.10.26](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.25...v0.10.26) (2026-03-09)


### Features

* persist Tailscale state across pod restarts via TS_KUBE_SECRET ([#265](https://github.com/paperclipinc/openclaw-operator/issues/265)) ([0a9601d](https://github.com/paperclipinc/openclaw-operator/commit/0a9601d06972f6d507705c350987d1c12d6c5759)), closes [#262](https://github.com/paperclipinc/openclaw-operator/issues/262)


### Bug Fixes

* remove invalid llmConfig from webhook validation and docs ([#261](https://github.com/paperclipinc/openclaw-operator/issues/261)) ([e8f7399](https://github.com/paperclipinc/openclaw-operator/commit/e8f739940e92d2a0a5b2b27ee7cadcab07df6026))
* respect pod-level runAsNonRoot in container security contexts ([#266](https://github.com/paperclipinc/openclaw-operator/issues/266)) ([ad21b4c](https://github.com/paperclipinc/openclaw-operator/commit/ad21b4c59260117ed9d2f13ba8278fb4fe7e56d1)), closes [#263](https://github.com/paperclipinc/openclaw-operator/issues/263)

## [0.10.25](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.24...v0.10.25) (2026-03-07)


### Features

* enable writable package installs on read-only root filesystem ([#254](https://github.com/paperclipinc/openclaw-operator/issues/254)) ([8d5f4ba](https://github.com/paperclipinc/openclaw-operator/commit/8d5f4ba3795980589a566cf2866d3af2bf588987))

## [0.10.24](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.23...v0.10.24) (2026-03-06)


### Bug Fixes

* use service DNS for Chromium CDP URL instead of localhost ([#252](https://github.com/paperclipinc/openclaw-operator/issues/252)) ([70b9ec4](https://github.com/paperclipinc/openclaw-operator/commit/70b9ec48c156cc20d9c9eaf6837e39350f232a40))

## [0.10.23](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.22...v0.10.23) (2026-03-06)


### Bug Fixes

* correct attachOnly field placement in browser configuration ([#250](https://github.com/paperclipinc/openclaw-operator/issues/250)) ([04d44af](https://github.com/paperclipinc/openclaw-operator/commit/04d44af9ff6a0ac9792e9746e23ed2b6d028aa4d))

## [0.10.22](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.21...v0.10.22) (2026-03-05)


### Features

* inject default anti-bot-detection flags for Chromium sidecar ([#247](https://github.com/paperclipinc/openclaw-operator/issues/247)) ([4a38b4d](https://github.com/paperclipinc/openclaw-operator/commit/4a38b4dd7a8b73108448d189561dd565bf10a633))

## [0.10.21](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.20...v0.10.21) (2026-03-05)


### Bug Fixes

* disable device auth for Control UI in K8s environments ([#238](https://github.com/paperclipinc/openclaw-operator/issues/238)) ([c5c420c](https://github.com/paperclipinc/openclaw-operator/commit/c5c420c40d0f01c58e49d5d535f60f5db4b0651d)), closes [#233](https://github.com/paperclipinc/openclaw-operator/issues/233)
* propagate nodeSelector and tolerations to backup CronJob pods ([#245](https://github.com/paperclipinc/openclaw-operator/issues/245)) ([98ef456](https://github.com/paperclipinc/openclaw-operator/commit/98ef4568d4156f5ce27c9d253dd3da6bf9350c12)), closes [#244](https://github.com/paperclipinc/openclaw-operator/issues/244)
* use localhost for Chromium CDP URL to support IPv6 clusters ([#243](https://github.com/paperclipinc/openclaw-operator/issues/243)) ([08dc2c2](https://github.com/paperclipinc/openclaw-operator/commit/08dc2c27ce98dc0b54b18a295ec0823d74b90d44)), closes [#228](https://github.com/paperclipinc/openclaw-operator/issues/228)

## [0.10.20](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.19...v0.10.20) (2026-03-04)


### Features

* add attachOnly and disable device auth in browser profiles ([#236](https://github.com/paperclipinc/openclaw-operator/issues/236)) ([44dd9ce](https://github.com/paperclipinc/openclaw-operator/commit/44dd9cee63c5a442f0dca2e6c1144224489b7ff3))

## [0.10.19](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.18...v0.10.19) (2026-03-04)


### Features

* auto-inject gateway.controlUi.allowedOrigins ([#234](https://github.com/paperclipinc/openclaw-operator/issues/234)) ([#235](https://github.com/paperclipinc/openclaw-operator/issues/235)) ([46b5445](https://github.com/paperclipinc/openclaw-operator/commit/46b5445e299e4af252568fcb043bb390373ca12c))

## [0.10.18](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.17...v0.10.18) (2026-03-03)


### Features

* switch probes from exec to httpGet ([#231](https://github.com/paperclipinc/openclaw-operator/issues/231)) ([d5b7754](https://github.com/paperclipinc/openclaw-operator/commit/d5b7754400e3b1d3e41c57237b03aefc8bc76525))

## [0.10.17](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.16...v0.10.17) (2026-03-02)


### Bug Fixes

* add configurable timeout for BackingUp phase to prevent stuck instances ([#226](https://github.com/paperclipinc/openclaw-operator/issues/226)) ([778a642](https://github.com/paperclipinc/openclaw-operator/commit/778a6426d4f2b26845f110887b3bc41f02ceb0ac)), closes [#224](https://github.com/paperclipinc/openclaw-operator/issues/224)
* make skill pack resolution non-blocking to prevent provisioning failures ([#227](https://github.com/paperclipinc/openclaw-operator/issues/227)) ([ffb2485](https://github.com/paperclipinc/openclaw-operator/commit/ffb24852190e66844c9fe0f69ba28db8c9fade8f)), closes [#225](https://github.com/paperclipinc/openclaw-operator/issues/225)

## [0.10.16](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.15...v0.10.16) (2026-03-01)


### Bug Fixes

* propagate nodeSelector and tolerations to backup/restore Jobs ([#221](https://github.com/paperclipinc/openclaw-operator/issues/221)) ([342c7ae](https://github.com/paperclipinc/openclaw-operator/commit/342c7ae032cad88b5f4e15009c2accca8fd1ddb6))

## [0.10.15](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.14...v0.10.15) (2026-02-27)


### Features

* GitHub-based skill pack resolution ([#218](https://github.com/paperclipinc/openclaw-operator/issues/218)) ([077dfa6](https://github.com/paperclipinc/openclaw-operator/commit/077dfa6b52ec71c312a1e9338788b331d2cfd27a))

## [0.10.14](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.13...v0.10.14) (2026-02-27)


### Bug Fixes

* prevent StatefulSet reconciliation loop from server-side defaults ([#217](https://github.com/paperclipinc/openclaw-operator/issues/217)) ([0617b46](https://github.com/paperclipinc/openclaw-operator/commit/0617b46147cf6221ece0020b613ec8364ec8bc53))

## [0.10.13](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.12...v0.10.13) (2026-02-27)


### Bug Fixes

* use Node.js TCP connect for health probes instead of wget ([#215](https://github.com/paperclipinc/openclaw-operator/issues/215)) ([ecb7474](https://github.com/paperclipinc/openclaw-operator/commit/ecb7474c2ea40775b3a5df5f008b299d319f0d75))

## [0.10.12](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.11...v0.10.12) (2026-02-26)


### Features

* add periodic scheduled backups via CronJob ([#207](https://github.com/paperclipinc/openclaw-operator/issues/207)) ([bf29965](https://github.com/paperclipinc/openclaw-operator/commit/bf299650b8aad631fdfa6c3427f414bda9b7d511))


### Bug Fixes

* add optional S3_REGION support for MinIO backups ([#212](https://github.com/paperclipinc/openclaw-operator/issues/212)) ([c5e96c8](https://github.com/paperclipinc/openclaw-operator/commit/c5e96c89660566d5b35a137d8a098ca18cf51234)), closes [#205](https://github.com/paperclipinc/openclaw-operator/issues/205)
* **chromium:** pass extraArgs via DEFAULT_LAUNCH_ARGS env instead of container Args ([#211](https://github.com/paperclipinc/openclaw-operator/issues/211)) ([ec79758](https://github.com/paperclipinc/openclaw-operator/commit/ec797581aa068ebd48cb55c11de048f41024c826)), closes [#209](https://github.com/paperclipinc/openclaw-operator/issues/209)

## [0.10.11](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.10...v0.10.11) (2026-02-26)


### Bug Fixes

* store plaintext username and password in auto-generated basic auth secret ([#208](https://github.com/paperclipinc/openclaw-operator/issues/208)) ([179b4a6](https://github.com/paperclipinc/openclaw-operator/commit/179b4a60b94cf2e7fae5ddb293c9bfcc534f40ea)), closes [#201](https://github.com/paperclipinc/openclaw-operator/issues/201)

## [0.10.10](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.9...v0.10.10) (2026-02-26)


### Features

* add performance benchmarks for resource builders ([#197](https://github.com/paperclipinc/openclaw-operator/issues/197)) ([41efb29](https://github.com/paperclipinc/openclaw-operator/commit/41efb29b1de83aa74520f06a6d0dd4483eb08dcc))

## [0.10.9](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.8...v0.10.9) (2026-02-26)


### Features

* add Operator SDK scorecard testing ([#198](https://github.com/paperclipinc/openclaw-operator/issues/198)) ([3b44d9a](https://github.com/paperclipinc/openclaw-operator/commit/3b44d9a4f4533988be44cca2f809ab70a2c93fc7))
* add topology spread constraints support ([#196](https://github.com/paperclipinc/openclaw-operator/issues/196)) ([98ba176](https://github.com/paperclipinc/openclaw-operator/commit/98ba176822bc72e1a27497ab66f122715ac024fc))

## [0.10.8](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.7...v0.10.8) (2026-02-26)


### Features

* **chromium:** add extraArgs and extraEnv fields to ChromiumSpec ([#187](https://github.com/paperclipinc/openclaw-operator/issues/187)) ([2878482](https://github.com/paperclipinc/openclaw-operator/commit/2878482029cf0d4f4f4976cacbb594c66bb8a339))
* **chromium:** add extraArgs/extraEnv to ChromiumSpec, fix issues [#189](https://github.com/paperclipinc/openclaw-operator/issues/189)-[#193](https://github.com/paperclipinc/openclaw-operator/issues/193) ([#194](https://github.com/paperclipinc/openclaw-operator/issues/194)) ([cba8d1a](https://github.com/paperclipinc/openclaw-operator/commit/cba8d1a66c972a434d182fd80e815a44e6e79990))

## [0.10.7](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.6...v0.10.7) (2026-02-25)


### Bug Fixes

* resolve chromium sidecar port conflict and unreachable CDP ([#183](https://github.com/paperclipinc/openclaw-operator/issues/183)) ([2d3d212](https://github.com/paperclipinc/openclaw-operator/commit/2d3d2127e6f2ee2566e748e993b5aceae22c80a4)), closes [#180](https://github.com/paperclipinc/openclaw-operator/issues/180)

## [0.10.6](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.5...v0.10.6) (2026-02-24)


### Bug Fixes

* make Probes a pointer type to accept null/omitted values ([#181](https://github.com/paperclipinc/openclaw-operator/issues/181)) ([df42069](https://github.com/paperclipinc/openclaw-operator/commit/df42069191e451a756ced3b74faaed3cf44acab0)), closes [#179](https://github.com/paperclipinc/openclaw-operator/issues/179)

## [0.10.5](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.4...v0.10.5) (2026-02-24)


### Features

* add Tailscale sidecar for working tailnet integration ([#177](https://github.com/paperclipinc/openclaw-operator/issues/177)) ([d956925](https://github.com/paperclipinc/openclaw-operator/commit/d956925bb60437aa67ff82f961228933e286e0fc))

## [0.10.4](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.3...v0.10.4) (2026-02-24)


### Features

* add nginx gateway proxy sidecar for loopback-bound gateway ([#175](https://github.com/paperclipinc/openclaw-operator/issues/175)) ([a52383b](https://github.com/paperclipinc/openclaw-operator/commit/a52383b121de80229b1ae7b03622cd10561ac5a5))

## [0.10.3](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.2...v0.10.3) (2026-02-23)


### Bug Fixes

* auto-set gateway.bind=loopback and use exec probes for Tailscale serve/funnel ([#170](https://github.com/paperclipinc/openclaw-operator/issues/170)) ([e26694b](https://github.com/paperclipinc/openclaw-operator/commit/e26694b5bd38d7bd4a286f1e6caf5d3438bf6366)), closes [#167](https://github.com/paperclipinc/openclaw-operator/issues/167)
* expose metrics port in Service, StatefulSet, and ServiceMonitor ([#169](https://github.com/paperclipinc/openclaw-operator/issues/169)) ([049d097](https://github.com/paperclipinc/openclaw-operator/commit/049d097d81a42493a224bf129a243162685b1e5d)), closes [#166](https://github.com/paperclipinc/openclaw-operator/issues/166)
* move CRDs from Helm crds/ to templates/ for upgrade support ([#173](https://github.com/paperclipinc/openclaw-operator/issues/173)) ([599b394](https://github.com/paperclipinc/openclaw-operator/commit/599b39450a91a858e15d5ccf5f702db0c2c82392)), closes [#168](https://github.com/paperclipinc/openclaw-operator/issues/168)

## [0.10.2](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.1...v0.10.2) (2026-02-23)


### Bug Fixes

* break reconciliation tight loop caused by unconditional status writes ([#163](https://github.com/paperclipinc/openclaw-operator/issues/163)) ([88921b7](https://github.com/paperclipinc/openclaw-operator/commit/88921b7fafe3e703441030d49885dc3277c61213)), closes [#161](https://github.com/paperclipinc/openclaw-operator/issues/161)
* use single-quoted node -e argument in merge mode scripts ([#164](https://github.com/paperclipinc/openclaw-operator/issues/164)) ([a661ed8](https://github.com/paperclipinc/openclaw-operator/commit/a661ed8d7fbaa91bbedcd9a198dbe74d52c27444)), closes [#162](https://github.com/paperclipinc/openclaw-operator/issues/162)

## [0.10.1](https://github.com/paperclipinc/openclaw-operator/compare/v0.10.0...v0.10.1) (2026-02-22)


### Features

* add ttyd web terminal managed sidecar ([#159](https://github.com/paperclipinc/openclaw-operator/issues/159)) ([a9e1bce](https://github.com/paperclipinc/openclaw-operator/commit/a9e1bceb9897391cdf75459c6b4beaddce64c201))

## [0.10.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.23...v0.10.0) (2026-02-22)


### ⚠ BREAKING CHANGES

* The backup credentials Secret name changes from `b2-backup-credentials` to `s3-backup-credentials`, and the expected keys change from B2_BUCKET/B2_KEY_ID/B2_APP_KEY/B2_ENDPOINT to S3_BUCKET/S3_ACCESS_KEY_ID/S3_SECRET_ACCESS_KEY/S3_ENDPOINT.

### Features

* rename B2/Backblaze to generic S3-compatible storage ([#157](https://github.com/paperclipinc/openclaw-operator/issues/157)) ([df76683](https://github.com/paperclipinc/openclaw-operator/commit/df766839d6fdef920dd85960e46f6a4d2299bc9f))

## [0.9.23](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.22...v0.9.23) (2026-02-22)


### Features

* add HPA auto-scaling and Auto Pilot capability level ([#155](https://github.com/paperclipinc/openclaw-operator/issues/155)) ([024e51a](https://github.com/paperclipinc/openclaw-operator/commit/024e51aac294851ae0c9102d89c3ab38e9768d59))

## [0.9.22](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.21...v0.9.22) (2026-02-22)


### Features

* add OpenClawSelfConfig CRD for agent self-modification ([#146](https://github.com/paperclipinc/openclaw-operator/issues/146)) ([2351737](https://github.com/paperclipinc/openclaw-operator/commit/235173750618afd32e64c571604c3bf753add2ef))

## [0.9.21](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.20...v0.9.21) (2026-02-21)


### Bug Fixes

* set CSV capabilities to Deep Insights (Level 4) ([#152](https://github.com/paperclipinc/openclaw-operator/issues/152)) ([1fe72ed](https://github.com/paperclipinc/openclaw-operator/commit/1fe72ed7369e3cc73255c7a6cde2dfb6ac841b39))

## [0.9.20](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.19...v0.9.20) (2026-02-21)


### Features

* add Level 4 Deep Insights - auto-provisioned PrometheusRule and Grafana dashboards ([#149](https://github.com/paperclipinc/openclaw-operator/issues/149)) ([3c46765](https://github.com/paperclipinc/openclaw-operator/commit/3c46765f5e86a24ca904b58be1fce863cfdce4ff))

## [0.9.19](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.18...v0.9.19) (2026-02-21)


### Features

* add custom service ports and ingress backend port support ([#144](https://github.com/paperclipinc/openclaw-operator/issues/144)) ([#145](https://github.com/paperclipinc/openclaw-operator/issues/145)) ([d0604c1](https://github.com/paperclipinc/openclaw-operator/commit/d0604c141889c37fb957c002ef7aeca7cecee10c))

## [0.9.18](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.17...v0.9.18) (2026-02-20)


### Features

* improve OperatorHub and ArtifactHub listing quality ([#142](https://github.com/paperclipinc/openclaw-operator/issues/142)) ([b15937c](https://github.com/paperclipinc/openclaw-operator/commit/b15937c87da175cf72f1b7981c6bcef1b8330bfe))

## [0.9.17](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.16...v0.9.17) (2026-02-20)


### Bug Fixes

* configMapRef bypasses gateway auth enrichment ([#138](https://github.com/paperclipinc/openclaw-operator/issues/138)) ([1322d5e](https://github.com/paperclipinc/openclaw-operator/commit/1322d5e8e3141a9c5a4ea7298ddc0149d2a1d11c))

## [0.9.16](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.15...v0.9.16) (2026-02-20)


### Features

* support npm packages in skills field and disable lifecycle scripts ([#137](https://github.com/paperclipinc/openclaw-operator/issues/137)) ([a9db9d0](https://github.com/paperclipinc/openclaw-operator/commit/a9db9d091a357bca07b260ed6e7f9560917f9e9d)), closes [#131](https://github.com/paperclipinc/openclaw-operator/issues/131) [#91](https://github.com/paperclipinc/openclaw-operator/issues/91)

## [0.9.15](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.14...v0.9.15) (2026-02-19)


### Bug Fixes

* prevent StatefulSet spec drift on every reconcile ([#133](https://github.com/paperclipinc/openclaw-operator/issues/133)) ([b6fa7b3](https://github.com/paperclipinc/openclaw-operator/commit/b6fa7b3f257e51593fb01aaa42a8e4b89e9de50b))

## [0.9.14](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.13...v0.9.14) (2026-02-19)


### Bug Fixes

* restore config on container restart via postStart hook ([#128](https://github.com/paperclipinc/openclaw-operator/issues/128)) ([38ea2c5](https://github.com/paperclipinc/openclaw-operator/commit/38ea2c53adc1a4ba71bf307780b8c415d18fad6d))

## [0.9.13](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.12...v0.9.13) (2026-02-19)


### Bug Fixes

* inject browser config for Chromium sidecar ([#126](https://github.com/paperclipinc/openclaw-operator/issues/126)) ([570344e](https://github.com/paperclipinc/openclaw-operator/commit/570344e167702092bd7e5640d7c1aff0340e01e1))
* use copyFileSync instead of renameSync in merge mode init container ([#121](https://github.com/paperclipinc/openclaw-operator/issues/121)) ([72bd962](https://github.com/paperclipinc/openclaw-operator/commit/72bd962d492fdfd3ccc525523e3bb6f12de3ac70)), closes [#120](https://github.com/paperclipinc/openclaw-operator/issues/120)

## [0.9.12](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.11...v0.9.12) (2026-02-18)


### Features

* add first-class Ollama sidecar support ([860225e](https://github.com/paperclipinc/openclaw-operator/commit/860225e8e656d228218056ed6be8937f897ba582))
* add native Tailscale integration via CRD fields ([#115](https://github.com/paperclipinc/openclaw-operator/issues/115)) ([c3a2ae4](https://github.com/paperclipinc/openclaw-operator/commit/c3a2ae49b73836780aebb192fdfe931d445a1751))

## [0.9.11](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.10...v0.9.11) (2026-02-18)


### Bug Fixes

* resolve flaky backup/restore tests and autorelease label API ([#117](https://github.com/paperclipinc/openclaw-operator/issues/117)) ([da7a2a5](https://github.com/paperclipinc/openclaw-operator/commit/da7a2a58f0cb82bac091e954179efdd4827d2b78))

## [0.9.10](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.9...v0.9.10) (2026-02-18)


### Bug Fixes

* emit provider-aware ingress annotations based on className ([#109](https://github.com/paperclipinc/openclaw-operator/issues/109)) ([#110](https://github.com/paperclipinc/openclaw-operator/issues/110)) ([c040df6](https://github.com/paperclipinc/openclaw-operator/commit/c040df69f77e95ea8e284e84ad7bf86cd03df1ed))
* graceful deletion when B2 backup credentials are not configured ([#112](https://github.com/paperclipinc/openclaw-operator/issues/112)) ([10b59be](https://github.com/paperclipinc/openclaw-operator/commit/10b59be0368d2f4c2ad5069621ecf62609d924eb)), closes [#111](https://github.com/paperclipinc/openclaw-operator/issues/111)
* use shell-capable images for distroless init containers ([#108](https://github.com/paperclipinc/openclaw-operator/issues/108)) ([2c87e68](https://github.com/paperclipinc/openclaw-operator/commit/2c87e68e2e8b6bb94c95fb2ae751084843ecf2af))

## [0.9.9](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.8...v0.9.9) (2026-02-17)


### Bug Fixes

* inject gateway.bind=lan so vanilla deployments pass health probes ([#102](https://github.com/paperclipinc/openclaw-operator/issues/102)) ([7c63d86](https://github.com/paperclipinc/openclaw-operator/commit/7c63d862b7e09c1d44368205c915064d4dbe25e1)), closes [#101](https://github.com/paperclipinc/openclaw-operator/issues/101)

## [0.9.8](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.7...v0.9.8) (2026-02-17)


### Bug Fixes

* sync Helm RBAC and add gateway.existingSecret ([#98](https://github.com/paperclipinc/openclaw-operator/issues/98)) ([33dbc2c](https://github.com/paperclipinc/openclaw-operator/commit/33dbc2c15344ef857ec8e5d70f15544f9f5a12a0))

## [0.9.7](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.6...v0.9.7) (2026-02-17)


### Bug Fixes

* use server-side apply for CRD installation and update README ([#94](https://github.com/paperclipinc/openclaw-operator/issues/94)) ([73b0677](https://github.com/paperclipinc/openclaw-operator/commit/73b0677d577ada187af7298e0b685cf25161ba12))

## [0.9.6](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.5...v0.9.6) (2026-02-16)


### Features

* add runtime dependency init containers for pnpm and Python/uv ([#89](https://github.com/paperclipinc/openclaw-operator/issues/89)) ([#90](https://github.com/paperclipinc/openclaw-operator/issues/90)) ([b6a583c](https://github.com/paperclipinc/openclaw-operator/commit/b6a583cfa368a49d0054943b5f055e925c2c2e5e))

## [0.9.5](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.4...v0.9.5) (2026-02-16)


### Features

* add Phase 2+3 features and CVE-2025-22868 fix ([#84](https://github.com/paperclipinc/openclaw-operator/issues/84)) ([94d4273](https://github.com/paperclipinc/openclaw-operator/commit/94d427300aece52946929d3360e612ebbdba8441))
* add read-only rootfs, config merge mode, skill installation, and secret rotation detection ([#82](https://github.com/paperclipinc/openclaw-operator/issues/82)) ([abd7911](https://github.com/paperclipinc/openclaw-operator/commit/abd7911b76c829127c133d251927f5862f3cdeda))
* auto-generate gateway token auth for OpenClaw instances ([#85](https://github.com/paperclipinc/openclaw-operator/issues/85)) ([6ee7eca](https://github.com/paperclipinc/openclaw-operator/commit/6ee7eca5002b7035f15e1042a18d6199e50823c1)), closes [#83](https://github.com/paperclipinc/openclaw-operator/issues/83)

## [0.9.4](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.3...v0.9.4) (2026-02-16)


### Bug Fixes

* auto-label release PRs to prevent release-please stalling ([#80](https://github.com/paperclipinc/openclaw-operator/issues/80)) ([c1bce0a](https://github.com/paperclipinc/openclaw-operator/commit/c1bce0add378e1917c086c6c6e7f4779ac7f45af))

## [0.9.3](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.2...v0.9.3) (2026-02-16)


### Features

* add auto-rollback on failed update with health check monitoring ([#75](https://github.com/paperclipinc/openclaw-operator/issues/75)) ([1dff347](https://github.com/paperclipinc/openclaw-operator/commit/1dff347ce46c8608be6a92640b9a5bf1bee53f28))

## [0.9.2](https://github.com/paperclipinc/openclaw-operator/compare/v0.9.1...v0.9.2) (2026-02-16)


### Features

* add opt-in auto-update for OCI registry version tracking ([#71](https://github.com/paperclipinc/openclaw-operator/issues/71)) ([5ce624e](https://github.com/paperclipinc/openclaw-operator/commit/5ce624e72e70a185700fced9b4cc6c2461d74b91))
* add webhook warning for latest image tag ([#67](https://github.com/paperclipinc/openclaw-operator/issues/67)) ([6791624](https://github.com/paperclipinc/openclaw-operator/commit/6791624d8ec5e46fc95b02f125ac38b1578a30c7))


### Bug Fixes

* add skip-backup annotation to E2E test instance ([#69](https://github.com/paperclipinc/openclaw-operator/issues/69)) ([33ab056](https://github.com/paperclipinc/openclaw-operator/commit/33ab0563ebfdfb4817a8cd4627400af35ae3a699))
* extract imageTagLatest constant to satisfy goconst linter ([#68](https://github.com/paperclipinc/openclaw-operator/issues/68)) ([fc14d1a](https://github.com/paperclipinc/openclaw-operator/commit/fc14d1a12406373cf0fd5a6cf779e64594577841))
* suppress gosec false positive and handle existing releases ([aef5468](https://github.com/paperclipinc/openclaw-operator/commit/aef5468b57b3e787b9dbff82470362f5918d4bd0))
* suppress gosec G101 false positive and handle existing releases ([182fafb](https://github.com/paperclipinc/openclaw-operator/commit/182fafb026d7733b4f6c9c5e918366fab71da79c))
* sync release-please manifest to v0.9.1 ([#72](https://github.com/paperclipinc/openclaw-operator/issues/72)) ([88f50a9](https://github.com/paperclipinc/openclaw-operator/commit/88f50a9a2492d20922d7dfe2781e1d8c0fcdb1dd))

## [0.6.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.5.0...v0.6.0) (2026-02-13)


### ⚠ BREAKING CHANGES

* CRD API group changed from `openclaw.openclaw.io` to `openclaw.openclaw.rocks`. Existing CRDs must be deleted and re-created. This is acceptable at v1alpha1 stability level.

### Features

* Add nautical banner image with crab captain at Kubernetes helm ([84d1854](https://github.com/paperclipinc/openclaw-operator/commit/84d1854cb7200587d6745c9ae3dc29c049fc3b8e))
* Add observability, testing, docs, and dev tooling ([5af27e1](https://github.com/paperclipinc/openclaw-operator/commit/5af27e1884285d385356ee548f32a62dc8608a95))
* Add observability, testing, docs, and dev tooling for production readiness ([a8c063b](https://github.com/paperclipinc/openclaw-operator/commit/a8c063b1e06143e06eacf84419ecbc31a9435684))
* Add support for custom sidecar containers ([#27](https://github.com/paperclipinc/openclaw-operator/issues/27)) ([f0071f7](https://github.com/paperclipinc/openclaw-operator/commit/f0071f72dd92b248223f53b7af9b2f3eb9a184be)), closes [#24](https://github.com/paperclipinc/openclaw-operator/issues/24)
* Initial OpenClaw Kubernetes Operator implementation ([6d873ff](https://github.com/paperclipinc/openclaw-operator/commit/6d873ff855273c9e51337181e657ced2b82af711))
* Inject CHROMIUM_URL env var into main container when sidecar is enabled ([8553cc7](https://github.com/paperclipinc/openclaw-operator/commit/8553cc7ec0b41675bd815a0f8febd0622bbf068c))
* Inject CHROMIUM_URL env var when sidecar is enabled ([b80dabe](https://github.com/paperclipinc/openclaw-operator/commit/b80dabe32dbf2ff0f755c92f4e83e2467050bd89))
* Replace manual release steps with GoReleaser ([#2](https://github.com/paperclipinc/openclaw-operator/issues/2)) ([fedc497](https://github.com/paperclipinc/openclaw-operator/commit/fedc49766d414e0a2d96bc9c080abcb82b2b2202))
* Support custom egress rules in NetworkPolicy ([#15](https://github.com/paperclipinc/openclaw-operator/issues/15)) ([#16](https://github.com/paperclipinc/openclaw-operator/issues/16)) ([c62dc09](https://github.com/paperclipinc/openclaw-operator/commit/c62dc09cec6112cf4625175423d033310ce50dc3))
* update banner with real OpenClaw logo and Kubernetes logo ([#37](https://github.com/paperclipinc/openclaw-operator/issues/37)) ([840adb9](https://github.com/paperclipinc/openclaw-operator/commit/840adb9e285c14dffe1bfed17e9b9411b05f4538))


### Bug Fixes

* Add leader election RBAC and E2E test infrastructure ([4eb6019](https://github.com/paperclipinc/openclaw-operator/commit/4eb601994f3b87bde07f4c28a51f1dd010f045d4))
* Apply same CreateOrUpdate pattern to ServiceMonitor reconciler ([#30](https://github.com/paperclipinc/openclaw-operator/issues/30)) ([c1aaa36](https://github.com/paperclipinc/openclaw-operator/commit/c1aaa3639b7cb82eb95696bbe06b98817d46a55e))
* Bump chromium /dev/shm from 256Mi to 1Gi ([925d7b8](https://github.com/paperclipinc/openclaw-operator/commit/925d7b84c0630b73f581fca32d8da68ca582cd19))
* Bump chromium /dev/shm sizeLimit from 256Mi to 1Gi ([a4b3fbb](https://github.com/paperclipinc/openclaw-operator/commit/a4b3fbb005fd40a743a54dcb8ce42c0db9ef7b27))
* change CRD API group domain from openclaw.io to openclaw.rocks ([#41](https://github.com/paperclipinc/openclaw-operator/issues/41)) ([5bae852](https://github.com/paperclipinc/openclaw-operator/commit/5bae852810d249358fad1453694503298851208c))
* Chromium sidecar crash (UID mismatch + read-only rootfs) and lint ([#14](https://github.com/paperclipinc/openclaw-operator/issues/14)) ([febe1d3](https://github.com/paperclipinc/openclaw-operator/commit/febe1d323bb80e4ffc0e36721fa1cd75a949b023))
* Disable SBOM upload-release-assets to avoid race condition ([d909808](https://github.com/paperclipinc/openclaw-operator/commit/d909808d5aecde367748691604be1fbc152a6619))
* Downgrade to Go 1.23 for golangci-lint compatibility ([dd33c08](https://github.com/paperclipinc/openclaw-operator/commit/dd33c08d1ffe962f358c14a6ae0a7b3105799d44))
* Increase golangci-lint timeout and update to v1.63.4 ([1d982ca](https://github.com/paperclipinc/openclaw-operator/commit/1d982ca114f3316ad656bf873ad0fc6694f71cc5))
* Link OpenClaw to openclaw.ai, not openclaw.rocks ([679b043](https://github.com/paperclipinc/openclaw-operator/commit/679b04340afc6835284f2e9e13e7aef02e69ed14))
* Link OpenClaw to openclaw.ai, not openclaw.rocks ([#22](https://github.com/paperclipinc/openclaw-operator/issues/22)) ([9bfdbd8](https://github.com/paperclipinc/openclaw-operator/commit/9bfdbd86b29ed43f043a16bdbd126f6960df32a2))
* Polish README copy and diagram alignment ([#19](https://github.com/paperclipinc/openclaw-operator/issues/19)) ([ef6a8d1](https://github.com/paperclipinc/openclaw-operator/commit/ef6a8d1f0973f36d3c790d34529f86e1c1d7343c))
* Pre-enable channel modules in config to prevent EBUSY on startup ([#13](https://github.com/paperclipinc/openclaw-operator/issues/13)) ([21ee585](https://github.com/paperclipinc/openclaw-operator/commit/21ee585fb4bef4f77afed0b0a8d730c0975af0e3)), closes [#11](https://github.com/paperclipinc/openclaw-operator/issues/11)
* Prevent endless Deployment reconciliation loop ([#29](https://github.com/paperclipinc/openclaw-operator/issues/29)) ([db942b9](https://github.com/paperclipinc/openclaw-operator/commit/db942b945586e6c3b49db7f0d3ca242f5fac7b44)), closes [#28](https://github.com/paperclipinc/openclaw-operator/issues/28)
* Remove chart-releaser, keep OCI-only Helm distribution ([#3](https://github.com/paperclipinc/openclaw-operator/issues/3)) ([99e3cdb](https://github.com/paperclipinc/openclaw-operator/commit/99e3cdba598ac46e5cd3cab2dc7dad00b456a0e8))
* Replace config subPath mount with init container to avoid EBUSY ([#10](https://github.com/paperclipinc/openclaw-operator/issues/10)) ([38b60d3](https://github.com/paperclipinc/openclaw-operator/commit/38b60d39b835fb1eca459f6c4d612d0304873c3e)), closes [#9](https://github.com/paperclipinc/openclaw-operator/issues/9)
* Resolve variable shadowing of err in PVC reconciliation ([a9ea85b](https://github.com/paperclipinc/openclaw-operator/commit/a9ea85b05770ba4123b4c0325e551ba93e90d103))
* Set HOME env var to match config mount path ([#5](https://github.com/paperclipinc/openclaw-operator/issues/5)) ([175ff92](https://github.com/paperclipinc/openclaw-operator/commit/175ff9219b5af8d80e37579627f8ebccce71cb1b)), closes [#4](https://github.com/paperclipinc/openclaw-operator/issues/4)
* skip GitHub release creation in release-please ([#47](https://github.com/paperclipinc/openclaw-operator/issues/47)) ([96a5be9](https://github.com/paperclipinc/openclaw-operator/commit/96a5be93020ef61d6a37f804e1a741b3a229e86d))
* Specify kind cluster name for image loading ([7ac2116](https://github.com/paperclipinc/openclaw-operator/commit/7ac21166b36e79fb0d93c23eb24de8c420dd8446))
* update banner alt text ([#38](https://github.com/paperclipinc/openclaw-operator/issues/38)) ([d00dc23](https://github.com/paperclipinc/openclaw-operator/commit/d00dc23516d79e6a95b6aeca9d5d29c1a777e0e6))
* Update Chart.yaml version/appVersion to match latest release v0.2.4 ([#26](https://github.com/paperclipinc/openclaw-operator/issues/26)) ([c475ffc](https://github.com/paperclipinc/openclaw-operator/commit/c475ffcd145b5109befb784a1a92672377be8936))
* Update copyright to 2026 Paperclip Inc. ([bca1f0f](https://github.com/paperclipinc/openclaw-operator/commit/bca1f0f626ebd8c7c1285882d20b87483380fba4))
* Update copyright to 2026 Paperclip Inc. ([4d462f9](https://github.com/paperclipinc/openclaw-operator/commit/4d462f97799865f48c13525074702f4b1163e54f))
* Update Go version to 1.24 for CI compatibility ([5c13d06](https://github.com/paperclipinc/openclaw-operator/commit/5c13d068f3cba6d19609a3ea78b237d4b0b311ff))
* Use correct GitHub org name (OpenClaw-rocks) in all references ([#20](https://github.com/paperclipinc/openclaw-operator/issues/20)) ([c157899](https://github.com/paperclipinc/openclaw-operator/commit/c1578998cb7bfb4824eae64ce390fba4e508e6ca))
* Use direct append instead of loop for image pull secrets ([187477c](https://github.com/paperclipinc/openclaw-operator/commit/187477c3e205ddf332bd9f2831936d141b1e4088))
* Use Go 1.24 with goinstall mode for golangci-lint ([3dd40c8](https://github.com/paperclipinc/openclaw-operator/commit/3dd40c8fc02e964c9edd98407ae44a7d94634b2b))
* Use govet enable list for shadow analyzer ([3f04b1b](https://github.com/paperclipinc/openclaw-operator/commit/3f04b1bd705f33df5c438e3ae9b22fd5e2cd7975))
* Use lowercase image names for OCI registry compatibility ([2d3c87e](https://github.com/paperclipinc/openclaw-operator/commit/2d3c87ea562800a98b5be6c63b71bafe1fbae1e8))
* Use lowercase owner name for Helm OCI registry ([67bc33e](https://github.com/paperclipinc/openclaw-operator/commit/67bc33e503a6f1329908dd437731b3bf96be2ea5))
* use PAT for release-please to trigger downstream workflows ([#45](https://github.com/paperclipinc/openclaw-operator/issues/45)) ([21191ed](https://github.com/paperclipinc/openclaw-operator/commit/21191ed3e29ff2d4aeb4c0aa269eca1fb7f6284a))

## [0.5.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.4.0...v0.5.0) (2026-02-13)


### ⚠ BREAKING CHANGES

* CRD API group changed from `openclaw.openclaw.io` to `openclaw.openclaw.rocks`. Existing CRDs must be deleted and re-created. This is acceptable at v1alpha1 stability level.

### Features

* Add nautical banner image with crab captain at Kubernetes helm ([84d1854](https://github.com/paperclipinc/openclaw-operator/commit/84d1854cb7200587d6745c9ae3dc29c049fc3b8e))
* Add observability, testing, docs, and dev tooling ([5af27e1](https://github.com/paperclipinc/openclaw-operator/commit/5af27e1884285d385356ee548f32a62dc8608a95))
* Add observability, testing, docs, and dev tooling for production readiness ([a8c063b](https://github.com/paperclipinc/openclaw-operator/commit/a8c063b1e06143e06eacf84419ecbc31a9435684))
* Add support for custom sidecar containers ([#27](https://github.com/paperclipinc/openclaw-operator/issues/27)) ([f0071f7](https://github.com/paperclipinc/openclaw-operator/commit/f0071f72dd92b248223f53b7af9b2f3eb9a184be)), closes [#24](https://github.com/paperclipinc/openclaw-operator/issues/24)
* Initial OpenClaw Kubernetes Operator implementation ([6d873ff](https://github.com/paperclipinc/openclaw-operator/commit/6d873ff855273c9e51337181e657ced2b82af711))
* Inject CHROMIUM_URL env var into main container when sidecar is enabled ([8553cc7](https://github.com/paperclipinc/openclaw-operator/commit/8553cc7ec0b41675bd815a0f8febd0622bbf068c))
* Inject CHROMIUM_URL env var when sidecar is enabled ([b80dabe](https://github.com/paperclipinc/openclaw-operator/commit/b80dabe32dbf2ff0f755c92f4e83e2467050bd89))
* Replace manual release steps with GoReleaser ([#2](https://github.com/paperclipinc/openclaw-operator/issues/2)) ([fedc497](https://github.com/paperclipinc/openclaw-operator/commit/fedc49766d414e0a2d96bc9c080abcb82b2b2202))
* Support custom egress rules in NetworkPolicy ([#15](https://github.com/paperclipinc/openclaw-operator/issues/15)) ([#16](https://github.com/paperclipinc/openclaw-operator/issues/16)) ([c62dc09](https://github.com/paperclipinc/openclaw-operator/commit/c62dc09cec6112cf4625175423d033310ce50dc3))
* update banner with real OpenClaw logo and Kubernetes logo ([#37](https://github.com/paperclipinc/openclaw-operator/issues/37)) ([840adb9](https://github.com/paperclipinc/openclaw-operator/commit/840adb9e285c14dffe1bfed17e9b9411b05f4538))


### Bug Fixes

* Add leader election RBAC and E2E test infrastructure ([4eb6019](https://github.com/paperclipinc/openclaw-operator/commit/4eb601994f3b87bde07f4c28a51f1dd010f045d4))
* Apply same CreateOrUpdate pattern to ServiceMonitor reconciler ([#30](https://github.com/paperclipinc/openclaw-operator/issues/30)) ([c1aaa36](https://github.com/paperclipinc/openclaw-operator/commit/c1aaa3639b7cb82eb95696bbe06b98817d46a55e))
* Bump chromium /dev/shm from 256Mi to 1Gi ([925d7b8](https://github.com/paperclipinc/openclaw-operator/commit/925d7b84c0630b73f581fca32d8da68ca582cd19))
* Bump chromium /dev/shm sizeLimit from 256Mi to 1Gi ([a4b3fbb](https://github.com/paperclipinc/openclaw-operator/commit/a4b3fbb005fd40a743a54dcb8ce42c0db9ef7b27))
* change CRD API group domain from openclaw.io to openclaw.rocks ([#41](https://github.com/paperclipinc/openclaw-operator/issues/41)) ([5bae852](https://github.com/paperclipinc/openclaw-operator/commit/5bae852810d249358fad1453694503298851208c))
* Chromium sidecar crash (UID mismatch + read-only rootfs) and lint ([#14](https://github.com/paperclipinc/openclaw-operator/issues/14)) ([febe1d3](https://github.com/paperclipinc/openclaw-operator/commit/febe1d323bb80e4ffc0e36721fa1cd75a949b023))
* Disable SBOM upload-release-assets to avoid race condition ([d909808](https://github.com/paperclipinc/openclaw-operator/commit/d909808d5aecde367748691604be1fbc152a6619))
* Downgrade to Go 1.23 for golangci-lint compatibility ([dd33c08](https://github.com/paperclipinc/openclaw-operator/commit/dd33c08d1ffe962f358c14a6ae0a7b3105799d44))
* Increase golangci-lint timeout and update to v1.63.4 ([1d982ca](https://github.com/paperclipinc/openclaw-operator/commit/1d982ca114f3316ad656bf873ad0fc6694f71cc5))
* Link OpenClaw to openclaw.ai, not openclaw.rocks ([679b043](https://github.com/paperclipinc/openclaw-operator/commit/679b04340afc6835284f2e9e13e7aef02e69ed14))
* Link OpenClaw to openclaw.ai, not openclaw.rocks ([#22](https://github.com/paperclipinc/openclaw-operator/issues/22)) ([9bfdbd8](https://github.com/paperclipinc/openclaw-operator/commit/9bfdbd86b29ed43f043a16bdbd126f6960df32a2))
* Polish README copy and diagram alignment ([#19](https://github.com/paperclipinc/openclaw-operator/issues/19)) ([ef6a8d1](https://github.com/paperclipinc/openclaw-operator/commit/ef6a8d1f0973f36d3c790d34529f86e1c1d7343c))
* Pre-enable channel modules in config to prevent EBUSY on startup ([#13](https://github.com/paperclipinc/openclaw-operator/issues/13)) ([21ee585](https://github.com/paperclipinc/openclaw-operator/commit/21ee585fb4bef4f77afed0b0a8d730c0975af0e3)), closes [#11](https://github.com/paperclipinc/openclaw-operator/issues/11)
* Prevent endless Deployment reconciliation loop ([#29](https://github.com/paperclipinc/openclaw-operator/issues/29)) ([db942b9](https://github.com/paperclipinc/openclaw-operator/commit/db942b945586e6c3b49db7f0d3ca242f5fac7b44)), closes [#28](https://github.com/paperclipinc/openclaw-operator/issues/28)
* Remove chart-releaser, keep OCI-only Helm distribution ([#3](https://github.com/paperclipinc/openclaw-operator/issues/3)) ([99e3cdb](https://github.com/paperclipinc/openclaw-operator/commit/99e3cdba598ac46e5cd3cab2dc7dad00b456a0e8))
* Replace config subPath mount with init container to avoid EBUSY ([#10](https://github.com/paperclipinc/openclaw-operator/issues/10)) ([38b60d3](https://github.com/paperclipinc/openclaw-operator/commit/38b60d39b835fb1eca459f6c4d612d0304873c3e)), closes [#9](https://github.com/paperclipinc/openclaw-operator/issues/9)
* Resolve variable shadowing of err in PVC reconciliation ([a9ea85b](https://github.com/paperclipinc/openclaw-operator/commit/a9ea85b05770ba4123b4c0325e551ba93e90d103))
* Set HOME env var to match config mount path ([#5](https://github.com/paperclipinc/openclaw-operator/issues/5)) ([175ff92](https://github.com/paperclipinc/openclaw-operator/commit/175ff9219b5af8d80e37579627f8ebccce71cb1b)), closes [#4](https://github.com/paperclipinc/openclaw-operator/issues/4)
* Specify kind cluster name for image loading ([7ac2116](https://github.com/paperclipinc/openclaw-operator/commit/7ac21166b36e79fb0d93c23eb24de8c420dd8446))
* update banner alt text ([#38](https://github.com/paperclipinc/openclaw-operator/issues/38)) ([d00dc23](https://github.com/paperclipinc/openclaw-operator/commit/d00dc23516d79e6a95b6aeca9d5d29c1a777e0e6))
* Update Chart.yaml version/appVersion to match latest release v0.2.4 ([#26](https://github.com/paperclipinc/openclaw-operator/issues/26)) ([c475ffc](https://github.com/paperclipinc/openclaw-operator/commit/c475ffcd145b5109befb784a1a92672377be8936))
* Update copyright to 2026 Paperclip Inc. ([bca1f0f](https://github.com/paperclipinc/openclaw-operator/commit/bca1f0f626ebd8c7c1285882d20b87483380fba4))
* Update copyright to 2026 Paperclip Inc. ([4d462f9](https://github.com/paperclipinc/openclaw-operator/commit/4d462f97799865f48c13525074702f4b1163e54f))
* Update Go version to 1.24 for CI compatibility ([5c13d06](https://github.com/paperclipinc/openclaw-operator/commit/5c13d068f3cba6d19609a3ea78b237d4b0b311ff))
* Use correct GitHub org name (OpenClaw-rocks) in all references ([#20](https://github.com/paperclipinc/openclaw-operator/issues/20)) ([c157899](https://github.com/paperclipinc/openclaw-operator/commit/c1578998cb7bfb4824eae64ce390fba4e508e6ca))
* Use direct append instead of loop for image pull secrets ([187477c](https://github.com/paperclipinc/openclaw-operator/commit/187477c3e205ddf332bd9f2831936d141b1e4088))
* Use Go 1.24 with goinstall mode for golangci-lint ([3dd40c8](https://github.com/paperclipinc/openclaw-operator/commit/3dd40c8fc02e964c9edd98407ae44a7d94634b2b))
* Use govet enable list for shadow analyzer ([3f04b1b](https://github.com/paperclipinc/openclaw-operator/commit/3f04b1bd705f33df5c438e3ae9b22fd5e2cd7975))
* Use lowercase image names for OCI registry compatibility ([2d3c87e](https://github.com/paperclipinc/openclaw-operator/commit/2d3c87ea562800a98b5be6c63b71bafe1fbae1e8))
* Use lowercase owner name for Helm OCI registry ([67bc33e](https://github.com/paperclipinc/openclaw-operator/commit/67bc33e503a6f1329908dd437731b3bf96be2ea5))
* use PAT for release-please to trigger downstream workflows ([#45](https://github.com/paperclipinc/openclaw-operator/issues/45)) ([21191ed](https://github.com/paperclipinc/openclaw-operator/commit/21191ed3e29ff2d4aeb4c0aa269eca1fb7f6284a))

## [0.4.0](https://github.com/paperclipinc/openclaw-operator/compare/v0.3.0...v0.4.0) (2026-02-13)


### ⚠ BREAKING CHANGES

* CRD API group changed from `openclaw.openclaw.io` to `openclaw.openclaw.rocks`. Existing CRDs must be deleted and re-created. This is acceptable at v1alpha1 stability level.

### Features

* Add nautical banner image with crab captain at Kubernetes helm ([84d1854](https://github.com/paperclipinc/openclaw-operator/commit/84d1854cb7200587d6745c9ae3dc29c049fc3b8e))
* update banner with real OpenClaw logo and Kubernetes logo ([#37](https://github.com/paperclipinc/openclaw-operator/issues/37)) ([840adb9](https://github.com/paperclipinc/openclaw-operator/commit/840adb9e285c14dffe1bfed17e9b9411b05f4538))


### Bug Fixes

* Apply same CreateOrUpdate pattern to ServiceMonitor reconciler ([#30](https://github.com/paperclipinc/openclaw-operator/issues/30)) ([c1aaa36](https://github.com/paperclipinc/openclaw-operator/commit/c1aaa3639b7cb82eb95696bbe06b98817d46a55e))
* change CRD API group domain from openclaw.io to openclaw.rocks ([#41](https://github.com/paperclipinc/openclaw-operator/issues/41)) ([5bae852](https://github.com/paperclipinc/openclaw-operator/commit/5bae852810d249358fad1453694503298851208c))
* Prevent endless Deployment reconciliation loop ([#29](https://github.com/paperclipinc/openclaw-operator/issues/29)) ([db942b9](https://github.com/paperclipinc/openclaw-operator/commit/db942b945586e6c3b49db7f0d3ca242f5fac7b44)), closes [#28](https://github.com/paperclipinc/openclaw-operator/issues/28)
* update banner alt text ([#38](https://github.com/paperclipinc/openclaw-operator/issues/38)) ([d00dc23](https://github.com/paperclipinc/openclaw-operator/commit/d00dc23516d79e6a95b6aeca9d5d29c1a777e0e6))

## [Unreleased]

### Added
- Custom Prometheus metrics (reconciliation duration, instance phases, resource failures)
- ServiceMonitor resource creation for Prometheus Operator integration
- Defaulting webhook for setting sensible defaults
- Comprehensive resource builder unit tests
- Webhook validation unit tests
- `.golangci.yaml` linter configuration
- `.dockerignore` for optimized Docker builds
- Architecture documentation
- API reference documentation
- Troubleshooting guide
- Deployment guides for EKS, GKE, AKS
- Grafana dashboard example
- PrometheusRule alert examples

## [0.1.0] - 2024-01-01

### Added
- Initial release of OpenClaw Kubernetes Operator
- OpenClawInstance CRD (v1alpha1)
- Controller with full reconciliation lifecycle
- Security-first design (non-root, dropped capabilities, seccomp, NetworkPolicy)
- Validating webhook (blocks root, warns on insecure config)
- Managed resources: Deployment, Service, ServiceAccount, Role, RoleBinding, NetworkPolicy, PDB, ConfigMap, PVC, Ingress
- Chromium sidecar support for browser automation
- Helm chart for installation
- CI/CD with GitHub Actions (lint, test, security scan, multi-arch build)
- Container image signing with Cosign
- SBOM generation
- E2E test infrastructure
