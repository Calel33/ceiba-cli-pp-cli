// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Typed read/write helpers for the Ceiba mirror tables (see
// ceiba_migrations.go). The cli package converts scrape results into these
// structs; SQL stays here. Writes are single-statement upserts (no
// store.Upsert inside a transaction), reads drain their cursor before any
// write, and every nullable column scans through sql.Null*.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// CeibaEnrichMarkerResource is the synthetic sync_state key whose
// last_cursor stores the session-start timestamp of the most recent
// enrichment pass (RFC3339Nano) and whose last_synced_at stores when the
// pass finished.
const CeibaEnrichMarkerResource = "ceiba-mirror-enrich"

// CeibaSimilarRef is one stored similar-listing reference on a detail row.
type CeibaSimilarRef struct {
	WPID     int64    `json:"wp_id,omitempty"`
	Slug     string   `json:"slug,omitempty"`
	URL      string   `json:"url,omitempty"`
	Title    string   `json:"title,omitempty"`
	PriceUSD *float64 `json:"price_usd,omitempty"`
	ImageURL string   `json:"image_url,omitempty"`
}

// CeibaFact mirrors one listing_facts row.
type CeibaFact struct {
	WPID          int64
	Slug          string
	PriceUSD      *float64
	Acres         *float64
	Bullets       []string
	Spotlight     bool
	UnderContract bool
	Sold          bool
	DistrictNames []string
	TownName      string
	PropertyType  string
	Waterfront    string
	UpdatedAt     string
}

// CeibaDetail mirrors one listing_details row.
type CeibaDetail struct {
	WPID         int64
	ListingID    string
	Beds         *float64
	Baths        *float64
	LivingSpace  *float64
	GalleryURLs  []string
	TourURL      string
	MapLat       *float64
	MapLng       *float64
	BrochureURLs []string
	Similar      []CeibaSimilarRef
	ScrapedAt    string
	SourceURL    string
}

// CeibaHistoryRow mirrors one listing_history row.
type CeibaHistoryRow struct {
	WPID          int64
	ObservedAt    string
	PriceUSD      *float64
	Acres         *float64
	Spotlight     bool
	UnderContract bool
	Sold          bool
}

