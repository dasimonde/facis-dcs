SHELL := /bin/bash -eu -o pipefail

PYTHON ?= python3
VENV_PATH ?= .venv
THIS_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
PROJECT_ROOT := $(abspath $(THIS_DIR)../..)
VENV_ABS := $(THIS_DIR)$(VENV_PATH)
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
DCS_API_BASE_PATH ?= /digital-contracting-service/api
ENABLE_KEYCLOAK_PORT_FORWARD ?= false
KEYCLOAK_SERVICE ?= $(HELM_RELEASE)-keycloak
KEYCLOAK_DEPLOYMENT ?= $(KEYCLOAK_SERVICE)
KEYCLOAK_SERVICE_PORT ?= 8080
KEYCLOAK_LOCAL_FORWARD_PORT ?= 18080
BDD_KEYCLOAK_REALM ?= gaia-x
HELM_CHART_PATH ?= ../../deployment/helm
HELM_TIMEOUT ?= 20m

KIND_CLUSTER_NAME ?= dcs-bdd
KIND_WAIT ?= 120s
DOCKER_IMAGE_REPOSITORY ?= digital-contracting-service
DOCKER_IMAGE_TAG ?= bdd
DOCKER_IMAGE ?= $(DOCKER_IMAGE_REPOSITORY):$(DOCKER_IMAGE_TAG)
KIND_NAMESPACE ?= dcs-bdd
KIND_HELM_VALUES ?=
KIND_HELM_SET ?=
KIND_BIN ?= kind
KUBECTL_BIN ?= kubectl
HELM_BIN ?= helm
BDD_RUNNER_SCRIPT ?= scripts/run_bdd_helm.sh

.PHONY: setup_dev run_bdd_dev run_bdd_all run_bdd_helm_dev run_bdd_helm_all ensure_kind ensure_k8s_tools kind_create kind_delete kind_load_image kind_deploy kind_uninstall run_bdd_kind_dev run_bdd_kind_all run_bdd_kind_ci build_dcs_image clean help

help:
	@echo "make setup_dev    # Create venv and install bdd-executor"
	@echo "make run_bdd_dev  # Run all BDD scenarios"
	@echo "make run_bdd_all  # Run all BDD scenarios"
	@echo "make run_bdd_helm_dev # Port-forward Helm service and run all scenarios"
	@echo "make run_bdd_helm_all # Port-forward Helm service and run all scenarios"
	@echo "make run_bdd_kind_dev # Build ephemeral kind+Helm stack and run all scenarios"
	@echo "make run_bdd_kind_all # Build ephemeral kind+Helm stack and run all scenarios"
	@echo "make ensure_kind     # Validate kind is available in PATH"
	@echo "make ensure_k8s_tools # Validate kubectl and helm are available in PATH"
	@echo "make kind_delete     # Delete kind cluster used for BDD"
	@echo "make clean        # Remove local test artifacts"
setup_dev:
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
	"$(VENV_PATH)/bin/pip" install --upgrade pip wheel; \
	"$(VENV_PATH)/bin/pip" install -e "$$SRC_PATH[dev]"

run_bdd_dev: setup_dev
	@"$(VENV_PATH)/bin/python" -c 'import eu.xfsc.bdd.core' 2>/dev/null || \
		(echo "ERROR: bdd-executor (eu.xfsc.bdd.core) not found in venv — run 'make setup_dev' first" && exit 1)
	source "$(VENV_PATH)/bin/activate" && \
	mkdir -p "$(REPORTS_ABS)/junit" && \
	cd "$(PROJECT_ROOT)" && \
	"$(VENV_ABS)/bin/coverage" run --append -m behave $(ARG_BDD_JUNIT) $(FEATURES_PATH) $(ARG_BDD)

run_bdd_all: setup_dev
	@"$(VENV_PATH)/bin/python" -c 'import eu.xfsc.bdd.core' 2>/dev/null || \
		(echo "ERROR: bdd-executor (eu.xfsc.bdd.core) not found in venv — run 'make setup_dev' first" && exit 1)
	source "$(VENV_PATH)/bin/activate" && \
	mkdir -p "$(REPORTS_ABS)/junit" && \
	cd "$(PROJECT_ROOT)" && \
	"$(VENV_ABS)/bin/coverage" run --append -m behave $(ARG_BDD_JUNIT) $(FEATURES_PATH) $(ARG_BDD)

run_bdd_helm_dev: setup_dev ensure_k8s_tools
	@VENV_PATH="$(VENV_ABS)" \
		PROJECT_ROOT="$(PROJECT_ROOT)" \
		FEATURES_PATH="$(FEATURES_PATH)" \
		KUBECTL_BIN="$(KUBECTL_BIN)" \
		K8S_NAMESPACE="$(K8S_NAMESPACE)" \
		DCS_DEPLOYMENT="$(DCS_DEPLOYMENT)" \
		DCS_SERVICE="$(DCS_SERVICE)" \
		LOCAL_FORWARD_PORT="$(LOCAL_FORWARD_PORT)" \
		SERVICE_PORT="$(SERVICE_PORT)" \
		DCS_API_BASE_PATH="$(DCS_API_BASE_PATH)" \
		KEYCLOAK_SERVICE="$(KEYCLOAK_SERVICE)" \
		KEYCLOAK_DEPLOYMENT="$(KEYCLOAK_DEPLOYMENT)" \
		KEYCLOAK_LOCAL_FORWARD_PORT="$(KEYCLOAK_LOCAL_FORWARD_PORT)" \
		KEYCLOAK_SERVICE_PORT="$(KEYCLOAK_SERVICE_PORT)" \
		BDD_KEYCLOAK_REALM="$(BDD_KEYCLOAK_REALM)" \
		ENABLE_KEYCLOAK_PORT_FORWARD="$(ENABLE_KEYCLOAK_PORT_FORWARD)" \
		BDD_RUN_MODE=dev \
		ARG_BDD_JUNIT="$(ARG_BDD_JUNIT)" \
		ARG_BDD="$(ARG_BDD)" \
		bash "$(BDD_RUNNER_SCRIPT)"

