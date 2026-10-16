// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Shared helpers for the hand-written Ceiba novel commands (listings, show,
// changes, value, compare, comps, market, coverage, developments,
// recent-sales). Local reads go through openCeibaStore and treat a missing
// mirror (no store file, or no Ceiba tables yet) as "run sync first" with an
// empty machine result, never an error.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ceiba-cli-pp-cli/internal/cliutil"
	"ceiba-cli-pp-cli/internal/store"
)

const ceibaStoreName = "ceiba-cli-pp-cli"

// ceibaDataCommand is the command users must run to build the local mirror.
const ceibaDataCommand = "ceiba-cli-pp-cli sync"

// openCeibaStore opens the read-only store and reports whether the Ceiba
// mirror tables are present. (nil, false, nil) means "mirror missing" — the
// caller must emit the missing-mirror contract, not an error.
func openCeibaStore(ctx context.Context) (*store.Store, bool, error) {
	db, err := openStoreForRead(ctx, ceibaStoreName)
	if err != nil {
		return nil, false, err
	}
	if db == nil {
		return nil, false, nil
	}
	present, err := store.CeibaMirrorPresent(ctx, db)
	if err != nil {
		_ = db.Close()
		return nil, false, err
	}
	if !present {
		_ = db.Close()
		return nil, false, nil
	}
	return db, true, nil
}

// writeCeibaEmptyMirror implements the missing-mirror contract: a stderr hint
// naming `sync`, machine output as an empty JSON array, exit 0.
func writeCeibaEmptyMirror(w io.Writer, flags *rootFlags, what string) error {
	fmt.Fprintf(os.Stderr, "hint: %s is missing or empty. Run '%s' to build the local mirror.\n", what, ceibaDataCommand)
	if wantsMachineOutput(flags) {
		return printJSONFiltered(w, []any{}, flags)
	}
	return nil
}

// writeCeibaEmptyMirrorRequested is writeCeibaEmptyMirror for commands that
// resolve named entities: the machine payload echoes the requested lookups so
// callers can see exactly which references could not be resolved before the
// mirror is built. Keep keys are pinned so --agent/--compact retain them.
func writeCeibaEmptyMirrorRequested(w io.Writer, flags *rootFlags, what string, requested []string) error {
	fmt.Fprintf(os.Stderr, "hint: %s is missing or empty. Run '%s' to build the local mirror.\n", what, ceibaDataCommand)
	if wantsMachineOutput(flags) {
		note := fmt.Sprintf("%s is missing or empty; run '%s' to build the local mirror", what, ceibaDataCommand)
		return printJSONFilteredKeep(w, struct {
			Requested []string `json:"requested,omitempty"`
			Note      string   `json:"note"`
		}{Requested: requested, Note: note}, flags, "requested", "note")
	}
	return nil
}

// ceibaResourceRow is the subset of a synced featured_listing row the novel
// commands need.
type ceibaResourceRow struct {
	ID       int64
	Slug     string
	Title    string
	Modified string
	Link     string
	Content  string
}

// loadCeibaListings reads the synced featured-listing rows (generic
// resources partition is authoritative; the typed table is a projection).
func loadCeibaListings(ctx context.Context, db *store.Store) ([]ceibaResourceRow, error) {
	var out []ceibaResourceRow
	err := db.ListScan("featured-listing", func(id string, data json.RawMessage) bool {
		var item struct {
			Modified string `json:"modified"`
			Slug     string `json:"slug"`
			Link     string `json:"link"`
			Title    struct {
				Rendered string `json:"rendered"`
			} `json:"title"`
			Content struct {
				Rendered string `json:"rendered"`
			} `json:"content"`
		}
		if json.Unmarshal(data, &item) != nil {
			return true
		}
		wpID, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
		if err != nil {
			return true
		}
		out = append(out, ceibaResourceRow{
			ID:       wpID,
			Slug:     item.Slug,
			Title:    cliutil.CleanText(item.Title.Rendered),
			Modified: item.Modified,
			Link:     item.Link,
			Content:  item.Content.Rendered,
		})
		return true
	})
	return out, err
}

// ceibaTerm is one synced district-taxonomy term.
type ceibaTerm struct {
	ID     int64
	Name   string
	Slug   string
	Parent int64
}

// loadCeibaTaxonomy reads the synced district term rows.
func loadCeibaTaxonomy(ctx context.Context, db *store.Store) ([]ceibaTerm, error) {
	var out []ceibaTerm
	err := db.ListScan("district", func(id string, data json.RawMessage) bool {
		var item struct {
			ID     int64  `json:"id"`
			Name   string `json:"name"`
			Slug   string `json:"slug"`
			Parent int64  `json:"parent"`
		}
		if json.Unmarshal(data, &item) != nil {
			return true
		}
		if item.ID == 0 {
			item.ID, _ = strconv.ParseInt(strings.TrimSpace(id), 10, 64)
		}
		out = append(out, ceibaTerm{ID: item.ID, Name: cliutil.CleanText(item.Name), Slug: item.Slug, Parent: item.Parent})
		return true
	})
	return out, err
}

