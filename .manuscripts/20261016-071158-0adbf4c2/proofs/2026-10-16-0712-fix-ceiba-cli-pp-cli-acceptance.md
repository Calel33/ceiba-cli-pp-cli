# Acceptance Report — ceiba-cli

> Run: 20261016-071158-0adbf4c2 | Phase 5 (18-dogfood-testing) | Level: Full Dogfood (live, read-only)
> Results: 2026-10-16-0712-dogfood-results.json (+ results2 rerun) | Marker: phase5-acceptance.json

Acceptance Report: ceiba-cli
  Level: Full Dogfood (live against ceibabelize.com; no-auth read-only API)
  Tests: 150/150 passed (0 failed, 112 classifier-skipped non-applicable rows), matrix 150
  Failures (first run, fixed inline): 6
    - compare: exit 3 — happy-args still passed a 4HM listing ID, which only resolves after a cached show; switched the annotated fixtures + example to two slugs that resolve from synced facts
    - development get / featured-listing get / listing-page detail / media: "missing Examples section" in --help — the spec endpoints lacked example: values, so the generator (by design) omitted placeholder examples; added spec example: values and mirrored them into the four command files after freeze (a regen would have risked the hand-verified phase-14/15 documentation fixes)
  Fixes applied: 2 (both CLI-side; spec updated as source of truth)
  Printing Press issues: 0 machine gaps — the Examples omission is spec-authoring guidance (retro note: provide example: for every required-positional endpoint during curation)
  Gate: PASS

  Coverage detail: 84 listings + 104 live district terms + 3 status tags + 6 developments synced live during the matrix; novel commands exercised their happy paths against real data (changes/value/compare/comps/market/coverage all passing; hollow coverage cleared).
  Read-only posture: every executed test is GET/local; no writes to the site; no PII in outputs.