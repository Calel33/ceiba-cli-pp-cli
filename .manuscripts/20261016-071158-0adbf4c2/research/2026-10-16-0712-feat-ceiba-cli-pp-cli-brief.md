# Ceiba (ceibabelize.com) CLI Brief

> Run: `20261016-071158-0adbf4c2` · Scope: `ceiba-cli-7f836d03` · Phase: 04-research-brief
> Inputs: `spec.md` (locked product scope), issues 01–07, live-site evidence in `../discovery/`.
> Route: `BROWSER_SNIFF_TARGET_URL=https://ceibabelize.com/our-listing/` (spec-approved website route).

## API Identity

- **Domain:** Belize real estate brokerage (Ceiba Realty & Development), `ceibabelize.com`.
- **Stack evidence (from HTML + REST):** WordPress 7.0.7, Elementor + Elementor Pro, JetEngine (Crocoblock), JetSmartFilters 3.8.11.1, ACF (image galleries), AnyWhere Elementor (card loops), PowerPack (photo grids), Rank Math SEO, CPT UI, ele-custom-skin, Simple Custom CSS/JS.
- **Users:** international buyers/investors/sellers; the *future consumer is an AI chatbot* — the CLI+MCP is the structured layer (spec.md).
- **Data profile:** small, high-value catalog — **84 active+archived listings** (`featured_listing` CPT; `X-WP-Total: 84`), **6 developments** (`development` CPT), 1 recent-sales gallery page, 100+ predefined location terms.

## Reachability Risk

- **Low with browser headers.** Plain curl UA gets ModSecurity `406` on `/robots.txt` and `/wp-json/`; a browser UA (Chrome 126) gets `200` everywhere probed. The generated client must send browser-like headers (UA at minimum).
- No CAPTCHA/login/Clearance gate observed on listing surfaces; `/our-listing/` = `200`, 1.13 MB.
- `X-WP-Total`/`X-WP-TotalPages` (`Access-Control-Expose-Headers` includes `Jet-Query-Total`) — cheap pagination.
- **No public wrappers/SDKs found** (registry/npm/PyPI clean; no `ceiba`/`belize` matches in the Press public library). Sibling artifact on this machine: `terracore-pp-cli` (same pattern class — WP REST list + HTML scrape for specs).
- Rate limiting unknown (no `Retry-After` observed; single-digit requests per full sync).

## Source Map (verified live 2026-10-16)

### A. REST base layer — `https://ceibabelize.com/wp-json/wp/v2/...` (UA required)