// ceibaSnapshot is the loaded mirror state shared by the local commands.
type ceibaSnapshot struct {
	Facts           []store.CeibaFact
	FactsByID       map[int64]store.CeibaFact
	ResourcesByID   map[int64]ceibaResourceRow
	ResourcesBySlug map[string]ceibaResourceRow
	DetailsByID     map[int64]store.CeibaDetail
	Terms           []ceibaTerm
	termsBySlug     map[string]ceibaTerm
	termsByName     map[string]ceibaTerm
	childrenOf      map[int64][]ceibaTerm
}

// ceibaEmptySnapshot returns a snapshot with no loaded rows (used when the
// local mirror is absent but a live path still needs lookup maps).
func ceibaEmptySnapshot() *ceibaSnapshot {
	return &ceibaSnapshot{
		FactsByID:       map[int64]store.CeibaFact{},
		ResourcesByID:   map[int64]ceibaResourceRow{},
		ResourcesBySlug: map[string]ceibaResourceRow{},
		DetailsByID:     map[int64]store.CeibaDetail{},
		termsBySlug:     map[string]ceibaTerm{},
		termsByName:     map[string]ceibaTerm{},
		childrenOf:      map[int64][]ceibaTerm{},
	}
}

func loadCeibaSnapshot(ctx context.Context, db *store.Store) (*ceibaSnapshot, error) {
	snap := ceibaEmptySnapshot()
	facts, err := db.CeibaFacts(ctx)
	if err != nil {
		return nil, err
	}
	snap.Facts = facts
	for _, f := range facts {
		snap.FactsByID[f.WPID] = f
	}
	res, err := loadCeibaListings(ctx, db)
	if err != nil {
		return nil, err
	}
	for _, r := range res {
		snap.ResourcesByID[r.ID] = r
		if r.Slug != "" {
			snap.ResourcesBySlug[strings.ToLower(r.Slug)] = r
		}
	}
	details, err := db.CeibaDetails(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range details {
		snap.DetailsByID[d.WPID] = d
	}
	terms, err := loadCeibaTaxonomy(ctx, db)
	if err != nil {
		return nil, err
	}
	snap.Terms = terms
	for _, t := range terms {
		if t.Slug != "" {
			if _, exists := snap.termsBySlug[strings.ToLower(t.Slug)]; !exists {
				snap.termsBySlug[strings.ToLower(t.Slug)] = t
			}
		}
		if t.Name != "" {
			if _, exists := snap.termsByName[strings.ToLower(t.Name)]; !exists {
				snap.termsByName[strings.ToLower(t.Name)] = t
			}
		}
		if t.Parent != 0 {
			snap.childrenOf[t.Parent] = append(snap.childrenOf[t.Parent], t)
		}
	}
	return snap, nil
}

// findTerm resolves a --district/--neighbourhood argument to a taxonomy term
// by slug or name (case-insensitive).
func (snap *ceibaSnapshot) findTerm(query string) (ceibaTerm, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return ceibaTerm{}, false
	}
	if t, ok := snap.termsBySlug[q]; ok {
		return t, true
	}
	t, ok := snap.termsByName[q]
	return t, ok
}

// radixTerms returns the term itself plus its child terms (a parent district
// filter includes its towns).
func (snap *ceibaSnapshot) radixTerms(t ceibaTerm) []ceibaTerm {
	out := []ceibaTerm{t}
	out = append(out, snap.childrenOf[t.ID]...)
	return out
}

// factMatchesAnyTerm reports whether the fact's location names match any of
// the given terms (district name or town name).
func factMatchesAnyTerm(f store.CeibaFact, terms []ceibaTerm) bool {
	for _, t := range terms {
		if containsFold(t.Name, f.TownName) || containsFoldN(t.Name, f.DistrictNames) {
			return true
		}
	}
	return false
}

// localDistrictFilter compiles a --district argument into a predicate. An
// unknown name falls back to a plain name match against the fact's location
// fields so a synced-catalog with an empty taxonomy still answers.
func (snap *ceibaSnapshot) localDistrictFilter(query string) func(store.CeibaFact) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if term, ok := snap.findTerm(query); ok {
		terms := snap.radixTerms(term)
		return func(f store.CeibaFact) bool { return factMatchesAnyTerm(f, terms) }
	}
	return func(f store.CeibaFact) bool {
		return containsFold(q, f.TownName) || containsFoldN(q, f.DistrictNames)
	}
}

