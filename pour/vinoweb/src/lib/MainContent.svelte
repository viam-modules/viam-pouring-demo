<script lang="ts">
  import type { Snippet } from "svelte";
  import PointCloud3D from "./PointCloud3D.svelte";
  import CameraFeed from "./CameraFeed.svelte";
  import CupMetricsBar from "./CupMetricsBar.svelte";
  import { pcInvalidPillRgba, pcValidPillRgba } from "./pcGradientColors.js";
  import type { Joint, SegmentedObject, CupDetectionMetrics } from "./types.js";

  interface Props {
    statusBar: Snippet;
    segmentedObjects: SegmentedObject[];
    leftJoints: Joint[];
    rightJoints: Joint[];
    status: string;
    cupHeightMm?: number;
    cupWidthMm?: number;
    cupDetectionMetrics: CupDetectionMetrics | null;
  }

  let {
    statusBar,
    segmentedObjects,
    leftJoints,
    rightJoints,
    status,
    cupHeightMm = 0,
    cupWidthMm = 0,
    cupDetectionMetrics = null,
  }: Props = $props();

  /** Stats panel below the viewer; closed by default so the canvas keeps space */
  let statsExpanded = $state(false);

  const detectionStatuses = new Set(["manual mode", "standby", "looking"]);
  const demoActive = $derived(!detectionStatuses.has(status));
</script>

<main class="main-content">
  <header class="status-bar">
    {@render statusBar()}
  </header>

  {#if demoActive}
    <section class="content-area cameras-only">
      <div class="cam-area cam-full-top">
        <CameraFeed name="left-cam" partID="xxx" label="Left Camera" />
      </div>
      <div class="cam-area cam-full-bottom">
        <CameraFeed name="right-cam" partID="xxx" label="Right Camera" />
      </div>
    </section>
  {:else}
    <section class="content-area debug-grid">
      <div class="pcd-area">
        <div class="pcd-view-wrap">
          <PointCloud3D objects={segmentedObjects} {cupHeightMm} {cupWidthMm} />
          {#if cupDetectionMetrics}
            <button
              type="button"
              class="validity-pill"
              class:valid={cupDetectionMetrics.valid}
              class:invalid={!cupDetectionMetrics.valid}
              style:background={cupDetectionMetrics.valid ? pcValidPillRgba() : pcInvalidPillRgba()}
              onclick={() => (statsExpanded = !statsExpanded)}
              aria-expanded={statsExpanded}
              aria-controls="cup-stats-panel"
              title={statsExpanded ? "Hide measurement details" : "Show measurement details"}
            >
              <span class="pill-caret" aria-hidden="true">{statsExpanded ? "▲" : "▼"}</span>
              <span class="pill-label">{cupDetectionMetrics.valid ? "Valid" : "Invalid"}</span>
            </button>
          {/if}
        </div>
        {#if cupDetectionMetrics && statsExpanded}
          <div id="cup-stats-panel" class="cup-stats-panel">
            <CupMetricsBar detectionMetrics={cupDetectionMetrics} />
          </div>
        {/if}
      </div>
      <div class="cam-area cam-top">
        <CameraFeed name="left-cam" partID="xxx" label="Left Camera" />
      </div>
      <div class="table-area">
        <table class="joint-table">
          <thead>
            <tr>
              <th>Joint</th>
              <th>Left Arm (°)</th>
              <th>Right Arm (°)</th>
            </tr>
          </thead>
          <tbody>
            {#each leftJoints as _, i}
              <tr>
                <td class="joint-idx">{i}</td>
                <td>{leftJoints[i]?.position.toFixed(2) ?? "—"}</td>
                <td>{rightJoints[i]?.position.toFixed(2) ?? "—"}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      <div class="cam-area cam-bottom">
        <CameraFeed name="right-cam" partID="xxx" label="Right Camera" />
      </div>
    </section>
  {/if}
</main>

<style>
  .main-content {
    background-color: white;
    display: flex;
    flex-direction: column;
    gap: 10px;
    height: 90%;
    width: 95%;
    border-radius: 12px;
    overflow: hidden;
    padding: 20px;
    margin: auto 0;
  }

  .status-bar {
    display: flex;
    align-items: center;
    flex-shrink: 0;
  }

  .content-area {
    flex: 1;
    min-height: 0;
    background: #ddd;
    border-radius: 16px;
    padding: 15px;
    overflow: hidden;
  }

  .content-area.cameras-only {
    display: grid;
    grid-template-rows: 1fr 1fr;
    gap: 12px;
    align-items: stretch;
  }

  .cam-full-top { grid-row: 1; }
  .cam-full-bottom { grid-row: 2; }

  .content-area.debug-grid {
    display: grid;
    grid-template-columns: minmax(0, 35%) minmax(0, 1fr);
    grid-template-rows: 1fr 1fr;
    gap: 12px;
    align-items: stretch;
  }

  .pcd-area {
    grid-column: 1;
    grid-row: 1;
    min-height: 0;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
    overflow: hidden;
  }

  .pcd-view-wrap {
    position: relative;
    flex: 1;
    min-height: 0;
    border-radius: 8px;
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }

  .pcd-view-wrap :global(.pcd-container) {
    flex: 1;
    min-height: 0;
  }

  .validity-pill {
    position: absolute;
    top: 12px;
    right: 12px;
    z-index: 10;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    margin: 0;
    padding: 0 12px 0 10px;
    height: 26px;
    line-height: 1;
    font-size: 0.8125rem;
    font-weight: 600;
    letter-spacing: 0.04em;
    border: none;
    border-radius: 15px;
    cursor: pointer;
    font-family: inherit;
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.2);
    transition: filter 0.15s ease, transform 0.1s ease;
  }

  .validity-pill:hover {
    filter: brightness(1.05);
  }

  .validity-pill:active {
    transform: scale(0.98);
  }

  .validity-pill:focus-visible {
    outline: 2px solid #4589ff;
    outline-offset: 2px;
  }

  .pill-caret {
    font-size: 0.65rem;
    opacity: 0.95;
    line-height: 1;
  }

  .pill-label {
    line-height: 1;
  }

  /* Text contrast on gradient t=1 fills (see pcGradientColors) */
  /* Carbon `Tag` green text on white theme (`bx--tag--green`) */
  .validity-pill.valid {
    color: #044317;
  }

  .validity-pill.invalid {
    color: #3b1216;
  }

  .cup-stats-panel {
    flex-shrink: 0;
  }

  .cam-top {
    grid-column: 2;
    grid-row: 1;
  }

  .table-area {
    grid-column: 1;
    grid-row: 2;
    min-height: 0;
    min-width: 0;
    overflow: auto;
    align-self: stretch;
  }

  .cam-bottom {
    grid-column: 2;
    grid-row: 2;
  }

  .joint-table {
    border-collapse: collapse;
    width: 100%;
    background: #fff;
    border-radius: 8px;
    overflow: hidden;
    font-family: "IBM Plex Mono", monospace;
    font-size: 0.8rem;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  }

  .joint-table th,
  .joint-table td {
    padding: 5px 10px;
    text-align: left;
  }

  .joint-table th {
    background: #f5f5f5;
    font-weight: 600;
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: #555;
  }

  .joint-table tr:not(:last-child) td {
    border-bottom: 1px solid #eee;
  }

  .joint-table .joint-idx {
    font-weight: 600;
    color: #888;
  }

  .cam-area {
    min-height: 0;
    min-width: 0;
    display: flex;
    justify-content: center;
    align-items: center;
  }
</style>
