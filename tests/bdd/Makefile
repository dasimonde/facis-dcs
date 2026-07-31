SHELL := /bin/bash -eu -o pipefail

PYTHON ?= python3
# Unter WSL gegen /mnt/c liegt das Repo auf dem 9P-Dateisystem — ein venv dort
# macht jeden Python-Import 5-20x langsamer. Default daher: venv im nativen
# ext4-Home der Distro. Explizites VENV_PATH=... überschreibt weiterhin;
# außerhalb von WSL (CI-Runner, macOS) bleibt der bisherige Default .venv.
ifneq ($(wildcard /proc/sys/fs/binfmt_misc/WSLInterop),)
VENV_PATH ?= $(HOME)/.dcs-bdd-venv
endif
VENV_PATH ?= .venv
THIS_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
PROJECT_ROOT := $(abspath $(THIS_DIR)../..)
VENV_ABS := $(if $(filter /%,$(VENV_PATH)),$(VENV_PATH),$(THIS_DIR)$(VENV_PATH))
REPORTS_ABS := $(THIS_DIR).reports
BDD_EXECUTOR_REPO ?= https://github.com/eclipse-xfsc/bdd-executor.git
BDD_EXECUTOR_PATH ?= ../../bdd-executor
BDD_EXECUTOR_FALLBACK_PATH ?= .tmp/bdd-executor
FEATURES_PATH ?= features
ARG_BDD ?=
ARG_BDD_JUNIT ?= --junit --junit-directory $(REPORTS_ABS)/junit
K8S_NAMESPACE ?= default
HELM_RELEASE ?= dcs
DCS_SERVICE ?= $(HELM_RELEASE)-digital-contracting-service
DCS_DEPLOYMENT ?= $(DCS_SERVICE)
LOCAL_FORWARD_PORT ?= 18991
SERVICE_PORT ?= 8991
ORCE_LOCAL_FORWARD_PORT ?= 18880
DCS_DID ?= localhost
DCS_API_BASE_PATH ?= /digital-contracting-service/api
BDD_INGRESS_PORT ?= 18080
BDD_PUBLIC_ORIGIN ?= http://localhost:$(BDD_INGRESS_PORT)
BDD_DCS_BASE_URL ?= $(BDD_PUBLIC_ORIGIN)$(DCS_API_BASE_PATH)
KIND_CONFIG ?= $(THIS_DIR)kind-config.yaml
IS_WSL := $(shell grep -qi microsoft /proc/sys/kernel/osrelease 2>/dev/null && echo 1)
ifeq ($(IS_WSL),1)
BDD_CHECKPOINT_SINK_HOST ?= $(shell ip -4 -o addr show dev veth-rd-wsl 2>/dev/null | awk '$$3 == "inet" { split($$4, address, "/"); print address[1]; exit }')
else
BDD_CHECKPOINT_SINK_HOST ?= host.docker.internal
endif
BDD_HELM_SET := \
	--set-string hydra.config.selfIssuerURL="$(BDD_PUBLIC_ORIGIN)" \
	--set-string route.publicBaseURL="$(BDD_DCS_BASE_URL)" \
	--set-string hydra.config.loginURL="$(BDD_PUBLIC_ORIGIN)/digital-contracting-service/ui/" \
	--set-string hydra.config.consentURL="$(BDD_PUBLIC_ORIGIN)/digital-contracting-service/api/auth/consent" \
	--set-string hydra.config.logoutURL="$(BDD_PUBLIC_ORIGIN)/digital-contracting-service/api/auth/logout-complete" \
	--set-string hydra.clients[0].redirect_uris[0]="$(BDD_PUBLIC_ORIGIN)/digital-contracting-service/api/auth/callback" \
	--set-string hydra.clients[0].post_logout_redirect_uris[0]="$(BDD_PUBLIC_ORIGIN)/digital-contracting-service/api/auth/logout-complete" \
	--set-string orce.checkpointSink.url="https://$(BDD_CHECKPOINT_SINK_HOST):18443/checkpoints"
HELM_CHART_PATH ?= ../../deployment/helm
HELM_TIMEOUT ?= 20m

KIND_CLUSTER_NAME ?= dcs-bdd
KIND_WAIT ?= 120s
DOCKER_IMAGE_REPOSITORY ?= digital-contracting-service
DOCKER_IMAGE_TAG ?= bdd
DOCKER_IMAGE ?= $(DOCKER_IMAGE_REPOSITORY):$(DOCKER_IMAGE_TAG)
PDF_CORE_IMAGE_REPOSITORY ?= pdf-core
PDF_CORE_IMAGE_TAG ?= bdd
PDF_CORE_IMAGE ?= $(PDF_CORE_IMAGE_REPOSITORY):$(PDF_CORE_IMAGE_TAG)
KIND_NAMESPACE ?= dcs-bdd
KIND_HELM_VALUES ?=
KIND_HELM_SET ?=
KIND_BIN ?= kind
KUBECTL_BIN ?= kubectl
HELM_BIN ?= helm
BDD_RUNNER_SCRIPT ?= scripts/run_bdd_helm.sh

