-- Google Keep connector tables
-- Migration: 004_keep.sql

-- OCR result cache: avoid re-processing images already OCR'd
CREATE TABLE IF NOT EXISTS ocr_cache (
    image_hash TEXT PRIMARY KEY,
    extracted_text TEXT NOT NULL,
    ocr_engine TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ocr_cache_created ON ocr_cache(created_at);

-- Keep export tracking: avoid reprocessing already-imported exports
CREATE TABLE IF NOT EXISTS keep_exports (
    export_path TEXT PRIMARY KEY,
    notes_parsed INTEGER NOT NULL DEFAULT 0,
    notes_failed INTEGER NOT NULL DEFAULT 0,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
