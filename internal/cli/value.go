// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
//
// `value` — the price-per-acre leaderboard: every mirrored listing that has
// BOTH a price and an acreage, ranked by price_usd / acres ascending. The
// site shows the two figures in separate strips and never divides them.

package cli

import (
	"fmt"
	"strings"

	"ceiba-cli-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

func newNovelValueCmd(flags *rootFlags) *cobra.Command {
	var (
		flagDistrict    string
		flagMaxPrice    float64
		flagMinAcres    float64
		flagLimit       int
		flagIncludeSold bool
	)

	cmd := &cobra.Command{
		Use:   "value",
		Short: "Rank budget-filtered listings by price per acre, cheapest first.",
		Long: `Rank mirrored listings by price per acre (price_usd / acres), ascending.
Only listings with both a price and an acreage are ranked; sold listings are
hidden unless --include-sold is passed. Run 'ceiba-cli-pp-cli sync' first to
build the mirror.`,
		Example:     "  ceiba-cli-pp-cli value --district cayo --max-price 250000",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			if dryRunOK(flags) {
				return writeDryRun(w, flags, "value (local price-per-acre ranking)")
			}
			if len(args) > 0 {
				return usageErr(fmt.Errorf("value takes no positional arguments"))
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
			if len(snap.Facts) == 0 {
				return writeCeibaEmptyMirror(w, flags, "the local listings mirror")
			}

			views := ceibaValueRanking(snap, flagDistrict, flagMaxPrice, flagMinAcres, flagIncludeSold, flagLimit)
			if wantsHumanTable(w, flags) {
				return ceibaPrintListingsTable(w, views)
			}
			return printJSONFilteredKeep(w, views, flags, ceibaListingKeepKeys...)
		},
	}
	cmd.Flags().StringVar(&flagDistrict, "district", "", "District name or slug (includes its towns)")
	cmd.Flags().Float64Var(&flagMaxPrice, "max-price", 0, "Maximum price in USD")
	cmd.Flags().Float64Var(&flagMinAcres, "min-acres", 0, "Minimum acres")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum rows to return (0 = all)")
	cmd.Flags().BoolVar(&flagIncludeSold, "include-sold", false, "Include sold listings (hidden by default)")
	return cmd
}

// ceibaValueRanking filters to price+acres rows and sorts by price-per-acre
// ascending (nil price-per-acre cannot occur here). Sold listings are
// excluded unless includeSold is set, matching the CLI-wide default.
func ceibaValueRanking(snap *ceibaSnapshot, district string, maxPrice, minAcres float64, includeSold bool, limit int) []ceibaListingView {
	var districtMatch func(store.CeibaFact) bool
	if strings.TrimSpace(district) != "" {
		districtMatch = snap.localDistrictFilter(district)
	}
	views := make([]ceibaListingView, 0, len(snap.Facts))
	for _, f := range snap.Facts {
		if f.PriceUSD == nil || f.Acres == nil || *f.Acres <= 0 {
			continue
		}
		if f.Sold && !includeSold {
			continue
		}
		if maxPrice > 0 && *f.PriceUSD > maxPrice {
			continue
		}
		if minAcres > 0 && *f.Acres < minAcres {
			continue
		}
		if districtMatch != nil && !districtMatch(f) {
			continue
		}
		views = append(views, ceibaViewOf(snap, f))
	}
	sortCeibaViews(views, "price-per-acre")
	if limit > 0 && len(views) > limit {
		views = views[:limit]
	}
	return views
}
