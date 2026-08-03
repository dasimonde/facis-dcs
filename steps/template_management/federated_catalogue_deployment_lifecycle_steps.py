"""Executable deployment assertions for the Federated Catalogue lifecycle.

Fresh installs use an isolated namespace and the production ``deploy.sh``.
Legacy upgrades extract and install the last Neo4j-based chart from Git history.
"""

import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import tarfile
import tempfile
import time
import uuid

from behave import given, then, when


REPO_ROOT = Path(__file__).resolve().parents[2]
CHART_DIR = REPO_ROOT / "deployment" / "helm"
DEPLOY_SCRIPT = CHART_DIR / "deploy.sh"
BDD_RUNNER = REPO_ROOT / "tests" / "bdd" / "scripts" / "run_bdd_helm.sh"
DEV_STACK = REPO_ROOT / "dev-stack.sh"
LEGACY_CHART_COMMIT = "c76fbb271ba3e4057b8f4fe8848b6ca627b88c89"


def _run(command, *, timeout=120, env=None, cwd=REPO_ROOT):
    started = time.monotonic()
    result = subprocess.run(
        [str(part) for part in command],
        cwd=cwd,
        capture_output=True,
        text=True,
        timeout=timeout,
        check=False,
        env=env,
    )
    result.elapsed_seconds = time.monotonic() - started
    return result


def _assert_success(result, label):
    assert result.returncode == 0, (
        f"{label} failed with exit {result.returncode}\n"
        f"stdout:\n{result.stdout}\nstderr:\n{result.stderr}"
    )


def _kubectl_binary():
    return os.environ.get("BDD_KUBECTL") or os.environ.get("KUBECTL_BIN", "kubectl")


def _kubectl(context, *args):
    kubectl = _kubectl_binary()
    namespace = os.environ.get("K8S_NAMESPACE")
    assert namespace, "K8S_NAMESPACE is required for Federated Catalogue deployment inspection"
    result = subprocess.run(
        [kubectl, "-n", namespace, *args],
        capture_output=True,
        text=True,
        timeout=30,
        check=False,
    )
    assert result.returncode == 0, (
        f"kubectl {' '.join(args)} failed with exit {result.returncode}: {result.stderr}"
    )
    return result.stdout


def _isolated_kubectl(context, *args, check=True):
    result = _run(
        [_kubectl_binary(), "-n", context.fc_lifecycle_namespace, *args],
        timeout=60,
    )
    if check:
        _assert_success(result, f"kubectl {' '.join(args)}")
    return result


def _create_isolated_namespace(context):
    configured = os.environ.get("BDD_FC_LIFECYCLE_NAMESPACE", "").strip()
    if configured:
        namespace = configured
        owned = False
    else:
        namespace = f"dcs-fc-lifecycle-{uuid.uuid4().hex[:8]}"
        result = _run([_kubectl_binary(), "create", "namespace", namespace], timeout=30)
        _assert_success(
            result,
            "creating the isolated FC lifecycle namespace; set "
            "BDD_FC_LIFECYCLE_NAMESPACE to a pre-created namespace when the "
            "BDD service account cannot create namespaces",
        )
        owned = True

    context.fc_lifecycle_namespace = namespace
    context.fc_lifecycle_owns_namespace = owned
    if owned:
        def cleanup():
            # The generated prefix makes the destructive target explicit.
            if not namespace.startswith("dcs-fc-lifecycle-"):
                return
            # These namespaces hold a full catalogue stack -- Postgres, fc-service
            # and, for the legacy upgrade, Neo4j -- on the same single kind node as
            # the two DCS instances. Deleting without waiting returns while every
            # pod is still running, so the features that follow (encryption at
            # rest, checkpoints, SLA federation) execute against a node that is
            # still carrying this scenario's load. Drop the pods first, then wait
            # for the namespace, so the compute is actually released here.
            # A teardown that overruns must not fail the scenario that owns it.
            try:
                _run(
                    [
                        _kubectl_binary(), "-n", namespace, "delete", "pod", "--all",
                        "--grace-period=0", "--force", "--wait=false",
                    ],
                    timeout=60,
                )
                _run(
                    [_kubectl_binary(), "delete", "namespace", namespace, "--wait=true"],
                    timeout=float(
                        os.environ.get("BDD_FC_LIFECYCLE_TEARDOWN_TIMEOUT_SECONDS", "180")
                    ),
                )
            except subprocess.TimeoutExpired:
                _run(
                    [_kubectl_binary(), "delete", "namespace", namespace, "--wait=false"],
                    timeout=30,
                )

        context.add_cleanup(cleanup)


def _deploy_current_chart(context, values_paths):
    release = getattr(context, "fc_lifecycle_release", "fc-lifecycle")
    values_args = []
    for values_path in values_paths:
        values_args.extend(["--values", values_path])
    return _run(
        [
            "bash",
            DEPLOY_SCRIPT,
            *values_args,
            "--namespace",
            context.fc_lifecycle_namespace,
            "--release",
            release,
            "--timeout",
            os.environ.get("BDD_FC_LIFECYCLE_HELM_TIMEOUT", "15m"),
        ],
        timeout=float(os.environ.get("BDD_FC_LIFECYCLE_PROCESS_TIMEOUT_SECONDS", "1200")),
    )


