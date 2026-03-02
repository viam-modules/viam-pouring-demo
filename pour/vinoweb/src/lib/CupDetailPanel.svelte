<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { Tag } from "carbon-components-svelte";

  export interface CupDetail {
    index: number;
    valid: boolean;
    height: number;
    expected_height: number;
    height_delta: number;
    height_pass: boolean;
    width: number;
    expected_width: number;
    width_delta: number;
    width_pass: boolean;
    good_delta: number;
    total_points: number;
    points_x: number[];
    points_y: number[];
    points_z: number[];
  }

  let {
    cups = [],
    onClose,
  }: { cups: CupDetail[]; onClose: () => void } = $props();

  const ROT_SPEED = 0.3;
  const PITCH = (Math.PI / 180) * 30;
  const COS_PITCH = Math.cos(PITCH);
  const SIN_PITCH = Math.sin(PITCH);
  const PADDING = 24;

  let canvasRefs: (HTMLCanvasElement | undefined)[] = $state([]);
  let animId: number | null = null;

  interface ScaleInfo {
    scale: number;
    minZ: number;
    rangeZ: number;
    cx: number;
    cy: number;
    cz: number;
  }

  let scaleCache = new Map<string, ScaleInfo>();

  function scaleKey(cup: CupDetail): string {
    const px = cup.points_x;
    const py = cup.points_y;
    return `${cup.index}:${px.length}:${px[0]}:${py[0]}:${px[px.length - 1]}`;
  }

  function computeScale(cup: CupDetail, w: number, h: number): ScaleInfo {
    const px = cup.points_x;
    const py = cup.points_y;
    const pz = cup.points_z;
    const n = px.length;

    let cx = 0, cy = 0, cz = 0;
    let minZ = Infinity, maxZ = -Infinity;
    for (let j = 0; j < n; j++) {
      cx += px[j]; cy += py[j]; cz += pz[j];
      if (pz[j] < minZ) minZ = pz[j];
      if (pz[j] > maxZ) maxZ = pz[j];
    }
    cx /= n; cy /= n; cz /= n;

    let maxHalfX = 0, maxHalfY = 0;
    for (let sampleYaw = 0; sampleYaw < Math.PI * 2; sampleYaw += Math.PI / 8) {
      const c = Math.cos(sampleYaw);
      const s = Math.sin(sampleYaw);

      let frameCx = 0, frameCy = 0;
      for (let j = 0; j < n; j++) {
        const dx = px[j] - cx;
        const dy = py[j] - cy;
        const dz = pz[j] - cz;
        const rx = dx * c - dy * s;
        const ry = dx * s + dy * c;
        frameCx += rx;
        frameCy += -dz * COS_PITCH + ry * SIN_PITCH;
      }
      frameCx /= n;
      frameCy /= n;

      for (let j = 0; j < n; j++) {
        const dx = px[j] - cx;
        const dy = py[j] - cy;
        const dz = pz[j] - cz;
        const rx = dx * c - dy * s;
        const ry = dx * s + dy * c;
        const sx = rx;
        const sy = -dz * COS_PITCH + ry * SIN_PITCH;
        const halfX = Math.abs(sx - frameCx);
        const halfY = Math.abs(sy - frameCy);
        if (halfX > maxHalfX) maxHalfX = halfX;
        if (halfY > maxHalfY) maxHalfY = halfY;
      }
    }

    maxHalfX = maxHalfX || 1;
    maxHalfY = maxHalfY || 1;
    const scale = Math.min(
      (w / 2 - PADDING) / maxHalfX,
      (h / 2 - PADDING) / maxHalfY,
    );
    const rangeZ = maxZ - minZ || 1;

    return { scale, minZ, rangeZ, cx, cy, cz };
  }

  function renderAllCanvases() {
    const yaw = (performance.now() / 1000) * ROT_SPEED;
    const cosYaw = Math.cos(yaw);
    const sinYaw = Math.sin(yaw);

    for (let i = 0; i < cups.length; i++) {
      const canvas = canvasRefs[i];
      if (!canvas) continue;
      const ctx = canvas.getContext("2d");
      if (!ctx) continue;
      const cup = cups[i];
      if (!cup) continue;

      const w = canvas.width;
      const h = canvas.height;

      if (cup.points_x.length === 0) {
        ctx.fillStyle = "#1a1a2e";
        ctx.fillRect(0, 0, w, h);
        ctx.fillStyle = "#525252";
        ctx.font = "12px IBM Plex Mono, monospace";
        ctx.textAlign = "center";
        ctx.fillText("No points", w / 2, h / 2);
        continue;
      }

      const key = scaleKey(cup);
      let info = scaleCache.get(key);
      if (!info) {
        info = computeScale(cup, w, h);
        scaleCache.set(key, info);
        if (scaleCache.size > 20) {
          const first = scaleCache.keys().next().value!;
          scaleCache.delete(first);
        }
      }

      renderFrame(ctx, w, h, cup, info, yaw, cosYaw, sinYaw);
    }

    animId = requestAnimationFrame(renderAllCanvases);
  }

  function renderFrame(
    ctx: CanvasRenderingContext2D,
    w: number, h: number,
    cup: CupDetail,
    info: ScaleInfo,
    yaw: number, cosYaw: number, sinYaw: number,
  ) {
    const { scale, minZ, rangeZ, cx, cy, cz } = info;
    const px = cup.points_x;
    const py = cup.points_y;
    const pz = cup.points_z;

    ctx.fillStyle = "#1a1a2e";
    ctx.fillRect(0, 0, w, h);

    const projected = new Array(px.length);
    let frameCx = 0, frameCy = 0;
    for (let i = 0; i < px.length; i++) {
      const dx = px[i] - cx;
      const dy = py[i] - cy;
      const dz = pz[i] - cz;
      const rx = dx * cosYaw - dy * sinYaw;
      const ry = dx * sinYaw + dy * cosYaw;
      const sx = rx;
      const sy = -dz * COS_PITCH + ry * SIN_PITCH;
      projected[i] = { sx, sy, z: pz[i], depth: ry };
      frameCx += sx;
      frameCy += sy;
    }
    frameCx /= px.length;
    frameCy /= px.length;

    projected.sort((a: any, b: any) => a.depth - b.depth);

    for (const p of projected) {
      const screenX = (p.sx - frameCx) * scale + w / 2;
      const screenY = (p.sy - frameCy) * scale + h / 2;

      const t = (p.z - minZ) / rangeZ;
      const r = Math.round(30 + t * 60);
      const g = Math.round(120 + t * 135);
      const b = Math.round(220 - t * 140);

      ctx.fillStyle = `rgb(${r}, ${g}, ${b})`;
      ctx.beginPath();
      ctx.arc(screenX, screenY, 2, 0, Math.PI * 2);
      ctx.fill();
    }

    const axLen = 20;
    const axOx = PADDING;
    const axOy = h - PADDING;

    ctx.strokeStyle = "#fa4d56";
    ctx.beginPath(); ctx.moveTo(axOx, axOy); ctx.lineTo(axOx + axLen * cosYaw, axOy + axLen * SIN_PITCH * sinYaw); ctx.stroke();
    ctx.strokeStyle = "#42be65";
    ctx.beginPath(); ctx.moveTo(axOx, axOy); ctx.lineTo(axOx - axLen * sinYaw, axOy + axLen * SIN_PITCH * cosYaw); ctx.stroke();
    ctx.strokeStyle = "#4589ff";
    ctx.beginPath(); ctx.moveTo(axOx, axOy); ctx.lineTo(axOx, axOy - axLen * COS_PITCH); ctx.stroke();

    ctx.font = "9px IBM Plex Mono";
    ctx.fillStyle = "#fa4d56";
    ctx.fillText("X", axOx + axLen * cosYaw + 4, axOy + axLen * SIN_PITCH * sinYaw);
    ctx.fillStyle = "#42be65";
    ctx.fillText("Y", axOx - axLen * sinYaw - 8, axOy + axLen * SIN_PITCH * cosYaw);
    ctx.fillStyle = "#4589ff";
    ctx.fillText("Z", axOx + 4, axOy - axLen * COS_PITCH - 2);
  }

  function fmt(v: number): string {
    return v.toFixed(1);
  }

  onMount(() => {
    animId = requestAnimationFrame(renderAllCanvases);
  });

  onDestroy(() => {
    if (animId !== null) cancelAnimationFrame(animId);
  });
