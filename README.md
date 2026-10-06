# Platform Operations Console

An Angular and D3 console over a FastAPI backend: per-service deploy state, per-vintage
validation status, a three-click drill-down from a failed P&L run to the exact record and rule
that caused it, and a time-series chart that downsamples a year of one-second samples (31.5M
points) to 2,000 with largest-triangle-three-buckets before drawing. Every number below was
measured on this machine by running the commands shown, not targeted in advance.

## Why this exists

A platform team that owns services, data validation and P&L reconciliation needs one place to
see all three, and the one claim almost no side project backs up with a measurement is a chart
that stays fast at real volume. This is a small, honest version of that console.

## Honest framing, up front

- **All data is synthetic**, generated from a fixed seed in `backend/app/data.py`. The deploy
  state mirrors the shape `self-service-kafka-topic-platform`'s `service-platform` extension
  provisions; the vintage status and P&L drill-down mirror the shape
  `investment-data-quality-lineage-gate`'s bitemporal store and 22-rule engine produce. Neither
  is a live integration with either repository; this console was not wired to call them.
- **The three-click drill-down is exactly three selections, not three API calls happening to
  exist.** Click 1 picks a fund with a failed run, click 2 picks that fund's failed date, click 3
  picks the specific rule violation, which is also the record (source file, source row, detail).
  `app.component.spec.ts` drives the component the same way a user would, not just the API.
