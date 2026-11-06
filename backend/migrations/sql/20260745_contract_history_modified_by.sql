-- A contract_history row is the snapshot a save takes of the version it is
-- about to replace. modified_by and modified_at record that save: the actor
-- whose accepted change replaced this version, and when (DCS-FR-CWE-08).
-- created_by stays the contract's creator and does not identify the editor.
--
-- Rows written before this migration stay NULL: the actor of those saves was
-- not recorded, and copying created_by would attribute every edit to the
-- creator.
ALTER TABLE contract_history
    ADD COLUMN IF NOT EXISTS modified_by VARCHAR(255),
    ADD COLUMN IF NOT EXISTS modified_at TIMESTAMP;