# What run_bdd_helm runs against the live stack: bdd (behave, default) or e2e
# (Playwright). See run_e2e_kind_ci.
RUN_MODE ?= bdd
CERT_CHAIN_SECRET ?= $(DCS_SERVICE)-hsm-c2pa-x5chain
COREDNS_REWRITE_SCRIPT ?= scripts/kind_coredns_rewrite.sh
ORCE_PERSISTENCE_SCRIPT ?= scripts/ensure_orce_persistence.sh

# Second DCS release ("dcs2", instance B) for the @two-instance peer-trust
# scenarios (features/17_peer_trust) — see deployment/helm/values.bdd2.yml.
# Deployed into the SAME kind namespace as the primary "dcs" release
# (instance A), strictly after it.
HELM_RELEASE_B ?= dcs2
DCS_SERVICE_B ?= $(HELM_RELEASE_B)-digital-contracting-service
DCS_DEPLOYMENT_B ?= $(DCS_SERVICE_B)
BDD_PUBLIC_ORIGIN_B ?= http://dcs-b.localhost:$(BDD_INGRESS_PORT)
BDD_DCS_BASE_URL_B ?= $(BDD_PUBLIC_ORIGIN_B)$(DCS_API_BASE_PATH)
CERT_CHAIN_SECRET_B ?= $(DCS_SERVICE_B)-hsm-c2pa-x5chain
# Mirrors BDD_HELM_SET, but for instance B's own Hydra/route pointed at the
# dcs-b.localhost origin instead of dcs's plain localhost origin.
BDD_HELM_SET_B := \
	--set-string hydra.config.selfIssuerURL="$(BDD_PUBLIC_ORIGIN_B)" \
	--set-string route.publicBaseURL="$(BDD_DCS_BASE_URL_B)" \
	--set-string hydra.config.loginURL="$(BDD_PUBLIC_ORIGIN_B)/digital-contracting-service/ui/" \
	--set-string hydra.config.consentURL="$(BDD_PUBLIC_ORIGIN_B)/digital-contracting-service/api/auth/consent" \
	--set-string hydra.config.logoutURL="$(BDD_PUBLIC_ORIGIN_B)/digital-contracting-service/api/auth/logout-complete" \
	--set-string hydra.clients[0].redirect_uris[0]="$(BDD_PUBLIC_ORIGIN_B)/digital-contracting-service/api/auth/callback" \
	--set-string hydra.clients[0].post_logout_redirect_uris[0]="$(BDD_PUBLIC_ORIGIN_B)/digital-contracting-service/api/auth/logout-complete"

.PHONY: kind_export_kubeconfig setup_environment ensure_venv run_bdd_test run_bdd_fast run_bdd_helm ensure_kind ensure_k8s_tools kind_create kind_delete kind_load_image kind_load_pdf_core_image kind_deploy kind_deploy_b kind_coredns_rewrite kind_uninstall run_bdd_kind run_bdd_kind_all run_bdd_kind_ci run_e2e_kind run_e2e_kind_ci run_bdd_audit_kind_ci build_dcs_image clean help kind_up kind_up_audit run_bdd_kind_once run_bdd_audit_kind_once kind_down

help:
	@echo "make setup_environment   # Create venv and install bdd-executor"
	@echo "make run_bdd_test		# Run all BDD scenarios (coverage + junit reports)"
	@echo "make run_bdd_fast T=@tag	# Fast dev loop: tag-scoped, --stop, no coverage/junit"
	@echo "make run_bdd_helm		# Run BDD via Traefik on http://localhost:18080"
	@echo "make run_bdd_kind		# Build ephemeral kind+Helm stack and run all scenarios"
	@echo "make kind_up_audit		# Build a single-instance audit BDD stack"
	@echo "make run_bdd_audit_kind_once # Run only the audit/archive/ORCE feature"
	@echo "make ensure_kind			# Validate kind is available in PATH"
	@echo "make ensure_k8s_tools	# Validate kubectl and helm are available in PATH"
	@echo "make kind_delete			# Delete kind cluster used for BDD"
	@echo "make clean				# Remove local test artifacts"

