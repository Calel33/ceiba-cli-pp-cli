// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
// Table-driven tests for the market report math (medians, status mix,
// acre bands, taxonomy parent-inclusion, name fallback).

package cli

import (
	"strings"
	"testing"

	"ceiba-cli-pp-cli/internal/store"
)

func TestCeibaMedian(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		want   *float64
	}{
		{"empty", nil, nil},
		{"single", []float64{42}, hp(42)},
		{"odd", []float64{3, 1, 2}, hp(2)},
		{"even averages middle pair", []float64{4, 1, 3, 2}, hp(2.5)},
		{"unsorted input is sorted", []float64{100, 1, 50}, hp(50)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ceibaMedian(tc.values)
			if (got == nil) != (tc.want == nil) {
				t.Fatalf("ceibaMedian(%v) = %v, want %v", tc.values, ptrOrNil(got), ptrOrNil(tc.want))
			}
			if got != nil && *got != *tc.want {
				t.Fatalf("ceibaMedian(%v) = %v, want %v", tc.values, *got, *tc.want)
			}
		})
	}
}

func ptrOrNil(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func TestCeibaMarketReport(t *testing.T) {
	snap := ceibaEmptySnapshot()
	// Taxonomy: Cayo (parent 50) with child San Ignacio Town (147).
	snap.Terms = []ceibaTerm{
		{ID: 50, Name: "Cayo", Slug: "cayo", Parent: 0},
		{ID: 147, Name: "San Ignacio Town", Slug: "san-ignacio-town", Parent: 50},
		{ID: 49, Name: "Belize", Slug: "belize", Parent: 0},
	}
	for _, term := range snap.Terms {
		snap.termsBySlug[term.Slug] = term
		snap.termsByName[strings.ToLower(term.Name)] = term
		if term.Parent != 0 {
			snap.childrenOf[term.Parent] = append(snap.childrenOf[term.Parent], term)
		}
	}
	snap.Facts = []store.CeibaFact{
		{WPID: 1, Slug: "a", DistrictNames: []string{"Cayo"}, TownName: "San Ignacio Town", PriceUSD: hp(100000), Acres: hp(10), Spotlight: true},
		{WPID: 2, Slug: "b", DistrictNames: []string{"Cayo"}, PriceUSD: hp(200000), Acres: hp(20), UnderContract: true},
		{WPID: 3, Slug: "c", DistrictNames: []string{"Belize"}, PriceUSD: hp(50000), Acres: hp(5)},
		{WPID: 4, Slug: "d", DistrictNames: []string{"Cayo"}, PriceUSD: nil, Acres: nil, Sold: true},
	}

	tests := []struct {
		name            string
		query           string
		wantCount       int
		wantMatchedBy   string
		wantMedianPrice *float64
		wantMedianPPA   *float64
		wantMedianAcres *float64
		wantBands       map[string]int
	}{
		{
			name:  "parent district includes children and sold rows",
			query: "cayo", wantCount: 3, wantMatchedBy: "taxonomy",
			wantMedianPrice: hp(150000), wantMedianPPA: hp(10000),
			wantMedianAcres: hp(15),
			wantBands:       map[string]int{"10-25": 2, "0-1": 0, "1-5": 0, "5-10": 0, "25-50": 0, "50-100": 0, "100+": 0},
		},
		{
			name:  "child town query matches only its listing",
			query: "san-ignacio-town", wantCount: 1, wantMatchedBy: "taxonomy",
			wantMedianPrice: hp(100000), wantMedianPPA: hp(10000),
			wantMedianAcres: hp(10),
		},
		{
			name:  "unknown district falls back to name match with zero count",
			query: "zzz-nonexistent", wantCount: 0, wantMatchedBy: "name",
		},
		{
			name:  "district without data reports zeros honestly",
			query: "belize", wantCount: 1, wantMatchedBy: "taxonomy",
			wantMedianPrice: hp(50000), wantMedianPPA: hp(10000),
			wantMedianAcres: hp(5),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			report := ceibaMarketReport(snap, tc.query)
			if report.Count != tc.wantCount {
				t.Fatalf("count = %d, want %d", report.Count, tc.wantCount)
			}
			if report.MatchedBy != tc.wantMatchedBy {
				t.Fatalf("matched_by = %q, want %q", report.MatchedBy, tc.wantMatchedBy)
			}
			assertPtrEqual(t, "median_price", report.MedianPrice, tc.wantMedianPrice)
			assertPtrEqual(t, "median_price_per_acre", report.MedianPricePerAcre, tc.wantMedianPPA)
			assertPtrEqual(t, "median_acres", report.MedianAcres, tc.wantMedianAcres)
			if sum := report.StatusMix["active"] + report.StatusMix["spotlight"] + report.StatusMix["under_contract"] + report.StatusMix["sold"]; sum != report.Count {
				t.Fatalf("status mix sums to %d, want count %d (%v)", sum, report.Count, report.StatusMix)
			}
			for _, key := range []string{"active", "spotlight", "under_contract", "sold"} {
				if _, present := report.StatusMix[key]; !present {
					t.Fatalf("status mix missing zero-inclusive key %q: %v", key, report.StatusMix)
				}
			}
			if tc.wantBands != nil {
				got := map[string]int{}
				for _, b := range report.AcreBands {
					got[b.Band] = b.Count
				}
				for band, want := range tc.wantBands {
					if got[band] != want {
						t.Fatalf("band %q = %d, want %d (all: %v)", band, got[band], want, got)
					}
				}
			}
		})
	}
}

func assertPtrEqual(t *testing.T, label string, got, want *float64) {
	t.Helper()
	if (got == nil) != (want == nil) {
		t.Fatalf("%s = %v, want %v", label, ptrOrNil(got), ptrOrNil(want))
	}
	if got != nil && *got != *want {
		t.Fatalf("%s = %v, want %v", label, *got, *want)
	}
}
