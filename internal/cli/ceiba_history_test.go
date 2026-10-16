// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
// Table-driven tests for the change-digest classifier (history-diff math)
// plus a scratch-DB integration test proving NEW detection end to end.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"ceiba-cli-pp-cli/internal/cliutil/testenv"
	"ceiba-cli-pp-cli/internal/store"
)

func hp(v float64) *float64 { return &v }

func TestCeibaClassifyChanges(t *testing.T) {
	sessionStart := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	windowStart := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	before := "2026-10-14T10:00:00Z"
	after := "2026-10-15T13:00:00Z"
	old := "2026-10-01T10:00:00Z"

	row := func(wp int64, at string, price *float64, spotlight, under, sold bool) store.CeibaHistoryRow {
		return store.CeibaHistoryRow{WPID: wp, ObservedAt: at, PriceUSD: price, Spotlight: spotlight, UnderContract: under, Sold: sold}
	}

	tests := []struct {
		name       string
		rows       []store.CeibaHistoryRow
		hasSession bool
		want       []string // "wp:change" in order
	}{
		{
			name:       "new listing after session start",
			rows:       []store.CeibaHistoryRow{row(2, after, hp(100000), false, false, false)},
			hasSession: true,
			want:       []string{"2:new"},
		},
		{
			name:       "baseline listing before session start is not new",
			rows:       []store.CeibaHistoryRow{row(1, before, hp(100000), false, false, false)},
			hasSession: true,
			want:       nil,
		},
		{
			name: "price down between latest snapshots",
			rows: []store.CeibaHistoryRow{
				row(3, before, hp(200000), false, false, false),
				row(3, after, hp(180000), false, false, false),
			},
			hasSession: true,
			want:       []string{"3:price_down"},
		},
		{
			name: "price up between latest snapshots",
			rows: []store.CeibaHistoryRow{
				row(3, before, hp(200000), false, false, false),
				row(3, after, hp(250000), false, false, false),
			},
			hasSession: true,
			want:       []string{"3:price_up"},
		},
		{
			name: "spotlight and under-contract flips",
			rows: []store.CeibaHistoryRow{
				row(4, before, hp(50000), false, true, false),
				row(4, after, hp(50000), true, false, false),
			},
			hasSession: true,
			want:       []string{"4:under_contract_off", "4:spotlight_on"},
		},
		{
			name: "sold flip",
			rows: []store.CeibaHistoryRow{
				row(5, before, hp(90000), false, false, false),
				row(5, after, hp(90000), false, false, true),
			},
			hasSession: true,
			want:       []string{"5:sold_on"},
		},
		{
			name: "change outside the window is excluded",
			rows: []store.CeibaHistoryRow{
				row(6, old, hp(100000), false, false, false),
				row(6, "2026-10-02T00:00:00Z", hp(110000), false, false, false),
			},
			hasSession: true,
			want:       nil,
		},
		{
			name: "no session marker disables new detection",
			rows: []store.CeibaHistoryRow{row(7, after, hp(100000), false, false, false)},
			want: nil,
		},
		{
			name:       "new outside window is excluded",
			rows:       []store.CeibaHistoryRow{row(8, old, hp(100000), false, false, false)},
			hasSession: true,
			want:       nil,
		},
		{
			name: "unchanged consecutive snapshots emit nothing",
			rows: []store.CeibaHistoryRow{
				row(9, before, hp(100000), true, false, false),
				row(9, after, hp(100000), true, false, false),
			},
			hasSession: true,
			want:       nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ceibaClassifyChanges(tc.rows, sessionStart, tc.hasSession, windowStart)
			var labels []string
			for _, c := range got {
				labels = append(labels, c.slugKey())
			}
			if len(labels) != len(tc.want) {
				t.Fatalf("changes = %v, want %v", labels, tc.want)
			}
			for i := range tc.want {
				if labels[i] != tc.want[i] {
					t.Fatalf("changes[%d] = %q, want %q (all: %v)", i, labels[i], tc.want[i], labels)
				}
			}
		})
	}
}

// slugKey is a test helper: "wpID:change".
func (c ceibaChangeRow) slugKey() string {
	return strconv.FormatInt(c.WPID, 10) + ":" + c.Change
}

