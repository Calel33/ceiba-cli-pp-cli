// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
//
// `comps` — nearest peer listings for one property from the local mirror:
// peers sit in the subject's primary district and within ~50% acre
// proximity; each peer carries price and price-per-acre deltas against the
// subject. Sold rows are excluded unless --include-sold (which can only add
// rows — a superset). When no peers match, the payload says so honestly and
// reports how many rows were scanned.

package cli

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"ceiba-cli-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

func newNovelCompsCmd(flags *rootFlags) *cobra.Command {
	var (
		flagIncludeSold bool
		flagLimit       int
	)

	cmd := &cobra.Command{
		Use:   "comps <id|slug>",
		Short: "Nearest peer listings for one property, with price and per-acre deltas; sold comps opt-in.",
		Long: `Find comparable listings for one property in the local mirror: same
primary district, acreage within ~50% of the subject. Each peer reports the
price and price-per-acre deltas versus the subject.

Sold listings are excluded from peers by default; --include-sold includes
them (a superset). Run 'ceiba-cli-pp-cli sync' first.`,
		Example:     "  ceiba-cli-pp-cli comps 20-acre-estate-parcel-w-waterfront-near-hopkins --include-sold",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "id=20-acre-estate-parcel-w-waterfront-near-hopkins"},
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				action := "comps"
				if len(args) > 0 {
					action += " " + strings.Join(args, " ")
				}
				return writeDryRun(w, flags, action)
			}
			if len(args) < 1 {
				return usageErr(fmt.Errorf("comps requires a listing id or slug"))
			}
			if len(args) > 1 {
				return usageErr(fmt.Errorf("comps takes exactly one listing reference, got %d", len(args)))
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
			wpID, found, err := resolveCeibaWPID(ctx, db, args[0])
			if err != nil {
				return classifyAPIError(w, err, flags)
			}
			subject, hasFact := snap.FactsByID[wpID]
			if !found || !hasFact {
				nf := notFoundErr(fmt.Errorf("listing %q has no fact row in the local mirror. Run '%s' first", args[0], ceibaDataCommand))
				writeAPIErrorEnvelope(w, flags, nf, ExitCode(nf))
				return nf
			}
			report := ceibaComps(snap, subject, flagIncludeSold, flagLimit)
			if len(report.Peers) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: no comparable listings matched (%d scanned)%s\n", report.Scanned, ceibaCompsHint(report))
			}
			if wantsHumanTable(w, flags) {
				return ceibaPrintCompsTable(w, report)
			}
			return printJSONFiltered(w, report, flags)
		},
	}
	cmd.Flags().BoolVar(&flagIncludeSold, "include-sold", false, "Include sold listings as peers (superset of the default)")
	cmd.Flags().IntVar(&flagLimit, "limit", 10, "Maximum peers to return (0 = all)")
	return cmd
}

// ceibaCompPeer is one comparable listing with its deltas vs the subject.
type ceibaCompPeer struct {
	ceibaListingView
	DeltaPriceUSD     *float64 `json:"delta_price_usd,omitempty"`
	DeltaPricePerAcre *float64 `json:"delta_price_per_acre,omitempty"`
	AcreDeltaPct      *float64 `json:"acre_delta_pct,omitempty"`
}

// ceibaCompsReport is the comps payload.
type ceibaCompsReport struct {
	Subject ceibaListingView `json:"subject"`
	Peers   []ceibaCompPeer  `json:"peers"`
	Scanned int              `json:"scanned"`
	Notes   []string         `json:"notes,omitempty"`
}

func ceibaCompsHint(report ceibaCompsReport) string {
	if len(report.Notes) > 0 {
		return " (" + strings.Join(report.Notes, "; ") + ")"
	}
	return ""
}