setup_environment:
	@if [[ -f "$(VENV_PATH)/bin/pip" ]] && ! "$(VENV_PATH)/bin/pip" --version >/dev/null 2>&1; then \
		echo "Existing virtualenv at $(VENV_PATH) is not usable in this environment; recreating"; \
		rm -rf "$(VENV_PATH)"; \
	fi
	@if [[ ! -f "$(VENV_PATH)/bin/activate" ]]; then \
		$(PYTHON) -m venv "$(VENV_PATH)"; \
	fi
	@SRC_PATH="$(BDD_EXECUTOR_PATH)"; \
	if [[ ! -f "$$SRC_PATH/pyproject.toml" ]]; then \
		mkdir -p .tmp; \
		if [[ ! -d "$(BDD_EXECUTOR_FALLBACK_PATH)/.git" ]]; then \
			git clone --depth 1 "$(BDD_EXECUTOR_REPO)" "$(BDD_EXECUTOR_FALLBACK_PATH)"; \
		fi; \
		SRC_PATH="$(BDD_EXECUTOR_FALLBACK_PATH)"; \
	fi; \
	"$(VENV_PATH)/bin/pip" install -q --upgrade pip wheel; \
	"$(VENV_PATH)/bin/pip" install -q -e "$$SRC_PATH[dev]"; \
	"$(VENV_PATH)/bin/pip" install -q psycopg2-binary jcs pypdf==5.7.0 pyld
	"$(VENV_PATH)/bin/pip" install -q -r "$(PROJECT_ROOT)/testWallet/requirements.txt"

# Bootstrappt das venv nur, wenn es fehlt/kaputt ist — der pip-Refresh bei
# jedem Lauf kostete sonst 10-60s pro Testzyklus.
ensure_venv:
	@"$(VENV_ABS)/bin/python" -c 'import eu.xfsc.bdd.core, pypdf, pyld' 2>/dev/null || \
		$(MAKE) setup_environment

run_bdd_test: ensure_venv
	source "$(VENV_ABS)/bin/activate" && \
	mkdir -p "$(REPORTS_ABS)/junit" && \
	cd "$(PROJECT_ROOT)" && \
	"$(VENV_ABS)/bin/coverage" run --append -m behave $(ARG_BDD_JUNIT) $(FEATURES_PATH) $(ARG_BDD)

# Schneller Dev-Loop für die Implementierungs-Iteration: kein pip-Refresh,
# kein coverage, kein junit; stoppt beim ersten Fehlschlag. Scope über
# T=<tag-expression> (z. B. T=@two-instance) und/oder F=<feature-pfad>.
# Der vollständige, berichtende Lauf bleibt run_bdd_test.
run_bdd_fast: ensure_venv
	@cd "$(PROJECT_ROOT)" && \
	"$(VENV_ABS)/bin/behave" --stop $(if $(T),--tags=$(T)) $(if $(F),$(F),$(FEATURES_PATH)) $(ARG_BDD)

run_bdd_helm: setup_environment ensure_k8s_tools
	@VENV_PATH="$(VENV_ABS)" \
		PROJECT_ROOT="$(PROJECT_ROOT)" \
		FEATURES_PATH="$(FEATURES_PATH)" \
		KUBECTL_BIN="$(KUBECTL_BIN)" \
		K8S_NAMESPACE="$(K8S_NAMESPACE)" \
		HELM_RELEASE="$(HELM_RELEASE)" \
		DCS_DEPLOYMENT="$(DCS_DEPLOYMENT)" \
		BDD_DCS_BASE_URL="$(BDD_DCS_BASE_URL)" \
		BDD_PUBLIC_ORIGIN="$(BDD_PUBLIC_ORIGIN)" \
		DCS_API_BASE_PATH="$(DCS_API_BASE_PATH)" \
		DCS_SERVICE="$(DCS_SERVICE)" \
		LOCAL_FORWARD_PORT="$(LOCAL_FORWARD_PORT)" \
		SERVICE_PORT="$(SERVICE_PORT)" \
		ORCE_LOCAL_FORWARD_PORT="$(ORCE_LOCAL_FORWARD_PORT)" \
		DCS_DID="$(DCS_DID)" \
		DCS_API_BASE_PATH="$(DCS_API_BASE_PATH)" \
		DCS_DEPLOYMENT_B="$(DCS_DEPLOYMENT_B)" \
		BDD_DCS_BASE_URL_B="$(BDD_DCS_BASE_URL_B)" \
		BDD_PUBLIC_ORIGIN_B="$(BDD_PUBLIC_ORIGIN_B)" \
		ARG_BDD_JUNIT="$(ARG_BDD_JUNIT)" \
		ARG_BDD_TAGS="$(ARG_BDD_TAGS)" \
		ARG_BDD="$(ARG_BDD)" \
		RUN_MODE="$(RUN_MODE)" \
		bash "$(BDD_RUNNER_SCRIPT)"

