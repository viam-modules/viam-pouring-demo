/**
 * Browser-console latency reporter for vision RPCs.
 * Uses console.info so Default log-level filters still show these lines
 * (console.debug is hidden unless Verbose is enabled).
 */

export type LatencySample = {
  label: string;
  ms: number;
  ok: boolean;
  detail?: string;
};

type Bucket = {
  n: number;
  sum: number;
  min: number;
  max: number;
  lastMs: number;
  lastOk: boolean;
  fails: number;
};

const buckets = new Map<string, Bucket>();
const SUMMARY_EVERY = 5;

function bucket(label: string): Bucket {
  let b = buckets.get(label);
  if (!b) {
    b = { n: 0, sum: 0, min: Infinity, max: 0, lastMs: 0, lastOk: true, fails: 0 };
    buckets.set(label, b);
  }
  return b;
}

/** Log one timed call and periodically log rolling min/avg/max. */
export function reportLatency(sample: LatencySample): void {
  const ms = Math.round(sample.ms);
  const status = sample.ok ? "ok" : "FAIL";
  const detail = sample.detail ? ` · ${sample.detail}` : "";

  // One row per RPC — easy to copy/paste for a latency report.
  console.info(
    `%c[latency] ${sample.label}  ${ms} ms  ${status}${detail}`,
    sample.ok ? "color:#0a0;font-weight:600" : "color:#c00;font-weight:600"
  );

  const b = bucket(sample.label);
  b.n += 1;
  b.sum += sample.ms;
  b.min = Math.min(b.min, sample.ms);
  b.max = Math.max(b.max, sample.ms);
  b.lastMs = sample.ms;
  b.lastOk = sample.ok;
  if (!sample.ok) b.fails += 1;

  if (b.n % SUMMARY_EVERY === 0) {
    const avg = Math.round(b.sum / b.n);
    console.info(
      `%c[latency] ${sample.label} summary (n=${b.n})  ` +
        `min=${Math.round(b.min)} ms  avg=${avg} ms  max=${Math.round(b.max)} ms  fails=${b.fails}`,
      "color:#06c;font-weight:600"
    );
  }
}

/** Wrap an async RPC so success/failure always produce a latency line. */
export async function timeVisionCall<T>(
  label: string,
  fn: () => Promise<T>,
  detailOnOk?: (result: T) => string
): Promise<T> {
  const t0 = performance.now();
  try {
    const result = await fn();
    reportLatency({
      label,
      ms: performance.now() - t0,
      ok: true,
      detail: detailOnOk?.(result),
    });
    return result;
  } catch (err) {
    const msg =
      err instanceof Error
        ? err.message.split("\n")[0].slice(0, 160)
        : String(err).slice(0, 160);
    reportLatency({
      label,
      ms: performance.now() - t0,
      ok: false,
      detail: msg,
    });
    throw err;
  }
}
