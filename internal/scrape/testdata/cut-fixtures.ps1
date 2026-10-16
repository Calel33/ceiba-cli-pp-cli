# cut-fixtures.ps1 — build small testdata fixtures from the discovery captures.
# Read-only against discovery/; writes only into internal/scrape/testdata/.
$ErrorActionPreference = 'Stop'
$disc = 'C:\Users\calvi\printing-press\.runstate\ceiba-cli-7f836d03\runs\20261016-071158-0adbf4c2\discovery'
$dest = 'C:\Users\calvi\printing-press\.runstate\ceiba-cli-7f836d03\runs\20261016-071158-0adbf4c2\working\ceiba-cli-pp-cli\internal\scrape\testdata'
New-Item -ItemType Directory -Force -Path $dest | Out-Null

function Slice([string]$file, [int]$from, [int]$to) {
  $lines = [System.IO.File]::ReadAllLines($file)
  if ($to -gt $lines.Count) { $to = $lines.Count }
  return ($lines[($from-1)..($to-1)] -join "`n")
}

function ArticleAt([string]$file, [int]$line) {
  $lines = [System.IO.File]::ReadAllLines($file)
  $start = $line - 1
  while ($start -ge 0 -and $lines[$start] -notmatch '<article class="ae-post-item') { $start-- }
  $end = $line - 1
  while ($end -lt $lines.Count -and $lines[$end] -notmatch '^\s*</article>') { $end++ }
  return ($lines[$start..$end] -join "`n")
}

# --- our-listing cards fixture (4 cards: spotlight, spotlight+sold, sold-only, spotlight+under-contract) ---
$our = Join-Path $disc 'our-listing.html'
$cards = @(
  (ArticleAt $our 2240),   # spotlight card (A Town Lot...)
  (ArticleAt $our 5310),   # spotlight + sold card
  (ArticleAt $our 5950),   # sold-only card
  (ArticleAt $our 7580)    # spotlight + under-contract card
)
$cardsOut = "<!-- Testdata: 4 complete article.ae-post-item cards cut from discovery/our-listing.html (lines ~2237, ~5240, ~5882, ~7513). Structure verified live 2026-10-16. -->`n" + ($cards -join "`n")
[System.IO.File]::WriteAllText((Join-Path $dest 'our-listing-cards.html'), $cardsOut)

# --- detail-100-acre fixture (assembled from verified ranges) ---
$d100 = Join-Path $disc 'detail-100-acre.html'
$chunks = @(
  "<!-- Testdata: assembled from discovery/detail-100-acre.html ranges 2110-2202, 2236-2262, 2781-2850, 3076-3235, 4107-4340, 4738-4800 -->",
  (Slice $d100 2110 2202),
  (Slice $d100 2236 2262),
  (Slice $d100 2781 2850),
  (Slice $d100 3076 3235),
  (Slice $d100 4107 4340),
  (Slice $d100 4738 4800)
)
[System.IO.File]::WriteAllText((Join-Path $dest 'detail-100-acre.html'), ($chunks -join "`n"))

# --- detail-treehouse fixture ---
$dtree = Join-Path $disc 'detail-treehouse.html'
$chunks = @(
  "<!-- Testdata: assembled from discovery/detail-treehouse.html ranges 2118-2212, 3398-3412, 3518-3532, 6528-6580 -->",
  (Slice $dtree 2118 2212),
  (Slice $dtree 3398 3412),
  (Slice $dtree 3518 3532),
  (Slice $dtree 6528 6580)
)
[System.IO.File]::WriteAllText((Join-Path $dest 'detail-treehouse.html'), ($chunks -join "`n"))

# --- detail-3bed fixture ---
# 2136-2167 = hero h1 + town/district icon lists; 3698-3745 = gc-listing-gallery
# e-gallery-image nodes (data-thumbnail); the rest as before.
$d3 = Join-Path $disc 'detail-3bed.html'
$chunks = @(
  "<!-- Testdata: assembled from discovery/detail-3bed.html ranges 2136-2167 (h1 + icon lists), 2165-2260 (price/meta strip), 3698-3745 (gc-listing-gallery), 3850-3862, 7355-7416 -->",
  (Slice $d3 2136 2167),
  (Slice $d3 2165 2260),
  (Slice $d3 3698 3745),
  (Slice $d3 3850 3862),
  (Slice $d3 7355 7416)
)
[System.IO.File]::WriteAllText((Join-Path $dest 'detail-3bed.html'), ($chunks -join "`n"))

Get-ChildItem $dest | Select-Object Name, Length | Format-Table -AutoSize
