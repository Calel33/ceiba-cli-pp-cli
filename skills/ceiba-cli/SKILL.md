---
name: ceiba-cli
description: "Drives the read-only ceiba-cli CLI + MCP server for Ceiba Realty Belize listings (ceibabelize.com), backed by a local SQLite mirror: search and filter listings, fetch full property payloads, track price and status changes, rank price per acre, compare properties, find comps, and report district markets. Use when the user asks about Belize real estate or property for sale — land, farms, homes, islands, developments — or wants Ceiba Realty / ceibabelize.com listings searched, wants properties compared or priced, needs comps or district market stats, asks what is new or under contract on Ceiba, mentions \"Belize property search\", \"land in Cayo under 150k\", \"price per acre in Belize\", or invokes /ceiba-cli or the ceiba-cli-pp-mcp MCP tools."
---

# Ceiba CLI — Belize Realty Listings

Answer Belize property questions from Ceiba Realty (ceibabelize.com) with a read-only CLI + MCP server over a local SQLite mirror (~84 listings, 6 developments, ~104 district terms on the 2026-10-16 mirror). Mirror-first: most commands answer locally; `sync` refreshes; a few commands scrape live.

## Hard rules

- READ-ONLY. GET/local only. Never use it for form submissions, contact or interest forms, lead-gen, or any write to the site. Only local state changes: mirror, learnings, feedback.
- Never fabricate comps or sales. `recent-sales list` returning `[]` is a valid, honest answer when the site's gallery is client-rendered.
- Keep PII out of `teach`/`feedback` text (stored locally): strip names, emails, phones.
- Verify the binary before use: `--version`; `doctor` checks config, auth (not required), and API reachability. Exit 0 = healthy.

## Install and locate

Look for the binary on PATH first (`ceiba-cli-pp-cli`, `ceiba-cli-pp-mcp`). Staged binaries verified on this machine:

```
C:\Users\calvi\printing-press\library\ceiba-cli\build\stage\bin\ceiba-cli-pp-cli.exe
C:\Users\calvi\printing-press\library\ceiba-cli\build\stage\bin\ceiba-cli-pp-mcp.exe
```

Installer (any machine; `--cli-only` skips the agent skill):

```bash
npx -y @mvanhorn/printing-press-library install ceiba-cli --cli-only
```

Go fallback, documented in the shipped skill (use when published to this module path):

```bash
go install github.com/mvanhorn/printing-press-library/library/other/ceiba-cli/cmd/ceiba-cli-pp-cli@latest
```

Source repository: https://github.com/Calel33/ceiba-cli-pp-cli. Claude Desktop ships a one-click `.mcpb` bundle on the `ceiba-cli-current` release (per README).

## Workflow

1. Verify (`--version`, optionally `doctor`).
2. Refresh the mirror with `sync` when absent or stale. Safe, live, read-only; takes seconds (84 listings + 104 district terms + 6 developments + 3 status tags). `changes` needs at least two syncs to show movement.
3. Pick the command from the table below.
4. Run it with `--agent`; parse the JSON envelope.
5. Report `.results`; if empty, say so plainly and name the fallback (e.g. `listings --query`).

## Command selection

| Intent | Command | Example / notes |
|---|---|---|
| Keyword scan (any words) | `listings --query` | `listings --query river --agent` → 25 rows; searches title, content, bullets |
| Filtered browse (price/acres/district/beds/status/waterfront) | `listings` | `listings --district cayo --max-price 100000 --min-acres 1 --agent` |
| One property, full payload | `show` | `show <slug\|wp-id\|4HM-id> --agent` → USD+BZD price, specs, bullets, description, gallery, brochure, tour, map, similar |
| What moved since a prior sync | `changes` | `changes --since 7d --agent`; new listings, price drops/raises, status flips |
| Price-per-acre ranking | `value` | `value --max-price 250000 --agent`; cheapest per acre first; rows need both price and acres |
| Side-by-side (2–4 refs) | `compare` | `compare <ref> <ref> [<ref> <ref>] --agent` |
| Peer deltas for one property | `comps` | `comps <ref> --include-sold --agent`; same-primary-district peers within ~50% acreage; price and $/acre deltas |
| District statistics | `market` | `market --district cayo --agent`; count, min/median/max price, median $/acre, status mix, acre bands |
| Mirror data health | `coverage` | `coverage --agent`; per-field fill rates before relying on sparse fields |
| Developments | `developments list` / `developments show <slug>` | list is local; show scrapes live images |
| Sold gallery | `recent-sales list` | live scrape; usually `[]` + note (see gotchas) |
| Find a command by capability | `which "<capability>"` | `which "price per acre"`; exit 2 = no confident match |

Flag notes (all verified in `--help`): `listings` accepts `--query --min-price --max-price --min-acres --max-acres --district --neighbourhood --listing-id --type --waterfront --status --beds --baths --sort --limit --include-sold`. `changes`: `--since 7d|36h|2w` (default 7d), `--limit`, `--listing`. `value`: `--district --max-price --min-acres --limit --include-sold`. `comps`: `--limit` (default 10), `--include-sold`. `district`: `--page` (page 2 may be needed), `--per-page` (max 100), `--fields`.

