"""Template management BDD step definitions."""

# Import all step modules to register them with behave
from . import contract_api_steps  # noqa: F401
from . import contract_workflow_steps  # noqa: F401
from . import contract_state_machine_steps  # noqa: F401
from . import template_api_steps  # noqa: F401
from . import template_workflow_steps  # noqa: F401
from . import schema_api_steps  # noqa: F401
from . import dcs_contract_hierarchy_steps  # noqa: F401
from . import template_catalogue_steps  # noqa: F401
from . import federated_catalogue_deployment_lifecycle_steps  # noqa: F401
from . import template_integrity_audit_steps  # noqa: F401
from . import contract_creation_extra_steps  # noqa: F401
from . import contract_approval_extra_steps  # noqa: F401
from . import contract_negotiation_draft_steps  # noqa: F401
from . import contract_accept_offer_steps  # noqa: F401
from . import contract_negotiation_extra_steps  # noqa: F401
from . import contract_format_review_extra_steps  # noqa: F401
from . import template_notification_steps  # noqa: F401
from . import template_provenance_steps  # noqa: F401
from . import document_number_removal_steps  # noqa: F401
from . import realistic_contract_field_artifact_flow_steps  # noqa: F401
# Last: this module reaches into the peer-trust, deployment and contract
# service packs, several of which import back into this package.
from . import sla_federation_steps  # noqa: F401