// ceibaComps computes the peer set and deltas.
func ceibaComps(snap *ceibaSnapshot, subject store.CeibaFact, includeSold bool, limit int) ceibaCompsReport {
	report := ceibaCompsReport{
		Subject: ceibaViewOf(snap, subject),
		Peers:   make([]ceibaCompPeer, 0),
	}
	var districtMatch func(store.CeibaFact) bool
	switch {
	case len(subject.DistrictNames) > 0:
		districtMatch = snap.localDistrictFilter(subject.DistrictNames[0])
	case subject.TownName != "":
		districtMatch = snap.localNeighbourhoodFilter(subject.TownName)
	default:
		report.Notes = append(report.Notes, "subject has no district/town data; peers matched on acre proximity only")
	}
	if subject.Acres == nil {
		report.Notes = append(report.Notes, "subject has no acreage; peers are district matches only (no proximity filter)")
	}

	type candidate struct {
		fact     store.CeibaFact
		acreDiff float64
		hasAcres bool
	}
	var candidates []candidate
	missingAcres := 0
	for _, f := range snap.Facts {
		if f.WPID == subject.WPID {
			continue
		}
		if !includeSold && f.Sold {
			continue
		}
		if districtMatch != nil && !districtMatch(f) {
			continue
		}
		c := candidate{fact: f}
		if subject.Acres != nil && f.Acres != nil {
			c.acreDiff = math.Abs(*f.Acres - *subject.Acres)
			c.hasAcres = true
		} else {
			missingAcres++
		}
		candidates = append(candidates, c)
	}
	report.Scanned = len(candidates)

	peers := make([]ceibaCompPeer, 0, len(candidates))
	for _, c := range candidates {
		if subject.Acres != nil {
			if !c.hasAcres {
				// proximity cannot be evaluated without both acreages
				continue
			}
			if c.acreDiff > 0.5**subject.Acres {
				continue
			}
		}
		peer := ceibaCompPeer{ceibaListingView: ceibaViewOf(snap, c.fact)}
		if subject.PriceUSD != nil && peer.PriceUSD != nil {
			d := *peer.PriceUSD - *subject.PriceUSD
			peer.DeltaPriceUSD = &d
		}
		subjPPA := ceibaPricePerAcre(subject.PriceUSD, subject.Acres)
		if subjPPA != nil && peer.PricePerAcre != nil {
			d := *peer.PricePerAcre - *subjPPA
			peer.DeltaPricePerAcre = &d
		}
		if subject.Acres != nil && c.fact.Acres != nil && *subject.Acres > 0 {
			pct := (*c.fact.Acres - *subject.Acres) / *subject.Acres * 100
			peer.AcreDeltaPct = &pct
		}
		peers = append(peers, peer)
	}
	sort.SliceStable(peers, func(i, j int) bool {
		ai, aj := math.MaxFloat64, math.MaxFloat64
		if peers[i].Acres != nil && subject.Acres != nil {
			ai = math.Abs(*peers[i].Acres - *subject.Acres)
		}
		if peers[j].Acres != nil && subject.Acres != nil {
			aj = math.Abs(*peers[j].Acres - *subject.Acres)
		}
		if ai != aj {
			return ai < aj
		}
		return peers[i].ID < peers[j].ID
	})
	if limit > 0 && len(peers) > limit {
		peers = peers[:limit]
	}
	report.Peers = peers
	if len(peers) == 0 {
		report.Notes = append(report.Notes, fmt.Sprintf("no peers matched in the subject's district within 50%% acre proximity (scanned %d candidates)", report.Scanned))
		if missingAcres > 0 {
			report.Notes = append(report.Notes, fmt.Sprintf("%d candidate(s) lacked acre data and could not be range-checked", missingAcres))
		}
		if !includeSold {
			report.Notes = append(report.Notes, "sold peers are hidden; pass --include-sold to widen the search")
		}
	}
	return report
}

func ceibaPrintCompsTable(w io.Writer, report ceibaCompsReport) error {
	subject := report.Subject
	fmt.Fprintf(w, "subject: %s (%d) — %s\n", truncate(subject.Title, 60), subject.ID, subject.Status)
	tw := newTabWriter(w)
	fmt.Fprintln(tw, strings.Join([]string{"ID", "PRICE", "$/ACRE", "ACRES", "Δ PRICE", "Δ $/ACRE", "Δ ACRE %", "STATUS", "SLUG"}, "\t"))
	for _, p := range report.Peers {
		fmt.Fprintln(tw, strings.Join([]string{
			fmt.Sprintf("%d", p.ID),
			ceibaFormatPrice(p.PriceUSD),
			ceibaFormatPrice(p.PricePerAcre),
			ceibaFormatFloat(p.Acres, "%.2f"),
			ceibaSignedPrice(p.DeltaPriceUSD),
			ceibaSignedPrice(p.DeltaPricePerAcre),
			ceibaFormatPct(p.AcreDeltaPct),
			p.Status,
			truncate(p.Slug, 40),
		}, "\t"))
	}
	return tw.Flush()
}

func ceibaSignedPrice(v *float64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%+.0f", *v)
}

func ceibaFormatPct(v *float64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%+.1f%%", *v)
}
