// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto
//
// `show` — the complete listing payload. Resolution accepts a WordPress id,
// a slug, or a Ceiba listing ID (4HM1091-style), using the local mirror when
// possible. --data-source controls behavior:
//
//	auto  (default): serve from a cached detail scraped < 24h ago; otherwise
//	                  fetch live (REST + detail page) and fall back to the
//	                  cache on network failure
//	live:             always fetch live
//	local:            cached data only
//
// A live read upserts facts+details into the local mirror (best-effort).
// The output envelope carries meta.source = live|local.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ceiba-cli-pp-cli/internal/cliutil"
	"ceiba-cli-pp-cli/internal/scrape"
	"ceiba-cli-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		root.AddCommand(newCeibaShowCmd(flags))
	})
}

// ceibaDetailFreshWindow is the auto-mode cache window for detail scrapes.
const ceibaDetailFreshWindow = 24 * time.Hour

var ceibaListingIDRE = regexp.MustCompile(`^[0-9A-Za-z]{2,4}[0-9]{3,5}$`)

// ceibaSimilarView is one similar-listing entry in the show payload.
type ceibaSimilarView struct {
	WPID     int64    `json:"wp_id,omitempty"`
	Slug     string   `json:"slug,omitempty"`
	URL      string   `json:"url,omitempty"`
	Title    string   `json:"title,omitempty"`
	PriceUSD *float64 `json:"price_usd,omitempty"`
	ImageURL string   `json:"image_url,omitempty"`
}

// ceibaShowResult is the complete listing payload.
type ceibaShowResult struct {
	ID            int64              `json:"id"`
	Slug          string             `json:"slug,omitempty"`
	Title         string             `json:"title,omitempty"`
	URL           string             `json:"url,omitempty"`
	PriceUSD      *float64           `json:"price_usd,omitempty"`
	PriceBZD      *float64           `json:"price_bzd,omitempty"`
	ListingID     string             `json:"listing_id,omitempty"`
	Acres         *float64           `json:"acres,omitempty"`
	Beds          *float64           `json:"beds,omitempty"`
	Baths         *float64           `json:"baths,omitempty"`
	LivingSpace   *float64           `json:"living_space,omitempty"`
	PropertyType  string             `json:"property_type,omitempty"`
	Waterfront    string             `json:"waterfront,omitempty"`
	District      string             `json:"district,omitempty"`
	Town          string             `json:"town,omitempty"`
	Spotlight     bool               `json:"spotlight"`
	UnderContract bool               `json:"under_contract"`
	Sold          bool               `json:"sold"`
	Status        string             `json:"status,omitempty"`
	Bullets       []string           `json:"bullets,omitempty"`
	Description   string             `json:"description,omitempty"`
	GalleryURLs   []string           `json:"gallery_urls,omitempty"`
	TourURL       string             `json:"tour_url,omitempty"`
	MapLat        *float64           `json:"map_lat,omitempty"`
	MapLng        *float64           `json:"map_lng,omitempty"`
	BrochureURLs  []string           `json:"brochure_urls,omitempty"`
	Similar       []ceibaSimilarView `json:"similar,omitempty"`
	Modified      string             `json:"modified,omitempty"`
	SourceURL     string             `json:"source_url,omitempty"`
	SyncedAt      string             `json:"synced_at,omitempty"`
	// DetailScraped marks that this read fetched AND parsed the detail page.
	// REST-only fallbacks leave it false so writeback preserves any previously
	// cached detail row instead of blanking it.
	DetailScraped bool `json:"-"`
}

func (r *ceibaShowResult) applyStatusFields() {
	if r.PriceUSD != nil {
		bzd := *r.PriceUSD * 2
		r.PriceBZD = &bzd
	}
	r.Status = ceibaStatusFromFlags(r.Sold, r.UnderContract, r.Spotlight)
}

