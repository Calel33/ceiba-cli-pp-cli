// Copyright 2026 calel33 and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Package scrape parses Ceiba (ceibabelize.com) server-rendered HTML into
// structured records. Every selector below was pinned against the live
// captures in discovery/ on 2026-10-16 (detail-100-acre.html,
// detail-3bed.html, detail-treehouse.html, our-listing.html,
// recent-sales.html, development-hopkins-grove.html); the test fixtures in
// testdata/ are small cuts of those captures. Elementor/JetEngine class
// names are the stable contract here — the theme template (post-1056 /
// elementor template 4008 / 4107 / 4614) renders the same markup per listing.
package scrape

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
)

// CardFacts is one listing card from the /our-listing/ stream
// (article.ae-post-item, AnyWhere Elementor loop template 4107).
type CardFacts struct {
	Slug          string   // from div[data-ae-url] or a[href=.../featured_listing/<slug>/]
	URL           string   // absolute listing URL
	PriceUSD      *float64 // div#price span.with-comma (USD)
	Acres         *float64 // "NN.NN ACRES /" overline in the first gc-listing-hidden-info section
	Bullets       []string // up to 3 feature bullets (widgets inside the gc-listing-description column)
	Spotlight     bool     // div.gc-listing-tag.ceiba-spotlight
	UnderContract bool     // div.gc-listing-tag.under-contract
	Sold          bool     // div.gc-listing-tag.sold
	// DistrictNames holds the location terms exactly as the card renders them
	// (jet-listing-dynamic-terms__link spans inside the info strip). The site
	// sorts terms alphabetically, so document order does NOT reveal which term
	// is the district and which is the town — callers resolve membership
	// against the synced district taxonomy instead.
	DistrictNames []string
}

// SimilarListing is one entry from the SIMILAR LISTING(S) grids on a detail page.
type SimilarListing struct {
	WPID     int    // from article id="post-<id>" when present, else 0
	Slug     string // from the featured_listing href, when present
	URL      string
	Title    string
	ImageURL string
}

// DetailFacts is everything the detail page template exposes. Nil pointers mean
// the page did not render a value (land listings print "0" in the beds/baths
// strip, which is treated as absent).
type DetailFacts struct {
	Title        string
	PriceUSD     *float64 // "$ <span class="with-comma">N</span> USD"
	ListingID    string   // "Listing ID:" label/value pair (e.g. 4HM1091)
	Acres        *float64 // ACRE/S strip value
	Beds         *float64 // BEDROOMS strip value (0 => nil)
	Baths        *float64 // BATHROOM strip value (0 => nil)
	LivingSpace  *float64 // LIVING SPACE strip value in sq ft
	TownName     string   // fa-map-marker-alt icon-list text
	DistrictName string   // fa-map-marked-alt icon-list text
	Waterfront   string   // "Waterfront:" label/value pair
	PropertyType string   // "Property Type:" label/value pair
	MapLat       *float64 // google maps embed q= param
	MapLng       *float64
	MapTitle     string   // iframe title attribute (also "lat, lng")
	GalleryURLs  []string // gc-listing-gallery data-thumbnails + ae-acf-gallery image links
	BrochureURLs []string // gc-brochure-1/2/3 image links
	TourURL      string   // youtube embed/watch link when the page carries one
	Similar      []SimilarListing
}

// RecentSale is a best-effort record from /recent-sales/. The live snapshot
// (2026-10-16) shows only intro copy + a form in the server HTML — the
// gallery is client-rendered — so callers must treat an empty result as
// "not extractable from the static page", not "no sales".
type RecentSale struct {
	Title    string
	Caption  string
	ImageURL string
	URL      string
}

// DevelopmentPage is the best-effort content of /development/<slug>/.
type DevelopmentPage struct {
	Title       string
	Description string
	Images      []string
}

var (
	numberRE     = regexp.MustCompile(`-?\d+(?:\.\d+)?`)
	imgExtRE     = regexp.MustCompile(`(?i)\.(jpe?g|png|webp|gif|avif)(\?|$)`)
	postIDRE     = regexp.MustCompile(`^post-(\d+)$`)
	slugFromHref = regexp.MustCompile(`/featured_listing/([^/?#]+)/?$`)
)