| Surface | Status | Key facts |
|---|---|---|
| `/wp/v2/featured_listing?per_page=100` | 200 | **All 84 in one page** (`X-WP-TotalPages: 1`). Supported: `search=` (works, e.g. river→24), `orderby=modified`, `_fields=`, **tax filters by TERM ID ONLY** (`?district=50` 200; `?district=cayo` → 400 `rest_invalid_param`) |
| `/wp/v2/featured_listing/<id>` | 200 | fields: `id, date, modified, slug, status, link, title.rendered, content.rendered` (description ~1.3 KB, all 84 have it), `featured_media`, `template`, `list_category[], featured_tags[], district[]` (ids), `class_list[]`, **`acf` = EMPTY** |
| `/wp/v2/development?per_page=10` | 200 | 6 items; same field shape; `content` mostly empty |
| `/wp/v2/media?parent=<id>` | 200 | Attachments *uploaded to* the listing (e.g. 2 for #20400) — **NOT the full gallery** |
| `/wp/v2/district?per_page=100` | 200 | 106 terms, hierarchical: parents 47–52 = districts (Corozal, Toledo, Belize, Cayo, Stann Creek, …); children = towns/villages/rivers (San Ignacio Town=147, Hopkins, Placencia…) |
| `/wp/v2/list_category` | 200 | 8 mostly-empty legacy terms (cayo, corozal-district, featured, for-rent, homesite, sale, sold, unsold) — **not** the lifestyle categories |
| `/wp/v2/featured_tags` | 200 | **Status taxonomy**: `ceiba-spotlight`=31 (64 listings), `sold`=30 (5), `under-contract`=32 (2) |
| `/wp/v2/taxonomies` | 200 | Confirms exactly 3 taxonomies on `featured_listing` |

**Status rule mapping (spec):** default exclude `featured_tags` id 30 (sold); under-contract (32) included with badge; spotlight (31) = flag. Mirror-side filtering is authoritative; live REST query can use `featured_tags_exclude=30`. Term IDs must be resolved from slugs at sync time (never hardcode long-term).

**BZD display rule:** raw HTML renders BZD span text identical to USD; a site script computes display BZD = `parseInt(usd) * 2` (`.bzd-price`, jQuery, ×2). Return `price_bzd = price_usd × 2` (displayed value), documented; raw value retained in scrape.

### B. HTML spec layer — per-listing detail page `/featured_listing/<slug>/` (UA required)

Exact patterns verified on `detail-100-acre.html`, `detail-3bed.html`, `detail-treehouse.html`:

- **Price USD:** `$ <span class="with-comma">200000</span> USD` → span text (strip commas). Cards also use `id="price"` + `.with-comma`.
- **BZD:** `<span class="with-comma bzd-price">…</span>` (+ ×2 rule).
- **Acres:** card/detail text `20.16 ACRES /` (overline pattern in card info strip).
- **Beds/Baths/Living space:** spec strip = value `<h4>` immediately preceding label `<p>BEDROOMS</p>` / `BATHROOM` / living space (`1,947 sq. ft`). Land listings omit values (dashboard blank) — treat as optional.
- **Listing ID:** plain text `Listing ID: 4HM1091` — **partial coverage** (present on 100-acre; absent on 3bed/treehouse). Extract when present.
- **Map:** `https://www.google.com/maps/embed/v1/place?key=…&q=17.356395,-88.683128&zoom=12` → parse `q` → `map_lat/map_lng` (iframe `title` also = "lat, lng"). Reliable on all 4 sampled pages.
- **Gallery + brochures:** `elementor-widget-ae-acf-gallery` blocks (`gc-brochure-1/2/3` wrappers). Items = `<a href="…/wp-content/uploads/….jpg">` around `<img …>` (full-size `src` + srcset); NO PDFs found on sampled pages. **Brochure model = image gallery(s)**; verify PDF variant during capture.
- **Tour video:** NOT in REST `content` (0/84 mentions); not an iframe on sampled pages (only GTM + Maps embeds). Template-level element; **coverage unknown → verify during capture** on a listing known to have a video (spec asserts it exists).
- **Feature bullets (3):** card info strip (`gc-listing-hidden-info`) shows e.g. `In town Access / Panoramic Views / Amenities Available`; the same strip pattern appears in cards on `/our-listing/`. Likely extractable for all 84 from the listing page card stream + detail page.
- **Similar listings:** detail page section `SIMILAR LISTING` → `elementor-widget-posts` 5-card grid; links are `featured_listing` URLs (parse first 5; note the widget is paginated — take first page only).
- **Property-type/waterfront vocabulary** (from the Property Preferences form script, exact stored values — site-side spelling preserved):
  - `property` field: lifestyle categories per site copy: riverfront, island, tropical homes, investment, farms/ranches, acreage, waterfront, homesites.
  - `propertysub`: `Farm/ranch`, `Ranch`, `Ranchette`→ value `"rachette"` (site typo, preserve), `Homestead`.
  - `waterfront`: `Riverfront`→`rf`, `Oceanfront`→`of`, `Beach`→`bch`, `Creekfront`→`cf`, `Lagoonfront`→`lf`.
  - setting: `Gated Community`→`gc`, `Subdivision`→`sub`, `Town Lots`→`tl`; home type: `Apartment`→`Aprt`, `Single Family Home`→`sfh`, `Townhouses`→`th`, `Villa`→`vil`.
  - **Mapping from listing pages → these fields is unverified** → resolve during capture/generation (detail-page chips/spec strip).
- **Site's own acre filter** = 8 coarse buckets (Over/Under 10/25/50/100). Spec's numeric `--min-acres/--max-acres` exceeds site parity (a transcendence feature).
- **`/our-listing/`** = card stream (AnyWhere Elementor `ae-post-item` + `data-ae-url`, bg image `data-bg`); 252 `featured_listing` href occurrences ≈ all cards in page. No map/price server-side JSON — scrape the stream.
- **`development` detail pages:** `/development/<slug>/` (e.g. hopkins-grove 206 KB, 25 images) — description + gallery in HTML; REST gives ids/titles/links only.
- **`/recent-sales/`** = Elementor/Jet gallery page (24 jet-listing, 33 pp-grid, 109 gc-listing classes); images + captions; no `featured_listing` links → treat as scraped comps gallery.
- **`/our-developments/`** = JS-rendered (admin-ajax); do not scrape — use `development` CPT REST + per-dev HTML.

## Top Workflows

1. **Chat Q&A loop:** `search` (compact JSON) → `show <id|slug>` (complete payload, no re-fetch).
2. **Budget/region browsing:** numeric price + acre ranges, district/town (one hierarchical taxonomy), type/waterfront facets, status defaults (sold hidden, under-contract badge, spotlight flag).
3. **Investor/agent:** `developments list|show` (6 pre-built communities); `recent-sales list` for comps.
4. **Operator:** `sync` (idempotent mirror rebuild; REST base + term resolution + per-listing HTML spec scrape) + `sync --daily`; freshness metadata everywhere.

## Table Stakes (rivals: remaxbelizerealty.com, 1stchoicebelize.com, remax-centralamerica)

- Location + type + price/beds/baths filters, keyword search, map presence, multi-image galleries, comparable sales. Rivals are human-web; none offer agent-native structured output. Ceiba parity must cover: its district/town taxonomy, its status badges, its preference vocabularies, and its acre buckets — plus the spec's numeric ranges.

## Data Layer

- **Primary entities:** `listing` (featured_listing), `development`, `recent_sale` (scraped), `term` (district hierarchy, featured_tags, list_category), `media` (gallery URLs).
- **Sync cursor:** `modified` (`orderby=modified&order=desc`); full snapshot = 1 request for base (per_page=100) + 84 detail scrapes + terms + developments + recent-sales page.
- **FTS:** SQLite FTS over title + content + bullets (keyword flavor of `search`).
- **Freshness:** `synced_at` + `source_url` on every payload; `show` revalidates live when mirror older than threshold (spec).

## Codebase Intelligence

- **Sibling pattern (`terracore-pp-cli`, local):** commands `wp-json list-properties/get-property [--specs]` (REST list + HTML scrape for price/beds/baths), `sync`, `workflow archive/status`, `doctor`, `which "<capability>"`, `profile`, `feedback`, `--agent` (= `--json --compact --no-input --no-color --yes`), `--select` field projection, response envelope `{meta:{source: live|local, synced_at, reason}, results}`. **Absorb:** envelope, `--agent`, `--select`, `which`, read-only guardrails.
- Not applicable: DeepWiki (no public source repo).

## User Vision

- Build best-suited CLI/MCP from `spec.md` + issues 01–07; use **reverse-engineer-anything** for evidence capture; read-only; hide-sold-by-default; compound MCP 1:1 with CLI; name **ceiba-cli** (artifact convention → binary `ceiba-cli-pp-cli`, MCP `ceiba-cli-pp-mcp`, library dir `ceiba-cli`).
- Issues 01–07 define the delivery sequence: discovery evidence → search → detail → developments/recent-sales → MCP parity → fixture verification → live dogfood.

## Product Thesis

- **Name:** ceiba-cli (project), `ceiba-cli-pp-cli` (binary).
- **Why it should exist:** Ceiba's 84-property inventory is locked inside JS-heavy Elementor pages; nothing agent-native or token-efficient exists for Belize real estate. One `search`/`show` pair replaces manual browsing for chat, comparison, and investment screening.

## Build Priorities

1. `sync` — REST base (84 in 1 page, `_fields`, term-id resolution) + per-listing HTML spec scrape (price/base display rules, spec strip, map, gallery, bullets, listing id when present, similar links) into SQLite with `synced_at`.
2. `search` — mirror-backed: query, min/max price, min/max acres, district + town (hierarchy aware), listing-id, type, waterfront, status (sold opt-in, under-contract visible, spotlight flag), beds/baths, sort, limit; compact JSON; FTS on title/content/bullets.
3. `show <id|slug>` — complete payload incl. media URLs (absolute), tour when present, map coords, brochure images, 3 bullets, similar (with detail), `synced_at` + `source_url`; live revalidate on stale.
4. `developments list|show`, `recent-sales list` — REST + HTML scrape; JSON-first with table fallback.
5. MCP — compound tools `search/show/sync/developments/recent-sales` mirroring CLI in/out; no SQL tool; structured errors.
6. Fixture-based black-box verification (recorded HTML + deterministic mirror) + Press shipcheck/verify/scorecard.
7. Live dogfood matrix (issue 07) incl. `show 4HM<id>`-class checks, link resolvability, drift notes.

## Open Items For Capture/Generation (do not guess)

- [ ] Tour video source + coverage (find a listing with a tour; then pin the selector/field).
- [ ] Brochure: confirm image-gallery-only vs PDF link presence (`Download Brochure`).
- [ ] Property-type/waterfront chips → exact detail-page selectors mapping to preference vocabulary.
- [ ] `listing_id` (4HMxxxx) coverage; extraction selector on detail pages.
- [ ] `/our-listing/` card stream: full 84 in one page? pagination behavior; bullets selector per card.
- [ ] Numeric acres always available (cards show `X ACRES /`); beds/baths only for homes.
- [ ] Rate-limit politeness: default throttle for 84-page detail crawl (e.g. small concurrency + delay); honor `Retry-After` if present.
- [ ] Featured image URL: from `featured_media` → media endpoint (REST), plus gallery scrape.

## Evidence Index (this run)

`../discovery/`: `our-listing.html`, `detail-100-acre.html`, `detail-3bed.html`, `detail-treehouse.html`, `recent-sales.html`, `our-developments.html`, `development-hopkins-grove.html`, `filter-v2.html`, `latest-listings.html`, `wp-json-root-ua.json`, `wp-types.json`, `wp-taxonomies.json`, `featured-collection.json`, `featured-item-full.json`, `pp100.json`, `all-content.json`, `district-terms(-parent).json`, `list_category-terms.json`, `featured_tags-terms.json`, `media-parent.json`, `sitemap.xml`, `featured_listing-sitemap.xml`.
