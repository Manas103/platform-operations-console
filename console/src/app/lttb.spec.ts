import { lttb, lttbReference } from './lttb';
import { Point } from './models';

function makeSeries(n: number, fn: (i: number) => number): Point[] {
  return Array.from({ length: n }, (_, i) => ({ x: i, y: fn(i) }));
}

describe('lttb', () => {
  it('returns the data unchanged when threshold >= length', () => {
    const data = makeSeries(10, (i) => i);
    expect(lttb(data, 20)).toEqual(data);
  });

  it('always keeps the first and last point', () => {
    const data = makeSeries(10000, (i) => Math.sin(i / 50));
    const out = lttb(data, 200);
    expect(out[0]).toEqual(data[0]);
    expect(out[out.length - 1]).toEqual(data[data.length - 1]);
  });

  it('returns exactly the requested number of points', () => {
    // The full 31,536,000-point (one year of 1Hz samples) case is measured by
    // bench/measure.mjs against the real built app in a real headless browser, not duplicated
    // here; this unit suite checks the same invariant at a size fast enough for every test run.
    const data = makeSeries(200_000, (i) => Math.sin(i / 1000));
    const out = lttb(data, 2000);
    expect(out.length).toBe(2000);
  });

  it('does not drop a single-sample spike (the "no spike dropped" claim)', () => {
    const n = 100_000;
    const data = makeSeries(n, () => 1.0); // flat baseline
    const spikeIndex = 54321;
    data[spikeIndex] = { x: spikeIndex, y: 500.0 }; // one sample, 500x the baseline

    const out = lttb(data, 1000);
    const spikeSurvived = out.some((p) => p.y === 500.0);
    expect(spikeSurvived).toBe(true);
  });

  it('does not drop a spike regardless of which bucket it lands in (sweep across the series)', () => {
    const n = 50_000;
    const threshold = 500;
    for (const spikeIndex of [1, 12345, 25000, 37654, n - 2]) {
      const data = makeSeries(n, () => 1.0);
      data[spikeIndex] = { x: spikeIndex, y: 999.0 };
      const out = lttb(data, threshold);
      const found = out.some((p) => p.y === 999.0);
      expect(found).withContext(`spike at index ${spikeIndex} was dropped`).toBe(true);
    }
  });

  it('matches the independently-written reference implementation exactly, point for point', () => {
    const sizes = [100, 1000, 10007, 50000];
    for (const n of sizes) {
      const data = makeSeries(n, (i) => Math.sin(i / 37) * 10 + (i % 97 === 0 ? 40 : 0));
      const threshold = 300;
      const fast = lttb(data, threshold);
      const reference = lttbReference(data, threshold);
      expect(fast).toEqual(reference);
    }
  });
});