// ceibaRESTListing is the subset of a WP REST featured_listing item used here.
type ceibaRESTListing struct {
	ID           int64   `json:"id"`
	Slug         string  `json:"slug"`
	Modified     string  `json:"modified"`
	Link         string  `json:"link"`
	FeaturedTags []int64 `json:"featured_tags"`
	Title        struct {
		Rendered string `json:"rendered"`
	} `json:"title"`
	Content struct {
		Rendered string `json:"rendered"`
	} `json:"content"`
}

func newCeibaShowCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id|slug|listing-id>",
		Short: "Complete listing payload: price, specs, map, gallery, description, similar listings",
		Long: `Fetch everything known about one listing in a single call: USD/BZD price,
listing ID, acres, beds/baths/living space, property type, waterfront,
district/town, status flags, bullets, description, gallery/brochure URLs,
tour URL, map coordinates, and similar listings.

Accepts a WordPress post id, a slug, or a Ceiba listing ID (4HM1091-style
when a detail was scraped before). --data-source auto serves a detail
scraped within the last 24h and refreshes otherwise.`,
		Example:     "  ceiba-cli-pp-cli show 100-acre-investment-property-on-the-belize-river-in-the-cayo-district",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:happy-args": "id=100-acre-investment-property-on-the-belize-river-in-the-cayo-district;--data-source=live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				action := "show"
				if len(args) > 0 {
					action += " " + strings.Join(args, " ")
				}
				return writeDryRun(w, flags, action)
			}
			if len(args) < 1 {
				return usageErr(fmt.Errorf("show requires a listing id, slug, or listing ID (e.g. 4HM1091)"))
			}
			if len(args) > 1 {
				return usageErr(fmt.Errorf("show takes exactly one listing reference, got %d", len(args)))
			}
			key := strings.TrimSpace(args[0])
			ctx := cmd.Context()

			snap := ceibaEmptySnapshot()
			db, mirrorOK, err := openCeibaStore(ctx)
			if err != nil {
				return classifyAPIError(w, err, flags)
			}
			if mirrorOK {
				defer db.Close()
				snap, err = loadCeibaSnapshot(ctx, db)
				if err != nil {
					return classifyAPIError(w, err, flags)
				}
			}

			if flags.dataSource == "local" {
				res, ok, _ := ceibaShowLocal(ctx, db, mirrorOK, snap, key)
				if !ok {
					nf := notFoundErr(fmt.Errorf("listing %q not found in the local mirror. Run '%s' first, or use --data-source live", key, ceibaDataCommand))
					writeAPIErrorEnvelope(w, flags, nf, ExitCode(nf))
					return nf
				}
				return emitCeibaShow(w, flags, res, "local", res.SyncedAt)
			}

			if flags.dataSource == "auto" {
				if res, ok, hasDetail := ceibaShowLocal(ctx, db, mirrorOK, snap, key); ok && hasDetail && ceibaDetailIsFresh(res.SyncedAt) {
					return emitCeibaShow(w, flags, res, "local", res.SyncedAt)
				}
			}

			res, liveErr := ceibaShowLive(ctx, flags, snap, key)
			if liveErr != nil {
				var typed *cliError
				if As(liveErr, &typed) {
					writeAPIErrorEnvelope(w, flags, liveErr, ExitCode(liveErr))
					return liveErr // not-found and other typed failures pass through
				}
				if flags.dataSource == "auto" && isNetworkError(liveErr) {
					if res, ok, _ := ceibaShowLocal(ctx, db, mirrorOK, snap, key); ok {
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: live fetch failed (%v); serving cached detail\n", liveErr)
						return emitCeibaShow(w, flags, res, "local", res.SyncedAt)
					}
				}
				return classifyAPIError(w, liveErr, flags)
			}
			now := time.Now().UTC().Format(time.RFC3339)
			if err := ceibaShowWriteBack(ctx, res); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not cache detail locally: %v\n", err)
			}
			return emitCeibaShow(w, flags, res, "live", now)
		},
	}
	return cmd
}

func ceibaDetailIsFresh(scrapedAt string) bool {
	t := cliutil.ParseStoredTime(scrapedAt)
	if t.IsZero() {
		return false
	}
	return time.Since(t) < ceibaDetailFreshWindow
}