// localNeighbourhoodFilter compiles a --neighbourhood argument into a
// predicate (child-town match; falls back to name substring). A resolved
// PARENT district term matches nothing here — callers must route districts to
// --district; listings raises a usage error before reaching this predicate.
func (snap *ceibaSnapshot) localNeighbourhoodFilter(query string) func(store.CeibaFact) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if term, ok := snap.findTerm(query); ok {
		if term.Parent == 0 {
			return func(store.CeibaFact) bool { return false }
		}
		name := strings.ToLower(term.Name)
		return func(f store.CeibaFact) bool {
			return containsFold(name, f.TownName) || containsFoldN(name, f.DistrictNames)
		}
	}
	return func(f store.CeibaFact) bool {
		return containsFold(q, f.TownName) || containsFoldN(q, f.DistrictNames)
	}
}

func containsFold(needle, haystack string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return false
	}
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func containsFoldN(needle string, list []string) bool {
	for _, item := range list {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(needle)) {
			return true
		}
	}
	return false
}

// resolveCeibaWPID maps an id | slug | listing-id argument to a WordPress
// post id using the local mirror. ok=false means the mirror has no match.
func resolveCeibaWPID(ctx context.Context, db *store.Store, key string) (int64, bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return 0, false, nil
	}
	if n, err := strconv.ParseInt(key, 10, 64); err == nil && n > 0 {
		if d, ok, err := db.CeibaDetailByID(ctx, n); err != nil {
			return 0, false, err
		} else if ok {
			return d.WPID, true, nil
		}
		// numeric keys may also be a wp id known only to resources
		facts, err := db.CeibaFacts(ctx)
		if err != nil {
			return 0, false, err
		}
		for _, f := range facts {
			if f.WPID == n {
				return n, true, nil
			}
		}
		return 0, false, nil
	}
	// listing-id shape (e.g. 4HM1091): exact detail match.
	if d, ok, err := db.CeibaDetailByListingID(ctx, key); err != nil {
		return 0, false, err
	} else if ok {
		return d.WPID, true, nil
	}
	// slug match against facts, then against the resource rows.
	lower := strings.ToLower(key)
	facts, err := db.CeibaFacts(ctx)
	if err != nil {
		return 0, false, err
	}
	for _, f := range facts {
		if strings.ToLower(f.Slug) == lower {
			return f.WPID, true, nil
		}
	}
	res, err := loadCeibaListings(ctx, db)
	if err != nil {
		return 0, false, err
	}
	for _, r := range res {
		if strings.ToLower(r.Slug) == lower {
			return r.ID, true, nil
		}
	}
	return 0, false, nil
}

// ceibaListingView is the shared per-listing projection emitted by the local
// commands. All fields are nullable-preserving so absent data is never
// rendered as zero.
type ceibaListingView struct {
	ID            int64    `json:"id"`
	Slug          string   `json:"slug,omitempty"`
	Title         string   `json:"title,omitempty"`
	URL           string   `json:"url,omitempty"`
	PriceUSD      *float64 `json:"price_usd,omitempty"`
	PriceBZD      *float64 `json:"price_bzd,omitempty"`
	PricePerAcre  *float64 `json:"price_per_acre,omitempty"`
	Acres         *float64 `json:"acres,omitempty"`
	Beds          *float64 `json:"beds,omitempty"`
	Baths         *float64 `json:"baths,omitempty"`
	LivingSpace   *float64 `json:"living_space,omitempty"`
	ListingID     string   `json:"listing_id,omitempty"`
	Bullets       []string `json:"bullets,omitempty"`
	Spotlight     bool     `json:"spotlight"`
	UnderContract bool     `json:"under_contract"`
	Sold          bool     `json:"sold"`
	Status        string   `json:"status,omitempty"`
	District      string   `json:"district,omitempty"`
	Town          string   `json:"town,omitempty"`
	PropertyType  string   `json:"property_type,omitempty"`
	Waterfront    string   `json:"waterfront,omitempty"`
	Modified      string   `json:"modified,omitempty"`
	SyncedAt      string   `json:"synced_at,omitempty"`
}

// ceibaListingKeepKeys are passed to printJSONFilteredKeep so --agent/--compact
// retains the full projection instead of the generic gravity allow-list.
var ceibaListingKeepKeys = []string{
	"id", "slug", "title", "url", "price_usd", "price_bzd", "price_per_acre",
	"acres", "beds", "baths", "living_space", "listing_id", "bullets",
	"spotlight", "under_contract", "sold", "status", "district", "town",
	"property_type", "waterfront", "modified", "synced_at",
}

