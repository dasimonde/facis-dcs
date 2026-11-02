-- DCS-NFR-BR-09: the Federated Catalogue asset a published template version
-- became. Deprecation revokes this asset, so only approved templates stay
-- consumable from the catalogue.
ALTER TABLE contract_templates ADD COLUMN catalogue_asset_hash TEXT;
