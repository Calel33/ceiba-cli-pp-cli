// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
//
// `coverage` — per-field fill rates across mirrored listings (facts +
// details) plus freshness watermarks, so operators can see which answers the
// mirror can currently support. An empty mirror follows the missing-mirror
// contract (stderr hint naming sync; empty JSON array; exit 0).

package cli

import (
	"context"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"ceiba-cli-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

func newNovelCoverageCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "Per-field fill rates across mirrored listings so operators know which answers the mirror can currently support.",
		Long: `Report how many mirrored listings carry each field (price, acres,
bullets, listing_id, beds, baths, living_space, gallery, tour, map, brochure,
similar) as counts and percentages, plus freshness watermarks. Run
'ceiba-cli-pp-cli sync' first.`,
		Example:     "  ceiba-cli-pp-cli coverage --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			if dryRunOK(flags) {
				return writeDryRun(w, flags, "coverage (local mirror fill rates)")
			}
			if len(args) > 0 {
				return usageErr(fmt.Errorf("coverage takes no positional arguments"))
			}
			ctx := cmd.Context()
			db, ok, err := openCeibaStore(ctx)
			if err != nil {
				return classifyAPIError(w, err, flags)
			}
			if !ok {
				return writeCeibaEmptyMirror(w, flags, "the local listings mirror")
			}
			defer db.Close()

			snap, err := loadCeibaSnapshot(ctx, db)
			if err != nil {
				return classifyAPIError(w, err, flags)
			}
			if len(snap.Facts) == 0 && len(snap.DetailsByID) == 0 {
				return writeCeibaEmptyMirror(w, flags, "the local listings mirror")
			}
			report, err := ceibaCoverageReport(ctx, db, snap)
			if err != nil {
				return classifyAPIError(w, err, flags)
			}
			if wantsHumanTable(w, flags) {
				return ceibaPrintCoverageTable(w, report)
			}
			return printJSONFiltered(w, report, flags)
		},
	}
	return cmd
}

// ceibaCoverageField is one field's fill rate.
type ceibaCoverageField struct {
	Field string  `json:"field"`
	Count int     `json:"count"`
	Total int     `json:"total"`
	Pct   float64 `json:"pct"`
}

// ceibaCoverage is the machine shape of the coverage report.
type ceibaCoverage struct {
	Listings  int                  `json:"listings"`
	Details   int                  `json:"details"`
	Fields    []ceibaCoverageField `json:"fields"`
	Freshness map[string]any       `json:"freshness"`
}

func ceibaCoverageReport(ctx context.Context, db *store.Store, snap *ceibaSnapshot) (ceibaCoverage, error) {
	total := len(snap.Facts)
	report := ceibaCoverage{
		Listings:  total,
		Details:   len(snap.DetailsByID),
		Freshness: map[string]any{},
	}

	counts := map[string]int{}
	fields := []string{"price", "acres", "bullets", "listing_id", "beds", "baths", "living_space", "gallery", "tour", "map", "brochure", "similar"}
	for _, name := range fields {
		counts[name] = 0
	}
	maxFactUpdated, maxDetailScraped := "", ""
	for _, f := range snap.Facts {
		if f.PriceUSD != nil {
			counts["price"]++
		}
		if f.Acres != nil {
			counts["acres"]++
		}
		if len(f.Bullets) > 0 {
			counts["bullets"]++
		}
		d, hasDetail := snap.DetailsByID[f.WPID]
		if hasDetail {
			if d.ListingID != "" {
				counts["listing_id"]++
			}
			if d.Beds != nil {
				counts["beds"]++
			}
			if d.Baths != nil {
				counts["baths"]++
			}
			if d.LivingSpace != nil {
				counts["living_space"]++
			}
			if len(d.GalleryURLs) > 0 {
				counts["gallery"]++
			}
			if d.TourURL != "" {
				counts["tour"]++
			}
			if d.MapLat != nil && d.MapLng != nil {
				counts["map"]++
			}
			if len(d.BrochureURLs) > 0 {
				counts["brochure"]++
			}
			if len(d.Similar) > 0 {
				counts["similar"]++
			}
			if d.ScrapedAt > maxDetailScraped {
				maxDetailScraped = d.ScrapedAt
			}
		}
		if f.UpdatedAt > maxFactUpdated {
			maxFactUpdated = f.UpdatedAt
		}
	}
	report.Fields = make([]ceibaCoverageField, 0, len(fields))
	for _, name := range fields {
		pct := 0.0
		if total > 0 {
			pct = math.Round(float64(counts[name])/float64(total)*1000) / 10
		}
		report.Fields = append(report.Fields, ceibaCoverageField{
			Field: name,
			Count: counts[name],
			Total: total,
			Pct:   pct,
		})
	}
	if maxFactUpdated != "" {
		report.Freshness["facts_updated_at"] = maxFactUpdated
	}
	if maxDetailScraped != "" {
		report.Freshness["details_scraped_at"] = maxDetailScraped
	}
	_, lastSynced, _, err := db.GetSyncState(store.CeibaEnrichMarkerResource)
	if err == nil && !lastSynced.IsZero() {
		report.Freshness["enriched_at"] = lastSynced.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	history, err := db.CeibaHistoryRows(ctx, 0)
	if err != nil {
		return report, err
	}
	report.Freshness["history_rows"] = len(history)
	sort.SliceStable(report.Fields, func(i, j int) bool { return report.Fields[i].Field < report.Fields[j].Field })
	return report, nil
}

func ceibaPrintCoverageTable(w io.Writer, report ceibaCoverage) error {
	fmt.Fprintf(w, "%d listings, %d detail rows\n", report.Listings, report.Details)
	tw := newTabWriter(w)
	fmt.Fprintln(tw, strings.Join([]string{"FIELD", "COUNT", "TOTAL", "PCT"}, "\t"))
	for _, f := range report.Fields {
		fmt.Fprintln(tw, strings.Join([]string{f.Field, fmt.Sprintf("%d", f.Count), fmt.Sprintf("%d", f.Total), fmt.Sprintf("%.1f", f.Pct)}, "\t"))
	}
	return tw.Flush()
}