// ceibaShowLocal assembles the payload from cached facts/details. ok=false
// means the mirror has no row for the key; hasDetail reports whether the
// assembled payload used a cached detail scrape (the only freshness signal
// for --data-source auto).
func ceibaShowLocal(ctx context.Context, db *store.Store, mirrorOK bool, snap *ceibaSnapshot, key string) (res ceibaShowResult, ok bool, hasDetail bool) {
	if !mirrorOK || db == nil {
		return ceibaShowResult{}, false, false
	}
	wpID, found, err := resolveCeibaWPID(ctx, db, key)
	if err != nil || !found {
		return ceibaShowResult{}, false, false
	}
	f, hasFact := snap.FactsByID[wpID]
	_, hasDetail = snap.DetailsByID[wpID]
	_, hasResource := snap.ResourcesByID[wpID]
	if !hasFact && !hasDetail && !hasResource {
		return ceibaShowResult{}, false, false
	}
	if !hasFact {
		f = store.CeibaFact{WPID: wpID}
	}
	return ceibaViewToShow(snap, f), true, hasDetail
}

// ceibaViewToShow assembles the show payload from a fact row plus resources
// and details.
func ceibaViewToShow(snap *ceibaSnapshot, f store.CeibaFact) ceibaShowResult {
	res := ceibaShowResult{
		ID:            f.WPID,
		Slug:          f.Slug,
		PriceUSD:      f.PriceUSD,
		Acres:         f.Acres,
		Bullets:       f.Bullets,
		Spotlight:     f.Spotlight,
		UnderContract: f.UnderContract,
		Sold:          f.Sold,
		PropertyType:  f.PropertyType,
		Waterfront:    f.Waterfront,
		Town:          f.TownName,
	}
	if len(f.DistrictNames) > 0 {
		res.District = f.DistrictNames[0]
	}
	if r, ok := snap.ResourcesByID[f.WPID]; ok {
		if res.Slug == "" {
			res.Slug = r.Slug
		}
		res.Title = r.Title
		res.Modified = r.Modified
		res.URL = r.Link
		res.Description = ceibaPlainText(r.Content)
	}
	if res.URL == "" && res.Slug != "" {
		res.URL = "https://ceibabelize.com/featured_listing/" + res.Slug + "/"
	}
	if d, ok := snap.DetailsByID[f.WPID]; ok {
		res.ListingID = d.ListingID
		res.Beds = d.Beds
		res.Baths = d.Baths
		res.LivingSpace = d.LivingSpace
		res.GalleryURLs = d.GalleryURLs
		res.TourURL = d.TourURL
		res.MapLat = d.MapLat
		res.MapLng = d.MapLng
		res.BrochureURLs = d.BrochureURLs
		res.SyncedAt = d.ScrapedAt
		if res.SourceURL == "" {
			res.SourceURL = d.SourceURL
		}
		res.Similar = ceibaSimilarViews(snap, d.Similar)
	}
	if res.SyncedAt == "" {
		res.SyncedAt = f.UpdatedAt
	}
	if res.SourceURL == "" {
		res.SourceURL = res.URL
	}
	res.applyStatusFields()
	return res
}

func ceibaSimilarViews(snap *ceibaSnapshot, refs []store.CeibaSimilarRef) []ceibaSimilarView {
	if len(refs) == 0 {
		return nil
	}
	out := make([]ceibaSimilarView, 0, len(refs))
	for _, s := range refs {
		item := ceibaSimilarView{
			WPID:     s.WPID,
			Slug:     s.Slug,
			URL:      s.URL,
			Title:    s.Title,
			PriceUSD: s.PriceUSD,
			ImageURL: s.ImageURL,
		}
		if item.WPID == 0 && s.Slug != "" {
			if r, ok := snap.ResourcesBySlug[strings.ToLower(s.Slug)]; ok {
				item.WPID = r.ID
			}
		}
		if item.WPID > 0 {
			if r, ok := snap.ResourcesByID[item.WPID]; ok {
				if item.Title == "" {
					item.Title = r.Title
				}
				if item.URL == "" {
					item.URL = r.Link
				}
			}
			if f, ok := snap.FactsByID[item.WPID]; ok && item.PriceUSD == nil {
				item.PriceUSD = f.PriceUSD
			}
		}
		out = append(out, item)
	}
	return out
}

