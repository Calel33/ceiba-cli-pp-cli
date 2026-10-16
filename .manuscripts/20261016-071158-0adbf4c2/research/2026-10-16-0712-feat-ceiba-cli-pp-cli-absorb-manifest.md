# Absorb Manifest — ceiba-cli

> Run: `20261016-071158-0adbf4c2` · Phase 1.5 (ecosystem-absorb-gate) · Stamp: 2026-10-16-0712
> Brief: `2026-10-16-0712-feat-ceiba-cli-pp-cli-brief.md` · Brainstorm: `2026-10-16-0712-novel-features-brainstorm.md`
> Ecosystem search: no pre-existing tooling for ceibabelize.com (no MCP servers, Claude skills, CLIs, npm/PyPI wrappers — registry + web verified). Sources below are the site's own surfaces, the sibling Press CLI `terracore-pp-cli` (local, same pattern class), and general WordPress/REST/real-estate tooling patterns found in the ecosystem sweep.

## Absorbed (match or beat everything that exists)

> **Naming note (framework collision):** root `search` is a generated framework built-in (FTS keyword + `--type/--limit/--db`). The spec's filtered compound surface (rows 1–13) therefore ships as `ceiba-cli-pp-cli listings` (hand-code), while the framework `search` covers pure keyword FTS over synced resources. Detail stays `show`.


