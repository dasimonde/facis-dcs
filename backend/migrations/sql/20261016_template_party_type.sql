ALTER TABLE contract_templates ADD COLUMN party_type TEXT NOT NULL CHECK (party_type IN ('SINGLE_PARTY', 'MULTI_PARTY'));
ALTER TABLE contract_templates_history ADD COLUMN party_type TEXT NOT NULL CHECK (party_type IN ('SINGLE_PARTY', 'MULTI_PARTY'));