// ceibaShowLive fetches the REST item and the detail page, merging both.
func ceibaShowLive(ctx context.Context, flags *rootFlags, snap *ceibaSnapshot, key string) (ceibaShowResult, error) {
	c, err := flags.newClient()
	if err != nil {
		return ceibaShowResult{}, err
	}
	var (
		wpID    int64
		slug    string
		rest    *ceibaRESTListing
		numeric bool
	)
	if n, err := strconv.ParseInt(key, 10, 64); err == nil && n > 0 {
		numeric = true
		wpID = n
	}
	switch {
	case numeric:
		rest, err = ceibaRESTByID(ctx, c, wpID)
		if err != nil {
			return ceibaShowResult{}, err
		}
		if rest != nil {
			slug = rest.Slug
		}
	case ceibaListingIDRE.MatchString(key):
		for id, d := range snap.DetailsByID {
			if strings.EqualFold(strings.TrimSpace(d.ListingID), key) {
				wpID = id
				break
			}
		}
	default:
		slug = key
	}
	if slug == "" && wpID > 0 {
		if f, ok := snap.FactsByID[wpID]; ok && f.Slug != "" {
			slug = f.Slug
		} else if r, ok := snap.ResourcesByID[wpID]; ok {
			slug = r.Slug
		}
	}
	if slug == "" {
		return ceibaShowResult{}, notFoundErr(fmt.Errorf("listing ID %q is not in the local mirror (live lookups cannot resolve 4HM-style IDs). Run '%s' first", key, ceibaDataCommand))
	}

	if rest == nil && !numeric {
		rest, err = ceibaRESTBySlug(ctx, c, slug)
		if err != nil {
			return ceibaShowResult{}, err
		}
		if rest != nil && wpID == 0 {
			wpID = rest.ID
		}
	}

	html, err := c.GetWithHeaders(ctx, "/featured_listing/"+cliutil.EscapePathParam(slug)+"/", nil, map[string]string{"X-Printing-Press-HTML-Response": "true"})
	if err != nil {
		if rest != nil {
			// REST answered; the detail page is optional enrichment. Leave
			// DetailScraped false so a failed fetch cannot blank cached detail.
			return ceibaMergeLive(snap, wpID, slug, rest, scrape.DetailFacts{}), nil
		}
		if strings.Contains(err.Error(), "HTTP 404") {
			return ceibaShowResult{}, notFoundErr(fmt.Errorf("listing %q not found on ceibabelize.com (detail page returned 404). Find the right slug with 'ceiba-cli-pp-cli listings --query <words>' or 'ceiba-cli-pp-cli search <words>'", key))
		}
		return ceibaShowResult{}, fmt.Errorf("fetching detail page for %q: %w", slug, err)
	}
	facts, err := scrape.ParseListingDetail(html)
	if err != nil {
		return ceibaShowResult{}, fmt.Errorf("parsing detail page for %q: %w", slug, err)
	}
	if rest == nil && facts.Title == "" && facts.PriceUSD == nil {
		return ceibaShowResult{}, notFoundErr(fmt.Errorf("listing %q not found on ceibabelize.com (no REST match and no detail-page data). Run '%s' to browse the local mirror", key, "ceiba-cli-pp-cli listings"))
	}
	res := ceibaMergeLive(snap, wpID, slug, rest, facts)
	// Only a page that yielded recognized detail content counts as a scraped
	// detail read; a soft-200 page must not blank the cached detail row.
	res.DetailScraped = facts.Title != "" || facts.PriceUSD != nil
	return res, nil
}

