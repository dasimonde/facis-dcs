"""The dcs-peer-v1 challenge every peer request carries (ADR-13; the backend
builds and checks it in backend/internal/dcstodcs/peerchallenge.go).

The signed secret_value is

    <op>|<contract_iri>|dcs-peer-v1|POST <route>|<from_did>|<to_did>|<iat>|<nonce>|<body sha256 hex or "">

The receiver refuses a challenge for another operation, contract, sender,
receiver or body, one whose iat is more than 120 s away from its clock, and
one whose nonce it already accepted. A step that sends a peer request
therefore builds a new challenge for each request.
"""

import hashlib
import time
import uuid

PEER_ROUTES = {
    "post_pdf": "/peer/contracts/pdf",
    "post_settlement": "/peer/contracts/settlement",
    "post_settlement_withdrawal": "/peer/contracts/settlement-withdrawal",
    "post_negotiation": "/peer/contracts/negotiation",
    "erase": "/peer/contracts/erase",
    "get_contract_info": "/peer/contracts/info",
    "terminate": "/peer/contracts/terminate",
}


def peer_challenge(op: str, contract_iri: str, from_did: str, to_did: str, body: bytes = b"") -> str:
    """The secret_value of one peer request; body is the field the operation
    binds (the PDF of post_pdf, the JAdES of a settlement)."""
    digest = hashlib.sha256(body).hexdigest() if body else ""
    return "|".join([
        op, contract_iri, "dcs-peer-v1", "POST " + PEER_ROUTES[op], from_did, to_did,
        str(int(time.time())), uuid.uuid4().hex, digest,
    ])
