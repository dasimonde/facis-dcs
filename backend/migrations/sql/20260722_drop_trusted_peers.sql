-- The static trusted_peers allowlist is replaced by the federation agreement
-- credential and the local policy endpoint (ADR-19): peer authorization is
-- expressed as policy at DCS_TRUST_PDP_URL rather than as a table row.
DROP TABLE IF EXISTS trusted_peers;
