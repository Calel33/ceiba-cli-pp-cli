// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Sync enrichment hook (hand-written; generator-owned sync.go/root.go are
// untouched). Wraps the generated `sync` RunE so that after a successful,
// non-dry-run sync the Ceiba mirror tables are refreshed from one
// /our-listing/ fetch: card facts are upserted, history snapshots are
// appended only when a tracked value changed, and the enrichment session
// marker advances for the `changes` digest. Enrichment is best-effort:
// failures warn on stderr and never fail the sync.

package cli

import (
	"fmt"
	"strings"
	"time"

	"ceiba-cli-pp-cli/internal/scrape"
	"ceiba-cli-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		syncCmd, _, err := root.Find([]string{"sync"})
		if err != nil || syncCmd == nil || syncCmd.RunE == nil {
			return
		}
		original := syncCmd.RunE
		syncCmd.RunE = func(cmd *cobra.Command, args []string) error {
			if err := original(cmd, args); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			if err := enrichCeibaListingFacts(cmd, flags); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: listing-fact enrichment failed: %v\n", err)
			}
			return nil
		}
	})
}

// ceibaLocationResolver maps a card's display names onto the synced
// district taxonomy: parent terms are districts, child terms are towns.
// Unresolvable names fall back into the district list only when no parent
// term resolved, so a mirror synced without the taxonomy still supports
// name-based district filtering.
type ceibaLocationResolver struct {
	byName map[string]ceibaTerm
}

func newCeibaLocationResolver(terms []ceibaTerm) *ceibaLocationResolver {
	r := &ceibaLocationResolver{byName: map[string]ceibaTerm{}}
	for _, t := range terms {
		key := strings.ToLower(strings.TrimSpace(t.Name))
		if key == "" {
			continue
		}
		if _, exists := r.byName[key]; !exists {
			r.byName[key] = t
		}
	}
	return r
}

func (r *ceibaLocationResolver) resolve(names []string) (districts []string, town string) {
	var fallback []string
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		term, ok := r.byName[strings.ToLower(name)]
		if !ok {
			fallback = append(fallback, name)
			continue
		}
		if term.Parent == 0 {
			districts = append(districts, term.Name)
		} else if town == "" {
			town = term.Name
		}
	}
	if len(districts) == 0 && len(fallback) > 0 {
		districts = fallback
	}
	return districts, town
}

// ceibaFactFromCard projects one parsed card into a mirror fact row.
func ceibaFactFromCard(wpID int64, card scrape.CardFacts, resolver *ceibaLocationResolver, now time.Time) store.CeibaFact {
	districts, town := resolver.resolve(card.DistrictNames)
	return store.CeibaFact{
		WPID:          wpID,
		Slug:          card.Slug,
		PriceUSD:      card.PriceUSD,
		Acres:         card.Acres,
		Bullets:       card.Bullets,
		Spotlight:     card.Spotlight,
		UnderContract: card.UnderContract,
		Sold:          card.Sold,
		DistrictNames: districts,
		TownName:      town,
		UpdatedAt:     now.UTC().Format(time.RFC3339Nano),
	}
}

// enrichCeibaListingFacts performs the one-request /our-listing/ refresh.
func enrichCeibaListingFacts(cmd *cobra.Command, flags *rootFlags) error {
	sessionStart := time.Now().UTC()
	ctx, cancel := boundCtx(cmd.Context(), flags)
	defer cancel()

	c, err := flags.newClient()
	if err != nil {
		return err
	}
	c.NoCache = true

	html, err := c.GetWithHeaders(ctx, "/our-listing/", nil, map[string]string{"X-Printing-Press-HTML-Response": "true"})
	if err != nil {
		return fmt.Errorf("fetching /our-listing/: %w", err)
	}
	cards := scrape.ParseListingCards(html)
	if len(cards) == 0 {
		return fmt.Errorf("parsed 0 listing cards from /our-listing/")
	}

	db, err := store.OpenWithContext(ctx, defaultDBPath(ceibaStoreName))
	if err != nil {
		return fmt.Errorf("opening local database: %w", err)
	}
	defer db.Close()
	if err := store.EnsureCeibaMirrorTables(ctx, db); err != nil {
		return fmt.Errorf("ensuring mirror tables: %w", err)
	}

	listings, err := loadCeibaListings(ctx, db)
	if err != nil {
		return fmt.Errorf("reading synced listings: %w", err)
	}
	if len(listings) == 0 {
		// Nothing synced to enrich (e.g. sync ran with a resource subset).
		return nil
	}
	slugToID := make(map[string]int64, len(listings))
	for _, r := range listings {
		if r.Slug != "" {
			slugToID[strings.ToLower(r.Slug)] = r.ID
		}
	}
	terms, err := loadCeibaTaxonomy(ctx, db)
	if err != nil {
		return fmt.Errorf("reading district taxonomy: %w", err)
	}
	resolver := newCeibaLocationResolver(terms)

	now := time.Now().UTC()
	matched, historyRows := 0, 0
	for _, card := range cards {
		wpID, ok := slugToID[strings.ToLower(card.Slug)]
		if !ok {
			continue
		}
		matched++
		fact := ceibaFactFromCard(wpID, card, resolver, now)
		if err := db.UpsertCeibaFact(ctx, fact); err != nil {
			return fmt.Errorf("upserting fact for %s: %w", card.Slug, err)
		}
		changed, err := db.AppendCeibaHistoryIfChanged(ctx, fact, now)
		if err != nil {
			return fmt.Errorf("appending history for %s: %w", card.Slug, err)
		}
		if changed {
			historyRows++
		}
	}
	if matched == 0 {
		return fmt.Errorf("no synced featured-listing rows matched the %d parsed cards", len(cards))
	}
	if err := db.SaveCeibaEnrichSessionStart(ctx, sessionStart, matched, now); err != nil {
		return fmt.Errorf("saving enrichment marker: %w", err)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "ceiba mirror: %d listing facts refreshed, %d history rows appended\n", matched, historyRows)
	return nil
}
