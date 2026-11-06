-- Nonces of the peer challenges this instance accepted (ADR-13). A challenge
-- whose (from_did, nonce) is already here is a replay. A row is kept until
-- its challenge can no longer pass the iat window, so the table stays small.
CREATE TABLE peer_challenge_nonces
(
    from_did   TEXT        NOT NULL,
    nonce      TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (from_did, nonce)
);
CREATE INDEX peer_challenge_nonces_expires_at ON peer_challenge_nonces (expires_at);