# Image build command + optional per-image layer-cache flags. CI overrides
# DOCKER_BUILD to `docker buildx build --load` and passes GitHub Actions layer
# cache flags (see .github/workflows/bdd-kind.yml) so the Go build's base +
# module-download layers persist across runs; locally these default to a plain
# docker build with no cache flags.
DOCKER_BUILD ?= docker build
DCS_BUILD_CACHE ?=
PDF_CORE_BUILD_CACHE ?=

build_dcs_image:
	@$(DOCKER_BUILD) $(DCS_BUILD_CACHE) -f ../../deployment/docker/Dockerfile -t "$(DOCKER_IMAGE)" ../..

build_pdf_core_image:
	@$(DOCKER_BUILD) $(PDF_CORE_BUILD_CACHE) -f ../../pdf-core/Dockerfile -t "$(PDF_CORE_IMAGE)" ../..

ensure_kind:
	@command -v "$(KIND_BIN)" >/dev/null 2>&1 || (echo "kind not found in PATH" && exit 1)

ensure_k8s_tools:
	@command -v "$(KUBECTL_BIN)" >/dev/null 2>&1 || (echo "kubectl not found in PATH" && exit 1)
	@command -v "$(HELM_BIN)" >/dev/null 2>&1 || (echo "helm not found in PATH" && exit 1)

# Every kind-targeted goal pins kubectl/helm to the kind cluster through a
# dedicated kubeconfig, independent of the developer's current context —
# otherwise deploys land wherever the context points while the suite's
# host-port datapath (18080) always reaches the kind cluster.
KIND_KUBECONFIG := $(CURDIR)/.tmp/kind-kubeconfig
KIND_GOALS := kind_create kind_load_image kind_load_pdf_core_image kind_deploy kind_deploy_b \
	kind_coredns_rewrite kind_uninstall run_bdd_kind run_bdd_kind_all run_bdd_kind_ci \
	run_e2e_kind run_e2e_kind_ci \
	run_bdd_audit_kind_ci kind_up kind_up_audit run_bdd_kind_once \
	run_bdd_audit_kind_once kind_down
$(KIND_GOALS): export KUBECONFIG := $(KIND_KUBECONFIG)

kind_export_kubeconfig:
	@mkdir -p "$(CURDIR)/.tmp"
	@"$(KIND_BIN)" export kubeconfig --name "$(KIND_CLUSTER_NAME)" --kubeconfig "$(KIND_KUBECONFIG)"
	@server="$$("$(KUBECTL_BIN)" --kubeconfig "$(KIND_KUBECONFIG)" config view \
		-o jsonpath='{.clusters[0].cluster.server}')"; \
	if ! curl -ksS --max-time 2 "$${server}/readyz" >/dev/null 2>&1 && \
		ip link show veth-rd-wsl >/dev/null 2>&1; then \
		rd_gateway="$$(ip -4 -o addr show dev veth-rd-wsl | \
			awk '{ split($$4, a, "[./]"); print a[1] "." a[2] "." a[3] ".1"; exit }')"; \
		api_port="$${server##*:}"; \
		if [[ -n "$$rd_gateway" ]] && \
			curl -ksS --max-time 2 "https://$${rd_gateway}:$${api_port}/readyz" >/dev/null 2>&1; then \
			cluster_name="$$("$(KUBECTL_BIN)" --kubeconfig "$(KIND_KUBECONFIG)" config view \
				-o jsonpath='{.clusters[0].name}')"; \
			echo "Using Rancher Desktop WSL gateway $${rd_gateway}:$${api_port} for kind API"; \
			"$(KUBECTL_BIN)" --kubeconfig "$(KIND_KUBECONFIG)" config set-cluster "$$cluster_name" \
				--server="https://$${rd_gateway}:$${api_port}" \
				--tls-server-name=localhost >/dev/null; \
		fi; \
	fi

kind_create: ensure_kind ensure_k8s_tools
	@cluster_healthy=false; \
	if "$(KIND_BIN)" get clusters | grep -qx "$(KIND_CLUSTER_NAME)"; then \
		if docker inspect "$(KIND_CLUSTER_NAME)-control-plane" --format '{{.State.Running}}' 2>/dev/null | grep -q true; then \
			echo "kind cluster $(KIND_CLUSTER_NAME) already exists"; \
			cluster_healthy=true; \
		else \
			echo "kind cluster $(KIND_CLUSTER_NAME) exists but control-plane container is not running, recreating..."; \
			"$(KIND_BIN)" delete cluster --name "$(KIND_CLUSTER_NAME)"; \
		fi; \
	fi; \
	if [[ "$$cluster_healthy" == "false" ]]; then \
		"$(KIND_BIN)" create cluster --name "$(KIND_CLUSTER_NAME)" --config "$(KIND_CONFIG)" --wait "$(KIND_WAIT)"; \
	fi
	@$(MAKE) kind_export_kubeconfig