def _current_bdd_values(context, override_env_name):
    configured = os.environ.get(override_env_name, "").strip()
    if configured:
        paths = [Path(value).resolve() for value in configured.split(os.pathsep) if value]
        assert paths and all(path.is_file() for path in paths), (
            f"{override_env_name} must contain readable values files separated by {os.pathsep!r}: "
            f"{configured}"
        )
        return paths

    temp_dir = Path(tempfile.mkdtemp(prefix="facis-dcs-fc-values-"))
    context.add_cleanup(lambda: shutil.rmtree(temp_dir, ignore_errors=True))
    image_override = temp_dir / "fresh-stack-values.yaml"
    lifecycle_host = f"{context.fc_lifecycle_namespace}.localhost:18080"
    host = lifecycle_host.split(":")[0]
    public_origin = f"http://{lifecycle_host}"
    api_base = f"{public_origin}/digital-contracting-service/api"
    routed_paths = [
        {"path": "/digital-contracting-service", "pathType": "Prefix"},
        {"path": "/.well-known/did.json", "pathType": "Prefix"},
        {"path": "/.well-known/dcs-agreement-credential.json", "pathType": "Prefix"},
        {"path": "/.well-known/dcs-federation-rules.md", "pathType": "Prefix"},
        {"path": "/orce", "pathType": "Prefix"},
    ]
    values = {
        "image": {
            "repository": os.environ.get(
                "BDD_FC_DCS_IMAGE_REPOSITORY", "digital-contracting-service"
            ),
            "tag": os.environ.get("BDD_FC_DCS_IMAGE_TAG", "bdd"),
        },
        "traefik": {"enabled": False},
        "ingress": {"hosts": [{"host": host, "paths": routed_paths}]},
        "route": {"publicBaseURL": api_base, "didHostname": lifecycle_host},
        "signing": {"issuerDID": f"did:web:{lifecycle_host.replace(':', '%3A')}"},
        "hydra": {
            "config": {
                "selfIssuerURL": public_origin,
                "loginURL": f"{public_origin}/digital-contracting-service/ui/",
                "consentURL": f"{api_base}/auth/consent",
                "logoutURL": f"{api_base}/auth/logout-complete",
            },
            "clients": [
                {
                    "client_id": "dcs-client",
                    "client_secret": "dcs-secret",
                    "grant_types": ["authorization_code", "refresh_token"],
                    "response_types": ["code"],
                    "scope": "openid offline_access",
                    "redirect_uris": [f"{api_base}/auth/callback"],
                    "post_logout_redirect_uris": [f"{api_base}/auth/logout-complete"],
                    "token_endpoint_auth_method": "client_secret_post",
                    "skip_consent": True,
                }
            ],
        },
        "statuslistService": {
            "ingress": {
                "hosts": [
                    {"host": host, "paths": [{"path": "/statuslist", "pathType": "Prefix"}]}
                ]
            }
        },
        "statusListLocalhostProxy": {"ingressHost": host},
        "orce": {"ingress": {"hosts": [{"host": host, "path": "/contract-target"}]}},
    }
    image_override.write_text(json.dumps(values), encoding="utf-8")
    context.fc_lifecycle_base_url = api_base
    return [CHART_DIR / "values.bdd.yml", image_override]


def _container_text(container):
    fields = [
        container.get("name", ""),
        container.get("image", ""),
        *container.get("command", []),
        *container.get("args", []),
    ]
    fields.extend(
        f"{entry.get('name', '')}={entry.get('value', '')}"
        for entry in container.get("env", [])
        # This explicit false switch prevents Spring from depending on Neo4j
        # health.  It is evidence of removal, not a runtime dependency.
        if not (
            entry.get("name") == "MANAGEMENT_HEALTH_NEO4J_ENABLED"
            and str(entry.get("value", "")).lower() == "false"
        )
    )
    return "\n".join(fields).lower()


@given("the co-deployed Federated Catalogue is enabled")
def step_given_catalogue_enabled(context):
    deployment = json.loads(_kubectl(context, "get", "deployment", "fc-service", "-o", "json"))
    desired = deployment.get("spec", {}).get("replicas", 1)
    available = deployment.get("status", {}).get("availableReplicas", 0)
    assert desired > 0 and available == desired, (
        f"fc-service is not an enabled, available co-deployment: desired={desired}, "
        f"available={available}"
    )
    context.fc_service_deployment = deployment


@when("I inspect the running Federated Catalogue graph-store deployment")
def step_when_inspect_catalogue_graph_store(context):
    context.fc_fuseki_deployment = json.loads(
        _kubectl(context, "get", "deployment", "fc-fuseki", "-o", "json")
    )
    context.fc_namespace_workloads = json.loads(
        _kubectl(context, "get", "deployment,statefulset,daemonset", "-o", "json")
    )


@then("the Federated Catalogue uses the compatible Fuseki graph store")
def step_then_catalogue_uses_fuseki(context):
    containers = (
        context.fc_service_deployment.get("spec", {})
        .get("template", {})
        .get("spec", {})
        .get("containers", [])
    )
    fc_container = next(
        (container for container in containers if container.get("name") == "fcservice"),
        None,
    )
    assert fc_container, "fc-service deployment has no fcservice container"
    env = {entry["name"]: entry.get("value") for entry in fc_container.get("env", [])}
    assert env.get("GRAPHSTORE_IMPL", "").lower() == "fuseki", (
        f"GRAPHSTORE_IMPL is not fuseki: {env.get('GRAPHSTORE_IMPL')!r}"
    )
    assert env.get("GRAPHSTORE_FUSEKI_URI"), "GRAPHSTORE_FUSEKI_URI is not configured"

    desired = context.fc_fuseki_deployment.get("spec", {}).get("replicas", 1)
    available = context.fc_fuseki_deployment.get("status", {}).get("availableReplicas", 0)
    assert desired > 0 and available == desired, (
        f"fc-fuseki is not available: desired={desired}, available={available}"
    )


