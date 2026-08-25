-- 065_synthesis_output_kind.sql
-- BUG-004-004 SCOPE-02 — output kind, evaluated counts, and source-class
-- dispositions, so quiet and partial are DURABLE OUTPUT rather than absence.
--
-- WHY THIS IS A SEPARATE MIGRATION. 064 has already been applied to disposable
-- test databases. Editing an applied migration would leave those schemas and
-- the file disagreeing with nothing detecting it, so the column arrives
-- additively. The three-step add/backfill/constrain below is the standard shape
-- for adding a NOT NULL column to a table that may already hold rows.
--
-- THE DISTINCTION THIS EXISTS TO MAKE. SCN-004-004-07 requires that a valid
-- window producing no insights READS DIFFERENTLY from never-run and from
-- failure. Without an explicit kind those three states are indistinguishable:
-- all of them are "no insight rows". A quiet output is a positive assertion
-- that the window WAS evaluated and produced nothing, which is why it carries
-- evaluated_artifact_count -- zero insights out of zero artifacts is a
-- different fact from zero insights out of four hundred.
--
-- NO-DEFAULTS / G028: output_kind and evaluated_artifact_count carry no DB-side
-- DEFAULT once constrained. The DEFAULT-free backfill below sets historical
-- rows explicitly; every future row is written by app code.

ALTER TABLE synthesis_outputs ADD COLUMN IF NOT EXISTS output_kind TEXT;
ALTER TABLE synthesis_outputs ADD COLUMN IF NOT EXISTS evaluated_artifact_count INTEGER;

-- Any row written by 064 was, by construction, a full output: the only writer
-- at that point had no notion of quiet or partial. 'full' rather than
-- 'complete' because OutputKindFull in synthesis_health.go already owns this
-- vocabulary and a second spelling would be a second answer.
UPDATE synthesis_outputs SET output_kind = 'full' WHERE output_kind IS NULL;
UPDATE synthesis_outputs SET evaluated_artifact_count = 0 WHERE evaluated_artifact_count IS NULL;

ALTER TABLE synthesis_outputs ALTER COLUMN output_kind SET NOT NULL;
ALTER TABLE synthesis_outputs ALTER COLUMN evaluated_artifact_count SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'synthesis_outputs_kind_known'
    ) THEN
        ALTER TABLE synthesis_outputs
            ADD CONSTRAINT synthesis_outputs_kind_known
            CHECK (output_kind IN ('full', 'quiet', 'partial'));
    END IF;

    -- A quiet output asserts "evaluated, produced nothing", so it MUST carry no
    -- insights. Enforced here rather than in Go: a producer bug cannot write a
    -- quiet output that secretly holds insights.
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'synthesis_outputs_quiet_is_empty'
    ) THEN
        ALTER TABLE synthesis_outputs
            ADD CONSTRAINT synthesis_outputs_quiet_is_empty
            CHECK (output_kind <> 'quiet' OR (insight_count = 0 AND citation_count = 0));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'synthesis_outputs_evaluated_nonneg'
    ) THEN
        ALTER TABLE synthesis_outputs
            ADD CONSTRAINT synthesis_outputs_evaluated_nonneg
            CHECK (evaluated_artifact_count >= 0);
    END IF;
END
$$;

-- Which source classes contributed and which were left out.
--
-- SCN-004-004-08 requires a partial output to NAME its omissions rather than
-- carry prose about them. Rows are the only representation that a reader can
-- count and filter; a sentence in a text column is not checkable.
CREATE TABLE IF NOT EXISTS synthesis_output_source_classes (
    id              BIGSERIAL   PRIMARY KEY,
    output_id       TEXT        NOT NULL
                                REFERENCES synthesis_outputs (id) ON DELETE CASCADE,
    source_class    TEXT        NOT NULL,
    disposition     TEXT        NOT NULL,   -- 'included' | 'omitted'
    CONSTRAINT synthesis_output_source_classes_unique UNIQUE (output_id, source_class),
    CONSTRAINT synthesis_output_source_classes_disposition_known
        CHECK (disposition IN ('included', 'omitted'))
);

CREATE INDEX IF NOT EXISTS idx_synthesis_output_source_classes_output
    ON synthesis_output_source_classes (output_id);
