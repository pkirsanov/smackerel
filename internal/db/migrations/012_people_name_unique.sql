-- 012_people_name_unique.sql
-- Add UNIQUE constraint to people.name for ON CONFLICT upsert in graph linker.
-- (GAP-001: findOrCreatePeople uses ON CONFLICT (name) but no unique constraint existed.)
--
-- ROLLBACK:
--   DROP INDEX IF EXISTS idx_people_name_unique;

-- Step 1: Merge duplicate people rows (keep the earliest ULID per name).
-- Re-parent edges that reference discarded duplicates to the kept row.
DO $$
DECLARE
    dup RECORD;
BEGIN
    FOR dup IN
        SELECT name, MIN(id) AS keep_id
        FROM people
        GROUP BY name
        HAVING COUNT(*) > 1
    LOOP
        -- Re-parent edges where duplicate person is dst
        UPDATE edges SET dst_id = dup.keep_id
        WHERE dst_type = 'person'
          AND dst_id IN (SELECT id FROM people WHERE name = dup.name AND id != dup.keep_id)
          AND NOT EXISTS (
              SELECT 1 FROM edges e2
              WHERE e2.src_type = edges.src_type
                AND e2.src_id = edges.src_id
                AND e2.dst_type = edges.dst_type
                AND e2.dst_id = dup.keep_id
                AND e2.edge_type = edges.edge_type
          );

        -- Re-parent edges where duplicate person is src
        UPDATE edges SET src_id = dup.keep_id
        WHERE src_type = 'person'
          AND src_id IN (SELECT id FROM people WHERE name = dup.name AND id != dup.keep_id)
          AND NOT EXISTS (
              SELECT 1 FROM edges e2
              WHERE e2.src_type = edges.src_type
                AND e2.src_id = dup.keep_id
                AND e2.dst_type = edges.dst_type
                AND e2.dst_id = edges.dst_id
                AND e2.edge_type = edges.edge_type
          );

        -- Delete orphaned edges (duplicates that couldn't be re-parented)
        DELETE FROM edges
        WHERE (dst_type = 'person' AND dst_id IN (SELECT id FROM people WHERE name = dup.name AND id != dup.keep_id))
           OR (src_type = 'person' AND src_id IN (SELECT id FROM people WHERE name = dup.name AND id != dup.keep_id));

        -- Merge interaction counts into the kept row
        UPDATE people SET
            interaction_count = interaction_count + COALESCE((
                SELECT SUM(interaction_count) FROM people WHERE name = dup.name AND id != dup.keep_id
            ), 0),
            last_interaction = GREATEST(last_interaction, (
                SELECT MAX(last_interaction) FROM people WHERE name = dup.name AND id != dup.keep_id
            ))
        WHERE id = dup.keep_id;

        -- Delete duplicate rows
        DELETE FROM people WHERE name = dup.name AND id != dup.keep_id;
    END LOOP;
END $$;

-- Step 2: Add the unique index (idempotent)
CREATE UNIQUE INDEX IF NOT EXISTS idx_people_name_unique ON people(name);
