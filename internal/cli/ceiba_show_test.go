// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
// Regression tests for the acceptance-driven show/changes fixes: the auto
// freshness signal must require a cached detail row, and an empty changes
// digest must serialize as [] (not null).

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ceiba-cli-pp-cli/internal/cliutil/testenv"
	"ceiba-cli-pp-cli/internal/store"
)

// TestCeibaShowLocalFreshnessSignal guards the bug where a facts-only row
// (no cached detail scrape) could satisfy the auto-mode freshness check.
func TestCeibaShowLocalFreshnessSignal(t *testing.T) {
	testenv.Isolate(t)
	ctx := context.Background()
	db, err := store.Open(defaultDBPath(ceibaStoreName))
	if err != nil {
		t.Fatalf("open scratch store: %v", err)
	}
	defer db.Close()
	if err := store.EnsureCeibaMirrorTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertCeibaFact(ctx, store.CeibaFact{WPID: 7, Slug: "freshness-probe", PriceUSD: hp(12345)}); err != nil {
		t.Fatal(err)
	}
	snap, err := loadCeibaSnapshot(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	res, ok, hasDetail := ceibaShowLocal(ctx, db, true, snap, "freshness-probe")
	if !ok {
		t.Fatal("facts-only row should resolve locally")
	}
	if hasDetail {
		t.Fatal("facts-only row must NOT claim a cached detail (auto-mode would treat it as fresh)")
	}
	if res.PriceUSD == nil || *res.PriceUSD != 12345 {
		t.Fatalf("price = %v, want 12345", res.PriceUSD)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := db.UpsertCeibaDetail(ctx, store.CeibaDetail{WPID: 7, ListingID: "4ZZ9999", ScrapedAt: now}); err != nil {
		t.Fatal(err)
	}
	snap, err = loadCeibaSnapshot(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	res, ok, hasDetail = ceibaShowLocal(ctx, db, true, snap, "freshness-probe")
	if !ok || !hasDetail {
		t.Fatalf("cached detail should resolve with hasDetail=true (ok=%v hasDetail=%v)", ok, hasDetail)
	}
	if !ceibaDetailIsFresh(res.SyncedAt) {
		t.Fatalf("just-scraped detail must be fresh (synced_at=%q)", res.SyncedAt)
	}
	if res.ListingID != "4ZZ9999" {
		t.Fatalf("listing_id = %q, want 4ZZ9999", res.ListingID)
	}
}

// TestCeibaChangesEmptyIsArray guards the acceptance-found bug where a
// no-change digest serialized as JSON null instead of [].
func TestCeibaChangesEmptyIsArray(t *testing.T) {
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
	// One baseline row observed before the session marker: no NEW, no diffs.
	if _, err := db.AppendCeibaHistoryIfChanged(ctx, store.CeibaFact{WPID: 5, Slug: "steady", PriceUSD: hp(1000)}, now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveCeibaEnrichSessionStart(ctx, now.Add(-time.Hour), 1, now); err != nil {
		t.Fatal(err)
	}
	db.Close()

	cmd := RootCmd()
	cmd.SetArgs([]string{"changes", "--since", "7d", "--json"})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("changes execute: %v\n%s", err, out.String())
	}
	// stdout must be a bare [] even though the stderr note explains the
	// empty digest (machine callers parse stdout only).
	if got := strings.TrimSpace(out.String()); got != "[]" {
		t.Fatalf("empty digest output = %q, want [] (stderr: %q)", got, errOut.String())
	}
	if !strings.Contains(errOut.String(), "no changes observed") {
		t.Fatalf("stderr note missing: %q", errOut.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("output not parseable as JSON array: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(rows))
	}
}