// ceibaStatusFromFlags is the single source of truth for the
// sold > under-contract > spotlight > active precedence shared by the search,
// show, and market projections.
func ceibaStatusFromFlags(sold, underContract, spotlight bool) string {
	switch {
	case sold:
		return "sold"
	case underContract:
		return "under-contract"
	case spotlight:
		return "spotlight"
	default:
		return "active"
	}
}

// ceibaStatus derives the single-word status from a fact row.
func ceibaStatus(f store.CeibaFact) string {
	return ceibaStatusFromFlags(f.Sold, f.UnderContract, f.Spotlight)
}

// ceibaHTMLTagRE strips markup when matching or echoing REST content.
var ceibaHTMLTagRE = regexp.MustCompile(`<[^>]*>`)

// ceibaPlainText renders REST content HTML as text for query matching and
// descriptions.
func ceibaPlainText(s string) string {
	if !strings.Contains(s, "<") {
		return cliutil.CleanText(s)
	}
	return cliutil.CleanText(ceibaHTMLTagRE.ReplaceAllString(s, " "))
}

// ceibaPricePerAcre computes price/acre when both values exist.
func ceibaPricePerAcre(price, acres *float64) *float64 {
	if price == nil || acres == nil || *acres <= 0 {
		return nil
	}
	v := *price / *acres
	return &v
}

// viewOf projects one fact into the shared listing view, joining resource and
// detail data when available.
func ceibaViewOf(snap *ceibaSnapshot, f store.CeibaFact) ceibaListingView {
	v := ceibaListingView{
		ID:            f.WPID,
		Slug:          f.Slug,
		PriceUSD:      f.PriceUSD,
		Acres:         f.Acres,
		Bullets:       f.Bullets,
		Spotlight:     f.Spotlight,
		UnderContract: f.UnderContract,
		Sold:          f.Sold,
		Status:        ceibaStatus(f),
		PropertyType:  f.PropertyType,
		Waterfront:    f.Waterfront,
	}
	if len(f.DistrictNames) > 0 {
		v.District = f.DistrictNames[0]
	}
	v.Town = f.TownName
	v.PricePerAcre = ceibaPricePerAcre(f.PriceUSD, f.Acres)
	if f.PriceUSD != nil {
		bzd := *f.PriceUSD * 2
		v.PriceBZD = &bzd
	}
	if r, ok := snap.ResourcesByID[f.WPID]; ok {
		if v.Slug == "" {
			v.Slug = r.Slug
		}
		v.Title = r.Title
		v.Modified = r.Modified
		v.URL = r.Link
	}
	if v.URL == "" && v.Slug != "" {
		v.URL = "https://ceibabelize.com/featured_listing/" + v.Slug + "/"
	}
	if d, ok := snap.DetailsByID[f.WPID]; ok {
		v.ListingID = d.ListingID
		v.Beds = d.Beds
		v.Baths = d.Baths
		v.LivingSpace = d.LivingSpace
		v.SyncedAt = d.ScrapedAt
	}
	if v.SyncedAt == "" {
		v.SyncedAt = f.UpdatedAt
	}
	return v
}

// ceibaViewsToMaps converts a typed slice into the map form printAutoTable
// expects for human output.
func ceibaViewsToMaps(v any) []map[string]any {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out []map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

// sortCeibaViews orders views by the requested sort key. Unknown keys fall
// back to modified-desc (the store's natural sync order).
func sortCeibaViews(views []ceibaListingView, key string) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "price":
		sort.SliceStable(views, func(i, j int) bool {
			return lessFloatPtr(views[i].PriceUSD, views[j].PriceUSD, true)
		})
	case "acres":
		sort.SliceStable(views, func(i, j int) bool {
			return lessFloatPtr(views[i].Acres, views[j].Acres, false)
		})
	case "title":
		sort.SliceStable(views, func(i, j int) bool {
			return strings.ToLower(views[i].Title) < strings.ToLower(views[j].Title)
		})
	case "price-per-acre":
		sort.SliceStable(views, func(i, j int) bool {
			return lessFloatPtr(views[i].PricePerAcre, views[j].PricePerAcre, true)
		})
	default: // modified desc
		sort.SliceStable(views, func(i, j int) bool {
			return views[i].Modified > views[j].Modified
		})
	}
}

// lessFloatPtr orders nullable floats with nil last. asc=true is ascending.
func lessFloatPtr(a, b *float64, asc bool) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	if asc {
		return *a < *b
	}
	return *a > *b
}

func ceibaFormatPrice(v *float64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("$%.0f", *v)
}

func ceibaFormatFloat(v *float64, format string) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf(format, *v)
}
