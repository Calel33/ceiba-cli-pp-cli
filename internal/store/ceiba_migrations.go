// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Ceiba mirror schema (hand-written; generator-owned store.go is untouched).
// These tables hold the scrape-enriched listing data the generated
// resources/typed tables do not carry: card facts, detail-page specs, and a
// per-observation history used by the `changes` digest.

package store

import (
	"context"
)

// ceibaMirrorSchemaStatements create the three Ceiba mirror tables. All
// statements are idempotent (CREATE TABLE IF NOT EXISTS) so the schema can be
// ensured lazily on every write path without a version gate.
var ceibaMirrorSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS listing_facts (
		wp_id INTEGER PRIMARY KEY,
		slug TEXT UNIQUE,
		price_usd REAL,
		acres REAL,
		bullets TEXT,
		spotlight INTEGER,
		under_contract INTEGER,
		sold INTEGER,
		district_names TEXT,
		town_name TEXT,
		property_type TEXT,
		waterfront TEXT,
		updated_at TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS listing_details (
		wp_id INTEGER PRIMARY KEY,
		listing_id TEXT,
		beds INTEGER,
		baths INTEGER,
		living_space REAL,
		gallery_json TEXT,
		tour_url TEXT,
		map_lat REAL,
		map_lng REAL,
		brochure_json TEXT,
		similar_json TEXT,
		scraped_at TEXT,
		source_url TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS listing_history (
		wp_id INTEGER,
		observed_at TEXT,
		price_usd REAL,
		acres REAL,
		spotlight INTEGER,
		under_contract INTEGER,
		sold INTEGER,
		PRIMARY KEY (wp_id, observed_at)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_listing_facts_slug ON listing_facts(slug)`,
	`CREATE INDEX IF NOT EXISTS idx_listing_details_listing_id ON listing_details(listing_id)`,
	`CREATE INDEX IF NOT EXISTS idx_listing_history_observed ON listing_history(observed_at)`,
}

// EnsureCeibaMirrorTables creates the Ceiba mirror tables when missing. Only
// call this on a write-capable store connection (store.Open/OpenWithContext);
// read-only opens must use CeibaMirrorPresent and treat absence as an empty
// mirror.
func EnsureCeibaMirrorTables(ctx context.Context, s *Store) error {
	if s == nil {
		return nil
	}
	for _, stmt := range ceibaMirrorSchemaStatements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// CeibaMirrorPresent reports whether all three Ceiba mirror tables exist.
// Pure SELECT against sqlite_master, safe on read-only connections.
func CeibaMirrorPresent(ctx context.Context, s *Store) (bool, error) {
	if s == nil {
		return false, nil
	}
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('listing_facts','listing_details','listing_history')`,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n == 3, nil
}
