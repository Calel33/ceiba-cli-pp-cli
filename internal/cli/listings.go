// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
//
// `listings` — local-mirror browsing over the scraped card facts: compound
// filters (query, price/acres ranges, district/town, listing-id, type,
// waterfront, status, beds/baths), sorting, and JSON/table output. Sold
// listings are excluded by default; --include-sold makes the result a
// superset. An empty mirror prints a stderr hint naming `sync` and emits an
// empty JSON array for machine callers (exit 0).

package cli

import (
	"fmt"
	"io"
	"strings"

	"ceiba-cli-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		root.AddCommand(newCeibaListingsCmd(flags))
	})
}

type ceibaListingsOptions struct {
	query         string
	minPrice      float64
	maxPrice      float64
	minAcres      float64
	maxAcres      float64
	district      string
	neighbourhood string
	listingID     string
	propertyType  string
	waterfront    string
	status        string
	includeSold   bool
	beds          int
	baths         int
	sortKey       string
	limit         int
}

func newCeibaListingsCmd(flags *rootFlags) *cobra.Command {
	var opts ceibaListingsOptions
	cmd := &cobra.Command{
		Use:   "listings",
		Short: "Browse mirrored listings with compound filters (the local mirror of /our-listing/)",
		Long: `Browse the local listing mirror with compound filters.

Filters: --query (title/content/bullets), --min-price/--max-price (USD),
--min-acres/--max-acres, --district (parent district, town-inclusive),
--neighbourhood (town), --listing-id (4HM... when known), --type,
--waterfront (name or code: rf, of, bch, cf, lf), --status
(active|spotlight|under-contract), --beds/--baths.

Sold listings are excluded by default; pass --include-sold to include them
(the result is then a superset). Run 'ceiba-cli-pp-cli sync' first.`,
		Example:     "  ceiba-cli-pp-cli listings --district cayo --max-price 100000 --min-acres 1",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			if len(args) > 0 {
				return usageErr(fmt.Errorf("listings takes no positional arguments; see 'listings --help' for filters"))
			}
			if dryRunOK(flags) {
				return writeDryRun(w, flags, "listings (local mirror query)")
			}
			if err := validateCeibaListingsOptions(opts); err != nil {
				return usageErr(err)
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
			if strings.TrimSpace(opts.neighbourhood) != "" {
				if t, ok := snap.findTerm(opts.neighbourhood); ok && t.Parent == 0 {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--neighbourhood %q is a district; use --district for districts or a town/village name (e.g. 'San Ignacio Town')", opts.neighbourhood))
				}
			}
			views := ceibaFilterListings(snap, opts)
			if wantsHumanTable(w, flags) {
				return ceibaPrintListingsTable(w, views)
			}
			return printJSONFilteredKeep(w, views, flags, ceibaListingKeepKeys...)
		},
	}
	cmd.Flags().StringVar(&opts.query, "query", "", "Keyword search across title, description content, and bullets")
	cmd.Flags().Float64Var(&opts.minPrice, "min-price", 0, "Minimum price in USD")
	cmd.Flags().Float64Var(&opts.maxPrice, "max-price", 0, "Maximum price in USD")
	cmd.Flags().Float64Var(&opts.minAcres, "min-acres", 0, "Minimum acres")
	cmd.Flags().Float64Var(&opts.maxAcres, "max-acres", 0, "Maximum acres")
	cmd.Flags().StringVar(&opts.district, "district", "", "District name or slug (includes its towns)")
	cmd.Flags().StringVar(&opts.neighbourhood, "neighbourhood", "", "Town/village name or slug")
	cmd.Flags().StringVar(&opts.listingID, "listing-id", "", "Ceiba listing ID (e.g. 4HM1091; only present once a detail was shown)")
	cmd.Flags().StringVar(&opts.propertyType, "type", "", "Property type facet (e.g. Acreage; best-effort, filled by 'show')")
	cmd.Flags().StringVar(&opts.waterfront, "waterfront", "", "Waterfront facet: name (Riverfront, Beach, ...) or code (rf, of, bch, cf, lf)")
	cmd.Flags().StringVar(&opts.status, "status", "", "Status filter: active | spotlight | under-contract | sold (default: all except sold)")
	cmd.Flags().BoolVar(&opts.includeSold, "include-sold", false, "Include sold listings (result becomes a superset of the default)")
	cmd.Flags().IntVar(&opts.beds, "beds", 0, "Minimum bedrooms (residential listings only)")
	cmd.Flags().IntVar(&opts.baths, "baths", 0, "Minimum bathrooms (residential listings only)")
	cmd.Flags().StringVar(&opts.sortKey, "sort", "modified", "Sort: modified | price | acres | title | price-per-acre")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "Maximum rows to return (0 = all)")
	return cmd
}

func validateCeibaListingsOptions(opts ceibaListingsOptions) error {
	switch strings.ToLower(strings.TrimSpace(opts.status)) {
	case "", "active", "spotlight", "under-contract", "under_contract", "sold":
	default:
		return fmt.Errorf("invalid --status %q: must be active, spotlight, under-contract, or sold", opts.status)
	}
	switch strings.ToLower(strings.TrimSpace(opts.sortKey)) {
	case "", "modified", "price", "acres", "title", "price-per-acre":
	default:
		return fmt.Errorf("invalid --sort %q: must be modified, price, acres, title, or price-per-acre", opts.sortKey)
	}
	if opts.minPrice > 0 && opts.maxPrice > 0 && opts.minPrice > opts.maxPrice {
		return fmt.Errorf("--min-price %v is greater than --max-price %v", opts.minPrice, opts.maxPrice)
	}
	if opts.minAcres > 0 && opts.maxAcres > 0 && opts.minAcres > opts.maxAcres {
		return fmt.Errorf("--min-acres %v is greater than --max-acres %v", opts.minAcres, opts.maxAcres)
	}
	return nil
}

