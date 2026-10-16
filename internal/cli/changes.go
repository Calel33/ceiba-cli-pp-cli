// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
//
// `changes` — a digest of what moved since the previous sync: newly appeared
// listings, price drops/raises, and status/spotlight flips. Classification
// runs over listing_history rows recorded by sync-time enrichment:
//
//   - NEW: a listing whose first snapshot was observed after the previous
//     enrichment session ended (and within the --since window).
//   - price_down / price_up: the latest snapshot's price differs from the
//     previous snapshot, observed within the window.
//   - sold_on/off, under_contract_on/off, spotlight_on/off: status flips
//     between the two most recent snapshots.
//
// No history yet (fresh mirror) => empty result with an honest stderr note.

package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"ceiba-cli-pp-cli/internal/cliutil"
	"ceiba-cli-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

type ceibaChangeRow struct {
	WPID       int64    `json:"wp_id"`
	Slug       string   `json:"slug,omitempty"`
	Title      string   `json:"title,omitempty"`
	Change     string   `json:"change"`
	From       *float64 `json:"from,omitempty"`
	To         *float64 `json:"to,omitempty"`
	ObservedAt string   `json:"observed_at,omitempty"`
}

func newNovelChangesCmd(flags *rootFlags) *cobra.Command {
	var (
		flagSince   string
		flagListing string
		flagLimit   int
	)

	cmd := &cobra.Command{
		Use:   "changes",
		Short: "See new listings, price drops, and status flips since a prior sync — a Saturday-morning scan of the whole catalog.",
		Long: `Digest of mirror changes recorded between syncs: new listings, price
drops/raises, and status or spotlight flips.

History is built by 'sync' (each successful sync snapshots changed listings).
Run sync at least twice to observe changes. The window covers snapshots
observed within --since; a listing is NEW when it first appeared after the
previous sync session.`,
		Example:     "  ceiba-cli-pp-cli changes --since 7d",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			if dryRunOK(flags) {
				return writeDryRun(w, flags, "changes (local history digest)")
			}
			if len(args) > 0 {
				return usageErr(fmt.Errorf("changes takes no positional arguments"))
			}
			window, err := cliutil.ParseDurationLoose(flagSince)
			if err != nil {
				return usageErr(fmt.Errorf("invalid --since value %q: %w", flagSince, err))
			}
			if window <= 0 {
				return usageErr(fmt.Errorf("--since must be a positive duration (got %q)", flagSince))
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

			var wpID int64
			if strings.TrimSpace(flagListing) != "" {
				resolved, found, err := resolveCeibaWPID(ctx, db, flagListing)
				if err != nil {
					return classifyAPIError(w, err, flags)
				}
				if !found {
					nf := notFoundErr(fmt.Errorf("listing %q not found in the local mirror. Run '%s' first", flagListing, ceibaDataCommand))
					writeAPIErrorEnvelope(w, flags, nf, ExitCode(nf))
					return nf
				}
				wpID = resolved
			}

			rows, err := db.CeibaHistoryRows(ctx, wpID)
			if err != nil {
				return classifyAPIError(w, err, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: no change history yet — run '%s' to build a baseline, then sync again to see changes\n", ceibaDataCommand)
				if wantsHumanTable(w, flags) {
					return nil
				}
				return printJSONFiltered(w, []ceibaChangeRow{}, flags)
			}
			sessionStart, hasSession, err := db.CeibaEnrichSessionStart(ctx)
			if err != nil {
				return classifyAPIError(w, err, flags)
			}

			windowStart := time.Now().UTC().Add(-window)
			changes := ceibaClassifyChanges(rows, sessionStart, hasSession, windowStart)
			if len(changes) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no changes observed in the window (this is a clean digest, not an error)")
			}
			ceibaAttachChangeTitles(ctx, db, changes)
			if flagLimit > 0 && len(changes) > flagLimit {
				changes = changes[:flagLimit]
			}
			if wantsHumanTable(w, flags) {
				return ceibaPrintChangesTable(w, changes)
			}
			return printJSONFiltered(w, changes, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "7d", "Look-back window for changes (e.g. 7d, 36h, 2w)")
	cmd.Flags().StringVar(&flagListing, "listing", "", "Restrict the digest to one listing (id, slug, or listing ID)")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum change rows to return (0 = all)")
	return cmd
}

// ceibaClassifyChanges is the pure history-diff classifier. rows must be
// ordered by (wp_id, observed_at). sessionStart is the start of the most
// recent enrichment session (hasSession false when no session marker exists —
// NEW classification is then disabled).
func ceibaClassifyChanges(rows []store.CeibaHistoryRow, sessionStart time.Time, hasSession bool, windowStart time.Time) []ceibaChangeRow {
	out := make([]ceibaChangeRow, 0)
	for i := 0; i < len(rows); {
		j := i
		for j < len(rows) && rows[j].WPID == rows[i].WPID {
			j++
		}
		group := rows[i:j]
		i = j
		first, latest := group[0], group[len(group)-1]
		firstAt := cliutil.ParseStoredTime(first.ObservedAt)
		latestAt := cliutil.ParseStoredTime(latest.ObservedAt)

		if hasSession && !firstAt.IsZero() && firstAt.After(sessionStart) && !latestAt.Before(windowStart) {
			out = append(out, ceibaChangeRow{
				WPID:       latest.WPID,
				Change:     "new",
				ObservedAt: latest.ObservedAt,
			})
			continue
		}
		if len(group) < 2 || latestAt.Before(windowStart) {
			continue
		}
		prev := group[len(group)-2]
		base := ceibaChangeRow{WPID: latest.WPID, ObservedAt: latest.ObservedAt}
		if prev.PriceUSD != nil && latest.PriceUSD != nil && *prev.PriceUSD != *latest.PriceUSD {
			row := base
			row.From = prev.PriceUSD
			row.To = latest.PriceUSD
			if *latest.PriceUSD < *prev.PriceUSD {
				row.Change = "price_down"
			} else {
				row.Change = "price_up"
			}
			out = append(out, row)
		}
		if prev.Sold != latest.Sold {
			row := base
			if latest.Sold {
				row.Change = "sold_on"
			} else {
				row.Change = "sold_off"
			}
			out = append(out, row)
		}
		if prev.UnderContract != latest.UnderContract {
			row := base
			if latest.UnderContract {
				row.Change = "under_contract_on"
			} else {
				row.Change = "under_contract_off"
			}
			out = append(out, row)
		}
		if prev.Spotlight != latest.Spotlight {
			row := base
			if latest.Spotlight {
				row.Change = "spotlight_on"
			} else {
				row.Change = "spotlight_off"
			}
			out = append(out, row)
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].ObservedAt != out[b].ObservedAt {
			return out[a].ObservedAt > out[b].ObservedAt
		}
		return out[a].WPID < out[b].WPID
	})
	return out
}

// ceibaAttachChangeTitles joins slug/title onto change rows (best-effort).
func ceibaAttachChangeTitles(ctx context.Context, db *store.Store, changes []ceibaChangeRow) {
	if len(changes) == 0 {
		return
	}
	listings, err := loadCeibaListings(ctx, db)
	if err != nil {
		return
	}
	byID := make(map[int64]ceibaResourceRow, len(listings))
	for _, r := range listings {
		byID[r.ID] = r
	}
	for i := range changes {
		if r, ok := byID[changes[i].WPID]; ok {
			changes[i].Slug = r.Slug
			changes[i].Title = r.Title
		}
	}
}

func ceibaPrintChangesTable(w io.Writer, changes []ceibaChangeRow) error {
	if len(changes) == 0 {
		fmt.Fprintln(w, "no changes in the window.")
		return nil
	}
	tw := newTabWriter(w)
	fmt.Fprintln(tw, strings.Join([]string{"WP_ID", "CHANGE", "FROM", "TO", "SLUG", "OBSERVED"}, "\t"))
	for _, c := range changes {
		fmt.Fprintln(tw, strings.Join([]string{
			fmt.Sprintf("%d", c.WPID),
			c.Change,
			ceibaFormatPrice(c.From),
			ceibaFormatPrice(c.To),
			truncate(c.Slug, 50),
			c.ObservedAt,
		}, "\t"))
	}
	return tw.Flush()
}