## Agent mode, output shapes

- `--agent` expands to `--json --compact --no-input --no-color`. Use it for every agent run.
- With `--agent`, commands emit a provenance envelope: `{"meta": {"source": "live"|"local"}, "results": ...}`. Parse `.results`; check `.meta.source` to know whether an answer came from the mirror or the live site.
- Plain `--json` (and the default piped output) on collection commands emits a bare JSON array without the envelope — prefer `--agent` for a stable shape.
- `--select <fields>` projects row fields on collection commands: `listings --agent --select title,price_usd`. It does not project inner fields of single-object commands like `show` (there it only matches envelope keys `meta`/`results` and warns).
- `--data-source auto|live|local` (global): auto = local when available with live fallback; local = mirror only; live = API only. `show` serves a detail cached within the last 24 h and refreshes otherwise.

## Sold, status, districts

- Sold listings are hidden by default everywhere. Opt in with `--include-sold` (result becomes a superset) or `--status sold` (implies it).
- `--status` matches the projected status exactly. Projection precedence: sold > under-contract > spotlight > active. Rows also carry booleans `sold`, `under_contract`, `spotlight`.
- 2026-10-16 mirror: default 79 rows; `--status active` 19, `spotlight` 58, `under-contract` 2, `sold` 5 (sums to 84).
- `--district <name|slug>` = parent districts, includes their towns (`cayo` → 51 rows). `--neighbourhood <name|slug>` = town/village only (`"San Ignacio Town"`, `silk-grass`). Passing a district to `--neighbourhood` is a usage error by design: exit 2.
- `market` reports the full status mix including sold (cayo: count 54 vs 51 in `listings`). An unknown district returns count 0 with a note — not an error (exit 0).

## Exit codes

`0` success · `2` usage error · `3` not found (e.g. `show <bogus-slug>` → `{"code":3,...}`, exit 3) · `5` API error · `7` rate limited/blocked (back off) · `10` config error. Codes 0/2/3 verified live; 5/7/10 per the shipped README.

## MCP server

`ceiba-cli-pp-mcp` exposes 41 tools — the CLI surface as MCP tools. Read tools include `context` (call once first), `listings`, `show`, `changes`, `value`, `compare`, `comps`, `market`, `coverage`, `developments_list`, `developments_show`, `recent_sales_list`, `district_list`, plus REST passthroughs `development_get`, `development_list`, `featured-listing_get`, `featured-listing_list`, `featured-tag_list`, `listing-page_detail`, `listing-page_our-listing`, `media_get`, and the operational `sync`, `sql` (read-only SQL over the mirror), `export`, `workflow_status`, `workflow_archive`.

- Arguments mirror CLI flags without the `--` prefix: `listings {district: "cayo", max-price: 100000, limit: 10}`.
- Positional arguments use generated names — inspect the tool schema (or `context`) before calling: `show {id_slug_listing-id: "<slug>"}`, `compare {id_slug: "<slugA>", id_slug_id_slug: "<slugB>"}` (both verified working).
- Result shapes vary by tool: `listings`/`market`/`sql`/`context` return the payload directly; `show` returns the `{meta, results}` envelope. Check for `.results` and use the payload as-is otherwise.
- Register with an MCP host:

```bash
claude mcp add ceiba-cli-pp-mcp -- ceiba-cli-pp-mcp
```

```json
{
  "mcpServers": {
    "ceiba-cli": {
      "command": "ceiba-cli-pp-mcp",
      "env": { "CEIBA_CLI_HOME": "/srv/ceiba-cli" }
    }
  }
}
```

(`env` is optional; it relocates local state. MCP servers do not inherit CLI flags.)

- Prefer MCP when the host is MCP-capable and answers flow through tool results (no shell quoting of user text; structured arguments). Prefer the CLI for scripts, CI, and shell pipelines.

## Gotchas and limitations (verified on the live binary)

- `search` (and the MCP `search` tool) returns `[]` with `hint: local store has not been synced yet` even after `sync` and `sync --full` in this build. Use `listings --query "<words>"` for keyword search instead.
- `recent-sales list` → `[]` + a note when the site's gallery is client-rendered; empty does not mean no sales.
- `developments show <slug>` may return images but no description on nav-only pages (e.g. `serendipity-hills`). Images still usable.
- No tour videos found in sampling; `show` returns `tour_url` when a listing has one.
- `compare` takes 2–4 refs. 4HM listing IDs resolve only after `show` cached that listing — prefer slugs; unresolvable refs exit 3.
- District term counts drift with the live site (docs say 106 terms; mirror has 104). Use `district --page 2` / `--per-page` to complete the list.
- `changes` compares sync sessions: run `sync` at least twice before trusting movement; a first digest can report most of the catalog as new.
- The CLI journals every invocation locally and caches teaches (learning loop). Pass `--no-learn` (or `CEIBA_CLI_NO_LEARN=true`) for deterministic flows; pass user questions as argv/MCP arguments, never spliced into shell strings.