// ---------------- DOM helpers ----------------

func parseDoc(raw []byte) (*xhtml.Node, error) {
	return xhtml.Parse(bytes.NewReader(raw))
}

// walkDFS visits n and its descendants in document order; fn returns whether
// to descend into the node's children.
func walkDFS(n *xhtml.Node, fn func(*xhtml.Node) bool) {
	if n == nil {
		return
	}
	if !fn(n) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkDFS(c, fn)
	}
}

func attr(n *xhtml.Node, name string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val
		}
	}
	return ""
}

func classes(n *xhtml.Node) []string {
	raw := attr(n, "class")
	if raw == "" {
		return nil
	}
	return strings.Fields(raw)
}

func hasClass(n *xhtml.Node, class string) bool {
	for _, c := range classes(n) {
		if c == class {
			return true
		}
	}
	return false
}

func hasAnyClass(n *xhtml.Node, names ...string) bool {
	for _, name := range names {
		if hasClass(n, name) {
			return true
		}
	}
	return false
}

// textContent concatenates descendant text nodes, skipping script/style/template
// content and collapsing whitespace. The node itself is included.
func textContent(n *xhtml.Node) string {
	var b strings.Builder
	walkDFS(n, func(node *xhtml.Node) bool {
		switch node.Type {
		case xhtml.TextNode:
			b.WriteString(node.Data)
			b.WriteByte(' ')
		case xhtml.ElementNode:
			switch node.Data {
			case "script", "style", "template":
				return false
			}
		}
		return true
	})
	return strings.Join(strings.Fields(b.String()), " ")
}

func findFirst(n *xhtml.Node, pred func(*xhtml.Node) bool) *xhtml.Node {
	var found *xhtml.Node
	walkDFS(n, func(node *xhtml.Node) bool {
		if found != nil {
			return false
		}
		if pred(node) {
			found = node
			return false
		}
		return true
	})
	return found
}

func findAll(n *xhtml.Node, pred func(*xhtml.Node) bool) []*xhtml.Node {
	var out []*xhtml.Node
	walkDFS(n, func(node *xhtml.Node) bool {
		if node != n && pred(node) {
			out = append(out, node)
		}
		return true
	})
	return out
}

// ancestorChain returns n's ancestors nearest-first (excluding n).
func ancestorChain(n *xhtml.Node) []*xhtml.Node {
	var out []*xhtml.Node
	for p := n.Parent; p != nil; p = p.Parent {
		out = append(out, p)
	}
	return out
}

func hasAncestorWithClass(n *xhtml.Node, class string) bool {
	for _, p := range ancestorChain(n) {
		if hasClass(p, class) {
			return true
		}
	}
	return false
}

// parseNumber extracts the first number from s after dropping thousands
// separators ("1,947 Sq. Ft." -> 1947). Returns nil when no number exists.
func parseNumber(s string) *float64 {
	clean := strings.ReplaceAll(s, ",", "")
	m := numberRE.FindString(clean)
	if m == "" {
		return nil
	}
	v, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return nil
	}
	return &v
}

func absURL(raw, baseURL string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	if strings.HasPrefix(raw, "/") {
		if baseURL == "" {
			return "https://ceibabelize.com" + raw
		}
		return strings.TrimSuffix(baseURL, "/") + raw
	}
	return raw
}

// ---------------- listing cards (/our-listing/) ----------------

// ParseListingCards returns one CardFacts per article.ae-post-item in
// document order. Cards without a resolvable listing link are skipped.
func ParseListingCards(raw []byte) []CardFacts {
	doc, err := parseDoc(raw)
	if err != nil {
		return nil
	}
	var cards []CardFacts
	articles := findAll(doc, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && n.Data == "article" &&
			(hasClass(n, "ae-post-item") || hasClass(n, "ae-post-list-item"))
	})
	for _, article := range articles {
		if card, ok := parseCard(article); ok {
			cards = append(cards, card)
		}
	}
	return cards
}

