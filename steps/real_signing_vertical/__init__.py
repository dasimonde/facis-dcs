"""Real-signing-vertical (PAdES + wallet-driven signing ceremony + PID binding,
features/22_real_signing_vertical) BDD step definitions.

NOTE: this __init__.py MUST actually
import the step module below — an empty __init__.py makes behave silently
fail to discover every step in this package (steps become "undefined" at
runtime with no import error). See steps/peer_trust/__init__.py and
steps/pki_consolidation/__init__.py for the same, correct pattern.
"""

from . import dcs_real_signing_vertical_steps  # noqa: F401
from . import dcs_real_signing_vertical_tamper_steps  # noqa: F401
from . import dcs_real_signing_vertical_orce_steps  # noqa: F401
from . import multi_signer_steps  # noqa: F401
from . import dcs_oid4vp_document_retrieval_steps  # noqa: F401
from . import poa_at_signing_steps  # noqa: F401
from . import dcs_signing_acceptance_hardening_steps  # noqa: F401