func ceibaRESTByID(ctx context.Context, c interface {
	GetWithHeaders(ctx context.Context, path string, params map[string]string, headers map[string]string) (json.RawMessage, error)
}, id int64) (*ceibaRESTListing, error) {
	raw, err := c.GetWithHeaders(ctx, fmt.Sprintf("/wp-json/wp/v2/featured_listing/%d", id), nil, nil)
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 404") {
			return nil, nil
		}
		return nil, err
	}
	var item ceibaRESTListing
	if json.Unmarshal(raw, &item) != nil || item.ID == 0 {
		return nil, nil
	}
	return &item, nil
}

func ceibaRESTBySlug(ctx context.Context, c interface {
	GetWithHeaders(ctx context.Context, path string, params map[string]string, headers map[string]string) (json.RawMessage, error)
}, slug string) (*ceibaRESTListing, error) {
	raw, err := c.GetWithHeaders(ctx, "/wp-json/wp/v2/featured_listing", map[string]string{"slug": slug}, nil)
	if err != nil {
		return nil, err
	}
	var items []ceibaRESTListing
	if json.Unmarshal(raw, &items) != nil || len(items) == 0 {
		return nil, nil
	}
	return &items[0], nil
}

// ceibaMergeLive merges REST + scrape + cached facts into the payload.
func ceibaMergeLive(snap *ceibaSnapshot, wpID int64, slug string, rest *ceibaRESTListing, facts scrape.DetailFacts) ceibaShowResult {
	res := ceibaShowResult{
		ID:           wpID,
		Slug:         slug,
		URL:          "https://ceibabelize.com/featured_listing/" + slug + "/",
		Title:        cliutil.CleanText(facts.Title),
		PriceUSD:     facts.PriceUSD,
		ListingID:    facts.ListingID,
		Acres:        facts.Acres,
		Beds:         facts.Beds,
		Baths:        facts.Baths,
		LivingSpace:  facts.LivingSpace,
		PropertyType: cliutil.CleanText(facts.PropertyType),
		Waterfront:   cliutil.CleanText(facts.Waterfront),
		District:     cliutil.CleanText(facts.DistrictName),
		Town:         cliutil.CleanText(facts.TownName),
		GalleryURLs:  facts.GalleryURLs,
		TourURL:      facts.TourURL,
		MapLat:       facts.MapLat,
		MapLng:       facts.MapLng,
		BrochureURLs: facts.BrochureURLs,
		SourceURL:    "https://ceibabelize.com/featured_listing/" + slug + "/",
	}
	if rest != nil {
		if rest.Slug != "" {
			res.Slug = rest.Slug
			res.URL = rest.Link
			res.SourceURL = rest.Link
		}
		res.Title = cliutil.CleanText(rest.Title.Rendered)
		res.Description = ceibaPlainText(rest.Content.Rendered)
		res.Modified = rest.Modified
		// REST tags are the live status truth (30=sold, 31=spotlight, 32=under-contract).
		res.Spotlight, res.UnderContract, res.Sold = false, false, false
		for _, tag := range rest.FeaturedTags {
			switch tag {
			case 30:
				res.Sold = true
			case 31:
				res.Spotlight = true
			case 32:
				res.UnderContract = true
			}
		}
		if res.ID == 0 {
			res.ID = rest.ID
		}
	}
	// Cached facts fill gaps (bullets, status when REST missing, descriptions).
	if f, ok := snap.FactsByID[res.ID]; ok {
		if res.PriceUSD == nil {
			res.PriceUSD = f.PriceUSD
		}
		if res.Acres == nil {
			res.Acres = f.Acres
		}
		if res.Bullets == nil {
			res.Bullets = f.Bullets
		}
		if res.PropertyType == "" {
			res.PropertyType = f.PropertyType
		}
		if res.Waterfront == "" {
			res.Waterfront = f.Waterfront
		}
		if res.District == "" && len(f.DistrictNames) > 0 {
			res.District = f.DistrictNames[0]
		}
		if res.Town == "" {
			res.Town = f.TownName
		}
		if rest == nil {
			res.Spotlight = f.Spotlight
			res.UnderContract = f.UnderContract
			res.Sold = f.Sold
		}
	}
	if res.ID > 0 {
		if r, ok := snap.ResourcesByID[res.ID]; ok {
			if res.Title == "" {
				res.Title = r.Title
			}
			if res.Description == "" {
				res.Description = ceibaPlainText(r.Content)
			}
			if res.Modified == "" {
				res.Modified = r.Modified
			}
		}
	}
	res.Similar = ceibaSimilarViews(snap, ceibaSimilarRefs(facts.Similar))
	res.SyncedAt = time.Now().UTC().Format(time.RFC3339)
	res.applyStatusFields()
	return res
}

