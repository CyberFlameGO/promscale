/******************************************************************************
    CREATE NEW DOMAIN FOR TAGS
******************************************************************************/
CALL SCHEMA_CATALOG.execute_everywhere('tracing_types', $ee$ DO $$ BEGIN
    CREATE DOMAIN SCHEMA_TRACING_PUBLIC.tag_maps jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(value) = 'array');
    GRANT USAGE ON DOMAIN SCHEMA_TRACING_PUBLIC.tag_maps TO prom_reader;
END $$ $ee$);

/******************************************************************************
    ALTER THE SPAN TABLE
******************************************************************************/
ALTER TABLE SCHEMA_TRACING.span ADD COLUMN IF NOT EXISTS tags SCHEMA_TRACING_PUBLIC.tag_maps NOT NULL DEFAULT '[{},{},{}]'::jsonb;
UPDATE SCHEMA_TRACING.span u
SET tags = jsonb_build_array(
    '{}'::jsonb, -- reserved for "special" tags
    u.span_tags,
    u.resource_tags
)
WHERE true
;
ALTER TABLE SCHEMA_TRACING.span
    DROP COLUMN IF EXISTS span_tags CASCADE ,
    DROP COLUMN IF EXISTS resource_tags CASCADE
;
CREATE INDEX IF NOT EXISTS span_tags_idx ON SCHEMA_TRACING.span USING gin (tags jsonb_path_ops);

/******************************************************************************
    RENAME OLD FUNCTIONS TO "MAKE ROOM" FOR NEW FUNCTIONS
******************************************************************************/
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.eval_equals                 (SCHEMA_TAG.tag_op_equals                                               ) RENAME TO tag_map_eval_equals                 ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.eval_greater_than           (SCHEMA_TAG.tag_op_greater_than                                         ) RENAME TO tag_map_eval_greater_than           ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.eval_greater_than_or_equal  (SCHEMA_TAG.tag_op_greater_than_or_equal                                ) RENAME TO tag_map_eval_greater_than_or_equal  ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.eval_jsonb_path_exists      (SCHEMA_TAG.tag_op_jsonb_path_exists                                    ) RENAME TO tag_map_eval_jsonb_path_exists      ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.eval_less_than              (SCHEMA_TAG.tag_op_less_than                                            ) RENAME TO tag_map_eval_less_than              ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.eval_less_than_or_equal     (SCHEMA_TAG.tag_op_less_than_or_equal                                   ) RENAME TO tag_map_eval_less_than_or_equal     ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.eval_not_equals             (SCHEMA_TAG.tag_op_not_equals                                           ) RENAME TO tag_map_eval_not_equals             ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.eval_regexp_matches         (SCHEMA_TAG.tag_op_regexp_matches                                       ) RENAME TO tag_map_eval_regexp_matches         ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.eval_regexp_not_matches     (SCHEMA_TAG.tag_op_regexp_not_matches                                   ) RENAME TO tag_map_eval_regexp_not_matches     ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.eval_tags_by_key            (SCHEMA_TRACING_PUBLIC.tag_k                                            ) RENAME TO tag_map_eval_tags_by_key            ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.get_tag_id                  (SCHEMA_TRACING_PUBLIC.tag_map, SCHEMA_TRACING_PUBLIC.tag_k             ) RENAME TO tag_map_get_tag_id                  ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.has_tag                     (SCHEMA_TRACING_PUBLIC.tag_map, SCHEMA_TRACING_PUBLIC.tag_k             ) RENAME TO tag_map_has_tag                     ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.match_equals                (SCHEMA_TRACING_PUBLIC.tag_map, SCHEMA_TAG.tag_op_equals                ) RENAME TO tag_map_match_equals                ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.match_greater_than          (SCHEMA_TRACING_PUBLIC.tag_map, SCHEMA_TAG.tag_op_greater_than          ) RENAME TO tag_map_match_greater_than          ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.match_greater_than_or_equal (SCHEMA_TRACING_PUBLIC.tag_map, SCHEMA_TAG.tag_op_greater_than_or_equal ) RENAME TO tag_map_match_greater_than_or_equal ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.match_jsonb_path_exists     (SCHEMA_TRACING_PUBLIC.tag_map, SCHEMA_TAG.tag_op_jsonb_path_exists     ) RENAME TO tag_map_match_jsonb_path_exists     ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.match_less_than             (SCHEMA_TRACING_PUBLIC.tag_map, SCHEMA_TAG.tag_op_less_than             ) RENAME TO tag_map_match_less_than             ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.match_less_than_or_equal    (SCHEMA_TRACING_PUBLIC.tag_map, SCHEMA_TAG.tag_op_less_than_or_equal    ) RENAME TO tag_map_match_less_than_or_equal    ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.match_not_equals            (SCHEMA_TRACING_PUBLIC.tag_map, SCHEMA_TAG.tag_op_not_equals            ) RENAME TO tag_map_match_not_equals            ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.match_regexp_matches        (SCHEMA_TRACING_PUBLIC.tag_map, SCHEMA_TAG.tag_op_regexp_matches        ) RENAME TO tag_map_match_regexp_matches        ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;
DO $do$ BEGIN ALTER FUNCTION SCHEMA_TRACING_PUBLIC.match_regexp_not_matches    (SCHEMA_TRACING_PUBLIC.tag_map, SCHEMA_TAG.tag_op_regexp_not_matches    ) RENAME TO tag_map_match_regexp_not_matches    ; EXCEPTION WHEN SQLSTATE '42883' THEN null; END; $do$;

