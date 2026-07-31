-- UC-14 / FR-SM-03: a fresh Power of Attorney credential is presented at the
-- moment of signing (deliberate, decoupled from the login credential — the
-- signatory may differ from whoever logged in). The signing ceremony captures the
-- verified PoA organization + roles; the organization is stamped onto the party
-- node at seal (dcs:hasPowerOfAttorney), so it rides the contract to peers and the
-- Signature Compliance Viewer (FR-SM-26) can flag any party — own or counterparty
-- — that signed with no PoA or a PoA authorizing a different organization than the
-- party it signed as.
ALTER TABLE signature_ceremonies ADD COLUMN poa_organization VARCHAR(255);
ALTER TABLE signature_ceremonies ADD COLUMN poa_roles        JSONB;