func parseCard(article *xhtml.Node) (CardFacts, bool) {
	var card CardFacts

	// Link: the loop template carries div[data-ae-url]; fall back to the
	// first featured_listing anchor (the SEE MORE button or image link).
	if u := findFirst(article, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && attr(n, "data-ae-url") != ""
	}); u != nil {
		card.URL = strings.TrimSpace(attr(u, "data-ae-url"))
	}
	if card.URL == "" {
		if a := findFirst(article, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && n.Data == "a" &&
				strings.Contains(attr(n, "href"), "/featured_listing/") &&
				!strings.Contains(attr(n, "href"), "/page/")
		}); a != nil {
			card.URL = strings.TrimSpace(attr(a, "href"))
		}
	}
	if card.URL == "" {
		return card, false
	}
	if m := slugFromHref.FindStringSubmatch(card.URL); m != nil {
		card.Slug = m[1]
	}

	// Price: the card template gives the price widget id="price"; read the
	// USD span inside it. Fall back to any with-comma span whose widget text
	// carries the USD marker.
	if priceWidget := findFirst(article, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && attr(n, "id") == "price"
	}); priceWidget != nil {
		card.PriceUSD = firstCommaSpanValue(priceWidget)
	}
	if card.PriceUSD == nil {
		card.PriceUSD = firstCommaSpanValue(article)
	}

	// Acres: "NN.NN ACRES /" overline text.
	for _, n := range findAll(article, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && hasClass(n, "elementor-widget-container")
	}) {
		txt := textContent(n)
		if strings.Contains(strings.ToUpper(txt), "ACRES") {
			if v := parseNumber(txt); v != nil {
				card.Acres = v
			}
			break
		}
	}

	// Status tags: div.gc-listing-tag.{ceiba-spotlight|under-contract|sold}.
	for _, tag := range findAll(article, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && hasClass(n, "gc-listing-tag")
	}) {
		switch {
		case hasClass(tag, "ceiba-spotlight"):
			card.Spotlight = true
		case hasClass(tag, "under-contract"):
			card.UnderContract = true
		case hasClass(tag, "sold"):
			card.Sold = true
		}
	}

	// Bullets: text-editor widgets inside the gc-listing-description column.
	if desc := findFirst(article, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && hasClass(n, "gc-listing-description")
	}); desc != nil {
		for _, w := range findAll(desc, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && hasClass(n, "elementor-widget-text-editor")
		}) {
			if c := findFirst(w, func(n *xhtml.Node) bool {
				return n.Type == xhtml.ElementNode && hasClass(n, "elementor-widget-container")
			}); c != nil {
				if txt := strings.TrimSpace(textContent(c)); txt != "" {
					card.Bullets = append(card.Bullets, txt)
				}
			}
			if len(card.Bullets) >= 3 {
				break
			}
		}
	}

	// Location names: dynamic-terms spans inside the info strip. The tag
	// widget at the top of the card also uses the same span class, so scope
	// to the first gc-listing-hidden-info section that is NOT the tags wrap.
	if info := firstInfoStrip(article); info != nil {
		for _, span := range findAll(info, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && hasClass(n, "jet-listing-dynamic-terms__link")
		}) {
			if txt := strings.TrimSpace(textContent(span)); txt != "" {
				card.DistrictNames = append(card.DistrictNames, txt)
			}
		}
	}

	// Hero image extraction removed: CardFacts.ImageURL was parsed but never
	// consumed by production callers (the mirror stores no image column).
	return card, card.Slug != ""
}

// firstInfoStrip returns the first gc-listing-hidden-info section that holds
// the location terms (the tags widget is not inside a hidden-info section).
func firstInfoStrip(article *xhtml.Node) *xhtml.Node {
	for _, s := range findAll(article, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && hasClass(n, "gc-listing-hidden-info")
	}) {
		if findFirst(s, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && hasClass(n, "jet-listing-dynamic-terms__link")
		}) != nil {
			return s
		}
	}
	return nil
}