@then("no running catalogue workload requires Neo4j, n10s or n10s.graphconfig.show")
def step_then_no_neo4j_runtime_dependency(context):
    forbidden = ("neo4j", "n10s", "n10s.graphconfig.show")
    offenders = []
    for workload in context.fc_namespace_workloads.get("items", []):
        replicas = workload.get("status", {}).get("availableReplicas", 0)
        if replicas < 1:
            continue
        pod_spec = workload.get("spec", {}).get("template", {}).get("spec", {})
        containers = pod_spec.get("initContainers", []) + pod_spec.get("containers", [])
        for container in containers:
            text = _container_text(container)
            matches = [term for term in forbidden if term in text]
            if matches:
                offenders.append(
                    f"{workload.get('kind')}/{workload.get('metadata', {}).get('name')}"
                    f":{container.get('name')} ({', '.join(matches)})"
                )
    assert not offenders, "Running workloads retain Neo4j/n10s dependencies: " + "; ".join(offenders)


@given("a fresh isolated Helm installation with the Federated Catalogue enabled")
def step_given_fresh_fc_install(context):
    _create_isolated_namespace(context)
    context.fc_lifecycle_release = os.environ.get("BDD_FC_LIFECYCLE_RELEASE", "dcs")
    values_paths = _current_bdd_values(context, "BDD_FC_FRESH_VALUES")

    context.fc_lifecycle_deploy = _deploy_current_chart(context, values_paths)
    _assert_success(context.fc_lifecycle_deploy, "fresh isolated DCS/FC Helm installation")
    context.base_url = context.fc_lifecycle_base_url
    context.catalogue_calls = []
    previous_dcs_base_url = os.environ.get("BDD_DCS_BASE_URL")
    os.environ["BDD_DCS_BASE_URL"] = context.fc_lifecycle_base_url
    previous_issuer_base = os.environ.get("ISSUER_BASE_URL")
    os.environ["ISSUER_BASE_URL"] = os.environ.get(
        "BDD_FC_LIFECYCLE_ISSUER_BASE_URL",
        "http://localhost:18080/issuer",
    )
    def restore_dcs_base_url():
        if previous_dcs_base_url is None:
            os.environ.pop("BDD_DCS_BASE_URL", None)
        else:
            os.environ["BDD_DCS_BASE_URL"] = previous_dcs_base_url

    def restore_issuer_base():
        if previous_issuer_base is None:
            os.environ.pop("ISSUER_BASE_URL", None)
        else:
            os.environ["ISSUER_BASE_URL"] = previous_issuer_base

    context.add_cleanup(restore_dcs_base_url)
    context.add_cleanup(restore_issuer_base)
    # Tokens are scoped by API base, but clearing removes any ambiguity when a
    # previous scenario happened to use the same generated namespace name.
    from steps.support.services.auth_service import AuthService  # noqa: PLC0415

    AuthService._token_cache.clear()


def _fc_verification_metric(context):
    query = "tag=uri:%2Fverification"
    raw_path = (
        f"/api/v1/namespaces/{context.fc_lifecycle_namespace}/services/"
        f"fc-service:http/proxy/actuator/metrics/http.server.requests?{query}"
    )
    result = _run([_kubectl_binary(), "get", "--raw", raw_path], timeout=30)
    assert result.returncode == 0, (
        f"reading live FC /verification request metrics from {raw_path} failed "
        f"with exit {result.returncode}\nstdout:\n{result.stdout}\nstderr:\n{result.stderr}"
    )
    try:
        payload = json.loads(result.stdout)
    except json.JSONDecodeError as error:
        raise AssertionError(
            f"FC /verification metric from {raw_path} is not valid JSON: "
            f"{result.stdout!r}"
        ) from error
    counts = [
        measurement.get("value")
        for measurement in payload.get("measurements", [])
        if measurement.get("statistic") == "COUNT"
    ]
    assert len(counts) == 1, (
        f"FC verification metric from {raw_path} has no unique COUNT: {payload}"
    )
    status_tags = [
        tag
        for tag in payload.get("availableTags", [])
        if tag.get("tag") == "status"
    ]
    assert len(status_tags) == 1, (
        f"FC verification metric from {raw_path} has no unique status "
        f"availableTags entry: {payload}"
    )
    statuses = {str(value) for value in status_tags[0].get("values", [])}
    return float(counts[0]), statuses


@when("the lifecycle runner observes the first Federated Catalogue readiness transition")
def step_when_observe_first_readiness(context):
    output = context.fc_lifecycle_deploy.stdout + context.fc_lifecycle_deploy.stderr
    assert "Waiting for native Federated Catalogue health readiness" in output, (
        "deploy.sh did not exercise the co-deployed FC native health gate"
    )
    assert "Waiting for the backend startup gate" in output, (
        "deploy.sh did not exercise the backend functional readiness gate"
    )
    (
        context.fc_verification_count,
        context.fc_verification_statuses,
    ) = _fc_verification_metric(context)


