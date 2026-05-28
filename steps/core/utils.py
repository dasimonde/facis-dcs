from datetime import datetime
import uuid


def is_date(value: str, formats: list[str] = None) -> bool:
    if formats is None:
        formats = [
            "%Y-%m-%d",               # 2024-01-01
            "%Y-%m-%dT%H:%M:%S",      # 2024-01-01T00:00:00
            "%Y-%m-%dT%H:%M:%SZ",     # 2024-01-01T00:00:00Z
            "%Y-%m-%dT%H:%M:%S.%f",   # 2024-01-01T00:00:00.000000
            "%d.%m.%Y",               # 01.01.2024
            "%d/%m/%Y",               # 01/01/2024
        ]
    return any(
        _try_parse(value, fmt) for fmt in formats
    )

def _try_parse(value: str, fmt: str) -> bool:
    try:
        datetime.strptime(value, fmt)
        return True
    except ValueError:
        return False


def is_uuid(value: str) -> bool:
    try:
        uuid.UUID(str(value))
        return True
    except ValueError:
        return False