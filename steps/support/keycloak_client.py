"""Keycloak bootstrap helpers for executable BDD scenarios."""

import os
import time

import requests


def keycloak_base_url() -> str:
    return os.getenv("BDD_KEYCLOAK_BASE_URL", "http://127.0.0.1:18080").rstrip("/")


def keycloak_realm() -> str:
    return os.getenv("BDD_KEYCLOAK_REALM", "master")


def _keycloak_host_override_headers() -> dict:
    """Return optional Host header override for local port-forward setups."""
    host_header = os.getenv("BDD_KEYCLOAK_HOST_HEADER", "").strip()
    return {"Host": host_header} if host_header else {}


def _request_with_retry(method: str, url: str, retries: int = 12, delay_seconds: float = 1.5, **kwargs):
    """Retry transient Keycloak connection failures during startup/port-forward races."""
    last_error = None
    for attempt in range(1, retries + 1):
        try:
            return requests.request(method=method, url=url, **kwargs)
        except requests.RequestException as err:
            last_error = err
            if attempt == retries:
                break
            time.sleep(delay_seconds)
    raise AssertionError(f"Keycloak request failed after {retries} attempts: {method} {url} ({last_error})")


def admin_token() -> str:
    response = _request_with_retry(
        "POST",
        f"{keycloak_base_url()}/realms/master/protocol/openid-connect/token",
        data={
            "grant_type": "password",
            "client_id": "admin-cli",
            "username": os.getenv("BDD_KEYCLOAK_ADMIN_USERNAME", "admin"),
            "password": os.getenv("BDD_KEYCLOAK_ADMIN_PASSWORD", "admin"),
        },
        headers=_keycloak_host_override_headers(),
        timeout=20,
    )
    assert response.status_code == 200, response.text
    return response.json()["access_token"]


def ensure_client(admin_access_token: str, client_id: str) -> str:
    headers = {"Authorization": f"Bearer {admin_access_token}", **_keycloak_host_override_headers()}
    realm = keycloak_realm()
    response = _request_with_retry(
        "GET",
        f"{keycloak_base_url()}/admin/realms/{realm}/clients",
        params={"clientId": client_id},
        headers=headers,
        timeout=20,
    )
    assert response.status_code == 200, response.text
    clients = response.json()
    if not clients:
        create_response = _request_with_retry(
            "POST",
            f"{keycloak_base_url()}/admin/realms/{realm}/clients",
            json={
                "clientId": client_id,
                "protocol": "openid-connect",
                "enabled": True,
                "publicClient": True,
                "directAccessGrantsEnabled": True,
                "standardFlowEnabled": True,
                "serviceAccountsEnabled": False,
            },
            headers={**headers, "Content-Type": "application/json"},
            timeout=20,
        )
        assert create_response.status_code in (201, 204), create_response.text
        response = _request_with_retry(
            "GET",
            f"{keycloak_base_url()}/admin/realms/{realm}/clients",
            params={"clientId": client_id},
            headers=headers,
            timeout=20,
        )
        assert response.status_code == 200, response.text
        clients = response.json()
    assert clients and clients[0].get("id"), clients
    return clients[0]["id"]


def ensure_client_role(admin_access_token: str, client_uuid: str, role_name: str) -> dict:
    headers = {"Authorization": f"Bearer {admin_access_token}", **_keycloak_host_override_headers()}
    realm = keycloak_realm()
    role_url = f"{keycloak_base_url()}/admin/realms/{realm}/clients/{client_uuid}/roles/{role_name}"
    response = _request_with_retry("GET", role_url, headers=headers, timeout=20)
    if response.status_code == 404:
        create_response = _request_with_retry(
            "POST",
            f"{keycloak_base_url()}/admin/realms/{realm}/clients/{client_uuid}/roles",
            json={"name": role_name},
            headers={**headers, "Content-Type": "application/json"},
            timeout=20,
        )
        assert create_response.status_code in (201, 204), create_response.text
        response = _request_with_retry("GET", role_url, headers=headers, timeout=20)
    assert response.status_code == 200, response.text
    return response.json()


def ensure_user(admin_access_token: str, username: str, password: str) -> str:
    headers = {"Authorization": f"Bearer {admin_access_token}", **_keycloak_host_override_headers()}
    realm = keycloak_realm()
    query_response = _request_with_retry(
        "GET",
        f"{keycloak_base_url()}/admin/realms/{realm}/users",
        params={"username": username, "exact": "true"},
        headers=headers,
        timeout=20,
    )
    assert query_response.status_code == 200, query_response.text
    users = query_response.json()
    if users:
        user_id = users[0]["id"]
    else:
        create_response = _request_with_retry(
            "POST",
            f"{keycloak_base_url()}/admin/realms/{realm}/users",
            json={
                "username": username,
                "email": f"{username}@bdd.test",
                "firstName": "BDD",
                "lastName": username,
                "enabled": True,
                "emailVerified": True,
                "requiredActions": [],
            },
            headers={**headers, "Content-Type": "application/json"},
            timeout=20,
        )
        assert create_response.status_code in (201, 204), create_response.text
        query_response = _request_with_retry(
            "GET",
            f"{keycloak_base_url()}/admin/realms/{realm}/users",
            params={"username": username, "exact": "true"},
            headers=headers,
            timeout=20,
        )
        assert query_response.status_code == 200, query_response.text
        users = query_response.json()
        assert users, "Failed to create Keycloak user"
        user_id = users[0]["id"]

    reset_response = _request_with_retry(
        "PUT",
        f"{keycloak_base_url()}/admin/realms/{realm}/users/{user_id}/reset-password",
        json={"type": "password", "temporary": False, "value": password},
        headers={**headers, "Content-Type": "application/json"},
        timeout=20,
    )
    assert reset_response.status_code in (204, 200), reset_response.text

    # Keycloak 26+ may still attach required actions after reset-password; clear them explicitly.
    clear_response = _request_with_retry(
        "PUT",
        f"{keycloak_base_url()}/admin/realms/{realm}/users/{user_id}",
        json={"requiredActions": []},
        headers={**headers, "Content-Type": "application/json"},
        timeout=20,
    )
    assert clear_response.status_code in (204, 200), clear_response.text
    return user_id


def assign_client_role(admin_access_token: str, user_id: str, client_uuid: str, role_repr: dict) -> None:
    headers = {
        "Authorization": f"Bearer {admin_access_token}",
        "Content-Type": "application/json",
        **_keycloak_host_override_headers(),
    }
    realm = keycloak_realm()
    response = _request_with_retry(
        "POST",
        f"{keycloak_base_url()}/admin/realms/{realm}/users/{user_id}/role-mappings/clients/{client_uuid}",
        json=[{"id": role_repr["id"], "name": role_repr["name"]}],
        headers=headers,
        timeout=20,
    )
    assert response.status_code in (204, 200), response.text


def user_token(client_id: str, username: str, password: str) -> str:
    realm = keycloak_realm()
    response = _request_with_retry(
        "POST",
        f"{keycloak_base_url()}/realms/{realm}/protocol/openid-connect/token",
        data={
            "grant_type": "password",
            "client_id": client_id,
            "username": username,
            "password": password,
            "scope": "openid",
        },
        headers=_keycloak_host_override_headers(),
        timeout=20,
    )
    assert response.status_code == 200, response.text
    return response.json()["access_token"]