@then("the readiness gate has observed a successful health check")
def step_then_health_gate_succeeded(context):
    output = context.fc_lifecycle_deploy.stdout + context.fc_lifecycle_deploy.stderr
    assert "co-deployed Federated Catalogue did not become healthy" not in output
    assert "backend startup gate failed" not in output


@then("the first functional catalogue verification succeeds without warm-up or repetition")
def step_then_first_verification_succeeds_once(context):
    assert context.fc_verification_count == 1, (
        f"Expected exactly one live startup /verification request, observed "
        f"{context.fc_verification_count}"
    )
    assert context.fc_verification_statuses == {"200"}, (
        "The atomic live startup /verification metric did not report exactly "
        f"HTTP status 200, observed statuses {sorted(context.fc_verification_statuses)}"
    )
    output = context.fc_lifecycle_deploy.stdout + context.fc_lifecycle_deploy.stderr
    assert "federated catalogue readiness gate failed" not in output.lower()


@then("the Federated Catalogue pod has not restarted")
def step_then_fc_pod_not_restarted(context):
    raw = _isolated_kubectl(
        context,
        "get",
        "pod",
        "-l",
        f"app.kubernetes.io/name=federated-catalogue,app.kubernetes.io/instance={context.fc_lifecycle_release}",
        "-o",
        "json",
    ).stdout
    pods = json.loads(raw).get("items", [])
    assert len(pods) == 1, f"Expected one Federated Catalogue pod, got {len(pods)}"
    restarts = sum(
        status.get("restartCount", 0)
        for status in pods[0].get("status", {}).get("containerStatuses", [])
    )
    assert restarts == 0, f"Federated Catalogue pod restarted {restarts} time(s)"


@then("the catalogue operation completed within the DCS Federated Catalogue client timeout")
def step_then_catalogue_operation_within_timeout(context):
    assert context.catalogue_calls, "No catalogue operation duration was recorded"
    call = context.catalogue_calls[-1]
    assert call["duration"] < 30, (
        f"{call['operation']} took {call['duration']:.3f}s, exceeding the DCS "
        "Federated Catalogue client's 30s timeout"
    )
    response_text = getattr(context.requests_response, "text", "").lower()
    assert "timeout" not in response_text and "deadline exceeded" not in response_text


@given("no business catalogue operation has run since the first readiness transition")
def step_given_no_business_catalogue_operation_since_readiness(context):
    assert context.catalogue_calls == [], (
        f"The fresh stack was already warmed by catalogue operations: "
        f"{context.catalogue_calls}"
    )


@then("the fresh-stack catalogue operations used no retry")
def step_then_fresh_catalogue_operations_used_no_retry(context):
    operations = [call["operation"] for call in context.catalogue_calls]
    assert operations == ["publish", "retrieve", "search"], (
        f"Expected exactly one immediate publish/retrieve/search call, observed {operations}"
    )


def _ensure_legacy_commit_present():
    """Make LEGACY_CHART_COMMIT's tree readable by `git archive`.

    CI checks out at depth 1, so a commit this far back is a ref this clone has
    never fetched and `git archive` fails with "not a tree object". Deepen just
    that one commit rather than the whole history.
    """
    if _run(["git", "cat-file", "-e", f"{LEGACY_CHART_COMMIT}^{{tree}}"], timeout=30).returncode == 0:
        return
    fetched = _run(
        ["git", "fetch", "--no-tags", "--depth=1", "origin", LEGACY_CHART_COMMIT],
        timeout=300,
    )
    _assert_success(
        fetched,
        f"deepening the shallow clone to reach Neo4j legacy chart commit {LEGACY_CHART_COMMIT}",
    )


