# DCS Federation Rules

These are the rules every DCS instance agrees to before it exchanges
contracts with another instance (see [ADR-19](../../../../docs/adr-19-federation-agreement-credential.md)).
An instance's agreement to them is embedded in the source it runs: the
document is compiled into the binary, and its hash is what every peer's
agreement credential names.

1. The federator of the DCS agrees that users operating the system that are
   designated signatories are legally allowed to represent the operating
   party.
2. The federator operates its instance's private keys (DID, VC, PAdES/C2PA)
   exclusively within a PKCS#11-conformant cryptographic module under its own
   control; no key material is shared with, or escrowed by, any other party.
3. The federator does not alter a contract's machine-readable payload after
   it has been shipped to a counterparty without shipping the corresponding
   amendment through the same federation channel.
4. The federator honors a counterparty's revocation of an applied signature
   by propagating the resulting contract state without delay.
