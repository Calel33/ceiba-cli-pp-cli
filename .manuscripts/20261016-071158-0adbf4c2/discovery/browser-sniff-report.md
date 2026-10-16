# Browser-Sniff Discovery Report — ceibabelize.com

> Run: `20261016-071158-0adbf4c2` · Phase 1.7 (browser-sniff-gate) · Decision: `pre-approved` (phase-0 website choice; `BROWSER_SNIFF_TARGET_URL=https://ceibabelize.com/our-listing/`)
> Backend: **agent-browser 0.38.1** (HAR with response bodies). browser-use v0.13.11 was present but ships the daemon/Python CLI surface, not the `open/eval` capture CLI this flow expects — agent-browser provides equivalent (stronger: `--content text`) HAR artifacts.
> Artifacts: `browser-sniff-capture.har` (merged, 452 entries, 14.8 MB), `browser-sniff-capture-listing.har` (256 reqs), `browser-sniff-detail.har` (196 reqs), `sniff-our-listing.png`, `sniff-detail.png`, `sniff-xhr-requests.json`, `traffic-analysis.json`, spec `../research/ceiba-cli-browser-sniff-spec.yaml` + `-samples/` (8 endpoints).

## 1. User Goal Flow

- **Goal:** "Fetch and filter data" — read-only listing discovery (search → detail), per the brief's #1 workflow (chat Q&A loop).
- **Steps completed:**
  1. HAR recording started → opened `/our-listing/` (listing page load captured; SSR HTML).
  2. Page-context WP-REST fetches fired from the loaded page (same-origin `fetch`), all captured with bodies:
     - `GET /wp-json/wp/v2/featured_listing?per_page=100&_fields=…` → 84 items (verified in-page)
     - `GET /wp-json/wp/v2/district?per_page=100&_fields=…` → 100 items (of 106)
     - `GET /wp-json/wp/v2/featured_tags?per_page=100` → `ceiba-spotlight=31,sold=30,under-contract=32` (verified in-page)
     - `GET /wp-json/wp/v2/development?per_page=10` → 6 items (verified in-page)
     - `GET /wp-json/wp/v2/media/20403` → source_url returned (verified in-page)
  3. Scrolled listing page (lazy content) + screenshot.
  4. Card-link click was **intercepted by the card's overlay element** (`.elementor-widget-container` inside `#gc-list-url`) — recorded as a finding: card links are click-blocked by design (link lives on `data-ae-url`/overlay), so automation must navigate card URLs directly.
  5. Opened detail page `/featured_listing/20-acre-estate-parcel-w-waterfront-near-hopkins/` directly (HAR session 2), waited, scrolled ×3, verified in-page: **24 `.ae-grid-item` gallery entries, 62 images**, screenshot.
- **Secondary flows:** not attempted in-browser (filter AJAX on `admin-ajax.php` requires JetSmartFilters interactions). These surfaces were already captured via direct HTTP in `../discovery/` during Phase 1 research (`filter-v2.html`, `our-developments.html`, `recent-sales.html`, `pp100.json`, `all-content.json`).
- **Coverage:** 5/5 planned browser steps executed (1 step substituted with direct navigation after interception).

## 2. Pages & Interactions

| # | URL | Interaction | Purpose |
|---|-----|-------------|---------|
| 1 | `https://ceibabelize.com/our-listing/` | open (page load) | Listing card stream + SSR/JSON-LD capture |
| 2 | (same, page context) | 5 × `fetch()` to `wp-json` (1.5 s pacing) | Persist real REST request/response bodies into HAR |
| 3 | (same) | scroll down; screenshot | Lazy-content evidence |
| 4 | (same) | click card link → **blocked by overlay** | Recorded as interaction finding (see §1.4) |
| 5 | `…/featured_listing/20-acre-estate-parcel-w-waterfront-near-hopkins/` | open, 3× scroll, screenshot | Detail page: price/specs/gallery/map surface |
| 6 | — | `network har stop` | Export both HARs |

## 3. Browser-Sniff Configuration

- Backend: agent-browser 0.38.1 (`network har start/stop --content text`; headless).
- Pacing: 1.0–1.5 s between agent-initiated fetches; no 429s observed.
- Proxy-envelope detection: **not detected** — traffic is direct multi-path REST (`/wp-json/wp/v2/*`) + SSR HTML. No single proxy route.
- Reachability probe (Phase 1.7 mandate): `probe-reachability` → `standard_http`, confidence 0.95, no browser capture or clearance cookie required. Plain-UA ModSecurity `406` was UA-sensitivity, not a gate; all surfaces replay over plain HTTP with browser-like headers (proven by direct captures in `../discovery/`).

## 4. Endpoints Discovered (unique, target-site)