| # | Feature | Best Source | Our Implementation | Added Value |
|---|---------|-----------|-------------------|-------------|
| 1 | Browse listings card stream (acres, district/town, price, 3 bullets, spotlight) | ceibabelize.com /our-listing/ (native) | `ceiba-cli-pp-cli listings` | Offline mirror, token-efficient JSON, compound filters |
| 2 | Keyword search | ceibabelize native ?s= | (behavior in `ceiba-cli-pp-cli listings`) `--query` over FTS5 (title+content+bullets) | Offline, instant, re-rankable |
| 3 | District filter (parent terms) | ceibabelize district taxonomy | (behavior in `ceiba-cli-pp-cli listings`) `--district <slug\|name>`, parent-inclusive | Hierarchy-aware; slugs resolved to term IDs at sync |
| 4 | Town/village filter (child terms) | ceibabelize district taxonomy | (behavior in `ceiba-cli-pp-cli listings`) `--neighbourhood <slug\|name>` | Target San Ignacio Town, Hopkins, etc. |
| 5 | Property type / lifestyle facet | ceibabelize preference vocabulary | (behavior in `ceiba-cli-pp-cli listings`) `--type` | Maps preference vocabulary onto listing facets |
| 6 | Waterfront filter | ceibabelize preferences (rf/of/bch/cf/lf) | (behavior in `ceiba-cli-pp-cli listings`) `--waterfront` (codes + names) | Human-friendly aliases |
| 7 | Acre bucket filters (Over/Under 10/25/50/100) | ceibabelize /filter-v2/ | (behavior in `ceiba-cli-pp-cli listings`) `--min-acres/--max-acres` | Numeric precision transcends the site's buckets |
| 8 | Price range USD | spec.md story 2 (site lacks) | (behavior in `ceiba-cli-pp-cli listings`) `--min-price/--max-price` | Budget targeting |
| 9 | Beds/baths filters | competitor 1stchoicebelize | (behavior in `ceiba-cli-pp-cli listings`) `--beds/--baths` | Residential-only optional fields |
| 10 | Sort | ceibabelize native orderby | (behavior in `ceiba-cli-pp-cli listings`) `--sort price\|acres\|modified\|title` | Mirror-side sorting |
| 11 | Sold hidden by default, opt-in | spec.md story 11 | (behavior in `ceiba-cli-pp-cli listings`) default excludes sold; `--include-sold` | Never mislead |
| 12 | Under Contract badge retained | ceibabelize featured_tags | (behavior in `ceiba-cli-pp-cli listings`) `status` field; `show` carries it too | Structured status |
| 13 | Ceiba Spotlight flag | ceibabelize featured_tags | (behavior in `ceiba-cli-pp-cli listings`) `spotlight` bool; `show` carries it too | Curated shortlist |
| 14 | Full listing detail in one call | ceibabelize detail pages | `ceiba-cli-pp-cli show` | Complete payload; accepts id, slug, or listing ID |
| 15 | Listing ID lookup (4HMxxxx) | ceibabelize detail pages | (behavior in `ceiba-cli-pp-cli show`) accepts listing_id \| slug \| WP id | Direct jump when present |
| 16 | Gallery image URLs | detail pages (ae-acf-gallery / pp-grid) | (behavior in `ceiba-cli-pp-cli show`) `gallery_urls` (absolute) | Carousel-ready for chat |
| 17 | YouTube tour URL | detail pages (when present) | (behavior in `ceiba-cli-pp-cli show`) `tour_url` | Media completeness |
| 18 | Map coordinates | detail pages (Maps embed q=) | (behavior in `ceiba-cli-pp-cli show`) `map_lat/map_lng` | Proximity answers |
| 19 | Brochure image galleries | detail pages (gc-brochure) | (behavior in `ceiba-cli-pp-cli show`) `brochure_urls` | Downloadables |
| 20 | Similar listings | detail "SIMILAR LISTING" grid | (behavior in `ceiba-cli-pp-cli show`) `similar[]` with compact detail | Discovery loop |
| 21 | Developments list/detail | development CPT + /our-developments/ | `ceiba-cli-pp-cli developments list` | Investor tracking; detail via the `developments show` subcommand |
| 22 | Recent Sales browsing | /recent-sales/ page | `ceiba-cli-pp-cli recent-sales list` | Seller comps |
| 23 | Freshness metadata + source | terracore-pp-cli envelope | (behavior in `ceiba-cli-pp-cli listings`) `{meta: source live\|local, synced_at}` + `source_url` per payload; `show` carries it too | Agents know data age |
| 24 | SQLite mirror + sync | terracore-pp-cli `sync`; wordpress-pp-cli mirror | (behavior in `ceiba-cli-pp-cli sync`) resources: featured_listing, district, featured_tags, development, recent_sales, media_meta; `--since/--full` | Offline answers |
| 25 | Idempotent upsert/dedup | propertyfinder-scraper dedup | (behavior in `ceiba-cli-pp-cli sync`) upsert by WP id; slug unique | Safe re-sync |
| 26 | Incremental refresh by `modified` | propertyfinder-scraper monitoring; WP REST `orderby=modified` | (behavior in `ceiba-cli-pp-cli sync --since`) modified-delta | Fast refresh |
| 27 | Agent mode + field projection | terracore-pp-cli `--agent/--select` | (behavior in `ceiba-cli-pp-cli listings`) generated global flags | Token efficiency |
| 28 | Capability finder | terracore-pp-cli `which` | (generated endpoint) `which "<capability>"` | Agent self-discovery |
| 29 | Health check (WP-REST reachability + browser-UA requirement) | terracore-pp-cli `doctor`; wordpress-pp-cli | (generated endpoint) `doctor` | Diagnose before work |
| 30 | Live route-table awareness | wpklx auto-discovery; wordpress-pp-cli `schema` | (behavior in `ceiba-cli-pp-cli doctor`) verifies wp-json routes reachable; sync reads the live route table | Survives site upgrades |
| 31 | Structured errors + typed exit codes | Press framework | (behavior in `ceiba-cli-pp-cli show`) JSON errors, typed exits | Agent-debuggable |
| 32 | Compound MCP tools (search/show/sync/developments/recent-sales) | spec.md story 24 | (behavior in `ceiba-cli-pp-cli listings`) exposed via the MCP cobratree mirror (listings/show/sync/developments/recent-sales tools); no per-filter tools, no SQL tool | Few round trips |
| 33 | Read-only guardrails | spec.md + terracore SKILL.md | (behavior in `ceiba-cli-pp-cli listings`) no write verbs; interest/preferences forms untouched; no PII | Safe by construction |
| 34 | Output formats (JSON-first + human fallback) | scrapers; Press framework | (behavior in `ceiba-cli-pp-cli listings`) `--json` default; `--csv/--plain`; human tables | Humans + agents |
| 35 | Absolute URL normalization for all media | spec.md stories 13/15 | (behavior in `ceiba-cli-pp-cli show`) absolutizes every URL | Hot-linkable |