</script>

<div class="panel-container">
  <div class="panel-header">
    <span class="panel-title">Cup Detection Details</span>
    <button class="close-btn" onclick={onClose} aria-label="Close panel">
      <svg viewBox="0 0 16 16" fill="currentColor" width="16" height="16">
        <path d="M12 4.7L11.3 4 8 7.3 4.7 4 4 4.7 7.3 8 4 11.3l.7.7L8 8.7l3.3 3.3.7-.7L8.7 8z"/>
      </svg>
    </button>
  </div>

  {#if cups.length === 0}
    <div class="empty-state">No objects detected</div>
  {:else}
    <div class="cups-grid">
      {#each cups as cup, i (cup.index)}
        <div class="cup-card" class:valid={cup.valid} class:invalid={!cup.valid}>
          <div class="card-header">
            <span class="cup-label">Object {cup.index}</span>
            <Tag type={cup.valid ? "green" : "red"}>
              {cup.valid ? "VALID" : "INVALID"}
            </Tag>
          </div>

          <div class="card-body">
            <div class="pointcloud-wrapper">
              <canvas
                bind:this={canvasRefs[i]}
                width="500"
                height="500"
                class="pointcloud-canvas"
              ></canvas>
              <span class="point-count">{cup.total_points} pts</span>
            </div>

            <div class="constraints">
              <div class="constraint-row" class:pass={cup.height_pass} class:fail={!cup.height_pass}>
                <div class="constraint-header">
                  <span class="constraint-icon">{cup.height_pass ? "✓" : "✗"}</span>
                  <span class="constraint-name">Height</span>
                </div>
                <div class="constraint-detail">
                  <span class="constraint-value">{fmt(cup.height)}mm</span>
                  <span class="constraint-expected">expected {fmt(cup.expected_height)}mm</span>
                </div>
                <div class="constraint-delta">
                  Δ {fmt(cup.height_delta)}mm
                  {#if !cup.height_pass}
                    <span class="threshold-note">exceeds {fmt(cup.good_delta)}mm threshold</span>
                  {/if}
                </div>
              </div>

              <div class="constraint-row" class:pass={cup.width_pass} class:fail={!cup.width_pass}>
                <div class="constraint-header">
                  <span class="constraint-icon">{cup.width_pass ? "✓" : "✗"}</span>
                  <span class="constraint-name">Width</span>
                </div>
                <div class="constraint-detail">
                  <span class="constraint-value">{fmt(cup.width)}mm</span>
                  <span class="constraint-expected">expected {fmt(cup.expected_width)}mm</span>
                </div>
                <div class="constraint-delta">
                  Δ {fmt(cup.width_delta)}mm
                  {#if !cup.width_pass}
                    <span class="threshold-note">exceeds {fmt(cup.good_delta)}mm threshold</span>
                  {/if}
                </div>
              </div>
            </div>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .panel-container {
    background: #1a1a1a;
    border: 1px solid #393939;
    border-radius: 8px;
    overflow: hidden;
    font-family: "IBM Plex Mono", monospace;
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: 100%;
  }

  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 16px;
    border-bottom: 1px solid #333;
    flex-shrink: 0;
  }

  .panel-title {
    color: #c6c6c6;
    font-size: 0.8rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }

  .close-btn {
    background: none;
    border: none;
    color: #a8a8a8;
    cursor: pointer;
    padding: 4px;
    border-radius: 4px;
    display: flex;
    align-items: center;
  }
  .close-btn:hover {
    background: #333;
    color: #fff;
  }

  .empty-state {
    padding: 24px;
    text-align: center;
    color: #6f6f6f;
    font-size: 0.85rem;
  }

  .cups-grid {
    padding: 12px;
    display: flex;
    flex-direction: row;
    gap: 12px;
    overflow-x: auto;
    overflow-y: hidden;
    flex: 1;
    min-height: 0;
  }

  .cup-card {
    background: #222;
    border-radius: 6px;
    border-top: 3px solid #525252;
    overflow: hidden;
    flex: 0 0 calc(50% - 6px);
    min-width: 280px;
    display: flex;
    flex-direction: column;
  }
  .cup-card.valid {
    border-top-color: #42be65;
  }
  .cup-card.invalid {
    border-top-color: #fa4d56;
  }

  .card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 12px;
    border-bottom: 1px solid #333;
    flex-shrink: 0;
  }

  .cup-label {
    color: #e0e0e0;
    font-size: 0.8rem;
    font-weight: 600;
  }

  .card-header :global(.bx--tag) {
    margin: 0;
    font-size: 0.7rem;
  }

  .card-body {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 12px;
    flex: 1;
    min-height: 0;
  }

  .pointcloud-wrapper {
    position: relative;
    flex: 1;
    min-height: 0;
  }

  .pointcloud-canvas {
    border-radius: 4px;
    display: block;
    width: 100%;
    height: 100%;
    object-fit: contain;
  }

  .point-count {
    position: absolute;
    bottom: 4px;
    right: 6px;
    font-size: 0.65rem;
    color: #6f6f6f;
  }

  .constraints {
    display: flex;
    flex-direction: column;
    gap: 8px;
    flex-shrink: 0;
  }

  .constraint-row {
    padding: 8px 10px;
    border-radius: 4px;
    background: #2a2a2a;
  }
  .constraint-row.pass {
    border-left: 2px solid #42be65;
  }
  .constraint-row.fail {
    border-left: 2px solid #fa4d56;
    background: #2a1f1f;
  }

  .constraint-header {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 4px;
  }

  .constraint-icon {
    font-size: 0.85rem;
  }
  .pass .constraint-icon {
    color: #42be65;
  }
  .fail .constraint-icon {
    color: #fa4d56;
  }

  .constraint-name {
    color: #e0e0e0;
    font-size: 0.8rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .constraint-detail {
    display: flex;
    gap: 8px;
    align-items: baseline;
  }

  .constraint-value {
    color: #f4f4f4;
    font-size: 0.85rem;
    font-weight: 600;
  }

  .constraint-expected {
    color: #6f6f6f;
    font-size: 0.75rem;
  }

  .constraint-delta {
    color: #a8a8a8;
    font-size: 0.75rem;
    margin-top: 2px;
  }
  .fail .constraint-delta {
    color: #fa4d56;
  }

  .threshold-note {
    color: #fa4d56;
    font-style: italic;
    margin-left: 4px;
  }
</style>