| Method | Path | Status | Content-Type | Auth |
|--------|------|--------|--------------|------|
| GET | `/our-listing/` | 200 | text/html | public |
| GET | `/featured_listing/20-acre-estate-parcel-w-waterfront-near-hopkins/` | 200 | text/html | public |
| GET | `/wp-json/wp/v2/featured_listing` | 200 | application/json | public |
| GET | `/wp-json/wp/v2/district` | 200 | application/json | public |
| GET | `/wp-json/wp/v2/featured_tags` | 200 | application/json | public |
| GET | `/wp-json/wp/v2/development` | 200 | application/json | public |
| GET | `/wp-json/wp/v2/media/{media_id}` | 200 | application/json | public |

Non-target noise also present in the HAR (excluded from the spec): `maps.googleapis.com/maps/api/*`, `unpkg.com/*` bundles, `googleads.g.doubleclick.net/pagead/*`, analytics (GTM), font files. The auto-generated spec captured some of this noise (`maps`, `pagead` resources, an inferred `api_key` auth from the Google Maps embed, and `base_url: maps.googleapis.com`) — **spec curation required before generation** (documented in the run's absorb manifest; do not feed the raw sniffed spec as-is).

## 5. Traffic Analysis (from `traffic-analysis.json`)

- Protocols: `ssr_embedded_data` (0.85 — `/our-listing/` ships `application/ld+json`), `rest_json` (0.75 — includes all 5 wp-json endpoints), `html_scrape` (0.55 — detail page).
- Auth signals: none for target endpoints (the only "auth" candidate was the Google Maps embed key; `per_page` was correctly rejected as an auth candidate).
- Protection signals: none observed in capture (no challenge/interstitial; ModSecurity UA gate only).
- Generation hints: treat as standard HTTP; HTML detail pages provide the spec-layer fields REST omits.
- Warnings (all noise, not blockers): `weak_schema_evidence` for fonts/JS/favicon/ad beacons; one `empty_payload` (Google Maps embed). **No `empty_response_shapes` warning — JSON bodies are intact.** `endpoint_clusters`: 8 clusters with small/medium/large size classes.
- Reachability field in analysis says `browser_required`: this reflects that the *sniffed spec's* dominant surfaces include HTML pages and 3rd-party JS; it does **not** contradict the Phase 1.7 probe (`standard_http`). Direct replays of every target endpoint over plain HTTP (throughout `../discovery/`) settle the runtime as standard HTTP.
- Candidate commands: listings list/get, district terms, featured_tags terms, developments list, media get, listing-page HTML extract, detail-page HTML extract.

## 6. Coverage Analysis

- Exercised: collections (listings, developments, terms), single media, listing page (SSR), one detail page (HTML spec layer).
- Matched against the brief: all 7 core surfaces captured. Pages not re-captured in-browser (already captured via direct HTTP in Phase 1): `/recent-sales/`, `/our-developments/`, `/filter-v2/`, `/latest-listings/`, additional detail pages (`3bed`, `treehouse`, `100-acre`), full 84-item content dump, sitemaps.
- Likely missed in-browser: JetSmartFilters `admin-ajax.php` filter calls (interaction-limited — known URL family, not needed for the mirror design; search/filtering is served by the local mirror + REST term-id queries).

## 7. Response Samples

- `../research/ceiba-cli-browser-sniff-spec-samples/` — 8 endpoints sampled from the HAR (JSON: full bodies; HTML: truncated).
- Additional direct-response samples in `../discovery/`: `pp100.json` (84-listing sync payload), `featured-item-full.json`, `district-terms(-parent).json`, `featured_tags-terms.json`, `development-collection-full.json`, `media-parent.json`, `all-content.json`, 4 × detail-page HTML, listing-page HTML, filter/developments/recent-sales HTML.

## 8. Rate Limiting Events

- None. Zero 429s across 452 recorded entries and all direct probes. Effective agent-initiated rate ≈ 0.7–1 req/s.

## 9. Authentication Context

- No authenticated session used or needed (all surfaces public; Phase 1.6 recorded the `no-auth-required` skip). No cookies or credential headers were written to any artifact; the HAR contains no auth headers for target endpoints (Google Maps embed key is a public in-page embed key; excluded from the spec, not a credential for this CLI).

## 10. Bundle Extraction

- Skipped intentionally: WordPress/Elementor site is SSR + per-plugin JS; no SPA bundle defines the API surface (REST is documented by `/wp-json/` route index, captured live). Endpoint discovery was exhaustive for the target surfaces.

---

### Fidelity notes for the next phases

1. The raw sniffed spec (`ceiba-cli-browser-sniff-spec.yaml`) needs curation before generation: drop `maps`/`pagead` resources, set `base_url: https://ceibabelize.com`, remove the inferred `api_key` auth (site needs none), keep the 5 wp-json endpoints + 2 HTML extract pages.
2. The HAR is the replayability proof for the wp-json surface; direct replays in `../discovery/` are the replayability proof for everything else.
3. REA verification of this HAR: see run receipts; `inspect_web_network_capture` evidence recorded (artifact SHA-256 + record counts).
