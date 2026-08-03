-- DCS-OR-C2PA-005: a contract's revocation entry is allocated, not hashed.
--
-- The index a lifecycle credential advertises used to be
-- BigEndian.Uint32(sha256(did)[:4]) % 131072 with nothing recording who held
-- what. Two subjects therefore landed on the same bit at even odds somewhere
-- past a few hundred of them, and because that bit IS the contract's revocation
-- state, terminating one contract marked an unrelated one revoked — silently,
-- and irreversibly from a verifier's point of view.
--
-- status_list_entries is the allocation: one row per subject (a contract or
-- template DID), written once and never rewritten, so the entry a credential
-- advertises stays valid for the life of the subject and across every
-- re-issuance of its credentials. There is deliberately no foreign key to
-- contracts: subjects come from two tables, and the row must outlive an erased
-- or deleted contract because the slot must never be handed to anyone else.
--
-- Subjects that predate this table keep the index their credentials in the wild
-- already carry: the backfill below reproduces the retired Go expression
-- exactly (verified against it for the full byte range) and marks those rows
-- 'hash', so a reader can tell an assignment this service chose from one it
-- inherited.
--
-- Inherited rows are NOT covered by the unique slot index. Two of them may
-- already share a slot — that is the defect — and rejecting them here would
-- fail the migration on precisely the deployments that have the problem, while
-- dropping one would silently move a contract off the entry its issued
-- credentials name. They are preserved as they are; new allocations are unique
-- among themselves and skip every occupied slot, inherited ones included, so
-- the damage cannot spread. Inherited collisions are reported by
--
--   SELECT list_id, entry_index, array_agg(subject_id)
--     FROM status_list_entries
--    GROUP BY list_id, entry_index HAVING count(*) > 1;
--
-- and what to do about a pair it finds — leave both sharing the bit, or reassign
-- one and orphan its issued credentials — is an operator decision, not this
-- migration's.
CREATE TABLE IF NOT EXISTS status_list_entries
(
    subject_id   VARCHAR(255) NOT NULL,
    list_id      INTEGER      NOT NULL,
    entry_index  INTEGER      NOT NULL,
    origin       VARCHAR(16)  NOT NULL,
    allocated_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT pk_status_list_entries PRIMARY KEY (subject_id),
    CONSTRAINT chk_status_list_entries_index CHECK (entry_index >= 0),
    CONSTRAINT chk_status_list_entries_origin CHECK (origin IN ('allocated', 'hash'))
);

-- Two subjects this service allocated for can never share a slot, whatever order
-- concurrent allocations interleave in. The allocator also refuses any slot an
-- inherited row holds, which this index cannot express (see above).
CREATE UNIQUE INDEX IF NOT EXISTS idx_status_list_entries_allocated_slot
    ON status_list_entries (list_id, entry_index)
 WHERE origin = 'allocated';

-- The occupancy lookup the allocator does before claiming a slot.
CREATE INDEX IF NOT EXISTS idx_status_list_entries_slot
    ON status_list_entries (list_id, entry_index);

-- One row per status list the statuslist-service serves for this tenant, holding
-- the next index to hand out and where the list ends. Bumping next_index is the
-- atomic step that makes an allocation exactly-once under concurrency.
--
-- Rolling over to a second list is an operator action in two parts, because the
-- XFSC statuslist-service only creates a list in response to a NATS "create"
-- event (see deployment/helm/charts/statuslist-service/templates/bdd-list-init-job.yaml)
-- and assigns the list id itself: create the list there, INSERT its id here,
-- then point STATUSLIST_LIST_ID at it. Until that happens a full list is a hard
-- failure — an allocator that wrapped would be reintroducing the collision this
-- migration exists to remove.
CREATE TABLE IF NOT EXISTS status_list_cursors
(
    list_id    INTEGER NOT NULL,
    next_index INTEGER NOT NULL DEFAULT 0,
    list_size  INTEGER NOT NULL,
    CONSTRAINT pk_status_list_cursors PRIMARY KEY (list_id),
    CONSTRAINT chk_status_list_cursors_bounds CHECK (next_index >= 0 AND next_index <= list_size)
);

-- List 1, 2^17 entries: the list the service is deployed with (statuslist-service
-- values.yaml listSize) and the only one any credential issued so far names.
INSERT INTO status_list_cursors (list_id, next_index, list_size)
VALUES (1, 0, 131072)
    ON CONFLICT (list_id) DO NOTHING;

INSERT INTO status_list_entries (subject_id, list_id, entry_index, origin)
SELECT did,
       1,
       ((('x' || encode(substring(sha256(convert_to(did, 'UTF8')) FROM 1 FOR 4), 'hex'))::bit(32)::bigint
           + 4294967296) % 4294967296) % 131072,
       'hash'
  FROM contracts
    ON CONFLICT (subject_id) DO NOTHING;

INSERT INTO status_list_entries (subject_id, list_id, entry_index, origin)
SELECT did,
       1,
       ((('x' || encode(substring(sha256(convert_to(did, 'UTF8')) FROM 1 FOR 4), 'hex'))::bit(32)::bigint
           + 4294967296) % 4294967296) % 131072,
       'hash'
  FROM contract_templates
    ON CONFLICT (subject_id) DO NOTHING;
