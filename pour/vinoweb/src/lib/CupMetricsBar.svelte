<script lang="ts">
  import type { CupDetectionMetrics } from "./types.js";

  interface Props {
    detectionMetrics?: CupDetectionMetrics | null;
  }

  let { detectionMetrics = null }: Props = $props();

  function fmt(n: number) {
    return Number.isFinite(n) ? n.toFixed(1) : "—";
  }
</script>

{#if detectionMetrics}
  <div class="metrics-bar">
    <p class="tol">
      Acceptance: <span class="mono">|Δ height|</span> and <span class="mono">|Δ width|</span> must be ≤
      <strong class="mono">±{fmt(detectionMetrics.toleranceMm)}</strong> mm
    </p>
    <div class="grid">
      <div class="dim">
        <div class="dim-title">Height <span class="unit">(mm)</span></div>
        <dl class="nums">
          <div class="pair">
            <dt>Required</dt>
            <dd class="mono">{fmt(detectionMetrics.expectedHeight)}</dd>
          </div>
          <div class="pair">
            <dt>Observed</dt>
            <dd class="mono">{fmt(detectionMetrics.observedHeight)}</dd>
          </div>
          <div class="pair delta" class:pass={detectionMetrics.heightPass} class:fail={!detectionMetrics.heightPass}>
            <dt>Δ</dt>
            <dd class="mono">{fmt(detectionMetrics.heightDelta)}</dd>
          </div>
        </dl>
      </div>
      <div class="dim">
        <div class="dim-title">Width <span class="unit">(mm)</span></div>
        <dl class="nums">
          <div class="pair">
            <dt>Required</dt>
            <dd class="mono">{fmt(detectionMetrics.expectedWidth)}</dd>
          </div>
          <div class="pair">
            <dt>Observed</dt>
            <dd class="mono">{fmt(detectionMetrics.observedWidth)}</dd>
          </div>
          <div class="pair delta" class:pass={detectionMetrics.widthPass} class:fail={!detectionMetrics.widthPass}>
            <dt>Δ</dt>
            <dd class="mono">{fmt(detectionMetrics.widthDelta)}</dd>
          </div>
        </dl>
      </div>
    </div>
  </div>
{:else}
  <div class="metrics-bar empty">
    <span class="muted">No cup in view — metrics appear when an object is detected.</span>
  </div>
{/if}

<style>
  .metrics-bar {
    flex-shrink: 0;
    background: #f4f4f4;
    border: 1px solid #e0e0e0;
    border-radius: 8px;
    padding: 8px 10px 10px;
    font-family: "IBM Plex Sans", system-ui, sans-serif;
    font-size: 0.75rem;
    color: #161616;
    line-height: 1.35;
  }

  .metrics-bar.empty {
    padding: 10px 12px;
    background: #f4f4f4;
    border-style: dashed;
  }

  .muted {
    color: #6f6f6f;
    font-size: 0.72rem;
  }

  .tol {
    margin: 0 0 8px;
    font-size: 0.68rem;
    color: #525252;
    letter-spacing: 0.02em;
  }

  .mono {
    font-family: "IBM Plex Mono", monospace;
    font-weight: 500;
  }

  .grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px 14px;
  }

  @media (max-width: 520px) {
    .grid {
      grid-template-columns: 1fr;
    }
  }

  .dim-title {
    font-weight: 600;
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: #393939;
    margin-bottom: 6px;
  }

  .unit {
    font-weight: 400;
    text-transform: none;
    letter-spacing: 0;
    color: #6f6f6f;
  }

  .nums {
    margin: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 8px 14px;
    align-items: baseline;
  }

  .pair {
    display: grid;
    gap: 2px;
  }

  .pair dt {
    margin: 0;
    font-size: 0.62rem;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: #6f6f6f;
  }

  .pair dd {
    margin: 0;
    font-size: 0.78rem;
    color: #161616;
  }

  .pair.delta.pass dd {
    color: #198038;
  }

  .pair.delta.fail dd {
    color: #da1e28;
  }
</style>