kind_delete: ensure_kind
	@if "$(KIND_BIN)" get clusters | grep -qx "$(KIND_CLUSTER_NAME)"; then \
		"$(KIND_BIN)" delete cluster --name "$(KIND_CLUSTER_NAME)"; \
	else \
		echo "kind cluster $(KIND_CLUSTER_NAME) does not exist"; \
	fi

kind_load_image: kind_create
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "docker is required to load $(DOCKER_IMAGE) into kind"; \
		exit 1; \
	fi
	@docker image inspect "$(DOCKER_IMAGE)" >/dev/null 2>&1 || (echo "Missing local image $(DOCKER_IMAGE). Build it first." && exit 1)
	@"$(KIND_BIN)" load docker-image "$(DOCKER_IMAGE)" --name "$(KIND_CLUSTER_NAME)"

kind_load_pdf_core_image: kind_create
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "docker is required to load $(PDF_CORE_IMAGE) into kind"; \
		exit 1; \
	fi
	@docker image inspect "$(PDF_CORE_IMAGE)" >/dev/null 2>&1 || (echo "Missing local image $(PDF_CORE_IMAGE). Build it first." && exit 1)
	@"$(KIND_BIN)" load docker-image "$(PDF_CORE_IMAGE)" --name "$(KIND_CLUSTER_NAME)"

