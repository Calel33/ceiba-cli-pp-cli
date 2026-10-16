# Novel-Features Brainstorm — ceiba-cli (audit trail)

> Subagent session: `ses_ebb82d2a4ffe28yKP4r3Q5zEF8` · Run: `20261016-071158-0adbf4c2`
> Persisted per `references/novel-features-subagent.md` §"Output handling 3". Customer model + killed candidates live here only.

## Customer model

**Audrey — the international lifestyle buyer.**

Today (without this CLI): 58-year-old from Colorado planning a move to Belize within two years, capped at $200k, focused on farmland in Cayo and beach-adjacent plots near Hopkins. Her week runs on four tabs: `/our-listing/` card stream, two or three mid-review detail pages, a Google-Maps view copied from an embed, and a Google Sheet where she hand-copies price, acres, town and notes for 10–15 candidates. Nothing shows price history, so she re-opens saved listings to check changes. Questions she cannot answer: "What appeared or changed since last Saturday?", "Which land is cheapest per acre within my budget?", "How do my top four compare side by side?"

Weekly ritual: Saturday triage of the card stream against last week's sheet — new cards opened, old ones re-checked, sheet updated, one or two promoted to "inquire".

Frustration: Nothing tells her what is new or changed; the ritual is a manual re-crawl of the same 84 cards.

**Rick — the investment scout.**

Today: Canadian investor/agent tracking the 6 developments and mining `/recent-sales/` before advising clients. Workspace: six development tabs, the recent-sales gallery, and a hand-maintained clipping doc. The galleries carry no numbers, so "what did things trade at" is answered by memory and text messages. Cannot answer: "Which district concentrates the most inventory under $250k?", "Is this asking price in line with its neighbors?"

Weekly ritual: scan new/changed listings in watch markets, pull freshest comps for the live client conversation, update the clipping doc.

Frustration: every comparative question is an eyeball exercise — no peer set, median, or delta exists.

**Sofia — the listing agent (seller side).**

Today: prepares pricing memos for Ceiba sellers; for each mandate opens competitor listings in the same district, checks under-contract/sold badges, hand-builds comparables tables. Cannot answer: "What is the median asking price per acre in this town?", "Which active listings are outliers for their area?"

Weekly ritual: pricing prep — every mandate that lands or refreshes gets a memo built from a fresh comps pass.

Frustration: the comps pass is pure transcription with no aggregates or per-acre numbers.

**Marco — the concierge operator.**

Today: builds and runs the AI concierge on ceibabelize — the CLI + MCP is his product surface. Today the bot fetches a 1.13 MB card stream and parses HTML per question; multi-listing questions are unanswerable at any token budget; missing coordinates/gallery/listing-ID fields surface only in production.

Weekly ritual: run the mirror sync, review the bot's failed/awkward-answer log, extend the tool surface.

Frustration: each unanswered multi-listing question becomes a one-off HTML script, and field gaps are discovered by user complaints.

## Candidates (pre-cut)

16 candidates; sources: (a) persona-driven, (b) service-specific content patterns, (c) cross-entity local queries, (f) Codebase Intelligence. (d)/(e) N/A.