// ceibaWaterfrontAliases expands the preference-form codes to display names.
var ceibaWaterfrontAliases = map[string]string{
	"rf":  "riverfront",
	"of":  "oceanfront",
	"bch": "beach",
	"cf":  "creekfront",
	"lf":  "lagoonfront",
}

func ceibaWaterfrontQuery(raw string) string {
	q := strings.ToLower(strings.TrimSpace(raw))
	if expanded, ok := ceibaWaterfrontAliases[q]; ok {
		return expanded
	}
	return q
}

// ceibaFilterListings applies every filter, sorts, and limits the result.
func ceibaFilterListings(snap *ceibaSnapshot, opts ceibaListingsOptions) []ceibaListingView {
	var districtMatch func(store.CeibaFact) bool
	if strings.TrimSpace(opts.district) != "" {
		districtMatch = snap.localDistrictFilter(opts.district)
	}
	var hoodMatch func(store.CeibaFact) bool
	if strings.TrimSpace(opts.neighbourhood) != "" {
		hoodMatch = snap.localNeighbourhoodFilter(opts.neighbourhood)
	}
	query := strings.ToLower(strings.TrimSpace(opts.query))
	waterfront := ceibaWaterfrontQuery(opts.waterfront)
	status := strings.ToLower(strings.TrimSpace(opts.status))
	if status == "under_contract" {
		status = "under-contract"
	}
	if status == "sold" {
		// Asking for sold listings implies including them.
		opts.includeSold = true
	}

	views := make([]ceibaListingView, 0, len(snap.Facts))
	for _, f := range snap.Facts {
		if !opts.includeSold && f.Sold {
			continue
		}
		switch status {
		case "active", "spotlight", "under-contract", "sold":
			// Match the projected status exactly so filtered rows agree with
			// their status field (sold > under-contract > spotlight > active).
			if ceibaStatus(f) != status {
				continue
			}
		}
		if opts.minPrice > 0 && (f.PriceUSD == nil || *f.PriceUSD < opts.minPrice) {
			continue
		}
		if opts.maxPrice > 0 && (f.PriceUSD == nil || *f.PriceUSD > opts.maxPrice) {
			continue
		}
		if opts.minAcres > 0 && (f.Acres == nil || *f.Acres < opts.minAcres) {
			continue
		}
		if opts.maxAcres > 0 && (f.Acres == nil || *f.Acres > opts.maxAcres) {
			continue
		}
		if districtMatch != nil && !districtMatch(f) {
			continue
		}
		if hoodMatch != nil && !hoodMatch(f) {
			continue
		}
		if opts.beds > 0 || opts.baths > 0 {
			d, ok := snap.DetailsByID[f.WPID]
			if !ok {
				continue
			}
			if opts.beds > 0 && (d.Beds == nil || *d.Beds < float64(opts.beds)) {
				continue
			}
			if opts.baths > 0 && (d.Baths == nil || *d.Baths < float64(opts.baths)) {
				continue
			}
		}
		if strings.TrimSpace(opts.listingID) != "" {
			d, ok := snap.DetailsByID[f.WPID]
			if !ok || !strings.EqualFold(strings.TrimSpace(d.ListingID), strings.TrimSpace(opts.listingID)) {
				continue
			}
		}
		if strings.TrimSpace(opts.propertyType) != "" && !containsFold(opts.propertyType, f.PropertyType) {
			continue
		}
		if waterfront != "" && !containsFold(waterfront, f.Waterfront) {
			continue
		}
		if query != "" {
			text := strings.ToLower(f.Slug + " " + strings.Join(f.Bullets, " ") + " " + f.TownName + " " + strings.Join(f.DistrictNames, " "))
			if r, ok := snap.ResourcesByID[f.WPID]; ok {
				text += " " + strings.ToLower(r.Title) + " " + strings.ToLower(ceibaPlainText(r.Content))
			}
			if !strings.Contains(text, query) {
				continue
			}
		}
		views = append(views, ceibaViewOf(snap, f))
	}
	sortCeibaViews(views, opts.sortKey)
	if opts.limit > 0 && len(views) > opts.limit {
		views = views[:opts.limit]
	}
	return views
}

func ceibaPrintListingsTable(w io.Writer, views []ceibaListingView) error {
	if len(views) == 0 {
		fmt.Fprintln(w, "no listings matched.")
		return nil
	}
	rows := make([][]string, 0, len(views))
	for _, v := range views {
		rows = append(rows, []string{
			fmt.Sprintf("%d", v.ID),
			ceibaFormatPrice(v.PriceUSD),
			ceibaFormatFloat(v.Acres, "%.2f"),
			ceibaFormatPrice(v.PricePerAcre),
			v.District,
			v.Town,
			v.Status,
			truncate(v.Title, 60),
		})
	}
	tw := newTabWriter(w)
	fmt.Fprintln(tw, strings.Join([]string{"ID", "PRICE", "ACRES", "$/ACRE", "DISTRICT", "TOWN", "STATUS", "TITLE"}, "\t"))
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	return tw.Flush()
}