// firstCommaSpanValue reads the first span.with-comma text under root whose
// widget text carries the USD marker.
func firstCommaSpanValue(root *xhtml.Node) *float64 {
	var out *float64
	walkDFS(root, func(n *xhtml.Node) bool {
		if out != nil {
			return false
		}
		if n.Type != xhtml.ElementNode || n.Data != "span" || !hasClass(n, "with-comma") {
			return true
		}
		// The marker sits as sibling text within the widget container
		// ("$ <span>N</span> USD"); scan the nearest widget container.
		context := textContent(n.Parent)
		for _, p := range ancestorChain(n) {
			if hasClass(p, "elementor-widget-container") || attr(p, "id") == "price" {
				context = textContent(p)
				break
			}
		}
		if !strings.Contains(strings.ToUpper(context), "USD") {
			return true
		}
		out = parseNumber(textContent(n))
		return false
	})
	return out
}

// ---------------- listing detail (/featured_listing/<slug>/) ----------------

// ParseListingDetail extracts the detail template's structured fields. A page
// with none of the known markers yields a zero-value result, not an error:
// callers decide whether missing data is fatal.
func ParseListingDetail(raw []byte) (DetailFacts, error) {
	var facts DetailFacts
	doc, err := parseDoc(raw)
	if err != nil {
		return facts, fmt.Errorf("parsing detail HTML: %w", err)
	}

	// Title: first elementor heading h1 (the hero title). The duplicated
	// bottom card (h1.ae-element-post-title) is not an elementor heading.
	if h1 := findFirst(doc, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && n.Data == "h1" && hasClass(n, "elementor-heading-title")
	}); h1 != nil {
		facts.Title = strings.TrimSpace(textContent(h1))
	}

	facts.PriceUSD = firstCommaSpanValue(doc)

	parseExtraFields(doc, &facts)
	parseSpecStrip(doc, &facts)
	parseMap(doc, &facts)
	parseGallery(doc, &facts)
	parseTour(doc, &facts)
	facts.Similar = parseSimilarListings(doc)

	return facts, nil
}

// parseExtraFields reads the label/value text-editor pairs of the meta block
// ("Listing ID:", "Waterfront:", "Property Type:", "Neighbourhood:",
// "District:") plus the fa-map-marker / fa-map-marked icon lists.
func parseExtraFields(doc *xhtml.Node, facts *DetailFacts) {
	// Icon lists: <i class="fas fa-map-marker-alt"> marks the town,
	// fa-map-marked-alt the district; the value is the sibling
	// span.elementor-icon-list-text.
	for _, li := range findAll(doc, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && n.Data == "li" && hasClass(n, "elementor-icon-list-item")
	}) {
		text := findFirst(li, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && hasClass(n, "elementor-icon-list-text")
		})
		if text == nil {
			continue
		}
		icon := findFirst(li, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && n.Data == "i"
		})
		if icon == nil {
			continue
		}
		switch {
		case hasClass(icon, "fa-map-marker-alt") && facts.TownName == "":
			facts.TownName = strings.TrimSpace(textContent(text))
		case hasClass(icon, "fa-map-marked-alt") && facts.DistrictName == "":
			facts.DistrictName = strings.TrimSpace(textContent(text))
		}
	}

	// Label/value pairs: every text-editor container; a text ending in ":"
	// labels the next container's text.
	var containers []string
	containerNodes := findAll(doc, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && hasClass(n, "elementor-widget-text-editor")
	})
	for _, w := range containerNodes {
		c := findFirst(w, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && hasClass(n, "elementor-widget-container")
		})
		if c == nil {
			continue
		}
		containers = append(containers, strings.TrimSpace(textContent(c)))
	}
	for i, label := range containers {
		if !strings.HasSuffix(label, ":") || i+1 >= len(containers) {
			continue
		}
		value := containers[i+1]
		switch strings.TrimSuffix(label, ":") {
		case "Listing ID":
			if facts.ListingID == "" && looksLikeListingID(value) {
				facts.ListingID = value
			}
		case "Waterfront":
			if facts.Waterfront == "" {
				facts.Waterfront = value
			}
		case "Property Type":
			if facts.PropertyType == "" {
				facts.PropertyType = value
			}
		case "Neighbourhood":
			if facts.TownName == "" {
				facts.TownName = value
			}
		case "District":
			if facts.DistrictName == "" {
				facts.DistrictName = value
			}
		}
	}
}

