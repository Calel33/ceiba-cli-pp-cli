// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
// Table-driven parser tests over fixture slices cut from the verified live
// captures (discovery/, 2026-10-16). The fixtures are small cuts, not full
// pages: see testdata file headers for source ranges.

package scrape

import (
	"os"
	"path/filepath"
	"testing"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func approx(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	d := *a - *b
	return d < 0.0001 && d > -0.0001
}

func TestParseListingCards(t *testing.T) {
	fixture := loadFixture(t, "our-listing-cards.html")
	cards := ParseListingCards(fixture)
	if len(cards) != 4 {
		t.Fatalf("ParseListingCards returned %d cards, want 4", len(cards))
	}

	bySlug := map[string]CardFacts{}
	for _, c := range cards {
		bySlug[c.Slug] = c
	}

	want := []struct {
		slug      string
		priceUSD  float64
		acres     float64
		spotlight bool
		under     bool
		sold      bool
		names     []string
		bullets   []string
	}{
		{
			slug:     "a-town-lot-w-stunning-views-in-the-highly-coveted-maya-vista-neighborhood",
			priceUSD: 99000, acres: 0.15, spotlight: true, sold: false, under: false,
			names:   []string{"Cayo", "San Ignacio Town"},
			bullets: []string{"In town Access", "Panoramic Views", "Amenities Available"},
		},
		{
			slug:     "stunning-5-8-acre-homestead-outside-bullet-tree-falls",
			priceUSD: 115000, acres: 5.8, spotlight: true, sold: true, under: false,
			names:   []string{"Bullet Tree Falls", "Cayo"},
			bullets: []string{"Fenced & Cleared", "Starter Cabin Available", "Fruit Trees Galore"},
		},
		{
			slug:     "twenty-three-acres-of-untouched-real-estate-in-the-young-gyal-mcrae-area",
			priceUSD: 35000, acres: 23.23, spotlight: false, sold: true, under: false,
			names: []string{"Buena Vista", "Cayo"},
		},
		{
			slug:     "a-stunning-20-acre-riverfront-property-in-hummingbird-highway-area",
			priceUSD: 135000, acres: 20, spotlight: true, sold: false, under: true,
			names: []string{"Middlesex", "Stann Creek"},
		},
	}

	for _, w := range want {
		got, ok := bySlug[w.slug]
		if !ok {
			t.Errorf("card %q missing from parse result (slugs: %v)", w.slug, slugsOf(cards))
			continue
		}
		if !approx(got.PriceUSD, &w.priceUSD) {
			t.Errorf("%s price_usd = %v, want %v", w.slug, ptr(got.PriceUSD), w.priceUSD)
		}
		if !approx(got.Acres, &w.acres) {
			t.Errorf("%s acres = %v, want %v", w.slug, ptr(got.Acres), w.acres)
		}
		if got.Spotlight != w.spotlight {
			t.Errorf("%s spotlight = %v, want %v", w.slug, got.Spotlight, w.spotlight)
		}
		if got.Sold != w.sold {
			t.Errorf("%s sold = %v, want %v", w.slug, got.Sold, w.sold)
		}
		if got.UnderContract != w.under {
			t.Errorf("%s under_contract = %v, want %v", w.slug, got.UnderContract, w.under)
		}
		if w.names != nil {
			if len(got.DistrictNames) != len(w.names) {
				t.Errorf("%s district names = %v, want %v", w.slug, got.DistrictNames, w.names)
			} else {
				for i := range w.names {
					if got.DistrictNames[i] != w.names[i] {
						t.Errorf("%s district names = %v, want %v", w.slug, got.DistrictNames, w.names)
						break
					}
				}
			}
		}
		if w.bullets != nil {
			if len(got.Bullets) != len(w.bullets) {
				t.Errorf("%s bullets = %v, want %v", w.slug, got.Bullets, w.bullets)
			} else {
				for i := range w.bullets {
					if got.Bullets[i] != w.bullets[i] {
						t.Errorf("%s bullets = %v, want %v", w.slug, got.Bullets, w.bullets)
						break
					}
				}
			}
		} else if len(got.Bullets) == 0 {
			t.Errorf("%s bullets empty, want non-empty", w.slug)
		}
		if got.URL == "" {
			t.Errorf("%s url=%q, want non-empty", w.slug, got.URL)
		}
	}
}

func slugsOf(cards []CardFacts) []string {
	out := make([]string, 0, len(cards))
	for _, c := range cards {
		out = append(out, c.Slug)
	}
	return out
}

func ptr(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func TestParseListingDetail(t *testing.T) {
	tests := []struct {
		fixture      string
		title        string
		price        float64
		listingID    string
		acres        float64
		beds         *float64
		baths        *float64
		living       *float64
		lat          float64
		lng          float64
		town         string
		district     string
		waterfront   string
		propertyType string
	}{
		{
			fixture: "detail-100-acre.html",
			title:   "100 Acre Investment Property on the Belize River in Cayo!",
			price:   200000, listingID: "4HM1091", acres: 100,
			beds: nil, baths: nil,
			lat: 17.356395, lng: -88.683128,
			town: "More Tomorrow", district: "Cayo",
			waterfront: "Belize River", propertyType: "Acreage",
		},
		{
			fixture: "detail-treehouse.html",
			title:   "A Unique Treehouse perched atop the Macal River!",
			price:   185000, listingID: "4NV1117", acres: 0.75,
			beds: float64p(1), baths: float64p(1),
			lat: 16.998484, lng: -89.042508,
			town: "Macal River", district: "Cayo",
			waterfront: "Macal River",
		},
		{
			fixture: "detail-3bed.html",
			// The hero h1 (discovery line 2144) and the WP REST title both
			// read "Home in Tropical Park..."; the old expectation here was
			// the page's meta description, not the rendered title.
			title: "Home in Tropical Park with Income Generating Potential!",
			price: 150000, listingID: "3KT1101", acres: 0.17,
			beds: float64p(3), baths: float64p(3), living: float64p(1947),
			lat: 17.466143, lng: -88.385413,
			town: "Hattieville", district: "Belize",
		},
	}

	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			facts, err := ParseListingDetail(loadFixture(t, tc.fixture))
			if err != nil {
				t.Fatalf("ParseListingDetail(%s) error = %v", tc.fixture, err)
			}
			if tc.title != "" && facts.Title != tc.title {
				t.Errorf("title = %q, want %q", facts.Title, tc.title)
			}
			if !approx(facts.PriceUSD, &tc.price) {
				t.Errorf("price_usd = %v, want %v", ptr(facts.PriceUSD), tc.price)
			}
			if tc.listingID != "" && facts.ListingID != tc.listingID {
				t.Errorf("listing_id = %q, want %q", facts.ListingID, tc.listingID)
			}
			if !approx(facts.Acres, &tc.acres) {
				t.Errorf("acres = %v, want %v", ptr(facts.Acres), tc.acres)
			}
			if !approx(facts.Beds, tc.beds) {
				t.Errorf("beds = %v, want %v", ptr(facts.Beds), ptr(tc.beds))
			}
			if !approx(facts.Baths, tc.baths) {
				t.Errorf("baths = %v, want %v", ptr(facts.Baths), ptr(tc.baths))
			}
			if !approx(facts.LivingSpace, tc.living) {
				t.Errorf("living_space = %v, want %v", ptr(facts.LivingSpace), ptr(tc.living))
			}
			if !approx(facts.MapLat, &tc.lat) {
				t.Errorf("map_lat = %v, want %v", ptr(facts.MapLat), tc.lat)
			}
			if !approx(facts.MapLng, &tc.lng) {
				t.Errorf("map_lng = %v, want %v", ptr(facts.MapLng), tc.lng)
			}
			if tc.town != "" && facts.TownName != tc.town {
				t.Errorf("town = %q, want %q", facts.TownName, tc.town)
			}
			if tc.district != "" && facts.DistrictName != tc.district {
				t.Errorf("district = %q, want %q", facts.DistrictName, tc.district)
			}
			if tc.waterfront != "" && facts.Waterfront != tc.waterfront {
				t.Errorf("waterfront = %q, want %q", facts.Waterfront, tc.waterfront)
			}
			if tc.propertyType != "" && facts.PropertyType != tc.propertyType {
				t.Errorf("property_type = %q, want %q", facts.PropertyType, tc.propertyType)
			}
			if len(facts.GalleryURLs) == 0 {
				t.Errorf("gallery_urls empty, want non-empty")
			}
		})
	}
}