@given("an isolated namespace contains the old Neo4j-based development installation")
def step_given_legacy_neo4j_install(context):
    _create_isolated_namespace(context)
    context.fc_lifecycle_release = os.environ.get("BDD_FC_LIFECYCLE_RELEASE", "dcs")
    legacy_root = Path(tempfile.mkdtemp(prefix="facis-dcs-legacy-chart-"))
    context.add_cleanup(lambda: shutil.rmtree(legacy_root, ignore_errors=True))
    archive_path = legacy_root / "legacy-chart.tar"
    _ensure_legacy_commit_present()
    archive = _run(
        [
            "git",
            "archive",
            "--format=tar",
            f"--output={archive_path}",
            LEGACY_CHART_COMMIT,
            "deployment/helm",
        ],
        timeout=60,
    )
    _assert_success(
        archive,
        f"extracting Neo4j legacy chart from Git commit {LEGACY_CHART_COMMIT}",
    )
    with tarfile.open(archive_path) as tar:
        tar.extractall(legacy_root, filter="data")
    legacy_chart_path = legacy_root / "deployment" / "helm"

    legacy_image_repository = os.environ.get(
        "BDD_FC_DCS_IMAGE_REPOSITORY", "digital-contracting-service"
    ).strip()
    legacy_image_tag = os.environ.get("BDD_FC_DCS_IMAGE_TAG", "bdd").strip()
    assert legacy_image_repository and not legacy_image_repository.startswith("["), (
        "BDD_FC_DCS_IMAGE_REPOSITORY must name the preloaded DCS image used by "
        f"the legacy fixture, got {legacy_image_repository!r}"
    )
    assert legacy_image_tag and not legacy_image_tag.startswith("["), (
        "BDD_FC_DCS_IMAGE_TAG must name the preloaded DCS image used by the "
        f"legacy fixture, got {legacy_image_tag!r}"
    )
    legacy_values = {
        "replicaCount": 0,
        "image": {
            "repository": legacy_image_repository,
            "tag": legacy_image_tag,
            "pullPolicy": "IfNotPresent",
        },
        "pkcs11": {"provisioning": {"enabled": False}},
        "pdfCore": {"enabled": False},
        "hydra": {"enabled": False},
        "nats": {"enabled": False},
        "orce": {"enabled": False},
        "dss": {"enabled": False},
        "statuslistService": {"enabled": False},
        "ipfs": {"enabled": False},
        "ipfsDocumentManager": {"enabled": False},
        "monitoring": {"enabled": False},
        "traefik": {"enabled": False},
        "postgresql": {"enabled": True},
        "keycloak": {"enabled": True},
        "neo4j": {"enabled": True, "persistence": {"enabled": True}},
        "federatedCatalogue": {"enabled": True, "portal": {"enabled": False}},
    }
    legacy_values_path = legacy_root / "legacy-values.yaml"
    legacy_values_path.write_text(
        json.dumps(legacy_values),
        encoding="utf-8",
    )

    # The historical and current Chart.lock pin these two unchanged upstream
    # archives. Seed them from the current locked dependency cache when
    # available; Helm still validates/builds the historical chart, but the
    # isolated runner does not need to redownload large disabled subcharts.
    for archive_name in (
        "kube-prometheus-stack-85.2.0.tgz",
        "traefik-41.0.0.tgz",
    ):
        cached = CHART_DIR / "charts" / archive_name
        if cached.is_file():
            shutil.copy2(cached, legacy_chart_path / "charts" / archive_name)

    for child_chart in sorted((legacy_chart_path / "charts").iterdir()):
        if child_chart.is_dir() and (child_chart / "Chart.yaml").is_file():
            package = _run(
                [
                    "helm",
                    "package",
                    child_chart,
                    "--destination",
                    legacy_chart_path / "charts",
                ],
                timeout=60,
            )
            _assert_success(
                package,
                f"packaging historical local dependency {child_chart.name}",
            )
    dependency_list = _run(["helm", "dependency", "list", legacy_chart_path], timeout=30)
    _assert_success(
        dependency_list,
        f"checking dependencies for legacy commit {LEGACY_CHART_COMMIT}",
    )
    assert "missing" not in dependency_list.stdout.lower(), dependency_list.stdout
    rendered = _run(
        [
            "helm",
            "template",
            context.fc_lifecycle_release,
            legacy_chart_path,
            "--namespace",
            context.fc_lifecycle_namespace,
            "--values",
            legacy_values_path,
        ],
        timeout=60,
    )
    _assert_success(rendered, "rendering the historical Neo4j fixture")
    rendered_lower = rendered.stdout.lower()
    assert "templates/hsm-provision-job.yaml" not in rendered_lower, (
        "Historical fixture unexpectedly renders the unrelated HSM provisioning hook"
    )
    assert "templates/hsm-token-pvc.yaml" not in rendered_lower, (
        "Historical fixture unexpectedly renders the unrelated HSM token PVC"
    )
    assert "name: graphstore_impl" in rendered_lower and 'value: "neo4j"' in rendered_lower, (
        "Historical fixture no longer renders the real Neo4j-backed Federated Catalogue"
    )
    install = _run(
        [
            "helm",
            "upgrade",
            "--install",
            context.fc_lifecycle_release,
            legacy_chart_path,
            "--namespace",
            context.fc_lifecycle_namespace,
            "--values",
            legacy_values_path,
            "--timeout",
            os.environ.get("BDD_FC_LIFECYCLE_HELM_TIMEOUT", "15m"),
        ],
        timeout=float(os.environ.get("BDD_FC_LIFECYCLE_PROCESS_TIMEOUT_SECONDS", "1200")),
    )
    _assert_success(install, "legacy Neo4j-based Helm installation")
    neo4j_ready = _isolated_kubectl(
        context,
        "rollout",
        "status",
        f"deployment/{context.fc_lifecycle_release}-neo4j",
        "--timeout=180s",
        check=False,
    )
    _assert_success(
        neo4j_ready,
        "historical Neo4j workload from commit "
        f"{LEGACY_CHART_COMMIT}; if n10s image drift prevents this, AC4 is "
        "blocked by a non-installable historical fixture",
    )
    legacy_resources = json.loads(
        _isolated_kubectl(context, "get", "deployment,statefulset,pvc", "-o", "json").stdout
    ).get("items", [])
    runtime = json.dumps(legacy_resources).lower()
    assert "neo4j" in runtime or "n10s" in runtime, (
        "The supplied legacy fixture is not demonstrably Neo4j/n10s-based"
    )
    context.fc_legacy_pvc_uids = {
        item.get("metadata", {}).get("uid")
        for item in legacy_resources
        if item.get("kind") == "PersistentVolumeClaim"
    }


@when("the lifecycle runner upgrades it with the current DCS Helm chart")
def step_when_upgrade_legacy_fc(context):
    values_paths = _current_bdd_values(context, "BDD_FC_UPGRADE_VALUES")
    context.fc_lifecycle_deploy = _deploy_current_chart(context, values_paths)
    _assert_success(context.fc_lifecycle_deploy, "current-chart upgrade of legacy FC release")