var listingIDRE = regexp.MustCompile(`^[0-9A-Z]{2,4}[0-9]{3,5}$`)

// looksLikeListingID guards against swallowing an unrelated following value
// when the "Listing ID:" row is blank (the next row's label would follow).
func looksLikeListingID(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && !strings.HasSuffix(v, ":") && listingIDRE.MatchString(v)
}

// parseSpecStrip reads the value/label heading pairs: an h4 value heading
// immediately preceding a p label heading (BEDROOMS / BATHROOM / LIVING SPACE /
// ACRE/S). The template renders the strip twice (desktop + mobile); the first
// occurrence of each label wins. A literal value of 0 marks a blank land
// dashboard and is stored as nil for beds/baths.
func parseSpecStrip(doc *xhtml.Node, facts *DetailFacts) {
	headings := findAll(doc, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && hasClass(n, "elementor-heading-title")
	})
	// Value/label pairs share a heading node: an h4 value heading immediately
	// precedes the p label heading. prevIsValue tracks whether the previous
	// heading was such a value (an intervening label means "no value").
	var prevText string
	prevIsValue := false
	for _, h := range headings {
		text := strings.TrimSpace(textContent(h))
		if text == "" {
			continue
		}
		if strings.EqualFold(h.Data, "h4") {
			prevText = text
			prevIsValue = true
			continue
		}
		if strings.EqualFold(h.Data, "p") {
			value := prevText
			hasValue := prevIsValue
			prevIsValue = false
			if !hasValue {
				continue
			}
			switch strings.ToUpper(text) {
			case "BEDROOMS":
				if facts.Beds == nil {
					facts.Beds = positiveNumber(value)
				}
			case "BATHROOM":
				if facts.Baths == nil {
					facts.Baths = positiveNumber(value)
				}
			case "LIVING SPACE":
				if facts.LivingSpace == nil {
					facts.LivingSpace = parseNumber(value)
				}
			case "ACRE/S":
				if facts.Acres == nil {
					facts.Acres = parseNumber(value)
				}
			}
		}
	}
}

// positiveNumber returns nil for a missing or zero value (land listings print
// 0 in the beds/baths strip when the dashboard field is empty).
func positiveNumber(s string) *float64 {
	v := parseNumber(s)
	if v == nil || *v == 0 {
		return nil
	}
	return v
}

func parseMap(doc *xhtml.Node, facts *DetailFacts) {
	iframe := findFirst(doc, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && n.Data == "iframe" &&
			strings.Contains(attr(n, "src"), "maps/embed")
	})
	if iframe == nil {
		return
	}
	facts.MapTitle = strings.TrimSpace(attr(iframe, "title"))
	src := strings.ReplaceAll(attr(iframe, "src"), "&#038;", "&")
	src = strings.ReplaceAll(src, "&amp;", "&")
	u, err := url.Parse(src)
	if err != nil {
		return
	}
	q := u.Query().Get("q")
	if q == "" {
		q = facts.MapTitle
	}
	parts := strings.Split(q, ",")
	if len(parts) != 2 {
		return
	}
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lng, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil {
		return
	}
	facts.MapLat = &lat
	facts.MapLng = &lng
}