run_bdd_helm_all: setup_dev ensure_k8s_tools
	@VENV_PATH="$(VENV_ABS)" \
		PROJECT_ROOT="$(PROJECT_ROOT)" \
		FEATURES_PATH="$(FEATURES_PATH)" \
		KUBECTL_BIN="$(KUBECTL_BIN)" \
		K8S_NAMESPACE="$(K8S_NAMESPACE)" \
		DCS_DEPLOYMENT="$(DCS_DEPLOYMENT)" \
		DCS_SERVICE="$(DCS_SERVICE)" \
		LOCAL_FORWARD_PORT="$(LOCAL_FORWARD_PORT)" \
		SERVICE_PORT="$(SERVICE_PORT)" \
		DCS_API_BASE_PATH="$(DCS_API_BASE_PATH)" \
		KEYCLOAK_SERVICE="$(KEYCLOAK_SERVICE)" \
		KEYCLOAK_DEPLOYMENT="$(KEYCLOAK_DEPLOYMENT)" \
		KEYCLOAK_LOCAL_FORWARD_PORT="$(KEYCLOAK_LOCAL_FORWARD_PORT)" \
		KEYCLOAK_SERVICE_PORT="$(KEYCLOAK_SERVICE_PORT)" \
		BDD_KEYCLOAK_REALM="$(BDD_KEYCLOAK_REALM)" \
		ENABLE_KEYCLOAK_PORT_FORWARD="$(ENABLE_KEYCLOAK_PORT_FORWARD)" \
		BDD_RUN_MODE=all \
		ARG_BDD_JUNIT="$(ARG_BDD_JUNIT)" \
		ARG_BDD="$(ARG_BDD)" \
		bash "$(BDD_RUNNER_SCRIPT)"

build_dcs_image:
	@docker build -f ../../backend/Dockerfile -t "$(DOCKER_IMAGE)" ../..

ensure_kind:
	@command -v "$(KIND_BIN)" >/dev/null 2>&1 || (echo "kind not found in PATH" && exit 1)

ensure_k8s_tools:
	@command -v "$(KUBECTL_BIN)" >/dev/null 2>&1 || (echo "kubectl not found in PATH" && exit 1)
	@command -v "$(HELM_BIN)" >/dev/null 2>&1 || (echo "helm not found in PATH" && exit 1)

kind_create: ensure_kind ensure_k8s_tools
	@if ! "$(KIND_BIN)" get clusters | grep -qx "$(KIND_CLUSTER_NAME)"; then \
		"$(KIND_BIN)" create cluster --name "$(KIND_CLUSTER_NAME)" --wait "$(KIND_WAIT)"; \
	else \
		echo "kind cluster $(KIND_CLUSTER_NAME) already exists"; \
	fi

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

kind_deploy: kind_load_image
	@"$(HELM_BIN)" dependency build "$(HELM_CHART_PATH)"
	@if ! "$(HELM_BIN)" upgrade --install "$(HELM_RELEASE)" "$(HELM_CHART_PATH)" \
		--namespace "$(KIND_NAMESPACE)" \
		--create-namespace \
		--wait \
		--timeout "$(HELM_TIMEOUT)" \
		--values "$(HELM_CHART_PATH)/values.bdd.yml" \
		--set image.repository="$(DOCKER_IMAGE_REPOSITORY)" \
		--set image.tag="$(DOCKER_IMAGE_TAG)" \
		$(KIND_HELM_VALUES) \
		$(KIND_HELM_SET); then \
		echo "=== PODS ==="; \
		"$(KUBECTL_BIN)" get pods -n "$(KIND_NAMESPACE)" -o wide || true; \
		echo "=== DEPLOYMENTS ==="; \
		"$(KUBECTL_BIN)" get deployments -n "$(KIND_NAMESPACE)" || true; \
		echo "=== EVENTS ==="; \
		"$(KUBECTL_BIN)" get events -n "$(KIND_NAMESPACE)" --sort-by=.lastTimestamp || true; \
		echo "=== DCS LOGS ==="; \
		"$(KUBECTL_BIN)" logs deployment/$(DCS_DEPLOYMENT) -n "$(KIND_NAMESPACE)" --tail=200 || true; \
		echo "=== KEYCLOAK LOGS ==="; \
		"$(KUBECTL_BIN)" logs deployment/$(HELM_RELEASE)-keycloak -n "$(KIND_NAMESPACE)" --tail=200 || true; \
		exit 1; \
	fi

kind_uninstall:
	@"$(HELM_BIN)" uninstall "$(HELM_RELEASE)" --namespace "$(KIND_NAMESPACE)" >/dev/null 2>&1 || true

run_bdd_kind_dev: kind_deploy
	@$(MAKE) run_bdd_helm_dev \
		K8S_NAMESPACE="$(KIND_NAMESPACE)" \
		HELM_RELEASE="$(HELM_RELEASE)" \
		ENABLE_KEYCLOAK_PORT_FORWARD=true

run_bdd_kind_all: kind_deploy
	@$(MAKE) run_bdd_helm_all \
		K8S_NAMESPACE="$(KIND_NAMESPACE)" \
		HELM_RELEASE="$(HELM_RELEASE)" \
		ENABLE_KEYCLOAK_PORT_FORWARD=true

run_bdd_kind_ci: build_dcs_image run_bdd_kind_dev

clean:
	rm -rf "$(VENV_PATH)" .tmp .reports