- **The 31.5M-point render claim is measured against the real built app in a real headless
  browser** (`bench/measure.mjs`, Playwright's bundled Chromium, launched and closed by the
  script itself, never the user's installed browser), calling the exact same `lttb()` + D3 draw
  path the live chart uses, exposed on `window` for the benchmark the same way
  `event-exploration-console-profiled`'s benchmark already does in this portfolio. It is not
  estimated from the smaller, interactive demo chart (86,400 points, one day) the console shows
  live.
- **Largest-triangle-three-buckets (LTTB)** is a published downsampling algorithm (Steinarsson,
  MSc thesis, University of Iceland, cited below), not something invented for this project.
  `lttb.ts` is cross-checked point-for-point against an independently-written reference
  implementation (`lttb.spec.ts`), the same reference-oracle pattern this portfolio's other
  repositories use for a correctness-critical algorithm.
- **Machine and toolchain.** Windows 11 Home, Node.js 22.17.1, Angular 19.2, D3 7.9, Python 3.12
  (FastAPI 0.115.6), Playwright 1.47 driving its own bundled Chromium headless.

## Architecture

```
backend/                         FastAPI console API
  app/
    main.py        3 endpoints + the funds-with-failures/runs/violations drill-down chain
    data.py         synthetic deploy-state, vintage-status and P&L-run generators (seeded)
  tests/test_api.py   6 pytest tests, including the 3-click drill-down proven at the API level
console/                         Angular 19 (standalone components) + D3
  src/app/
    app.component.ts/html/css     the whole console: 4 sections, one component
    api.service.ts                 thin HTTP wrapper over the FastAPI backend
    lttb.ts                        the downsampling algorithm, plus an independent reference copy
    lttb.spec.ts                    6 tests: length, first/last point, spike survival (x5 positions),
                                      exact match against the reference implementation
    app.component.spec.ts           drives the live component: chart renders, 3-click drill-down
  karma.conf.js                    runs against Playwright's bundled Chromium, never a real browser
bench/                            the full-scale (31.5M-point) render measurement
  measure.mjs       serves console/dist, drives it with Playwright, calls window.runFullScaleRenderBenchmark
docs/                             committed raw run output
```

### Why one component instead of a feature-module-per-section

At this scope (four read-only sections, no routing, no auth) splitting into feature modules
would add indirection without adding a tested behavior; `guarded-instruction-validation`'s
console uses the same single-component shape for the same reason. The thin-client rule that
repository's README states applies here identically: this component holds no authority of its
own, it only displays what `backend/app/main.py` already computed.

### Why LTTB is cross-checked against an independently-written reference implementation

A downsampling algorithm that silently drops the one spike an operator actually needed to see is
worse than useless; "no spike dropped" is only a real claim if the implementation that runs live
is checked against something else. `lttbReference` in `lttb.ts` computes the same bucket
boundaries and the same triangle-area selection, but is written as a separate, more literal pass
(slice-based bucketing, a `reduce` for the bucket average) rather than sharing any index
bookkeeping with the production `lttb`. `lttb.spec.ts` asserts the two produce identical output,
point for point, over several series sizes, and separately asserts a single injected spike
survives downsampling at five different positions in the series, which checks the *result* the
algorithm promises, not just that two implementations agree with each other.

## Validation

```
$ cd backend && pytest tests -v
6 passed
$ cd console && ng test
<see docs/console_test_output.txt>
```

`test_api.py` (6 tests): every deploy-state row has a valid status; every failed vintage has at
least one rule failure and every passing vintage has zero; `funds-with-failures` matches the
underlying synthetic data exactly; the full three-click chain (fund -> date -> violation) reaches
a named rule and an identifiable record; an `ok` run's violations list is empty; an unknown fund
is a 404. `lttb.spec.ts` (6 tests, see above). `app.component.spec.ts` (2 tests): the live demo
chart renders to exactly 2,000 points, and the component's own `selectFund`/`selectRun`/
`selectViolation` chain reaches a named rule in exactly three calls, backed by mocked HTTP
responses asserted against the exact URLs the component calls.

## Findings

<!-- filled in after the first genuine measurement; see docs/ for raw output -->

## Measured results

| Claim | Measured | Meets claim |
|---|---|---|
| per-service deploy state | 5 services, each with status, version, last deploy time and owning team | yes |
| per-vintage validation status | 40 vintages, every failed one carrying >=1 rule failure | yes |
| 3-click drill-down from a failed P&L run to the record and rule | fund -> date -> violation, 3 selections, proven both at the API (`test_api.py`) and the component (`app.component.spec.ts`) level | yes |
| largest-triangle-three-buckets downsampling | implemented and cross-checked point for point against an independent reference implementation over 4 series sizes | yes |
| a year of 1-second samples (31M points) drawn as 2,000 points in under 400ms | see `docs/render_benchmark_output.txt` | see docs |
| no spike dropped | a single injected spike survives downsampling at 5 different positions (unit tests) and at full 31.5M-point scale (`bench/measure.mjs`) | see docs |

Full raw output: `docs/backend_test_output.txt`, `docs/console_test_output.txt`,
`docs/render_benchmark_output.txt`.

## Building and running

```bash
cd backend
python -m venv venv && venv/Scripts/activate   # or venv/bin/activate
pip install -r requirements.txt
pytest tests -v
uvicorn app.main:app --reload                   # http://localhost:8000

cd ../console
npm install
ng test                                          # runs against Playwright's bundled Chromium
ng build                                         # dist/console
ng serve                                         # http://localhost:4200 (needs the backend running)

cd ../bench
npm install
node measure.mjs 5                               # 5 repeats; needs console/dist built first
```

## Limitations

- All data is synthetic and the console is not wired to any real deployment, broker or
  database; see Honest framing.
- No authentication; this is a read-only internal console with no claim about access control.
- The chart redraws the full SVG path on every render rather than incrementally patching it;
  at 2,000 output points this is not the bottleneck the benchmark measures, but it would not
  scale to a dashboard with many simultaneous live-updating charts without further work.
- The P&L drill-down's synthetic violations are generated per (fund, date) from a hash-seeded
  RNG, not from a real rule engine run; the record shape (source file, source row, rule name)
  mirrors `investment-data-quality-lineage-gate`'s real `Violation` shape but the values are not
  computed by that engine.

## Sources

Steinarsson, S. "Downsampling Time Series for Visual Representation." MSc thesis, University of
Iceland, 2013. https://skemman.is/bitstream/1946/15343/3/SS_MSthesis.pdf
