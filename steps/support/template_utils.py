"""Template-related helper functions for executable BDD scenarios."""

import re


def template_env_key(name: str) -> str:
    normalized = re.sub(r"[^A-Za-z0-9]+", "_", name).strip("_").upper()
    return f"BDD_TEMPLATE_DID_{normalized}"


def template_type_for_category(category: str) -> str:
    category_key = category.strip().lower()
    return {
        "legal": "FRAME_CONTRACT",
        "procurement": "SUB_CONTRACT",
    }.get(category_key, category.strip().upper().replace(" ", "_"))
