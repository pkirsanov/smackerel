-- Migration: 009_maps.sql
-- Location clusters for Maps Timeline connector.
-- Supports commute/trip detection (Scope 3) and dedup by spatial clustering.

CREATE TABLE IF NOT EXISTS location_clusters (
    id TEXT PRIMARY KEY,
    source_ref TEXT NOT NULL,
    start_cluster_lat DOUBLE PRECISION NOT NULL,
    start_cluster_lng DOUBLE PRECISION NOT NULL,
    end_cluster_lat DOUBLE PRECISION NOT NULL,
    end_cluster_lng DOUBLE PRECISION NOT NULL,
    activity_type TEXT NOT NULL,
    activity_date DATE NOT NULL,
    day_of_week SMALLINT NOT NULL,
    departure_hour SMALLINT NOT NULL,
    distance_km DOUBLE PRECISION NOT NULL,
    duration_min DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_location_clusters_route ON location_clusters (start_cluster_lat, start_cluster_lng, end_cluster_lat, end_cluster_lng);
CREATE INDEX IF NOT EXISTS idx_location_clusters_day ON location_clusters (day_of_week, departure_hour);
CREATE INDEX IF NOT EXISTS idx_location_clusters_date ON location_clusters (activity_date);