@then("the Federated Catalogue is replaced completely without migrating old catalogue data")
def step_then_fc_replaced_without_migration(context):
    resources = json.loads(
        _isolated_kubectl(context, "get", "deployment,statefulset,pod,pvc", "-o", "json").stdout
    ).get("items", [])
    mounted_claims = {
        volume.get("persistentVolumeClaim", {}).get("claimName")
        for item in resources
        if item.get("kind") == "Pod"
        for volume in item.get("spec", {}).get("volumes", [])
        if volume.get("persistentVolumeClaim")
    }
    current_pvcs = {
        item.get("metadata", {}).get("name"): item.get("metadata", {}).get("uid")
        for item in resources
        if item.get("kind") == "PersistentVolumeClaim"
    }
    reused = {
        name for name, uid in current_pvcs.items()
        if uid in context.fc_legacy_pvc_uids and name in mounted_claims
    }
    assert not reused, f"Current FC pods reuse legacy catalogue PVCs: {sorted(reused)}"


@then("no obsolete Neo4j or n10s workload remains")
def step_then_no_obsolete_legacy_workload(context):
    resources = json.loads(
        _isolated_kubectl(context, "get", "deployment,statefulset,daemonset", "-o", "json").stdout
    ).get("items", [])
    offenders = []
    for workload in resources:
        containers = (
            workload.get("spec", {}).get("template", {}).get("spec", {}).get("containers", [])
        )
        if any(
            term in _container_text(container)
            for container in containers
            for term in ("neo4j", "n10s", "n10s.graphconfig.show")
        ):
            offenders.append(workload.get("metadata", {}).get("name"))
    assert not offenders, f"Obsolete Neo4j/n10s workloads remain: {offenders}"


@then("the upgraded catalogue satisfies the fresh-install readiness gate")
def step_then_upgrade_meets_readiness(context):
    step_when_observe_first_readiness(context)
    step_then_health_gate_succeeded(context)
    step_then_first_verification_succeeds_once(context)
    step_then_fc_pod_not_restarted(context)


def _write_executable(path, content):
    path.write_text(content, encoding="utf-8")
    path.chmod(path.stat().st_mode | stat.S_IXUSR)


def _stub_command(stub_bin, name, body):
    _write_executable(
        stub_bin / name,
        "#!/bin/bash\nset -u\n"
        'echo "$0 $*" >> "${BDD_STUB_LOG}"\n'
        f"{body}\n",
    )


def _entrypoint_stub_environment(context, mode):
    root = Path(tempfile.mkdtemp(prefix="facis-dcs-entrypoint-"))
    context.add_cleanup(lambda: shutil.rmtree(root, ignore_errors=True))
    stub_bin = root / "bin"
    stub_bin.mkdir()
    log_path = root / "commands.log"
    log_path.touch()
    env = {
        **os.environ,
        "PATH": f"{stub_bin}{os.pathsep}{os.environ['PATH']}",
        "BDD_STUB_LOG": str(log_path),
        "BDD_STUB_MODE": mode,
        "BDD_STUB_STATE": str(root / "gate-observed"),
    }
    context.fc_entrypoint_root = root
    context.fc_entrypoint_log_path = log_path
    return root, stub_bin, env


def _install_common_stubs(stub_bin):
    for name in (
        "helm",
        "make",
        "cp",
        "base64",
        "npm",
        "go",
        "goa",
        "docker",
        "fuser",
        "nc",
        "python",
    ):
        _stub_command(stub_bin, name, "exit 0")
    _stub_command(stub_bin, "curl", 'printf "%s" "${BDD_STUB_CURL_CODE:-200}"; exit 0')


def _run_development_entrypoint(context, mode):
    root, stub_bin, env = _entrypoint_stub_environment(context, mode)
    _install_common_stubs(stub_bin)
    _stub_command(stub_bin, "kubectl", "exit 0")
    _stub_command(stub_bin, "bash", "exit 0")
    _stub_command(
        stub_bin,
        "npm",
        r'''
echo "VITE_PROCESS_STARTED" >> "${BDD_STUB_LOG}"
if [[ "${BDD_STUB_MODE}" == "disabled" ]]; then
  for _ in $(seq 1 100); do
    [[ -f "${BDD_STUB_STATE}.continued" ]] && break
    sleep 0.05
  done
  if [[ ! -f "${BDD_STUB_STATE}.continued" ]]; then
    echo "Vite fixture did not observe backend continuation" >&2
    exit 70
  fi
  echo "FIXTURE_STOP after continued-past-FC-gate observation" >> "${BDD_STUB_LOG}"
  exit 0
fi
while true; do sleep 1; done''',
    )
    _stub_command(
        stub_bin,
        "air",
        r'''
if [[ "$PWD" == */pdf-core ]]; then
  echo "PDF_CORE_PROCESS_STARTED" >> "${BDD_STUB_LOG}"
  while true; do sleep 1; done
fi
if [[ "$PWD" == */backend && "${BDD_STUB_MODE}" == "disabled" ]]; then
  echo "CONTINUED_PAST_FC_GATE backend-started" >> "${BDD_STUB_LOG}"
  touch "${BDD_STUB_STATE}.continued"
  while true; do sleep 1; done
fi
if [[ "$PWD" == */backend ]]; then
  echo "GATE_CALL functional-verification" >> "${BDD_STUB_LOG}"
  echo "federated catalogue readiness gate failed: terminal functional verification" >&2
  exit 42
fi
exit 71''',
    )

    for directory in (
        "backend/certs/dev",
        "frontend/ClientApp",
        "pdf-core/certs/dev",
        "scripts",
        "testWallet",
        "deployment/helm",
    ):
        (root / directory).mkdir(parents=True, exist_ok=True)
    script = root / "dev-stack.sh"
    script_text = DEV_STACK.read_text(encoding="utf-8")
    script_text = script_text.replace("/tmp/pdf-core-live.log", str(root / "pdf-core.log"))
    script_text = script_text.replace("/tmp/backend-live.log", str(root / "backend.log"))
    script.write_text(script_text, encoding="utf-8")
    env["DCS_DEV_DSS"] = "0"
    result = _run(["/bin/bash", script], timeout=20, env=env, cwd=root)
    result.observation = (
        context.fc_entrypoint_log_path.read_text(encoding="utf-8")
        + (root / "backend.log").read_text(encoding="utf-8", errors="replace")
        if (root / "backend.log").exists()
        else context.fc_entrypoint_log_path.read_text(encoding="utf-8")
    )
    return result


