# Ceiba Realty CLI

**All 84 Ceiba listings offline, with market math, comps, and change tracking over a local mirror.**

ceiba-cli mirrors the full Ceiba Realty catalog — land, farms, homes, islands and developments — into local SQLite, then answers with structured one-call commands: filtered search, complete detail payloads, price-per-acre rankings, district market reports, comparables, and what-changed digests. Sold listings stay hidden by default, and payloads carry their source (live or local) plus freshness metadata where the mirror tracks it.

Learn more at [Ceiba Realty](https://ceibabelize.com).

Created by [@Calel33](https://github.com/Calel33).

## Install

The recommended path installs both the `ceiba-cli-pp-cli` binary and the `pp-ceiba-cli` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install ceiba-cli
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install ceiba-cli --cli-only
```

For skill only — installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install ceiba-cli --skill-only
```

To constrain the skill install to one or more specific agents (repeatable — agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install ceiba-cli --agent claude-code
npx -y @mvanhorn/printing-press-library install ceiba-cli --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.9 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/other/ceiba-cli/cmd/ceiba-cli-pp-cli@latest
```

This installs the CLI only — no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/ceiba-cli-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine ceiba-cli-pp-cli`. On Unix, mark it executable: `chmod +x ceiba-cli-pp-cli`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install ceiba-cli --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-ceiba-cli --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-ceiba-cli --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install ceiba-cli --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle — Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/ceiba-cli-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/other/ceiba-cli/cmd/ceiba-cli-pp-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "ceiba-cli": {
      "command": "ceiba-cli-pp-mcp"
    }
  }
}
```

</details>

## Quick Start

```bash
# Verify the CLI installs and the Ceiba source is reachable before fetching anything.
ceiba-cli-pp-cli doctor --dry-run

# Mirror all 84 listings, districts, developments, and media metadata into local SQLite.
ceiba-cli-pp-cli sync

# Budget-filtered search across the mirror; sold listings stay hidden by default.
ceiba-cli-pp-cli listings --query "river" --district cayo --max-price 200000

# One complete payload: USD/BZD pricing, specs, gallery, map, brochure, and similar listings.
ceiba-cli-pp-cli show 100-acre-investment-property-on-the-belize-river-in-the-cayo-district

# Rank budget listings by price per acre — math the website never shows.
ceiba-cli-pp-cli value --max-price 250000

```

## Unique Features

### Market intelligence
- **`changes`** — See new listings, price drops, and status flips since a prior sync — a Saturday-morning scan of the whole catalog.

  _Reach for this when someone asks what is new or changed instead of re-reading all 84 cards._

  ```bash
  ceiba-cli-pp-cli changes --since 7d --agent
  ```
- **`value`** — Rank budget-filtered listings by price per acre, cheapest first.

  _Use it when a user cares about land value per acre, not asking price alone._

  ```bash
  ceiba-cli-pp-cli value --district cayo --max-price 250000 --agent
  ```
- **`comps`** — Nearest peer listings for one property, with price and per-acre deltas; sold comps opt-in.

  _Use it before pricing or offers to ground one property against its market._

  ```bash
  ceiba-cli-pp-cli comps 20-acre-estate-parcel-w-waterfront-near-hopkins --include-sold --agent
  ```
- **`market`** — Counts, median price, median price per acre, and status mix for a district and its towns.

  _Use it for regional summaries before drilling into individual listings._

  ```bash
  ceiba-cli-pp-cli market --district cayo --agent
  ```

### Agent-native workspace
- **`compare`** — Aligned feature table for 2-4 named listings in one call.

  _Use it when the user names a specific set of listings to weigh against each other._

  ```bash
  ceiba-cli-pp-cli compare 20-acre-estate-parcel-w-waterfront-near-hopkins a-unique-treehouse-perched-atop-the-macal-river --agent
  ```
- **`coverage`** — Per-field fill rates across mirrored listings so operators know which answers the mirror can currently support.

  _Run it after a sync to know which questions the local mirror can answer today._

  ```bash
  ceiba-cli-pp-cli coverage --agent
  ```

## Recipes

### Saturday market scan

```bash
ceiba-cli-pp-cli changes --since 7d
```

New listings, price drops, and status flips since last week's sync.

### Comps for a pricing memo

```bash
ceiba-cli-pp-cli comps 20-acre-estate-parcel-w-waterfront-near-hopkins --include-sold
```

Nearest peer listings with price and per-acre deltas, sold comps included.

### District market report

```bash
ceiba-cli-pp-cli market --district cayo
```

Counts, median price, median price per acre, and status mix for Cayo and its towns.

### Token-tight listing scan for agents

```bash
ceiba-cli-pp-cli search "investment" --type featured_listing --agent --select id,title,price_usd,acres
```

Projected JSON keeps a many-hit scan inside a small token budget.

## Usage

Run `ceiba-cli-pp-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data such as `data.db` |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `CEIBA_CLI_CONFIG_DIR`, `CEIBA_CLI_DATA_DIR`, `CEIBA_CLI_STATE_DIR`, or `CEIBA_CLI_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `CEIBA_CLI_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export CEIBA_CLI_HOME=/srv/ceiba-cli
ceiba-cli-pp-cli doctor
```

Under `CEIBA_CLI_HOME=/srv/ceiba-cli`, the four dirs resolve to `/srv/ceiba-cli/config`, `/srv/ceiba-cli/data`, `/srv/ceiba-cli/state`, and `/srv/ceiba-cli/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "ceiba-cli": {
      "command": "ceiba-cli-pp-mcp",
      "env": {
        "CEIBA_CLI_HOME": "/srv/ceiba-cli"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `CEIBA_CLI_DATA_DIR` overrides an explicit `--home` for that kind. Use `CEIBA_CLI_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `CEIBA_CLI_HOME` does not move files back to platform defaults, and `doctor` cannot find files left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. Run `ceiba-cli-pp-cli doctor --fail-on warn` to check path warnings in automation.

## Commands

### development

Our Developments — pre-built communities (6 items)

- **`ceiba-cli-pp-cli development get`** - Get one development by WordPress post ID
- **`ceiba-cli-pp-cli development list`** - List developments

### district

District/town taxonomy terms (106 hierarchical location terms on featured_listing)

- **`ceiba-cli-pp-cli district`** - List district and town terms (parent 0-style district parents 47-52; towns as children)

### featured-listing

Ceiba featured listings (WP REST featured_listing; sold excluded via featured_tags_exclude=30)

- **`ceiba-cli-pp-cli featured-listing get`** - Get one listing by WordPress post ID (title, slug, description content, taxonomies, media reference)
- **`ceiba-cli-pp-cli featured-listing list`** - List Ceiba listings (search, sort, and taxonomy-ID filters; all 84 fit in one per_page=100 page)

### featured-tag

Status taxonomy terms (ceiba-spotlight=31, under-contract=32, sold=30)

- **`ceiba-cli-pp-cli featured-tag`** - List status tags with term IDs and counts

### listing-page

HTML listing surfaces (server-rendered Elementor pages)

- **`ceiba-cli-pp-cli listing-page detail`** - Fetch a listing detail page's extracted content and links (price, specs, gallery, map, similar listings live in the page HTML)
- **`ceiba-cli-pp-cli listing-page our-listing`** - Extract listing-card links from the Our Listings page

### media

WordPress media items (featured images and attachments)

- **`ceiba-cli-pp-cli media <media_id>`** - Get one media item (source_url, alt text, dimensions)


### changes


- **`ceiba-cli-pp-cli changes`** - See new listings, price drops, and status flips since a prior sync — a Saturday-morning scan of the whole catalog.


### compare


- **`ceiba-cli-pp-cli compare`** - Aligned feature table for 2-4 named listings in one call.


### comps


- **`ceiba-cli-pp-cli comps`** - Nearest peer listings for one property, with price and per-acre deltas; sold comps opt-in.


### coverage


- **`ceiba-cli-pp-cli coverage`** - Per-field fill rates across mirrored listings so operators know which answers the mirror can currently support.


### market


- **`ceiba-cli-pp-cli market`** - Counts, median price, median price per acre, and status mix for a district and its towns.


### value


- **`ceiba-cli-pp-cli value`** - Rank budget-filtered listings by price per acre, cheapest first.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`ceiba-cli-pp-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`ceiba-cli-pp-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`ceiba-cli-pp-cli learnings list`** - Inspect taught rows
- **`ceiba-cli-pp-cli learnings forget <query>`** - Undo a teach
- **`ceiba-cli-pp-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`ceiba-cli-pp-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`ceiba-cli-pp-cli teach-pattern`** - Install a query/resource template up front
- **`ceiba-cli-pp-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `CEIBA_CLI_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `ceiba-cli-pp-cli` opens the database, older binaries refuse it with a version error — upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
ceiba-cli-pp-cli development list

# JSON for scripting and agents
ceiba-cli-pp-cli development list --json
# Filter to specific fields
ceiba-cli-pp-cli development list --json --select _links,acf,class_list

# Dry run — show the request without sending
ceiba-cli-pp-cli development list --dry-run

# Agent mode — JSON + compact + no prompts in one flag
ceiba-cli-pp-cli development list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts; every input is a positional argument or flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `5` API error, `7` rate limited or blocked, `10` config error.

## Freshness

This CLI owns bounded freshness for registered store-backed read command paths. In `--data-source auto` mode, covered commands check the local SQLite store before serving results; stale or missing resources trigger a bounded refresh, and refresh failures fall back to the existing local data with a warning. `--data-source local` never refreshes, and `--data-source live` reads the API — only `show` caches its fetched detail back into the local mirror for later reuse.

Set `CEIBA_CLI_NO_AUTO_REFRESH=1` to disable the pre-read freshness hook while preserving the selected data source.

Covered command paths:
- `ceiba-cli-pp-cli development get`
- `ceiba-cli-pp-cli development list`
- `ceiba-cli-pp-cli district`
- `ceiba-cli-pp-cli featured-listing get`
- `ceiba-cli-pp-cli featured-listing list`
- `ceiba-cli-pp-cli featured-tag`
- `ceiba-cli-pp-cli listing-page detail`
- `ceiba-cli-pp-cli listing-page our-listing`

JSON outputs that use the generated provenance envelope include freshness metadata at `meta.freshness`. This metadata describes the freshness decision for the covered command path; it does not claim full historical backfill or API-specific enrichment.

## Known Gaps

- `recent-sales list` returns `[]` with a note when the site's gallery is client-rendered — it never fabricates comps.
- `developments show` returns images but may have an empty description on nav-only development pages.
- No listing tour videos were found across the sampled detail pages; `show` returns `tour_url` when a listing actually has one.
- `compare` resolves 4HM listing IDs only after `show` has cached that listing; slugs and WordPress ids always work.

## Health Check

```bash
ceiba-cli-pp-cli doctor
```

Verifies configuration and connectivity to the API.

## Configuration

Run `ceiba-cli-pp-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/ceiba-cli-pp-cli/config.toml`; `--home`, `CEIBA_CLI_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

## Troubleshooting
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **403 or 406 responses from the site** — Run ceiba-cli-pp-cli doctor — the site's ModSecurity gate requires browser-like headers; doctor reports when the block resurfaces.
- **Search returns nothing after a first run** — Run ceiba-cli-pp-cli sync --full to rebuild the mirror, then re-run the search.
- **An expected sold listing never appears** — Sold listings are hidden by design; add --include-sold to the search.
- **Fields like map coordinates or listing IDs are empty for some listings** — Run ceiba-cli-pp-cli coverage to see fill rates; the site itself omits these on some listings.

## Discovery Signals

This CLI was generated with browser-captured traffic analysis.
- Target observed: https://ceibabelize.com/our-listing/
- Capture coverage: 15 API entries from 452 total network entries
- Reachability: standard_http (95% confidence)
- Protocols: ssr_embedded_data (85% confidence), rest_json (75% confidence), html_scrape (55% confidence)
- Generation hints: weak_schema_confidence
- Candidate command ideas: get_conversion — Derived from observed GET /pagead/conversion/{conversion_id}/ traffic.; get_media — Derived from observed GET /wp-json/wp/v2/media/{media_id} traffic.; list_development — Derived from observed GET /wp-json/wp/v2/development traffic.; list_district — Derived from observed GET /wp-json/wp/v2/district traffic.; list_featured_listing — Derived from observed GET /wp-json/wp/v2/featured_listing traffic.; list_featured_tags — Derived from observed GET /wp-json/wp/v2/featured_tags traffic.; list_gen_204 — Derived from observed GET /maps/api/mapsjs/gen_204 traffic.; list_js — Derived from observed GET /maps/api/js traffic.

Warnings from discovery:
- empty_payload: API-looking request returned an empty or null payload; schema confidence is weak.
- weak_schema_evidence: Binary or protobuf response cannot provide reliable JSON schema evidence.
- weak_schema_evidence: Binary or protobuf response cannot provide reliable JSON schema evidence.
- weak_schema_evidence: Binary or protobuf response cannot provide reliable JSON schema evidence.
- weak_schema_evidence: Binary or protobuf response cannot provide reliable JSON schema evidence.
- weak_schema_evidence: Binary or protobuf response cannot provide reliable JSON schema evidence.
- weak_schema_evidence: Binary or protobuf response cannot provide reliable JSON schema evidence.
- weak_schema_evidence: Binary or protobuf response cannot provide reliable JSON schema evidence.
- weak_schema_evidence: Binary or protobuf response cannot provide reliable JSON schema evidence.
- weak_schema_evidence: Binary or protobuf response cannot provide reliable JSON schema evidence.

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**wordpress-pp-cli**](https://github.com/mvanhorn/printing-press-library/tree/main/library/developer-tools/wordpress) — Go
- [**propertyfinder-scraper**](https://github.com/Dory168/propertyfinder-scraper) — Python
- [**wpklx**](https://github.com/KLIXPERT-io/wpklx) — TypeScript
- [**wp-json-scraper**](https://github.com/MickaelWalter/wp-json-scraper) — Python
- [**property_web_scraper**](https://github.com/RealEstateWebTools/property_web_scraper) — Ruby

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
