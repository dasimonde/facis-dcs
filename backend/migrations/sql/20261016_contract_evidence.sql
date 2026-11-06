CREATE TABLE contract_evidence (evidence_id TEXT PRIMARY KEY, contract_did TEXT NOT NULL, artifact_cid TEXT NOT NULL);
CREATE INDEX contract_evidence_did ON contract_evidence(contract_did);