kind_deploy: kind_load_image kind_load_pdf_core_image kind_load_dss_image
	@"$(HELM_BIN)" repo add prometheus-community https://prometheus-community.github.io/helm-charts --force-update
	@"$(HELM_BIN)" repo add traefik https://traefik.github.io/charts --force-update
	@"$(HELM_BIN)" repo update
	@"$(HELM_BIN)" dependency build "$(HELM_CHART_PATH)"
	@"$(HELM_BIN)" show crds "$(HELM_CHART_PATH)/charts/kube-prometheus-stack-"*.tgz | "$(KUBECTL_BIN)" apply --server-side -f -
	@# DID identity for DCS (DCS_DID is fatal on startup when missing);
	@# mounted via identity.enabled in values.bdd.yml. The <fullname>-identity
	@# Secret (did.json, identitySecretName helper) and the signing key itself
	@# are both produced by the hsm-provision-job.yaml post-install hook from the freshly generated
	@# SoftHSM token key — a static did.json fixture would mismatch a random
	@# per-run token key and crash-loop the backend (identity.NewDIDDocument
	@# is fatal on mismatch), so it is deliberately NOT pre-created here.
	@"$(KUBECTL_BIN)" create namespace "$(KIND_NAMESPACE)" --dry-run=client -o yaml | "$(KUBECTL_BIN)" apply -f -
	@KUBECTL_BIN="$(KUBECTL_BIN)" K8S_NAMESPACE="$(KIND_NAMESPACE)" HELM_RELEASE="$(HELM_RELEASE)" \
		bash "$(ORCE_PERSISTENCE_SCRIPT)"
	@if [[ -z "$(BDD_CHECKPOINT_SINK_HOST)" ]]; then \
		echo "Could not determine a host address reachable from kind for the BDD checkpoint sink."; \
		exit 1; \
	fi
	@# Deploy without --wait: Helm defers post-install hooks until all deployments are
	@# ready, but DCS can't become ready until the devCertChain post-install hook has
	@# created its Secret — a deadlock. We break the cycle by letting helm return as
	@# soon as resources are applied, then waiting for the secret and DCS separately.
	@"$(HELM_BIN)" upgrade --install "$(HELM_RELEASE)" "$(HELM_CHART_PATH)" \
		--namespace "$(KIND_NAMESPACE)" \
		--create-namespace \
		--timeout "$(HELM_TIMEOUT)" \
		--values "$(HELM_CHART_PATH)/values.bdd.yml" \
		--set image.repository="$(DOCKER_IMAGE_REPOSITORY)" \
		--set image.tag="$(DOCKER_IMAGE_TAG)" \
		--set pdfCore.image.repository="$(PDF_CORE_IMAGE_REPOSITORY)" \
		--set pdfCore.image.tag="$(PDF_CORE_IMAGE_TAG)" \
		$(BDD_HELM_SET) \
		$(KIND_HELM_VALUES) \
		$(KIND_HELM_SET)
	@# CoreDNS must resolve dcs-a.localhost / dcs-b.localhost in-cluster to the
	@# Traefik Service just created above (did:web challenge-response, both
	@# self-peer and cross-instance — see kind_coredns_rewrite.sh). Runs on
	@# every kind_deploy (idempotent) rather than only once at cluster-create
	@# time: a prior version wired only to cluster creation was observed to
	@# not reliably stick.
	@$(MAKE) kind_coredns_rewrite
	@# The image tag is static, so an unchanged manifest never rolls the pods
	@# even though kind load just replaced the node image — restart both
	@# freshly-built workloads so the run tests the image that was built.
	@"$(KUBECTL_BIN)" -n "$(KIND_NAMESPACE)" rollout restart \
		deployment/"$(DCS_DEPLOYMENT)" deployment/"$(DCS_DEPLOYMENT)"-pdf-core 2>/dev/null || true
	@echo "Waiting for cert chain secret $(CERT_CHAIN_SECRET) (created by post-install hook)..."
	@deadline=$$(( $$(date +%s) + 1200 )); \
	until "$(KUBECTL_BIN)" get secret "$(CERT_CHAIN_SECRET)" -n "$(KIND_NAMESPACE)" >/dev/null 2>&1; do \
		if [ $$(date +%s) -gt $$deadline ]; then \
			echo "Timed out waiting for $(CERT_CHAIN_SECRET)"; \
			"$(KUBECTL_BIN)" get pods -n "$(KIND_NAMESPACE)" -l app.kubernetes.io/instance=$(HELM_RELEASE) -o wide || true; \
			"$(KUBECTL_BIN)" get events -n "$(KIND_NAMESPACE)" --sort-by=.lastTimestamp || true; \
			exit 1; \
		fi; \
		sleep 5; \
	done
	@echo "Waiting for DCS deployment to become available..."
	@if ! "$(KUBECTL_BIN)" -n "$(KIND_NAMESPACE)" rollout status \
		deployment/"$(DCS_DEPLOYMENT)" --timeout "$(HELM_TIMEOUT)"; then \
		echo "=== PODS (instance A) ==="; \
		"$(KUBECTL_BIN)" get pods -n "$(KIND_NAMESPACE)" -l app.kubernetes.io/instance=$(HELM_RELEASE) -o wide || true; \
		echo "=== DEPLOYMENTS ==="; \
		"$(KUBECTL_BIN)" get deployments -n "$(KIND_NAMESPACE)" || true; \
		echo "=== EVENTS ==="; \
		"$(KUBECTL_BIN)" get events -n "$(KIND_NAMESPACE)" --sort-by=.lastTimestamp || true; \
		echo "=== DCS LOGS (current, instance A) ==="; \
		"$(KUBECTL_BIN)" logs -n "$(KIND_NAMESPACE)" \
			-l "app.kubernetes.io/component=backend,app.kubernetes.io/instance=$(HELM_RELEASE)" \
			-c digital-contracting-service --tail=200 --prefix || true; \
		echo "=== DCS LOGS (previous crash, instance A) ==="; \
		"$(KUBECTL_BIN)" logs -n "$(KIND_NAMESPACE)" \
			-l "app.kubernetes.io/component=backend,app.kubernetes.io/instance=$(HELM_RELEASE)" \
			-c digital-contracting-service --tail=200 --prefix --previous || true; \
		echo "=== DCS POD DESCRIBE (instance A) ==="; \
		"$(KUBECTL_BIN)" describe pod -n "$(KIND_NAMESPACE)" \
			-l "app.kubernetes.io/component=backend,app.kubernetes.io/instance=$(HELM_RELEASE)" || true; \
		echo "=== HSM PROVISION JOB LOGS (instance A) ==="; \
		"$(KUBECTL_BIN)" logs -n "$(KIND_NAMESPACE)" \
			job/"$(DCS_DEPLOYMENT)"-hsm-provision --tail=100 || true; \
		echo "=== FC REALM PROVISION JOB LOGS ==="; \
		"$(KUBECTL_BIN)" logs -n "$(KIND_NAMESPACE)" \
			job/"$(DCS_DEPLOYMENT)"-fc-realm-provision --tail=100 || true; \
		exit 1; \
	fi

# Idempotently patches CoreDNS with rewrite rules for the BDD did:web
# hostnames (dcs-a.localhost, dcs-b.localhost) -> the shared Traefik Service,
# and verifies in-cluster resolution actually took effect before returning.
# See tests/bdd/scripts/kind_coredns_rewrite.sh for the full rationale.
kind_coredns_rewrite: ensure_k8s_tools
	@KUBECTL_BIN="$(KUBECTL_BIN)" KIND_NAMESPACE="$(KIND_NAMESPACE)" \
		bash "$(COREDNS_REWRITE_SCRIPT)"