// parseGallery collects absolute image URLs from:
//   - every .e-gallery-image node (the listing gallery widget's source list
//     carries the image on data-thumbnail; the theme's lightbox JS reads the
//     same attribute),
//   - elementor-widget-ae-acf-gallery blocks that are NOT gc-brochure-* (their
//     items are <a href="..uploads/*.jpg"> wrappers).
//
// Brochure images come from the gc-brochure-1/2/3 ae-acf-gallery widgets,
// which are image galleries (no PDFs on any sampled page).
func parseGallery(doc *xhtml.Node, facts *DetailFacts) {
	// Gallery and brochure widgets reuse the same image files (the gc-brochure
	// sets are curated subsets of the listing photography), so dedupe each
	// list independently: a URL may legitimately appear in both gallery_urls
	// and brochure_urls.
	seenGallery := map[string]bool{}
	seenBrochure := map[string]bool{}
	add := func(seen map[string]bool, dst *[]string, u string) {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
		*dst = append(*dst, u)
	}

	for _, img := range findAll(doc, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && hasClass(n, "e-gallery-image") && attr(n, "data-thumbnail") != ""
	}) {
		add(seenGallery, &facts.GalleryURLs, absURL(attr(img, "data-thumbnail"), ""))
	}

	acfBlocks := findAll(doc, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && hasClass(n, "elementor-widget-ae-acf-gallery")
	})
	for _, block := range acfBlocks {
		isBrochure := hasAnyClass(block, "gc-brochure-1", "gc-brochure-2", "gc-brochure-3")
		for _, a := range findAll(block, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && n.Data == "a" && attr(n, "href") != ""
		}) {
			href := attr(a, "href")
			if !strings.Contains(href, "/wp-content/uploads/") || !imgExtRE.MatchString(href) {
				continue
			}
			if isBrochure {
				add(seenBrochure, &facts.BrochureURLs, absURL(href, ""))
			} else {
				add(seenGallery, &facts.GalleryURLs, absURL(href, ""))
			}
		}
	}

	// Brochure blocks also carry figure>img src variants on some pages.
	for _, block := range acfBlocks {
		if !hasAnyClass(block, "gc-brochure-1", "gc-brochure-2", "gc-brochure-3") {
			continue
		}
		for _, img := range findAll(block, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && n.Data == "img" && attr(n, "src") != ""
		}) {
			src := attr(img, "src")
			if strings.Contains(src, "/wp-content/uploads/") {
				add(seenBrochure, &facts.BrochureURLs, absURL(src, ""))
			}
		}
	}
}

// parseTour looks for an embedded video inside the page content. Social icon
// links (youtube.com/@handle, /c/, /channel/) are site chrome, not tours, so
// only embed/watch URLs count.
func parseTour(doc *xhtml.Node, facts *DetailFacts) {
	candidate := findFirst(doc, func(n *xhtml.Node) bool {
		if n.Type != xhtml.ElementNode {
			return false
		}
		src := attr(n, "src")
		href := attr(n, "href")
		for _, u := range []string{src, href} {
			if u == "" {
				continue
			}
			low := strings.ToLower(u)
			if strings.Contains(low, "youtube.com/embed/") ||
				strings.Contains(low, "youtube.com/watch") ||
				strings.Contains(low, "youtu.be/") ||
				strings.Contains(low, "player.vimeo.com/video/") ||
				strings.Contains(low, "vimeo.com/") {
				return true
			}
		}
		return false
	})
	if candidate == nil {
		return
	}
	if src := attr(candidate, "src"); src != "" {
		facts.TourURL = absURL(src, "")
		return
	}
	facts.TourURL = absURL(attr(candidate, "href"), "")
}

