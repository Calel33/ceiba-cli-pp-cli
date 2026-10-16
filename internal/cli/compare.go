// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
//
// `compare` — an aligned feature table for 2-4 named listings from the local
// mirror. Accepts WordPress ids, slugs, and cached 4HM listing IDs.

package cli

import (
	"fmt"
	"io"
	"strings"

	"ceiba-cli-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

func newNovelCompareCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compare <id|slug> <id|slug> [<id|slug> [<id|slug>]]",
		Short: "Aligned feature table for 2-4 named listings in one call.",
		Long: `Compare 2-4 listings side by side from the local mirror: price (USD and
BZD), price per acre, acres, beds/baths/living space, district/town, status,
type, waterfront, and listing ID. Run 'ceiba-cli-pp-cli sync' first; listing
IDs (4HM...) resolve once a detail was fetched with 'show'.`,
		Example:     "  ceiba-cli-pp-cli compare 20-acre-estate-parcel-w-waterfront-near-hopkins a-unique-treehouse-perched-atop-the-macal-river",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "id1=20-acre-estate-parcel-w-waterfront-near-hopkins;id2=a-unique-treehouse-perched-atop-the-macal-river"},
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(w, flags, "compare "+strings.Join(args, " "))
			}
			if len(args) < 2 || len(args) > 4 {
				return usageErr(fmt.Errorf("compare requires 2 to 4 listings, got %d", len(args)))
			}
			ctx := cmd.Context()
			db, ok, err := openCeibaStore(ctx)
			if err != nil {
				return classifyAPIError(w, err, flags)
			}
			if !ok {
				return writeCeibaEmptyMirrorRequested(w, flags, "the local listings mirror", args)
			}
			defer db.Close()

			snap, err := loadCeibaSnapshot(ctx, db)
			if err != nil {
				return classifyAPIError(w, err, flags)
			}
			views := make([]ceibaListingView, 0, len(args))
			var missing []string
			for _, key := range args {
				id, found, err := resolveCeibaWPID(ctx, db, key)
				if err != nil {
					return classifyAPIError(w, err, flags)
				}
				if !found {
					missing = append(missing, key)
					continue
				}
				f, hasFact := snap.FactsByID[id]
				if !hasFact {
					f = store.CeibaFact{WPID: id}
				}
				views = append(views, ceibaViewOf(snap, f))
			}
			if len(missing) > 0 {
				nf := notFoundErr(fmt.Errorf("listing(s) %s not found in the local mirror. Run '%s' first (or cache a detail with 'show --data-source live')", strings.Join(missing, ", "), ceibaDataCommand))
				writeAPIErrorEnvelope(w, flags, nf, ExitCode(nf))
				return nf
			}
			if wantsHumanTable(w, flags) {
				return ceibaPrintCompareTable(w, views)
			}
			return printJSONFilteredKeep(w, views, flags, ceibaListingKeepKeys...)
		},
	}
	return cmd
}

// ceibaCompareField is one aligned row of the human comparison table.
type ceibaCompareField struct {
	Label  string
	Values func(ceibaListingView) string
}

var ceibaCompareFields = []ceibaCompareField{
	{"PRICE (USD)", func(v ceibaListingView) string { return ceibaFormatPrice(v.PriceUSD) }},
	{"PRICE (BZD)", func(v ceibaListingView) string { return ceibaFormatPrice(v.PriceBZD) }},
	{"$/ACRE", func(v ceibaListingView) string { return ceibaFormatPrice(v.PricePerAcre) }},
	{"ACRES", func(v ceibaListingView) string { return ceibaFormatFloat(v.Acres, "%.2f") }},
	{"BEDS", func(v ceibaListingView) string { return ceibaFormatFloat(v.Beds, "%.0f") }},
	{"BATHS", func(v ceibaListingView) string { return ceibaFormatFloat(v.Baths, "%.0f") }},
	{"LIVING SQFT", func(v ceibaListingView) string { return ceibaFormatFloat(v.LivingSpace, "%.0f") }},
	{"DISTRICT", func(v ceibaListingView) string { return v.District }},
	{"TOWN", func(v ceibaListingView) string { return v.Town }},
	{"STATUS", func(v ceibaListingView) string { return v.Status }},
	{"TYPE", func(v ceibaListingView) string { return v.PropertyType }},
	{"WATERFRONT", func(v ceibaListingView) string { return v.Waterfront }},
	{"LISTING ID", func(v ceibaListingView) string { return v.ListingID }},
	{"MODIFIED", func(v ceibaListingView) string { return v.Modified }},
}

func ceibaPrintCompareTable(w io.Writer, views []ceibaListingView) error {
	tw := newTabWriter(w)
	header := []string{"FIELD"}
	for i, v := range views {
		label := v.Title
		if label == "" {
			label = v.Slug
		}
		if label == "" {
			label = fmt.Sprintf("listing %d", v.ID)
		}
		header = append(header, fmt.Sprintf("[%d] %s", i+1, truncate(label, 32)))
	}
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, field := range ceibaCompareFields {
		row := []string{field.Label}
		for _, v := range views {
			row = append(row, field.Values(v))
		}
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	return tw.Flush()
}