func ceibaSimilarRefs(in []scrape.SimilarListing) []store.CeibaSimilarRef {
	if len(in) == 0 {
		return nil
	}
	out := make([]store.CeibaSimilarRef, 0, len(in))
	for _, s := range in {
		out = append(out, store.CeibaSimilarRef{
			WPID:     int64(s.WPID),
			Slug:     s.Slug,
			URL:      s.URL,
			Title:    cliutil.CleanText(s.Title),
			ImageURL: s.ImageURL,
		})
	}
	return out
}

// ceibaShowWriteBack caches the live read into the mirror (best-effort).
func ceibaShowWriteBack(ctx context.Context, res ceibaShowResult) error {
	if res.ID <= 0 || res.Slug == "" {
		return nil
	}
	db, err := store.OpenWithContext(ctx, defaultDBPath(ceibaStoreName))
	if err != nil {
		return err
	}
	defer db.Close()
	if err := store.EnsureCeibaMirrorTables(ctx, db); err != nil {
		return err
	}
	now := time.Now().UTC()
	var districts []string
	if res.District != "" {
		districts = []string{res.District}
	}
	if err := db.UpsertCeibaFactObservation(ctx, res.ID, res.Slug, res.PriceUSD, res.Acres,
		res.PropertyType, res.Waterfront, districts, res.Town, now); err != nil {
		return err
	}
	if !res.DetailScraped {
		// REST-only read: keep any previously cached detail row (and its
		// scrape timestamp) instead of overwriting it with empty fields.
		return nil
	}
	detail := store.CeibaDetail{
		WPID:         res.ID,
		ListingID:    res.ListingID,
		Beds:         res.Beds,
		Baths:        res.Baths,
		LivingSpace:  res.LivingSpace,
		GalleryURLs:  res.GalleryURLs,
		TourURL:      res.TourURL,
		MapLat:       res.MapLat,
		MapLng:       res.MapLng,
		BrochureURLs: res.BrochureURLs,
		Similar:      ceibaSimilarRefsFromViews(res.Similar),
		ScrapedAt:    now.UTC().Format(time.RFC3339Nano),
		SourceURL:    res.SourceURL,
	}
	return db.UpsertCeibaDetail(ctx, detail)
}

func ceibaSimilarRefsFromViews(in []ceibaSimilarView) []store.CeibaSimilarRef {
	if len(in) == 0 {
		return nil
	}
	out := make([]store.CeibaSimilarRef, 0, len(in))
	for _, s := range in {
		out = append(out, store.CeibaSimilarRef{WPID: s.WPID, Slug: s.Slug, URL: s.URL, Title: s.Title, PriceUSD: s.PriceUSD, ImageURL: s.ImageURL})
	}
	return out
}

// emitCeibaShow renders the payload with its provenance envelope.
func emitCeibaShow(w io.Writer, flags *rootFlags, res ceibaShowResult, source, syncedAt string) error {
	payload := map[string]any{
		"results": res,
		"meta": map[string]any{
			"source":    source,
			"synced_at": syncedAt,
		},
	}
	if wantsHumanTable(w, flags) {
		raw, _ := json.Marshal(res)
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			return nil
		}
		return printAutoCards(w, []map[string]any{m})
	}
	return printJSONFiltered(w, payload, flags)
}