def _kubectl_entrypoint_stub():
    return r'''
args="$*"
if [[ "$args" == *"get deployment/fc-service"* ]]; then
  [[ "${BDD_STUB_MODE}" == "disabled" ]] && exit 1
  exit 0
fi
if [[ "$args" == *"rollout status deployment/fc-service"* ]]; then
  exit 0
fi
if [[ "$args" == *"rollout status deployment/dcs-digital-contracting-service"* ]]; then
  if [[ "${BDD_STUB_MODE}" == "disabled" ]]; then
    exit 0
  fi
  echo "GATE_CALL functional-verification" >> "${BDD_STUB_LOG}"
  echo "federated catalogue readiness gate failed: terminal functional verification" >&2
  exit 1
fi
if [[ "$args" == *"logs deployment/dcs-digital-contracting-service"* ]]; then
  if [[ "${BDD_STUB_MODE}" == "disabled" ]]; then
    echo "HTTP server listening"
  else
    echo "federated catalogue readiness gate failed: terminal functional verification"
  fi
  exit 0
fi
if [[ "$args" == *"get pod"* && "$args" == *"orce"* ]]; then
  echo "orce-pod"
  exit 0
fi
if [[ "$args" == *"get pod"* && "$args" == *"ipfs"* ]]; then
  echo "ipfs-pod"
  exit 0
fi
if [[ "$args" == *"get pod"* ]]; then
  echo "dcs-pod"
  exit 0
fi
if [[ "$args" == *"exec orce-pod"* && "$args" == *"printenv"* ]]; then
  echo "stub-archive-token"
  exit 0
fi
if [[ "$args" == *"get deployment/dcs2"* ]]; then
  exit 1
fi
exit 0'''


def _run_helm_entrypoint(context, mode):
    root, stub_bin, env = _entrypoint_stub_environment(context, mode)
    _install_common_stubs(stub_bin)
    _stub_command(stub_bin, "kubectl", _kubectl_entrypoint_stub())
    values_path = root / "values.yaml"
    values_path.write_text(
        "federatedCatalogue:\n"
        f"  enabled: {'false' if mode == 'disabled' else 'true'}\n",
        encoding="utf-8",
    )
    result = _run(
        [
            "/bin/bash",
            DEPLOY_SCRIPT,
            "--values",
            values_path,
            "--namespace",
            "stub-lifecycle",
            "--release",
            "dcs",
        ],
        timeout=20,
        env=env,
    )
    result.observation = context.fc_entrypoint_log_path.read_text(encoding="utf-8")
    return result


def _run_bdd_entrypoint(context, mode):
    root, stub_bin, env = _entrypoint_stub_environment(context, mode)
    _install_common_stubs(stub_bin)
    _stub_command(stub_bin, "kubectl", _kubectl_entrypoint_stub())
    _stub_command(
        stub_bin,
        "curl",
        r'''
code="200"
if [[ "${BDD_STUB_MODE}" != "disabled" && "$*" == *"/auth/login"* ]]; then
  code="503"
  if ( set -o noclobber; > "${BDD_STUB_STATE}" ) 2>/dev/null; then
    echo "GATE_CALL functional-verification" >> "${BDD_STUB_LOG}"
  fi
fi
if [[ "$*" == *"-w"* ]]; then
  printf "%s" "$code"
fi
exit 0''',
    )

    scripts_dir = root / "scripts"
    scripts_dir.mkdir()
    shutil.copy2(BDD_RUNNER, scripts_dir / "run_bdd_helm.sh")
    shutil.copy2(
        REPO_ROOT / "tests" / "bdd" / "scripts" / "keep_port_forward.sh",
        scripts_dir / "keep_port_forward.sh",
    )
    (scripts_dir / "check_status_list.py").touch()
    venv = root / "venv"
    (venv / "bin").mkdir(parents=True)
    (venv / "bin" / "activate").touch()
    _write_executable(
        venv / "bin" / "coverage",
        "#!/usr/bin/env bash\n"
        'echo "coverage $*" >> "${BDD_STUB_LOG}"\n'
        "exit 0\n",
    )
    project_root = root / "project"
    (project_root / "features").mkdir(parents=True)
    env.update(
        {
            "VENV_PATH": str(venv),
            "FEATURES_PATH": str(project_root / "features"),
            "KUBECTL_BIN": str(stub_bin / "kubectl"),
            "K8S_NAMESPACE": "stub-lifecycle",
            "DCS_DEPLOYMENT": "dcs-digital-contracting-service",
            "BDD_DCS_BASE_URL": "http://stub.localhost/api",
            "PROJECT_ROOT": str(project_root),
            "HELM_RELEASE": "dcs",
            "BDD_STUB_CURL_CODE": "200",
        }
    )
    result = _run(
        ["/bin/bash", scripts_dir / "run_bdd_helm.sh"],
        timeout=20,
        env=env,
        cwd=root,
    )
    result.observation = context.fc_entrypoint_log_path.read_text(encoding="utf-8")
    return result