// parseSimilarListings reads the "SIMILAR LISTING(S)" grids. The theme renders
// two widgets: a posts grid of CTA cards keyed only by post id, and an
// ae-post-blocks grid whose figure links carry the slug. Pass 1 collects the
// linked cards (document order after the first SIMILAR LISTING heading);
// pass 2 falls back to the id-keyed CTA cards. First 5, deduped.
func parseSimilarListings(doc *xhtml.Node) []SimilarListing {
	headingOrder := -1
	order := 0
	type articleEntry struct {
		node  *xhtml.Node
		order int
	}
	var articles []articleEntry
	walkDFS(doc, func(n *xhtml.Node) bool {
		if n.Type == xhtml.ElementNode && n.Data == "article" {
			articles = append(articles, articleEntry{node: n, order: order})
		}
		if headingOrder < 0 && n.Type == xhtml.ElementNode {
			switch strings.ToUpper(n.Data) {
			case "H1", "H2", "H3", "H4":
				if strings.Contains(strings.ToUpper(textContent(n)), "SIMILAR LISTING") {
					headingOrder = order
				}
			}
		}
		order++
		return true
	})

	var linked []SimilarListing
	seenSlug := map[string]bool{}
	for _, entry := range articles {
		if headingOrder >= 0 && entry.order < headingOrder {
			continue
		}
		a := findFirst(entry.node, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && n.Data == "a" &&
				strings.Contains(attr(n, "href"), "/featured_listing/") &&
				!strings.Contains(attr(n, "href"), "/page/")
		})
		if a == nil {
			continue
		}
		href := strings.TrimSpace(attr(a, "href"))
		m := slugFromHref.FindStringSubmatch(href)
		if m == nil {
			continue
		}
		slug := m[1]
		if seenSlug[slug] {
			continue
		}
		seenSlug[slug] = true
		item := SimilarListing{Slug: slug, URL: href}
		if id := attr(entry.node, "id"); id != "" {
			if mm := postIDRE.FindStringSubmatch(id); mm != nil {
				item.WPID, _ = strconv.Atoi(mm[1])
			}
		}
		if cap := findFirst(entry.node, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && hasClass(n, "widget-image-caption")
		}); cap != nil {
			item.Title = strings.TrimSpace(textContent(cap))
		}
		if img := findFirst(entry.node, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && n.Data == "img" && attr(n, "src") != ""
		}); img != nil {
			item.ImageURL = absURL(attr(img, "src"), "")
		}
		linked = append(linked, item)
		if len(linked) >= 5 {
			break
		}
	}
	if len(linked) > 0 {
		return linked
	}

	var fallback []SimilarListing
	seenID := map[int]bool{}
	for _, entry := range articles {
		if headingOrder >= 0 && entry.order < headingOrder {
			continue
		}
		idAttr := attr(entry.node, "id")
		m := postIDRE.FindStringSubmatch(idAttr)
		if m == nil {
			continue
		}
		id, _ := strconv.Atoi(m[1])
		if seenID[id] {
			continue
		}
		title := findFirst(entry.node, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && hasClass(n, "elementor-cta__title")
		})
		if title == nil {
			continue
		}
		seenID[id] = true
		item := SimilarListing{WPID: id, Title: strings.TrimSpace(textContent(title))}
		if bg := findFirst(entry.node, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && strings.Contains(attr(n, "style"), "background-image")
		}); bg != nil {
			if u := extractCSSURL(attr(bg, "style")); u != "" {
				item.ImageURL = absURL(u, "")
			}
		}
		fallback = append(fallback, item)
		if len(fallback) >= 5 {
			break
		}
	}
	return fallback
}

var cssURLRE = regexp.MustCompile(`url\(\s*([^)]+?)\s*\)`)

func extractCSSURL(style string) string {
	if m := cssURLRE.FindStringSubmatch(style); m != nil {
		return strings.Trim(strings.TrimSpace(m[1]), `"'`)
	}
	return ""
}

// ---------------- /recent-sales/ ----------------

var excludedCaptionImages = []string{"Ceiba-Logo", "favicon", "Footer", "CTA", "cropped-"}

