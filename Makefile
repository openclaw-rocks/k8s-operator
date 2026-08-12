# Image URL to use all building/pushing image targets
IMG ?= ghcr.io/paperclipinc/openclaw-operator:latest

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.31.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: sync-chart-crds
sync-chart-crds: manifests ## Sync CRDs from config/crd/bases/ into Helm chart templates.
	bash hack/sync-chart-crds.sh

.PHONY: sync-bundle-crds
sync-bundle-crds: manifests ## Sync CRDs from config/crd/bases/ into the OLM bundle.
	bash hack/sync-bundle-crds.sh

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: test-e2e
test-e2e: ## Run end-to-end tests.
# go test defaults to a 10 minute timeout for the WHOLE package. The suite
# already runs 7-8 minutes on a healthy kind runner and has exceeded 10 on a
# loaded one, where the binary is killed mid-spec and reports a goroutine dump
# instead of the spec that was slow. The budget has to leave room for the
# multi-minute pod-readiness waits these tests legitimately need.
	go test ./test/e2e/... -v -count=1 -timeout 30m

.PHONY: scorecard
scorecard: operator-sdk ## Run operator-sdk scorecard tests.
	$(OPERATOR_SDK) scorecard bundle --wait-time 120s

.PHONY: bench
bench: ## Run benchmarks for resource builders.
	go test ./internal/resources/ -bench=. -benchmem -run=^$$ -count=1

##@ Conformance

CONFORMANCE_KIND_CLUSTER ?= openclaw-conformance

.PHONY: conformance-kind-up
conformance-kind-up: ## Spin up a fresh kind cluster for conformance.
	kind create cluster --name $(CONFORMANCE_KIND_CLUSTER) --config hack/kind-config.yaml || true

.PHONY: conformance-kind-down
conformance-kind-down: ## Tear down the conformance kind cluster.
	kind delete cluster --name $(CONFORMANCE_KIND_CLUSTER)

.PHONY: conformance-install
conformance-install: docker-build ## Install operator + CRDs onto the conformance cluster.
	kind load docker-image $(IMG) --name $(CONFORMANCE_KIND_CLUSTER)
	helm upgrade --install openclaw-operator charts/openclaw-operator \
	  --namespace openclaw-system --create-namespace \
	  --set image.repository=$(shell echo $(IMG) | cut -d: -f1) \
	  --set image.tag=$(shell echo $(IMG) | cut -d: -f2) \
	  --set image.pullPolicy=IfNotPresent \
	  --wait --timeout=10m \
	  || { \
	    echo "::group::diagnostics"; \
	    kubectl get all -n openclaw-system || true; \
	    kubectl describe deploy/openclaw-operator -n openclaw-system || true; \
	    kubectl get pods -n openclaw-system -o wide || true; \
	    kubectl logs -n openclaw-system -l app.kubernetes.io/name=openclaw-operator --all-containers=true --tail=200 || true; \
	    echo "::endgroup::"; \
	    exit 1; \
	  }

.PHONY: conformance
conformance: ## Run the full conformance suite. Requires KUBECONFIG to a cluster with operator installed.
	cd test/conformance && go test -v -timeout 60m -ginkgo.v ./...

.PHONY: conformance-negative
conformance-negative: ## Run the negative (API server deny path) conformance category.
	cd test/conformance && go test -v -timeout 10m -ginkgo.v -ginkgo.focus="negative" ./...

.PHONY: conformance-idempotency
conformance-idempotency: ## Run the idempotency conformance category.
	cd test/conformance && go test -v -timeout 30m -ginkgo.v -ginkgo.focus="idempotency" ./...

.PHONY: conformance-upgrade
conformance-upgrade: ## Run the upgrade-path conformance category.
	cd test/conformance && go test -v -timeout 60m -ginkgo.v -ginkgo.focus="upgrade-path matrix" ./...

.PHONY: conformance-gitops
conformance-gitops: ## Run the GitOps coexistence conformance category.
	cd test/conformance && go test -v -timeout 20m -ginkgo.v -ginkgo.focus="GitOps coexistence" ./...

.PHONY: conformance-failure
conformance-failure: ## Run the failure-modes conformance category.
	cd test/conformance && go test -v -timeout 20m -ginkgo.v -ginkgo.focus="failure modes" ./...