func marshalStrings(v []string) string {
	if v == nil {
		return "[]"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func marshalSimilar(v []CeibaSimilarRef) string {
	if v == nil {
		return "[]"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func unmarshalStrings(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	if json.Unmarshal([]byte(s), &out) != nil {
		return nil
	}
	return out
}

func unmarshalSimilar(s string) []CeibaSimilarRef {
	if s == "" {
		return nil
	}
	var out []CeibaSimilarRef
	if json.Unmarshal([]byte(s), &out) != nil {
		return nil
	}
	return out
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// UpsertCeibaFact writes a card-sourced fact row. Card columns overwrite
// (the card stream is the authoritative price/status surface), while
// property_type and waterfront are preserved when the incoming value is
// empty because only detail-page reads can supply them.
func (s *Store) UpsertCeibaFact(ctx context.Context, f CeibaFact) error {
	if s == nil {
		return errors.New("ceiba: nil store")
	}
	_, err := s.DB().ExecContext(ctx, `
		INSERT INTO listing_facts
			(wp_id, slug, price_usd, acres, bullets, spotlight, under_contract, sold, district_names, town_name, property_type, waterfront, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(wp_id) DO UPDATE SET
			slug = CASE WHEN excluded.slug <> '' THEN excluded.slug ELSE listing_facts.slug END,
			price_usd = excluded.price_usd,
			acres = excluded.acres,
			bullets = excluded.bullets,
			spotlight = excluded.spotlight,
			under_contract = excluded.under_contract,
			sold = excluded.sold,
			district_names = excluded.district_names,
			town_name = excluded.town_name,
			property_type = CASE WHEN excluded.property_type <> '' THEN excluded.property_type ELSE listing_facts.property_type END,
			waterfront = CASE WHEN excluded.waterfront <> '' THEN excluded.waterfront ELSE listing_facts.waterfront END,
			updated_at = excluded.updated_at`,
		f.WPID, f.Slug, f.PriceUSD, f.Acres, marshalStrings(f.Bullets),
		boolInt(f.Spotlight), boolInt(f.UnderContract), boolInt(f.Sold),
		marshalStrings(f.DistrictNames), f.TownName, f.PropertyType, f.Waterfront, f.UpdatedAt,
	)
	return err
}

// UpsertCeibaFactObservation refreshes only the columns a live detail-page
// read can observe (price, acres, type, waterfront, district/town). The
// card-sourced columns (bullets, status flags) are preserved on update, and
// a missing row is inserted with them empty.
func (s *Store) UpsertCeibaFactObservation(ctx context.Context, wpID int64, slug string, priceUSD, acres *float64, propertyType, waterfront string, districtNames []string, townName string, at time.Time) error {
	if s == nil {
		return errors.New("ceiba: nil store")
	}
	_, err := s.DB().ExecContext(ctx, `
		INSERT INTO listing_facts
			(wp_id, slug, price_usd, acres, bullets, spotlight, under_contract, sold, district_names, town_name, property_type, waterfront, updated_at)
		VALUES (?, ?, ?, ?, '[]', 0, 0, 0, ?, ?, ?, ?, ?)
		ON CONFLICT(wp_id) DO UPDATE SET
			slug = CASE WHEN excluded.slug <> '' THEN excluded.slug ELSE listing_facts.slug END,
			price_usd = COALESCE(excluded.price_usd, listing_facts.price_usd),
			acres = COALESCE(excluded.acres, listing_facts.acres),
			property_type = CASE WHEN excluded.property_type <> '' THEN excluded.property_type ELSE listing_facts.property_type END,
			waterfront = CASE WHEN excluded.waterfront <> '' THEN excluded.waterfront ELSE listing_facts.waterfront END,
			district_names = CASE WHEN excluded.district_names NOT IN ('', '[]') THEN excluded.district_names ELSE listing_facts.district_names END,
			town_name = CASE WHEN excluded.town_name <> '' THEN excluded.town_name ELSE listing_facts.town_name END,
			updated_at = excluded.updated_at`,
		wpID, slug, priceUSD, acres,
		marshalStrings(districtNames), townName, propertyType, waterfront, at.UTC().Format(time.RFC3339Nano),
	)
	return err
}

// UpsertCeibaDetail writes a detail row, replacing all scraped columns.
func (s *Store) UpsertCeibaDetail(ctx context.Context, d CeibaDetail) error {
	if s == nil {
		return errors.New("ceiba: nil store")
	}
	_, err := s.DB().ExecContext(ctx, `
		INSERT INTO listing_details
			(wp_id, listing_id, beds, baths, living_space, gallery_json, tour_url, map_lat, map_lng, brochure_json, similar_json, scraped_at, source_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(wp_id) DO UPDATE SET
			listing_id = excluded.listing_id,
			beds = excluded.beds,
			baths = excluded.baths,
			living_space = excluded.living_space,
			gallery_json = excluded.gallery_json,
			tour_url = excluded.tour_url,
			map_lat = excluded.map_lat,
			map_lng = excluded.map_lng,
			brochure_json = excluded.brochure_json,
			similar_json = excluded.similar_json,
			scraped_at = excluded.scraped_at,
			source_url = excluded.source_url`,
		d.WPID, d.ListingID, nullableInt(d.Beds), nullableInt(d.Baths), d.LivingSpace,
		marshalStrings(d.GalleryURLs), d.TourURL, d.MapLat, d.MapLng,
		marshalStrings(d.BrochureURLs), marshalSimilar(d.Similar), d.ScrapedAt, d.SourceURL,
	)
	return err
}

func nullableInt(v *float64) any {
	if v == nil {
		return nil
	}
	return int64(*v + 0.5)
}

// AppendCeibaHistoryIfChanged appends one history snapshot when the fact's
// tracked values differ from the latest stored snapshot for that listing
// (or no snapshot exists yet). Returns whether a row was written. The read
// cursor is fully drained before the insert.
func (s *Store) AppendCeibaHistoryIfChanged(ctx context.Context, f CeibaFact, observedAt time.Time) (bool, error) {
	if s == nil {
		return false, errors.New("ceiba: nil store")
	}
	var (
		price sql.NullFloat64
		acres sql.NullFloat64
		spot  sql.NullInt64
		under sql.NullInt64
		sold  sql.NullInt64
	)
	err := s.DB().QueryRowContext(ctx, `
		SELECT price_usd, acres, spotlight, under_contract, sold
		FROM listing_history WHERE wp_id = ? ORDER BY observed_at DESC LIMIT 1`,
		f.WPID,
	).Scan(&price, &acres, &spot, &under, &sold)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// No snapshot yet: fall through to the insert (baseline).
	case err != nil:
		return false, err
	default:
		if sameHistoryValues(price, acres, spot, under, sold, f) {
			return false, nil
		}
	}
	_, err = s.DB().ExecContext(ctx, `
		INSERT OR REPLACE INTO listing_history
			(wp_id, observed_at, price_usd, acres, spotlight, under_contract, sold)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		f.WPID, observedAt.UTC().Format(time.RFC3339Nano), f.PriceUSD, f.Acres,
		boolInt(f.Spotlight), boolInt(f.UnderContract), boolInt(f.Sold),
	)
	if err != nil {
		return false, err
	}
	return true, nil
}

func sameHistoryValues(price sql.NullFloat64, acres sql.NullFloat64, spot, under, sold sql.NullInt64, f CeibaFact) bool {
	if !sameNullFloat(price, f.PriceUSD) || !sameNullFloat(acres, f.Acres) {
		return false
	}
	if spot.Int64 != boolInt(f.Spotlight) || under.Int64 != boolInt(f.UnderContract) || sold.Int64 != boolInt(f.Sold) {
		return false
	}
	return true
}

func sameNullFloat(n sql.NullFloat64, v *float64) bool {
	if !n.Valid || v == nil {
		return n.Valid == (v != nil)
	}
	return n.Float64 == *v
}

// CeibaFacts returns every fact row ordered by wp_id.
func (s *Store) CeibaFacts(ctx context.Context) ([]CeibaFact, error) {
	if s == nil {
		return nil, nil
	}
	rows, err := s.DB().QueryContext(ctx, `
		SELECT wp_id, slug, price_usd, acres, bullets, spotlight, under_contract, sold,
		       district_names, town_name, property_type, waterfront, updated_at
		FROM listing_facts ORDER BY wp_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CeibaFact
	for rows.Next() {
		var (
			f         CeibaFact
			slug      sql.NullString
			bullets   sql.NullString
			districts sql.NullString
			town      sql.NullString
			ptype     sql.NullString
			water     sql.NullString
			updated   sql.NullString
			price     sql.NullFloat64
			acres     sql.NullFloat64
			spot      sql.NullInt64
			under     sql.NullInt64
			sold      sql.NullInt64
		)
		if err := rows.Scan(&f.WPID, &slug, &price, &acres, &bullets, &spot, &under, &sold,
			&districts, &town, &ptype, &water, &updated); err != nil {
			return nil, err
		}
		f.Slug = slug.String
		f.PriceUSD = nullFloatPtr(price)
		f.Acres = nullFloatPtr(acres)
		f.Bullets = unmarshalStrings(bullets.String)
		f.Spotlight = spot.Int64 != 0
		f.UnderContract = under.Int64 != 0
		f.Sold = sold.Int64 != 0
		f.DistrictNames = unmarshalStrings(districts.String)
		f.TownName = town.String
		f.PropertyType = ptype.String
		f.Waterfront = water.String
		f.UpdatedAt = updated.String
		out = append(out, f)
	}
	return out, rows.Err()
}

func nullFloatPtr(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}

// ceibaDetailSelectColumns is the shared projection for the detail lookups so
// a schema change updates one place instead of three.
const ceibaDetailSelectColumns = "wp_id, listing_id, beds, baths, living_space, gallery_json, tour_url, map_lat, map_lng, brochure_json, similar_json, scraped_at, source_url"

// CeibaDetails returns every detail row ordered by wp_id.
func (s *Store) CeibaDetails(ctx context.Context) ([]CeibaDetail, error) {
	if s == nil {
		return nil, nil
	}
	rows, err := s.DB().QueryContext(ctx, "SELECT "+ceibaDetailSelectColumns+" FROM listing_details ORDER BY wp_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CeibaDetail
	for rows.Next() {
		d, err := scanCeibaDetail(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CeibaDetailByID returns one detail row; ok=false when absent.
func (s *Store) CeibaDetailByID(ctx context.Context, wpID int64) (CeibaDetail, bool, error) {
	if s == nil {
		return CeibaDetail{}, false, nil
	}
	row := s.DB().QueryRowContext(ctx, "SELECT "+ceibaDetailSelectColumns+" FROM listing_details WHERE wp_id = ?", wpID)
	d, err := scanCeibaDetail(row)
	if errors.Is(err, sql.ErrNoRows) {
		return CeibaDetail{}, false, nil
	}
	if err != nil {
		return CeibaDetail{}, false, err
	}
	return d, true, nil
}

// CeibaDetailByListingID returns the detail row whose listing_id matches
// (case-insensitive); ok=false when absent.
func (s *Store) CeibaDetailByListingID(ctx context.Context, listingID string) (CeibaDetail, bool, error) {
	if s == nil || listingID == "" {
		return CeibaDetail{}, false, nil
	}
	row := s.DB().QueryRowContext(ctx, "SELECT "+ceibaDetailSelectColumns+" FROM listing_details WHERE listing_id = ? COLLATE NOCASE", listingID)
	d, err := scanCeibaDetail(row)
	if errors.Is(err, sql.ErrNoRows) {
		return CeibaDetail{}, false, nil
	}
	if err != nil {
		return CeibaDetail{}, false, err
	}
	return d, true, nil
}

type ceibaRowScanner interface {
	Scan(dest ...any) error
}

func scanCeibaDetail(sc ceibaRowScanner) (CeibaDetail, error) {
	var (
		d        CeibaDetail
		listing  sql.NullString
		gallery  sql.NullString
		tour     sql.NullString
		brochure sql.NullString
		similar  sql.NullString
		scraped  sql.NullString
		source   sql.NullString
		beds     sql.NullInt64
		baths    sql.NullInt64
		living   sql.NullFloat64
		lat      sql.NullFloat64
		lng      sql.NullFloat64
	)
	if err := sc.Scan(&d.WPID, &listing, &beds, &baths, &living, &gallery, &tour, &lat, &lng,
		&brochure, &similar, &scraped, &source); err != nil {
		return CeibaDetail{}, err
	}
	d.ListingID = listing.String
	if beds.Valid {
		v := float64(beds.Int64)
		d.Beds = &v
	}
	if baths.Valid {
		v := float64(baths.Int64)
		d.Baths = &v
	}
	d.LivingSpace = nullFloatPtr(living)
	d.GalleryURLs = unmarshalStrings(gallery.String)
	d.TourURL = tour.String
	d.MapLat = nullFloatPtr(lat)
	d.MapLng = nullFloatPtr(lng)
	d.BrochureURLs = unmarshalStrings(brochure.String)
	d.Similar = unmarshalSimilar(similar.String)
	d.ScrapedAt = scraped.String
	d.SourceURL = source.String
	return d, nil
}

// CeibaHistoryRows returns history rows, oldest first. wpID<=0 returns rows
// for every listing.
func (s *Store) CeibaHistoryRows(ctx context.Context, wpID int64) ([]CeibaHistoryRow, error) {
	if s == nil {
		return nil, nil
	}
	query := `SELECT wp_id, observed_at, price_usd, acres, spotlight, under_contract, sold FROM listing_history`
	args := []any{}
	if wpID > 0 {
		query += ` WHERE wp_id = ?`
		args = append(args, wpID)
	}
	query += ` ORDER BY wp_id, observed_at`
	rows, err := s.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CeibaHistoryRow
	for rows.Next() {
		var (
			h     CeibaHistoryRow
			price sql.NullFloat64
			acres sql.NullFloat64
			spot  sql.NullInt64
			under sql.NullInt64
			sold  sql.NullInt64
			obs   sql.NullString
		)
		if err := rows.Scan(&h.WPID, &obs, &price, &acres, &spot, &under, &sold); err != nil {
			return nil, err
		}
		h.ObservedAt = obs.String
		h.PriceUSD = nullFloatPtr(price)
		h.Acres = nullFloatPtr(acres)
		h.Spotlight = spot.Int64 != 0
		h.UnderContract = under.Int64 != 0
		h.Sold = sold.Int64 != 0
		out = append(out, h)
	}
	return out, rows.Err()
}

// CeibaEnrichSessionStart returns the session-start timestamp recorded by the
// most recent enrichment pass. The timestamp is stored in the sync_state
// cursor column at full precision (the DATETIME column is second-precision).
func (s *Store) CeibaEnrichSessionStart(ctx context.Context) (time.Time, bool, error) {
	if s == nil {
		return time.Time{}, false, nil
	}
	cursor, lastSynced, _, err := s.GetSyncState(CeibaEnrichMarkerResource)
	if err != nil {
		return time.Time{}, false, err
	}
	if trimmed := strings.TrimSpace(cursor); trimmed != "" {
		if t, perr := time.Parse(time.RFC3339Nano, trimmed); perr == nil {
			return t, true, nil
		}
	}
	if !lastSynced.IsZero() {
		return lastSynced, true, nil
	}
	return time.Time{}, false, nil
}

// SaveCeibaEnrichSessionStart records the session-start timestamp for the
// enrichment pass that just completed and the completion time as the
// freshness watermark.
func (s *Store) SaveCeibaEnrichSessionStart(ctx context.Context, sessionStart time.Time, count int, finishedAt time.Time) error {
	return s.SaveSyncStateAt(CeibaEnrichMarkerResource, sessionStart.UTC().Format(time.RFC3339Nano), count, finishedAt)
}