func excludedImage(u string) bool {
	low := strings.ToLower(u)
	for _, marker := range excludedCaptionImages {
		if strings.Contains(low, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

// ParseRecentSales returns best-effort records for the /recent-sales/ gallery.
// The static page served on 2026-10-16 has no server-rendered gallery (the
// grid is client-rendered), so the live default is an empty slice; any gallery
// markup that does appear is extracted: elementor gallery data-thumbnails and
// figure.wp-caption image/figcaption pairs holding uploads images.
func ParseRecentSales(raw []byte) []RecentSale {
	doc, err := parseDoc(raw)
	if err != nil {
		return nil
	}
	var out []RecentSale
	seen := map[string]bool{}
	add := func(r RecentSale) {
		if r.ImageURL == "" || seen[r.ImageURL] {
			return
		}
		seen[r.ImageURL] = true
		out = append(out, r)
	}

	for _, img := range findAll(doc, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && attr(n, "data-thumbnail") != ""
	}) {
		u := absURL(attr(img, "data-thumbnail"), "")
		if u == "" || excludedImage(u) {
			continue
		}
		r := RecentSale{ImageURL: u, Title: strings.TrimSpace(attr(img, "aria-label"))}
		for _, p := range ancestorChain(img) {
			if p.Type != xhtml.ElementNode {
				continue
			}
			if p.Data == "figure" && r.Caption == "" {
				if c := findFirst(p, func(n *xhtml.Node) bool {
					return n.Type == xhtml.ElementNode && hasClass(n, "wp-caption-text")
				}); c != nil {
					r.Caption = strings.TrimSpace(textContent(c))
				}
			}
			if p.Data == "a" && r.URL == "" {
				if href := attr(p, "href"); href != "" {
					r.URL = absURL(href, "")
				}
			}
		}
		add(r)
	}

	for _, fig := range findAll(doc, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && n.Data == "figure" && hasClass(n, "wp-caption")
	}) {
		img := findFirst(fig, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && n.Data == "img" && attr(n, "src") != ""
		})
		if img == nil {
			continue
		}
		u := absURL(attr(img, "src"), "")
		if !strings.Contains(u, "/wp-content/uploads/") || excludedImage(u) {
			continue
		}
		r := RecentSale{ImageURL: u}
		if c := findFirst(fig, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && hasClass(n, "wp-caption-text")
		}); c != nil {
			r.Caption = strings.TrimSpace(textContent(c))
		}
		if a := findFirst(fig, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && n.Data == "a" && attr(n, "href") != ""
		}); a != nil {
			r.URL = absURL(attr(a, "href"), "")
		}
		add(r)
	}
	return out
}

// ---------------- /development/<slug>/ ----------------

// ParseDevelopmentPage extracts title, description paragraphs and gallery
// images from a development detail page, best-effort. Navigation and footer
// text is excluded via ancestry checks.
func ParseDevelopmentPage(raw []byte) DevelopmentPage {
	page := DevelopmentPage{Images: make([]string, 0)}
	doc, err := parseDoc(raw)
	if err != nil {
		return page
	}

	if h1 := findFirst(doc, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && n.Data == "h1" &&
			(hasClass(n, "entry-title") || hasClass(n, "elementor-heading-title"))
	}); h1 != nil {
		page.Title = strings.TrimSpace(textContent(h1))
	}

	seen := map[string]bool{}
	addImage := func(raw string) {
		u := absURL(raw, "")
		if u == "" || excludedImage(u) || seen[u] {
			return
		}
		if !strings.Contains(u, "/wp-content/uploads/") {
			return
		}
		seen[u] = true
		page.Images = append(page.Images, u)
	}
	for _, img := range findAll(doc, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && attr(n, "data-thumbnail") != ""
	}) {
		addImage(attr(img, "data-thumbnail"))
	}
	// Development pages render their imagery as section/column background
	// attributes (DCE + theme lazy-bg) rather than gallery nodes — collect
	// those uploads URLs too.
	for _, el := range findAll(doc, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode &&
			(attr(n, "data-dce-background-image-url") != "" ||
				attr(n, "data-dce-background-overlay-image-url") != "" ||
				attr(n, "data-bg") != "")
	}) {
		for _, name := range []string{"data-dce-background-image-url", "data-dce-background-overlay-image-url", "data-bg"} {
			if v := attr(el, name); v != "" {
				addImage(v)
			}
		}
	}

	var paras []string
	for _, p := range findAll(doc, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && n.Data == "p"
	}) {
		if inChrome(p) {
			continue
		}
		txt := strings.TrimSpace(textContent(p))
		if len(txt) < 40 {
			continue
		}
		paras = append(paras, txt)
		if len(paras) >= 8 {
			break
		}
	}
	page.Description = strings.Join(paras, "\n\n")
	return page
}

func inChrome(n *xhtml.Node) bool {
	for _, p := range ancestorChain(n) {
		if p.Type != xhtml.ElementNode {
			continue
		}
		switch strings.ToLower(p.Data) {
		case "nav", "footer", "header":
			return true
		}
		for _, c := range classes(p) {
			lc := strings.ToLower(c)
			if strings.Contains(lc, "footer") || strings.Contains(lc, "menu") ||
				strings.Contains(lc, "breadcrumb") || strings.Contains(lc, "copyright") {
				return true
			}
		}
	}
	return false
}