// TestCeibaChangesScratchDB proves end-to-end NEW detection and the
// no-fabricated-rows contract on a scratch mirror database.
func TestCeibaChangesScratchDB(t *testing.T) {
	testenv.Isolate(t)
	ctx := context.Background()
	db, err := store.Open(defaultDBPath(ceibaStoreName))
	if err != nil {
		t.Fatalf("open scratch store: %v", err)
	}
	defer db.Close()
	if err := store.EnsureCeibaMirrorTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}

	now := time.Now().UTC()
	sessionStart := now.Add(-30 * time.Minute)
	before := now.Add(-90 * time.Minute)
	after := now.Add(-20 * time.Minute)

	baseline := store.CeibaFact{WPID: 1, Slug: "unchanged-listing", PriceUSD: hp(100000), Acres: hp(10)}
	newcomer := store.CeibaFact{WPID: 2, Slug: "brand-new-listing", PriceUSD: hp(75000), Acres: hp(5)}
	dropper := store.CeibaFact{WPID: 3, Slug: "price-dropper", PriceUSD: hp(200000), Acres: hp(20)}
	dropped := store.CeibaFact{WPID: 3, Slug: "price-dropper", PriceUSD: hp(180000), Acres: hp(20)}

	// First observations happen before the current session for 1 and 3.
	if _, err := db.AppendCeibaHistoryIfChanged(ctx, baseline, before); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AppendCeibaHistoryIfChanged(ctx, dropper, before); err != nil {
		t.Fatal(err)
	}
	// "Second sync": the newcomer appears; the dropper's price fell; the
	// baseline listing is unchanged and must not append.
	changed, err := db.AppendCeibaHistoryIfChanged(ctx, newcomer, after)
	if err != nil || !changed {
		t.Fatalf("newcomer append: changed=%v err=%v", changed, err)
	}
	if changed, err := db.AppendCeibaHistoryIfChanged(ctx, dropped, after); err != nil || !changed {
		t.Fatalf("dropper append: changed=%v err=%v", changed, err)
	}
	if changed, err := db.AppendCeibaHistoryIfChanged(ctx, baseline, after); err != nil || changed {
		t.Fatalf("unchanged listing appended a row (changed=%v err=%v); write-on-change-only violated", changed, err)
	}
	if err := db.SaveCeibaEnrichSessionStart(ctx, sessionStart, 3, now); err != nil {
		t.Fatal(err)
	}

	rows, err := db.CeibaHistoryRows(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	marker, hasSession, err := db.CeibaEnrichSessionStart(ctx)
	if err != nil || !hasSession {
		t.Fatalf("session marker: has=%v err=%v", hasSession, err)
	}
	changes := ceibaClassifyChanges(rows, marker, hasSession, now.Add(-7*24*time.Hour))

	byID := map[int64][]ceibaChangeRow{}
	for _, c := range changes {
		byID[c.WPID] = append(byID[c.WPID], c)
	}
	if got := byID[1]; len(got) != 0 {
		t.Fatalf("unchanged listing produced changes: %+v", got)
	}
	if got := byID[2]; len(got) != 1 || got[0].Change != "new" {
		t.Fatalf("newcomer changes = %+v, want one NEW", got)
	}
	if got := byID[3]; len(got) != 1 || got[0].Change != "price_down" {
		t.Fatalf("dropper changes = %+v, want one price_down", got)
	}
	if got := byID[3]; len(got) == 1 && (got[0].From == nil || got[0].To == nil || *got[0].From != 200000 || *got[0].To != 180000) {
		t.Fatalf("price deltas not preserved: %+v", got)
	}
}

// TestCeibaChangesCommandJSON runs the real command against the scratch DB.
func TestCeibaChangesCommandJSON(t *testing.T) {
	testenv.Isolate(t)
	ctx := context.Background()
	db, err := store.Open(defaultDBPath(ceibaStoreName))
	if err != nil {
		t.Fatalf("open scratch store: %v", err)
	}
	now := time.Now().UTC()
	if err := store.EnsureCeibaMirrorTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	newcomer := store.CeibaFact{WPID: 42, Slug: "fresh-listing", PriceUSD: hp(120000), Acres: hp(8)}
	if _, err := db.AppendCeibaHistoryIfChanged(ctx, newcomer, now.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveCeibaEnrichSessionStart(ctx, now.Add(-20*time.Minute), 1, now); err != nil {
		t.Fatal(err)
	}
	db.Close()

	cmd := RootCmd()
	cmd.SetArgs([]string{"changes", "--since", "7d", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("changes execute: %v\n%s", err, out.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("changes output is not JSON: %v\n%s", err, out.String())
	}
	if len(rows) != 1 || rows[0]["change"] != "new" {
		t.Fatalf("command output = %s, want one NEW row", out.String())
	}
}