# Deploys the SECOND DCS release (instance B, features/17_peer_trust) into
# the same kind namespace, strictly after instance A: B probes
# several of A's already-deployed shared services (Traefik, ORCE TSA, Status
# List Service, IPFS) at its own startup, so it depends on kind_deploy rather
# than only on kind_create/kind_load_image.
kind_deploy_b: kind_deploy
	@echo "Deploying instance B (release $(HELM_RELEASE_B)) after instance A ($(HELM_RELEASE))..."
	@"$(HELM_BIN)" upgrade --install "$(HELM_RELEASE_B)" "$(HELM_CHART_PATH)" \
		--namespace "$(KIND_NAMESPACE)" \
		--create-namespace \
		--timeout "$(HELM_TIMEOUT)" \
		--values "$(HELM_CHART_PATH)/values.bdd2.yml" \
		--set image.repository="$(DOCKER_IMAGE_REPOSITORY)" \
		--set image.tag="$(DOCKER_IMAGE_TAG)" \
		--set pdfCore.image.repository="$(PDF_CORE_IMAGE_REPOSITORY)" \
		--set pdfCore.image.tag="$(PDF_CORE_IMAGE_TAG)" \
		$(BDD_HELM_SET_B) \
		$(KIND_HELM_VALUES) \
		$(KIND_HELM_SET)
	@"$(KUBECTL_BIN)" -n "$(KIND_NAMESPACE)" rollout restart \
		deployment/"$(DCS_DEPLOYMENT_B)" 2>/dev/null || true
	@echo "Waiting for cert chain secret $(CERT_CHAIN_SECRET_B) (created by post-install hook)..."
	@deadline=$$(( $$(date +%s) + 1200 )); \
	until "$(KUBECTL_BIN)" get secret "$(CERT_CHAIN_SECRET_B)" -n "$(KIND_NAMESPACE)" >/dev/null 2>&1; do \
		if [ $$(date +%s) -gt $$deadline ]; then \
			echo "Timed out waiting for $(CERT_CHAIN_SECRET_B)"; \
			"$(KUBECTL_BIN)" get pods -n "$(KIND_NAMESPACE)" -l app.kubernetes.io/instance=$(HELM_RELEASE_B) -o wide || true; \
			"$(KUBECTL_BIN)" get events -n "$(KIND_NAMESPACE)" --sort-by=.lastTimestamp || true; \
			exit 1; \
		fi; \
		sleep 5; \
	done
	@echo "Waiting for DCS deployment (instance B) to become available..."
	@if ! "$(KUBECTL_BIN)" -n "$(KIND_NAMESPACE)" rollout status \
		deployment/"$(DCS_DEPLOYMENT_B)" --timeout "$(HELM_TIMEOUT)"; then \
		echo "=== PODS (instance B) ==="; \
		"$(KUBECTL_BIN)" get pods -n "$(KIND_NAMESPACE)" -l app.kubernetes.io/instance=$(HELM_RELEASE_B) -o wide || true; \
		echo "=== DEPLOYMENTS ==="; \
		"$(KUBECTL_BIN)" get deployments -n "$(KIND_NAMESPACE)" || true; \
		echo "=== EVENTS ==="; \
		"$(KUBECTL_BIN)" get events -n "$(KIND_NAMESPACE)" --sort-by=.lastTimestamp || true; \
		echo "=== DCS LOGS (current, instance B) ==="; \
		"$(KUBECTL_BIN)" logs -n "$(KIND_NAMESPACE)" \
			-l "app.kubernetes.io/component=backend,app.kubernetes.io/instance=$(HELM_RELEASE_B)" \
			-c digital-contracting-service --tail=200 --prefix || true; \
		echo "=== DCS LOGS (previous crash, instance B) ==="; \
		"$(KUBECTL_BIN)" logs -n "$(KIND_NAMESPACE)" \
			-l "app.kubernetes.io/component=backend,app.kubernetes.io/instance=$(HELM_RELEASE_B)" \
			-c digital-contracting-service --tail=200 --prefix --previous || true; \
		echo "=== DCS POD DESCRIBE (instance B) ==="; \
		"$(KUBECTL_BIN)" describe pod -n "$(KIND_NAMESPACE)" \
			-l "app.kubernetes.io/component=backend,app.kubernetes.io/instance=$(HELM_RELEASE_B)" || true; \
		echo "=== HSM PROVISION JOB LOGS (instance B) ==="; \
		"$(KUBECTL_BIN)" logs -n "$(KIND_NAMESPACE)" \
			job/"$(DCS_DEPLOYMENT_B)"-hsm-provision --tail=100 || true; \
		exit 1; \
	fi