def _run_entrypoint(context, entrypoint, mode):
    if entrypoint == "development":
        return _run_development_entrypoint(context, mode)
    if entrypoint == "Helm deployment":
        return _run_helm_entrypoint(context, mode)
    return _run_bdd_entrypoint(context, mode)


@given('Federated Catalogue integration is disabled for the isolated "{entrypoint}"')
def step_given_fc_disabled_for_entrypoint(context, entrypoint):
    assert entrypoint in {"development", "Helm deployment", "BDD runner"}
    context.fc_entrypoint = entrypoint
    context.fc_gate_mode = "disabled"


@given('Federated Catalogue integration is enabled for the isolated "{entrypoint}"')
def step_given_fc_enabled_for_entrypoint(context, entrypoint):
    assert entrypoint in {"development", "Helm deployment", "BDD runner"}
    context.fc_entrypoint = entrypoint
    context.fc_gate_mode = "enabled"


@given("catalogue health succeeds but functional verification fails")
def step_given_health_up_verification_fails(context):
    context.fc_failure_kind = "functional"


@given("the first functional catalogue verification returns a terminal error")
def step_given_terminal_verification_error(context):
    context.fc_failure_kind = "terminal"


@when('the lifecycle runner starts the "{entrypoint}"')
def step_when_lifecycle_runner_starts_entrypoint(context, entrypoint):
    assert entrypoint == context.fc_entrypoint
    mode = context.fc_gate_mode
    if getattr(context, "fc_failure_kind", None):
        mode = context.fc_failure_kind
    context.fc_entrypoint_result = _run_entrypoint(context, entrypoint, mode)


@then("it continues without executing a Federated Catalogue check")
def step_then_entrypoint_skips_fc(context):
    if context.fc_entrypoint == "development":
        observation = context.fc_entrypoint_result.observation
        assert context.fc_entrypoint_result.returncode == 1, (
            "The disabled development fixture must end only through the "
            f"controlled Vite stop, got exit {context.fc_entrypoint_result.returncode}"
        )
        assert "PDF_CORE_PROCESS_STARTED" in observation, observation
        assert "VITE_PROCESS_STARTED" in observation, observation
        assert "CONTINUED_PAST_FC_GATE backend-started" in observation, observation
        assert "FIXTURE_STOP after continued-past-FC-gate observation" in observation, (
            observation
        )
    else:
        assert context.fc_entrypoint_result.returncode == 0
    assert "GATE_CALL" not in context.fc_entrypoint_result.observation, (
        f"{context.fc_entrypoint} executed an FC gate while FC was disabled:\n"
        f"{context.fc_entrypoint_result.observation}"
    )


@then("it does not continue past the Federated Catalogue readiness gate")
def step_then_entrypoint_stops_at_fc_gate(context):
    assert context.fc_entrypoint_result.returncode != 0, (
        f"{context.fc_entrypoint} continued successfully after the FC gate failed"
    )
    assert context.fc_entrypoint_result.observation.count("GATE_CALL") == 1, (
        f"Expected one FC gate attempt, observed:\n{context.fc_entrypoint_result.observation}"
    )


@then("it exits immediately with the terminal Federated Catalogue diagnosis")
def step_then_terminal_error_diagnosed(context):
    step_then_entrypoint_stops_at_fc_gate(context)
    assert context.fc_failure_kind == "terminal"
    diagnostic = (
        context.fc_entrypoint_result.stdout
        + context.fc_entrypoint_result.stderr
        + context.fc_entrypoint_result.observation
    ).lower()
    assert "federated catalogue readiness gate failed" in diagnostic
    assert "terminal functional verification" in diagnostic


@then("it performs no blanket multi-minute catalogue wait")
def step_then_no_blanket_fc_wait(context):
    assert context.fc_entrypoint_result.elapsed_seconds < 5, (
        f"{context.fc_entrypoint} took {context.fc_entrypoint_result.elapsed_seconds:.2f}s "
        "after a deterministic terminal FC response"
    )


@then("it performs no schema-sync retry or artificial warm-up")
def step_then_no_fc_retry_or_warmup(context):
    observation = context.fc_entrypoint_result.observation
    assert observation.count("GATE_CALL") == 1, observation
    lowered = observation.lower()
    assert "schema-sync retry" not in lowered
    assert "verification warm-up" not in lowered
    assert "wait_for_fc" not in lowered
