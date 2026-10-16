// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
//
// `market` — district-level aggregate report over the local mirror: counts,
// price min/median/max, median price per acre, median acres, an exclusive
// status mix (sold > under_contract > spotlight > active, summing to the
// count), and acre-band distribution. A parent district includes its towns
// via the synced taxonomy. An unknown district falls back to a plain name
// match; when nothing matches, the report is honestly zeroed.

package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"ceiba-cli-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

func newNovelMarketCmd(flags *rootFlags) *cobra.Command {
	var flagDistrict string

	cmd := &cobra.Command{
		Use:   "market",
		Short: "Counts, median price, median price per acre, and status mix for a district and its towns.",
		Long: `Aggregate statistics for a district over the mirrored catalog: count,
min/median/max price, median price per acre, median acres, the status mix
(each listing counted once: sold > under-contract > spotlight > active), and
acre bands. Parent districts include their towns (synced taxonomy). A
district name that is not in the taxonomy is matched against listing location
names instead; a zero result is reported as zero, not an error.`,
		Example:     "  ceiba-cli-pp-cli market --district cayo",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			if dryRunOK(flags) {
				return writeDryRun(w, flags, fmt.Sprintf("market --district %s (local mirror statistics)", flagDistrict))
			}
			if len(args) > 0 {
				return usageErr(fmt.Errorf("market takes no positional arguments"))
			}
			if strings.TrimSpace(flagDistrict) == "" {
				return usageErr(fmt.Errorf("market requires --district <slug|name>"))
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
			report := ceibaMarketReport(snap, flagDistrict)
			if report.Count == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: no listings matched district %q (matched by %s); check the spelling or run '%s' to refresh the taxonomy\n", flagDistrict, report.MatchedBy, ceibaDataCommand)
			}
			if wantsHumanTable(w, flags) {
				return ceibaPrintMarketReport(w, report)
			}
			return printJSONFiltered(w, report, flags)
		},
	}
	cmd.Flags().StringVar(&flagDistrict, "district", "", "District name or slug (parent districts include their towns)")
	return cmd
}

// ceibaAcreBand is one acre-band count.
type ceibaAcreBand struct {
	Band  string `json:"band"`
	Count int    `json:"count"`
}

// ceibaMarketStats is the machine shape of the market report.
type ceibaMarketStats struct {
	District           string          `json:"district"`
	MatchedBy          string          `json:"matched_by"` // taxonomy | name
	Count              int             `json:"count"`
	MinPrice           *float64        `json:"min_price,omitempty"`
	MedianPrice        *float64        `json:"median_price,omitempty"`
	MaxPrice           *float64        `json:"max_price,omitempty"`
	MedianPricePerAcre *float64        `json:"median_price_per_acre,omitempty"`
	MedianAcres        *float64        `json:"median_acres,omitempty"`
	StatusMix          map[string]int  `json:"status_mix"`
	AcreBands          []ceibaAcreBand `json:"acre_bands"`
}

// ceibaMarketReport computes the aggregate for a district query.
func ceibaMarketReport(snap *ceibaSnapshot, query string) ceibaMarketStats {
	report := ceibaMarketStats{
		District:  strings.TrimSpace(query),
		MatchedBy: "name",
		StatusMix: map[string]int{"active": 0, "spotlight": 0, "under_contract": 0, "sold": 0},
	}
	var match func(store.CeibaFact) bool
	if term, ok := snap.findTerm(query); ok {
		report.MatchedBy = "taxonomy"
		terms := snap.radixTerms(term)
		match = func(f store.CeibaFact) bool { return factMatchesAnyTerm(f, terms) }
	} else {
		q := strings.ToLower(strings.TrimSpace(query))
		match = func(f store.CeibaFact) bool {
			return containsFold(q, f.TownName) || containsFoldN(q, f.DistrictNames)
		}
	}

	var prices, ppas, acres []float64
	bands := []ceibaAcreBand{
		{Band: "0-1"}, {Band: "1-5"}, {Band: "5-10"}, {Band: "10-25"},
		{Band: "25-50"}, {Band: "50-100"}, {Band: "100+"},
	}
	bandIndex := func(a float64) int {
		switch {
		case a < 1:
			return 0
		case a < 5:
			return 1
		case a < 10:
			return 2
		case a < 25:
			return 3
		case a < 50:
			return 4
		case a < 100:
			return 5
		default:
			return 6
		}
	}
	for _, f := range snap.Facts {
		if !match(f) {
			continue
		}
		report.Count++
		statusKey := ceibaStatusFromFlags(f.Sold, f.UnderContract, f.Spotlight)
		if statusKey == "under-contract" {
			statusKey = "under_contract"
		}
		report.StatusMix[statusKey]++
		if f.PriceUSD != nil {
			prices = append(prices, *f.PriceUSD)
		}
		if f.Acres != nil {
			acres = append(acres, *f.Acres)
			bands[bandIndex(*f.Acres)].Count++
		}
		if ppa := ceibaPricePerAcre(f.PriceUSD, f.Acres); ppa != nil {
			ppas = append(ppas, *ppa)
		}
	}
	report.AcreBands = bands
	if len(prices) > 0 {
		report.MinPrice = floatPtr(minOf(prices))
		report.MedianPrice = ceibaMedian(prices)
		report.MaxPrice = floatPtr(maxOf(prices))
	}
	report.MedianPricePerAcre = ceibaMedian(ppas)
	report.MedianAcres = ceibaMedian(acres)
	return report
}

func floatPtr(v float64) *float64 { return &v }

func minOf(values []float64) float64 {
	m := values[0]
	for _, v := range values[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func maxOf(values []float64) float64 {
	m := values[0]
	for _, v := range values[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

// ceibaMedian returns the median of values (nil for empty input).
func ceibaMedian(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		v := sorted[n/2]
		return &v
	}
	v := (sorted[n/2-1] + sorted[n/2]) / 2
	return &v
}

func ceibaPrintMarketReport(w io.Writer, report ceibaMarketStats) error {
	fmt.Fprintf(w, "district: %s (matched by %s)\n", report.District, report.MatchedBy)
	fmt.Fprintf(w, "listings: %d\n", report.Count)
	fmt.Fprintf(w, "price (USD): min %s / median %s / max %s\n",
		ceibaFormatPrice(report.MinPrice), ceibaFormatPrice(report.MedianPrice), ceibaFormatPrice(report.MaxPrice))
	fmt.Fprintf(w, "median $/acre: %s\n", ceibaFormatPrice(report.MedianPricePerAcre))
	fmt.Fprintf(w, "median acres: %s\n", ceibaFormatFloat(report.MedianAcres, "%.2f"))
	fmt.Fprintf(w, "status mix: active %d / spotlight %d / under-contract %d / sold %d\n",
		report.StatusMix["active"], report.StatusMix["spotlight"], report.StatusMix["under_contract"], report.StatusMix["sold"])
	tw := newTabWriter(w)
	fmt.Fprintln(tw, strings.Join([]string{"ACRE BAND", "COUNT"}, "\t"))
	for _, b := range report.AcreBands {
		fmt.Fprintln(tw, strings.Join([]string{b.Band, fmt.Sprintf("%d", b.Count)}, "\t"))
	}
	return tw.Flush()
}