func float64p(v float64) *float64 { return &v }

func TestParseListingDetail100AcreExtras(t *testing.T) {
	facts, err := ParseListingDetail(loadFixture(t, "detail-100-acre.html"))
	if err != nil {
		t.Fatalf("ParseListingDetail error = %v", err)
	}
	if len(facts.BrochureURLs) == 0 {
		t.Errorf("brochure_urls empty, want gc-brochure image links")
	}
	if len(facts.Similar) < 5 {
		t.Errorf("similar entries = %d, want >= 5", len(facts.Similar))
	}
	if len(facts.Similar) > 0 {
		first := facts.Similar[0]
		if first.Slug != "a-town-lot-w-stunning-views-in-the-highly-coveted-maya-vista-neighborhood" {
			t.Errorf("similar[0].slug = %q", first.Slug)
		}
		if first.Title == "" {
			t.Errorf("similar[0].title empty")
		}
		if first.URL == "" {
			t.Errorf("similar[0].url empty")
		}
	}
	for _, g := range facts.GalleryURLs {
		if len(g) < 8 || g[:8] != "https://" {
			t.Errorf("gallery url %q not absolute", g)
			break
		}
	}
}

func TestParseListingDetailTolerantOfEmptyHTML(t *testing.T) {
	facts, err := ParseListingDetail([]byte("<html><body><p>nothing here</p></body></html>"))
	if err != nil {
		t.Fatalf("ParseListingDetail error = %v", err)
	}
	if facts.PriceUSD != nil || facts.ListingID != "" || facts.Acres != nil {
		t.Errorf("expected zero-value facts, got %+v", facts)
	}
}