kind_uninstall:
	@"$(HELM_BIN)" uninstall "$(HELM_RELEASE_B)" --namespace "$(KIND_NAMESPACE)" >/dev/null 2>&1 || true
	@"$(HELM_BIN)" uninstall "$(HELM_RELEASE)" --namespace "$(KIND_NAMESPACE)" >/dev/null 2>&1 || true

run_bdd_kind: kind_deploy_b
	@$(MAKE) run_bdd_helm \
		K8S_NAMESPACE="$(KIND_NAMESPACE)" \
		HELM_RELEASE="$(HELM_RELEASE)"

run_bdd_kind_all: kind_deploy_b
	@$(MAKE) run_bdd_helm_all \
		K8S_NAMESPACE="$(KIND_NAMESPACE)" \
		HELM_RELEASE="$(HELM_RELEASE)"

run_bdd_kind_ci: build_dcs_image build_pdf_core_image run_bdd_kind

# The Playwright E2E counterpart of run_bdd_kind_ci: same build + two-instance
# deploy, but RUN_MODE=e2e makes run_bdd_helm run the Playwright suite against
# the live stack (with the DSS forward + both instances up) instead of behave.
# Split into its own CI job so it runs in parallel with the BDD suite on a
# separate runner (own kind cluster, own disk/memory budget).
run_e2e_kind: kind_deploy_b
	@$(MAKE) run_bdd_helm \
		K8S_NAMESPACE="$(KIND_NAMESPACE)" \
		HELM_RELEASE="$(HELM_RELEASE)" \
		RUN_MODE="e2e"

run_e2e_kind_ci: build_dcs_image build_pdf_core_image run_e2e_kind

run_bdd_audit_kind_ci: build_dcs_image build_pdf_core_image kind_deploy
	@$(MAKE) run_bdd_audit_kind_once

# The community-packaged EU DSS 6.2 demonstration webapp (visible-PAdES wallet-based AES).
DSS_IMAGE ?= conectx/dss-demo:6.2.1

# Pull (if absent) and load the DSS image into the kind cluster. The DSS is the
# signature VALIDATOR for the wallet-driven /signature/submit path (ADR-12,
# DCS-FR-SM-18); the DCS holds no signing key.
kind_load_dss_image: kind_create
	@docker image inspect "$(DSS_IMAGE)" >/dev/null 2>&1 || docker pull "$(DSS_IMAGE)"
	@"$(KIND_BIN)" load docker-image "$(DSS_IMAGE)" --name "$(KIND_CLUSTER_NAME)"

clean:
	rm -rf "$(VENV_PATH)" .tmp .reports

# start kind cluster, deploy DCS and dependencies (both instances), and
# prepare for BDD runs
kind_up: build_dcs_image build_pdf_core_image kind_deploy_b setup_environment ensure_k8s_tools
	@echo "kind cluster $(KIND_CLUSTER_NAME) is up with DCS deployed in namespace $(KIND_NAMESPACE)"
	@echo "You can now run 'make run_bdd_kind_once' to execute the BDD suite against this cluster without redeploying between runs."

# Audit/archive/ORCE needs only instance A. ORCE still uses persistent storage
# so the receipt chain survives the restart scenario.
kind_up_audit: build_dcs_image build_pdf_core_image kind_deploy setup_environment ensure_k8s_tools
	@echo "kind cluster $(KIND_CLUSTER_NAME) is up with the single-instance audit BDD stack"
	@echo "Run 'make run_bdd_audit_kind_once' to execute the focused feature."

# run BDD suite once against kind deployment (without redeploying cluster between runs)
# Optionally pass F=<path> to run a single feature file, e.g. make run_bdd_kind_once F=features/login.feature
run_bdd_kind_once:
	@$(MAKE) run_bdd_helm \
		K8S_NAMESPACE="$(KIND_NAMESPACE)" \
		HELM_RELEASE="$(HELM_RELEASE)" \
		FEATURES_PATH="$(if $(F),$(F),$(FEATURES_PATH))"

run_bdd_audit_kind_once:
	@$(MAKE) run_bdd_helm \
		K8S_NAMESPACE="$(KIND_NAMESPACE)" \
		HELM_RELEASE="$(HELM_RELEASE)" \
		DCS_DEPLOYMENT_B="" \
		ARG_BDD_TAGS="--tags=isolated_stack" \
		FEATURES_PATH="features/08_audit_compliance/audit_ui_archive_orce.feature"

# Shutdown deployment, cluster and clean test artifacts
kind_down: kind_uninstall kind_delete clean
