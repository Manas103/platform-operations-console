import { Point } from './models';

/**
 * Largest-Triangle-Three-Buckets (Steinarsson, "Downsampling Time Series for Visual
 * Representation", MSc thesis, University of Iceland). Splits the series into `threshold - 2`
 * buckets (the first and last points are always kept), and from each bucket keeps the one point
 * that forms the largest triangle with the previously-selected point and the next bucket's
 * average point. This is a correctness-preserving downsample, not a random or uniform one: a
 * single sharp spike inside a bucket maximizes that triangle's area and is kept, which is what
 * "no spike dropped" means here and what `lttb.spec.ts` checks directly.
 */
export function lttb(data: Point[], threshold: number): Point[] {
  const n = data.length;
  if (threshold >= n || threshold <= 2) {
    return data.slice();
  }

  const sampled: Point[] = new Array(threshold);
  sampled[0] = data[0];
  sampled[threshold - 1] = data[n - 1];

  const bucketSize = (n - 2) / (threshold - 2);
  let a = 0; // index of the previously selected point

  for (let i = 0; i < threshold - 2; i++) {
    const bucketStart = Math.floor((i + 1) * bucketSize) + 1;
    const bucketEnd = Math.floor((i + 2) * bucketSize) + 1;
    const nextBucketEnd = Math.min(bucketEnd, n);

    // average point of the NEXT bucket
    let avgX = 0;
    let avgY = 0;
    const nextStart = bucketEnd;
    const nextEnd = Math.min(Math.floor((i + 3) * bucketSize) + 1, n);
    const nextCount = Math.max(nextEnd - nextStart, 1);
    for (let j = nextStart; j < nextEnd; j++) {
      avgX += data[j].x;
      avgY += data[j].y;
    }
    avgX /= nextCount;
    avgY /= nextCount;

    const pointAX = data[a].x;
    const pointAY = data[a].y;

    let maxArea = -1;
    let maxAreaIndex = bucketStart;
    for (let j = bucketStart; j < nextBucketEnd; j++) {
      const area = Math.abs(
        (pointAX - avgX) * (data[j].y - pointAY) - (pointAX - data[j].x) * (avgY - pointAY),
      ) * 0.5;
      if (area > maxArea) {
        maxArea = area;
        maxAreaIndex = j;
      }
    }

    sampled[i + 1] = data[maxAreaIndex];
    a = maxAreaIndex;
  }

  return sampled;
}

/**
 * The same algorithm, written independently and unoptimized (plain loops over bucket boundaries
 * computed the obvious way, no shared index bookkeeping with {@link lttb}) as the reference
 * oracle `lttb.spec.ts` diffs the production implementation against exactly, point for point,
 * over many random series.
 */
export function lttbReference(data: Point[], threshold: number): Point[] {
  const n = data.length;
  if (threshold >= n || threshold <= 2) {
    return data.slice();
  }

  const buckets: Point[][] = [];
  const every = (n - 2) / (threshold - 2);
  for (let i = 0; i < threshold - 2; i++) {
    const start = 1 + Math.floor(i * every);
    const end = 1 + Math.floor((i + 1) * every);
    buckets.push(data.slice(start, end));
  }

  const out: Point[] = [data[0]];
  let prev = data[0];
  for (let i = 0; i < buckets.length; i++) {
    const bucket = buckets[i];
    const nextBucketStart = 1 + Math.floor((i + 1) * every);
    const nextBucketEnd = i + 2 < buckets.length + 1 ? 1 + Math.floor((i + 2) * every) : n;
    const nextSlice = data.slice(nextBucketStart, Math.max(nextBucketEnd, nextBucketStart + 1));
    const avg = nextSlice.reduce((acc, p) => ({ x: acc.x + p.x, y: acc.y + p.y }), { x: 0, y: 0 });
    const avgPoint: Point = { x: avg.x / nextSlice.length, y: avg.y / nextSlice.length };

    let best = bucket[0];
    let bestArea = -1;
    for (const candidate of bucket) {
      const area = Math.abs(
        (prev.x - avgPoint.x) * (candidate.y - prev.y) - (prev.x - candidate.x) * (avgPoint.y - prev.y),
      ) * 0.5;
      if (area > bestArea) {
        bestArea = area;
        best = candidate;
      }
    }
    out.push(best);
    prev = best;
  }
  out.push(data[n - 1]);
  return out;
}