.PHONY: verify-signing
verify-signing: ## Verify the latest published release is Cosign-signed and SBOM-attested.
	@VERSION=$$(gh release view --repo paperclipinc/openclaw-operator --json tagName --jq .tagName); \
	IMAGE="ghcr.io/paperclipinc/openclaw-operator:$${VERSION}"; \
	echo "Verifying $${IMAGE}..."; \
	cosign verify "$${IMAGE}" \
	  --certificate-identity-regexp 'https://github.com/paperclipinc/openclaw-operator/.github/workflows/.*' \
	  --certificate-oidc-issuer https://token.actions.githubusercontent.com >/dev/null || { echo "::error::signature verification failed for $${IMAGE}"; exit 1; }; \
	echo "Verifying SBOM attestation..."; \
	cosign verify-attestation "$${IMAGE}" --type spdxjson \
	  --certificate-identity-regexp 'https://github.com/paperclipinc/openclaw-operator/.github/workflows/.*' \
	  --certificate-oidc-issuer https://token.actions.githubusercontent.com >/dev/null || { echo "::error::SBOM attestation verification failed for $${IMAGE}"; exit 1; }; \
	echo "OK: $${IMAGE} is signed and SBOM-attested."

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter.
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes.
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for cross-platform support.
	- $(CONTAINER_TOOL) buildx create --use
	$(CONTAINER_TOOL) buildx build --push --platform linux/amd64,linux/arm64 -t ${IMG} .

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply --server-side -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply --server-side -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
OPERATOR_SDK ?= $(LOCALBIN)/operator-sdk

## Tool Versions
KUSTOMIZE_VERSION ?= v5.3.0
CONTROLLER_TOOLS_VERSION ?= v0.17.2
ENVTEST_VERSION ?= release-0.19
GOLANGCI_LINT_VERSION ?= v1.64.5
OPERATOR_SDK_VERSION ?= v1.38.0

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: operator-sdk
operator-sdk: $(OPERATOR_SDK) ## Download operator-sdk locally if necessary.
$(OPERATOR_SDK): $(LOCALBIN)
	@[ -f $(OPERATOR_SDK) ] || { \
	set -e; \
	OS=$$(go env GOOS); ARCH=$$(go env GOARCH); \
	echo "Downloading operator-sdk $(OPERATOR_SDK_VERSION)"; \
	curl -sSLo $(OPERATOR_SDK) "https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$${OS}_$${ARCH}"; \
	chmod +x $(OPERATOR_SDK); \
	}

##@ Docs generation

CRD_REF_DOCS ?= $(LOCALBIN)/crd-ref-docs
CRD_REF_DOCS_VERSION ?= v0.3.0

.PHONY: crd-ref-docs
crd-ref-docs: $(CRD_REF_DOCS) ## Download crd-ref-docs locally if necessary.
$(CRD_REF_DOCS): $(LOCALBIN)
	test -s $(LOCALBIN)/crd-ref-docs || GOBIN=$(LOCALBIN) go install github.com/elastic/crd-ref-docs@$(CRD_REF_DOCS_VERSION)

.PHONY: api-docs
api-docs: manifests crd-ref-docs ## Regenerate docs/api-reference.md from CRD types.
	$(CRD_REF_DOCS) \
	  --config docs-site/crd-ref-docs.yaml \
	  --source-path api/v1alpha1 \
	  --output-path docs/api-reference.md \
	  --renderer markdown

##@ Docs Site

.PHONY: docs-venv
docs-venv: docs-site/.venv/bin/activate ## Create the docs-site Python virtualenv
docs-site/.venv/bin/activate: docs-site/requirements.txt
	python3 -m venv docs-site/.venv
	docs-site/.venv/bin/pip install --upgrade pip
	docs-site/.venv/bin/pip install -r docs-site/requirements.txt
	touch docs-site/.venv/bin/activate

.PHONY: docs-serve
docs-serve: docs-venv ## Run the docs site locally (http://127.0.0.1:8000)
	docs-site/.venv/bin/mkdocs serve -f docs-site/mkdocs.yml

.PHONY: docs-build
docs-build: docs-venv ## Build the docs site (strict mode -- fails on broken links / warnings)
	docs-site/.venv/bin/mkdocs build --strict -f docs-site/mkdocs.yml

# go-install-tool will 'go install' any package with custom target and target version.
define go-install-tool
@[ -f $(1) ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
}
endef
