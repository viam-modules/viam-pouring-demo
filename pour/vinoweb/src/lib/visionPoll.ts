/**
 * Module-level vision pollers.
 *
 * Two independent serial loops so SAM stills and cup PCD never block each other.
 * Single-flight per stream avoids remount/HMR stacking past Viam's 100 limit.
 */

export type VisionPollHandlers = {
  shouldPollCup: () => boolean;
  shouldPollStills: () => boolean;
  fetchCup: () => Promise<void>;
  captureStill: (index: number) => Promise<void>;
  stillCooldownMs: number;
  cupCooldownMs: number;
};

let handlersRef: VisionPollHandlers | null = null;
let stopRequested = false;
let stillLoopRunning = false;
let cupLoopRunning = false;

let cupInFlight: Promise<void> | null = null;
const stillInFlight: (Promise<void> | null)[] = [null, null];
const lastStillOkAt: (number | null)[] = [null, null];
let lastFullStillPairAt: number | null = null;

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function logLine(msg: string, color: string): void {
  console.info(`%c${msg}`, `color:${color};font-weight:700`);
}

async function runCupOnce(fetchCup: () => Promise<void>): Promise<number> {
  const t0 = performance.now();
  if (cupInFlight) {
    await cupInFlight.catch(() => {});
    return performance.now() - t0;
  }
  cupInFlight = (async () => {
    try {
      await fetchCup();
    } finally {
      cupInFlight = null;
    }
  })();
  await cupInFlight;
  return performance.now() - t0;
}

async function runStillOnce(
  index: number,
  captureStill: (i: number) => Promise<void>
): Promise<{ workMs: number; refreshIntervalMs: number | null }> {
  const t0 = performance.now();
  if (stillInFlight[index]) {
    await stillInFlight[index]!.catch(() => {});
    return { workMs: performance.now() - t0, refreshIntervalMs: null };
  }
  stillInFlight[index] = (async () => {
    try {
      await captureStill(index);
    } finally {
      stillInFlight[index] = null;
    }
  })();
  await stillInFlight[index];
  const workMs = performance.now() - t0;
  const now = performance.now();
  const prev = lastStillOkAt[index];
  lastStillOkAt[index] = now;
  return { workMs, refreshIntervalMs: prev == null ? null : now - prev };
}

async function stillLoop(): Promise<void> {
  if (stillLoopRunning) return;
  stillLoopRunning = true;
  logLine("[visionPoll] still loop started (SAM highlights only)", "#a0f");
  try {
    while (!stopRequested && handlersRef) {
      const h = handlersRef;
      if (!h.shouldPollStills()) {
        await sleep(400);
        continue;
      }
      const cycleT0 = performance.now();
      const parts: string[] = [];

      const left = await runStillOnce(0, h.captureStill);
      if (stopRequested || !handlersRef) break;
      parts.push(
        `L=${Math.round(left.workMs)}ms` +
          (left.refreshIntervalMs != null
            ? ` (Δrefresh=${Math.round(left.refreshIntervalMs)}ms)`
            : "")
      );

      const right = await runStillOnce(1, h.captureStill);
      if (stopRequested || !handlersRef) break;
      parts.push(
        `R=${Math.round(right.workMs)}ms` +
          (right.refreshIntervalMs != null
            ? ` (Δrefresh=${Math.round(right.refreshIntervalMs)}ms)`
            : "")
      );

      const pairAt = performance.now();
      if (lastFullStillPairAt != null) {
        parts.push(`pairInterval=${Math.round(pairAt - lastFullStillPairAt)}ms`);
      }
      lastFullStillPairAt = pairAt;

      if (h.stillCooldownMs > 0) {
        await sleep(h.stillCooldownMs);
        parts.push(`stillIdle=${h.stillCooldownMs}ms`);
      }

      logLine(
        `[latency] STILL E2E  ${Math.round(performance.now() - cycleT0)} ms  (${parts.join(" · ")})`,
        "#a0f"
      );
    }
  } finally {
    stillLoopRunning = false;
    logLine("[visionPoll] still loop stopped", "#a0f");
    if (handlersRef && !stopRequested) void stillLoop();
  }
}

async function cupLoop(): Promise<void> {
  if (cupLoopRunning) return;
  cupLoopRunning = true;
  logLine("[visionPoll] cup PCD loop started", "#06c");
  try {
    while (!stopRequested && handlersRef) {
      const h = handlersRef;
      if (!h.shouldPollCup()) {
        await sleep(400);
        continue;
      }
      const t0 = performance.now();
      const cupMs = await runCupOnce(h.fetchCup);
      if (stopRequested || !handlersRef) break;
      logLine(
        `[latency] CUP E2E  ${Math.round(performance.now() - t0)} ms  (rpc=${Math.round(cupMs)}ms · idle=${h.cupCooldownMs}ms)`,
        "#06c"
      );
      await sleep(h.cupCooldownMs);
    }
  } finally {
    cupLoopRunning = false;
    logLine("[visionPoll] cup PCD loop stopped", "#06c");
    if (handlersRef && !stopRequested) void cupLoop();
  }
}

/** Attach handlers and ensure both independent loops are running. */
export function acquireVisionPoll(handlers: VisionPollHandlers): () => void {
  handlersRef = handlers;
  stopRequested = false;
  void stillLoop();
  void cupLoop();

  return () => {
    if (handlersRef === handlers) {
      handlersRef = null;
      stopRequested = true;
    }
  };
}

export function disposeVisionPoll(): void {
  handlersRef = null;
  stopRequested = true;
}

if (import.meta.hot) {
  import.meta.hot.dispose(() => {
    disposeVisionPoll();
  });
}