1. **Change digest** — `changes --since 7d` — diff the mirror vs prior sync: new listings, price changes, status flips, spotlight changes. (Audrey, Marco; a+c) — KEEP.
2. **Price-per-acre leaderboard** — `value [--district] [--max-price]` — rank by USD/acre, cheapest first. (Audrey, Rick; b+c) — KEEP.
3. **District market report** — `market --district` — counts, median/quartile price, median $/acre, acre bands, status mix, spotlight count incl. child towns. (Sofia, Rick, Marco; c) — KEEP.
4. **Comparables with deltas** — `comps <id|slug> [--include-sold]` — nearest peers (same district, similar acres/price) with price and $/acre deltas. (Sofia, Rick; a+c) — KEEP.
5. **Side-by-side compare** — `compare <id|slug> ...` — aligned table for 2–4 named listings. (Audrey, Marco; a) — KEEP.
6. **Mirror coverage report** — `coverage` — per-field fill rates across mirrored listings. (Marco; f) — KEEP (borderline; weekly-use decider).
7. **Proximity search** — `near <id|--lat/lng> --radius` — (Audrey; b) — KEEP → cut in Pass 3 (weekly-use fail; coordinate coverage open item).
8. **Trip planner** — `trip --base --days` — KILL: scope creep (routing is trip software), monthly-at-best ritual.
9. **Cohort outliers** — `outliers --district` — KEEP → cut in Pass 3 (redundant with value+market).
10. **Spotlight shortlist** — `spotlight` — KILL: thin projection of one tag already absorbed (#13).
11. **BZD price mode** — `--bzd` — KILL: two-line formatting; BZD display already absorbed (#1/#14).
12. **Watchlist** — `watch add|remove|list` — KILL: stateful write surface; read-only spec; `changes` covers alerts.
13. **Natural-language ask** — `ask "…"` — KILL: LLM dependency (rubric); mechanical version is `search`.
14. **Weekly digest composer** — `digest` — KILL: pure composition of survivors; no independent data.
15. **Per-listing price history** — `history <id|slug>` — KILL: strict subset of `changes`; use `--listing` filter.
16. **Media link verifier** — `verify-media` — KILL: build-time QA owned by dogfood matrix; no weekly ritual; network-heavy.

## Survivors and kills

### Survivors

Cut work: `changes` (weekly: yes; no endpoint exposes prior values; local snapshot history + cross-sync diff; sibling-kill: `history`; hand-code). `value` (weekly: yes; price+acres appear separately, nothing divides; computed $/acre; sibling-kill: `outliers`; hand-code). `comps` (weekly: yes; show's similar[] is editorial with no numbers; local peer scoring + sold rows; sibling-kill: `near`; hand-code). `compare` (weekly: yes; one local join replaces N show calls; sibling-kill: `watchlist`; hand-code). `market` (weekly: yes; no WP aggregation endpoint; percentile stats + hierarchy rollup; sibling-kill: `digest`; hand-code). `coverage` (weekly: yes — post-sync review; nothing reports fill rates; COUNT-based local report; sibling-kill: `verify-media`; hand-code).

| # | Feature | Command | Buildability | Score | Persona | Why Only We Can Do This | Long Description |
|---|---------|---------|--------------|-------|---------|--------------------------|------------------|
| 1 | Change digest | `changes --since 7d` | hand-code | 8/10 | Audrey, Marco | Sync-time snapshots into a local history table power a cross-sync diff; WP REST exposes only current state and the site shows no history. | Use this command for a digest of what changed since a prior sync (new listings, price drops, status flips, spotlight changes). Do NOT use it for district-level market statistics; use 'market' instead. |
| 2 | Price-per-acre leaderboard | `value` | hand-code | 8/10 | Audrey, Rick | Computes price_usd ÷ acres across all 84 mirrored listings; the site shows the two figures in separate strips and never divides. | Use this command to rank individual listings by price per acre. Do NOT use it for district-level aggregate statistics; use 'market' instead. Do NOT use it to size up one listing against its peers; use 'comps' instead. |
| 3 | Side-by-side compare | `compare <id\|slug> <id\|slug> […]` | hand-code | 8/10 | Audrey, Marco | One local join over N stored rows emits an aligned USD/BZD/$-per-acre table, replacing N show round-trips. | Use this command for a side-by-side of a specific set of listings the user names. Do NOT use it to find which listings are comparable to a given listing; use 'comps' instead. |
| 4 | Comparables with deltas | `comps <id\|slug>` | hand-code | 7/10 | Sofia, Rick | SQLite peer scoring (same district, acre/price proximity) computes price and $/acre deltas, with sold-price rows behind `--include-sold`. | Use this command to find comparable listings for one listing and see price and price-per-acre deltas. Do NOT use it for a side-by-side of a specific set of listings the user names; use 'compare' instead. |
| 5 | District market report | `market --district <slug\|name>` | hand-code | 7/10 | Sofia, Rick, Marco | Percentile statistics over the mirrored catalog plus the synced 106-term district hierarchy; no WP endpoint aggregates. | Use this command for district-level aggregate statistics (counts, median price, median price per acre, status mix). Do NOT use it for a digest of recent changes; use 'changes' instead. Do NOT use it to rank individual listings; use 'value' instead. |
| 6 | Mirror coverage report | `coverage` | hand-code | 7/10 | Marco | COUNT-based fill rates over scraped columns (coords, listing_id, gallery, bullets, beds) surface exactly the capture gaps the brief left open. | none |

### Killed candidates

| Feature | Kill reason | Closest surviving sibling |
|---------|-------------|---------------------------|
| Proximity search (`near`) | Pass 3 weekly-use fail — monthly at best; district/town filters answer most location questions; coordinate coverage incomplete. | `comps` |
| Cohort outliers (`outliers`) | Redundant view — `value` surfaces under-priced leaders and `market` reports medians. | `value` |
| Trip planner (`trip`) | Scope creep — itinerary clustering is trip software; monthly-at-best ritual. | `value` |
| Spotlight shortlist (`spotlight`) | Thin projection of one tag already exposed as a search flag. | `changes` |
| BZD price mode (`--bzd`) | Two-line formatting; BZD display already carried by show/search. | `compare` |
| Watchlist (`watch add\|remove\|list`) | Scope creep — stateful write surface the read-only spec doesn't need. | `changes` |
| Natural-language ask (`ask`) | LLM dependency (rubric). | `search` (absorbed #2) |
| Weekly digest composer (`digest`) | Pure composition of survivors with no independent data. | `changes` |
| Per-listing price history (`history`) | Strict subset of `changes`; fold in as `--listing` filter. | `changes` |
| Media link verifier (`verify-media`) | Build-time QA owned by the dogfood matrix; no weekly ritual; network-heavy. | `coverage` |