**Out of scope by locked spec (not absorbed deliberately):** all WordPress write/CRUD surfaces of generic WP tools (create/update/delete/publish, media upload, user/plugin/theme management) — this CLI is read-only (spec.md "Out of Scope": no lead-submit form POST, no PII, no accounts).

**Stubs:** none. All absorbed rows are shipping scope.

## Transcendence (only possible with our approach)

| # | Feature | Command | Buildability | Score | Why Only We Can Do This | Long Description |
|---|---------|---------|--------------|-------|------------------------|------------------|
| 1 | Change digest | `changes --since 7d` | hand-code | 8/10 | Sync-time snapshots into a local history table power a cross-sync diff; WP REST exposes only current state and the site shows no history. | Use this command for a digest of what changed since a prior sync (new listings, price drops, status flips, spotlight changes). Do NOT use it for district-level market statistics; use 'market' instead. |
| 2 | Price-per-acre leaderboard | `value` | hand-code | 8/10 | Computes price_usd ÷ acres across all 84 mirrored listings; the site shows the two figures in separate strips and never divides. | Use this command to rank individual listings by price per acre. Do NOT use it for district-level aggregate statistics; use 'market' instead. Do NOT use it to size up one listing against its peers; use 'comps' instead. |
| 3 | Side-by-side compare | `compare` | hand-code | 8/10 | One local join over N stored rows emits an aligned USD/BZD/$-per-acre table, replacing N show round-trips (accepts 2-4 id/slug/listing-ID args). | Use this command for a side-by-side of a specific set of listings the user names. Do NOT use it to find which listings are comparable to a given listing; use 'comps' instead. |
| 4 | Comparables with deltas | `comps` | hand-code | 7/10 | SQLite peer scoring (same district, acre/price proximity) computes price and $/acre deltas, with sold-price rows behind `--include-sold`. | Use this command to find comparable listings for one listing and see price and price-per-acre deltas. Do NOT use it for a side-by-side of a specific set of listings the user names; use 'compare' instead. |
| 5 | District market report | `market --district <slug\|name>` | hand-code | 7/10 | Percentile statistics over the mirrored catalog plus the synced 106-term district hierarchy; no WP endpoint aggregates. | Use this command for district-level aggregate statistics (counts, median price, median price per acre, status mix). Do NOT use it for a digest of recent changes; use 'changes' instead. Do NOT use it to rank individual listings; use 'value' instead. |
| 6 | Mirror coverage report | `coverage` | hand-code | 7/10 | COUNT-based fill rates over scraped columns (coords, listing_id, gallery, bullets, beds) surface exactly the capture gaps the brief left open. | none |

**Hand-code commitment:** 6 of 6 transcendence features are hand-code (~50-150 LoC each plus root.go wiring); 0 are spec-emitted.
**Killed at brainstorm:** `near`, `outliers`, `trip`, `spotlight`, `--bzd`, `watch`, `ask`, `digest`, `history`, `verify-media` (see brainstorm audit for reasons).

## Ride-along risks (present at the gate)

1. **Raw sniffed spec is polluted** — `research/ceiba-cli-browser-sniff-spec.yaml` picked `maps.googleapis.com` as base URL and inferred an `api_key` auth from the Google Maps embed. Phase 10 curates it before generation: drop `maps`/`pagead`, set base URL to `https://ceibabelize.com`, remove the fake api_key auth, keep 5 wp-json endpoints + 2 HTML extract surfaces.
2. **Open coverage items** (surveyed in-brief, verified by `show` extraction at build): listing_id (4HMxxxx) partial coverage; tour video coverage unknown; brochure model = image galleries (no PDFs found); property-type facet mapping from detail chips.
3. **Naming:** spec-locked `ceiba-cli` → binary `ceiba-cli-pp-cli`, MCP `ceiba-cli-pp-mcp`, library dir `ceiba-cli` (Press convention, same as user's `terracore-pp-cli`).