func TestParseRecentSalesEmpty(t *testing.T) {
	records := ParseRecentSales([]byte("<html><body>no gallery</body></html>"))
	if len(records) != 0 {
		t.Fatalf("records = %d, want 0", len(records))
	}
}

func TestParseRecentSalesSynthetic(t *testing.T) {
	htmlDoc := []byte(`<html><body>
	<section class="gc-listing-gallery">
	  <div class="e-gallery-item"><div class="e-gallery-image elementor-gallery-item__image" data-thumbnail="https://ceibabelize.com/wp-content/uploads/2025/04/DJI_0449.jpg" aria-label="Sold Home"></div></div>
	</section>
	<figure class="wp-caption"><a href="https://ceibabelize.com/recent-sales-item/"><img src="https://ceibabelize.com/wp-content/uploads/2024/01/House-A.jpg" /></a><figcaption class="widget-image-caption wp-caption-text">Beachfront Villa, Placencia</figcaption></figure>
	</body></html>`)
	records := ParseRecentSales(htmlDoc)
	if len(records) != 2 {
		t.Fatalf("records = %d (%+v), want 2", len(records), records)
	}
	if records[0].ImageURL != "https://ceibabelize.com/wp-content/uploads/2025/04/DJI_0449.jpg" {
		t.Errorf("record[0].image = %q", records[0].ImageURL)
	}
	if records[1].ImageURL != "https://ceibabelize.com/wp-content/uploads/2024/01/House-A.jpg" {
		t.Errorf("record[1].image = %q", records[1].ImageURL)
	}
	if records[1].Caption != "Beachfront Villa, Placencia" {
		t.Errorf("record[1].caption = %q", records[1].Caption)
	}
}

func TestParseDevelopmentPageSynthetic(t *testing.T) {
	htmlDoc := []byte(`<html><body>
	<h1 class="entry-title">Hopkins Grove</h1>
	<div class="elementor"><p>A serene beachfront community in Hopkins, Stann Creek.</p>
	<div class="e-gallery-image elementor-gallery-item__image" data-thumbnail="https://ceibabelize.com/wp-content/uploads/2026/04/Hopkins-Grove.jpg"></div>
	<section data-dce-background-image-url="https://ceibabelize.com/wp-content/uploads/2021/07/Untitled-12-scaled.jpg"></section>
	</div></body></html>`)
	page := ParseDevelopmentPage(htmlDoc)
	if page.Title != "Hopkins Grove" {
		t.Errorf("title = %q", page.Title)
	}
	if len(page.Images) != 2 {
		t.Errorf("images = %v, want thumbnail + background url", page.Images)
	}
	if page.Images[0] != "https://ceibabelize.com/wp-content/uploads/2026/04/Hopkins-Grove.jpg" {
		t.Errorf("images[0] = %q", page.Images[0])
	}
	if page.Images[1] != "https://ceibabelize.com/wp-content/uploads/2021/07/Untitled-12-scaled.jpg" {
		t.Errorf("images[1] = %q", page.Images[1])
	}
	if page.Description == "" {
		t.Errorf("description empty, want paragraph text")
	}
}
