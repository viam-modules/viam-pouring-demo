<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { CameraClient } from "@viamrobotics/sdk";
  import type { RobotClient } from "@viamrobotics/sdk";
  import { parsePCD } from "./parsePCD.js";

  export interface SegmentedObject {
    index: number;
    totalPoints: number;
    points_x: number[];
    points_y: number[];
    points_z: number[];
    rawPCD: Uint8Array;
    dims?: { x: number; y: number; z: number };
    position?: { x: number; y: number; z: number };
  }

  let {
    objects = [],
    robotClient = null,
    onClose,
  }: {
    objects: SegmentedObject[];
    robotClient: RobotClient | null;
    onClose: () => void;
  } = $props();

  // --- Hardcoded cup dimensions (mm) ---
  const CUP_HEIGHT = 109;

  // Stemless wine glass profile rings: circumference → radius = C / (2π)
  const RINGS = [
    { label: "Base",  circumference: 200, heightFromFloor: 0,          color: "rgba(232,160,245,0.6)" },
    { label: "Belly", circumference: 270, heightFromFloor: 45,         color: "rgba(213,128,232,0.7)" },
    { label: "Rim",   circumference: 220, heightFromFloor: CUP_HEIGHT, color: "rgba(240,192,255,0.6)" },
  ].map(r => ({ ...r, radius: r.circumference / (2 * Math.PI) }));

  const CUP_COLOR = "#e8a0f5";
  const ROT_SPEED = 0.3;
  const PITCH = (Math.PI / 180) * 30;
  const COS_PITCH = Math.cos(PITCH);
  const SIN_PITCH = Math.sin(PITCH);
  const PAD = 24;

  let canvasRefs: (HTMLCanvasElement | undefined)[] = $state([]);
  let leftOverlayRefs: (HTMLCanvasElement | undefined)[] = $state([]);
  let rightOverlayRefs: (HTMLCanvasElement | undefined)[] = $state([]);
  let animId: number | null = null;

  let leftCamImage: ImageBitmap | null = null;
  let rightCamImage: ImageBitmap | null = null;
  let leftCamStatus: string[] = ["Waiting for robot client..."];
  let rightCamStatus: string[] = ["Waiting for robot client..."];
  let leftCamClient: CameraClient | null = null;
  let rightCamClient: CameraClient | null = null;
  let camFetchInterval: ReturnType<typeof setInterval> | null = null;

  interface CamIntrinsics {
    width: number; height: number;
    fx: number; fy: number;
    cx: number; cy: number;
  }
  let leftIntrinsics: CamIntrinsics | null = null;
  let rightIntrinsics: CamIntrinsics | null = null;
  let intrinsicsFetched = false;

  let leftFramePoints: Map<number, { x: number[]; y: number[]; z: number[] }> = new Map();
  let rightFramePoints: Map<number, { x: number[]; y: number[]; z: number[] }> = new Map();

  interface ScaleInfo {
    scale: number; minZ: number; maxZ: number; rangeZ: number;
    cx: number; cy: number; cz: number;
  }
  let scaleCache = new Map<string, ScaleInfo>();

  function scaleKey(obj: SegmentedObject): string {
    const px = obj.points_x, py = obj.points_y;
    return `${obj.index}:${px.length}:${px[0]}:${py[0]}:${px[px.length - 1]}`;
  }

  function computeScale(obj: SegmentedObject, w: number, h: number): ScaleInfo {
    const px = obj.points_x, py = obj.points_y, pz = obj.points_z;
    const n = px.length;
    let cx = 0, cy = 0, cz = 0;
    let minZ = Infinity, maxZ = -Infinity;
    for (let j = 0; j < n; j++) {
      cx += px[j]; cy += py[j]; cz += pz[j];
      if (pz[j] < minZ) minZ = pz[j];
      if (pz[j] > maxZ) maxZ = pz[j];
    }
    cx /= n; cy /= n; cz /= n;

    const maxRingRadius = Math.max(...RINGS.map(r => r.radius));
    let maxHalfX = maxRingRadius, maxHalfY = 0;

    for (let a = 0; a < Math.PI * 2; a += Math.PI / 8) {
      const ca = Math.cos(a), sa = Math.sin(a);
      for (let j = 0; j < n; j++) {
        const dx = px[j] - cx, dy = py[j] - cy, dz = pz[j] - cz;
        const hx = Math.abs(dx * ca - dy * sa);
        const hy = Math.abs(-dz * COS_PITCH + (dx * sa + dy * ca) * SIN_PITCH);
        if (hx > maxHalfX) maxHalfX = hx;
        if (hy > maxHalfY) maxHalfY = hy;
      }
      for (const ring of RINGS) {
        const ringDz = (minZ + ring.heightFromFloor) - cz;
        const hy = Math.abs(-ringDz * COS_PITCH + ring.radius * SIN_PITCH);
        if (hy > maxHalfY) maxHalfY = hy;
      }
    }
    maxHalfX = maxHalfX || 1;
    maxHalfY = maxHalfY || 1;
    const zoomPad = PAD + 16;
    const scale = Math.min((w / 2 - zoomPad) / maxHalfX, (h / 2 - zoomPad) / maxHalfY);
    return { scale, minZ, maxZ, rangeZ: (maxZ - minZ) || 1, cx, cy, cz };
  }

  function renderAllCanvases() {
    const yaw = (performance.now() / 1000) * ROT_SPEED;
    const cosYaw = Math.cos(yaw);
    const sinYaw = Math.sin(yaw);

    for (let i = 0; i < objects.length; i++) {
      const canvas = canvasRefs[i];
      if (!canvas) continue;
      const ctx = canvas.getContext("2d");
      if (!ctx) continue;
      const obj = objects[i];
      if (!obj || obj.points_x.length === 0) {
        ctx.fillStyle = "#1a1a2e";
        ctx.fillRect(0, 0, canvas.width, canvas.height);
        ctx.fillStyle = "#525252";
        ctx.font = "12px IBM Plex Mono, monospace";
        ctx.textAlign = "center";
        ctx.fillText("No points", canvas.width / 2, canvas.height / 2);
        continue;
      }

      const key = scaleKey(obj);
      let info = scaleCache.get(key);
      if (!info) {
        info = computeScale(obj, canvas.width, canvas.height);
        scaleCache.set(key, info);
        if (scaleCache.size > 20) scaleCache.delete(scaleCache.keys().next().value!);
      }

      render3DView(ctx, canvas.width, canvas.height, obj, info, cosYaw, sinYaw);
      renderCameraOverlay(leftOverlayRefs[i], obj, leftCamImage, leftIntrinsics, leftFramePoints.get(obj.index), leftCamStatus, "Left");
      renderCameraOverlay(rightOverlayRefs[i], obj, rightCamImage, rightIntrinsics, rightFramePoints.get(obj.index), rightCamStatus, "Right");
    }
    animId = requestAnimationFrame(renderAllCanvases);
  }

  function render3DView(
    ctx: CanvasRenderingContext2D, w: number, h: number,
    obj: SegmentedObject, info: ScaleInfo,
    cosYaw: number, sinYaw: number,
  ) {
    const { scale, minZ, rangeZ, cx, cy, cz } = info;
    const px = obj.points_x, py = obj.points_y, pz = obj.points_z;

    ctx.fillStyle = "#1a1a2e";
    ctx.fillRect(0, 0, w, h);

    // Compute projected centroid
    let fCx = 0, fCy = 0;
    for (let i = 0; i < px.length; i++) {
      const dx = px[i] - cx, dy = py[i] - cy, dz = pz[i] - cz;
      fCx += dx * cosYaw - dy * sinYaw;
      fCy += -dz * COS_PITCH + (dx * sinYaw + dy * cosYaw) * SIN_PITCH;
    }
    fCx /= px.length; fCy /= px.length;

    // Draw order: grid → rings → center line → points (on top) → axes
    drawTableGrid(ctx, w, h, info, cosYaw, sinYaw, fCx, fCy);
    drawCupRings(ctx, w, h, info, cosYaw, sinYaw, fCx, fCy);
    drawCenterLine(ctx, w, h, info, fCx, fCy);

    // --- Points (depth-sorted, drawn ON TOP of rings) ---
    const projected = new Array(px.length);
    for (let i = 0; i < px.length; i++) {
      const dx = px[i] - cx, dy = py[i] - cy, dz = pz[i] - cz;
      const rx = dx * cosYaw - dy * sinYaw;
      const ry = dx * sinYaw + dy * cosYaw;
      projected[i] = { sx: rx, sy: -dz * COS_PITCH + ry * SIN_PITCH, z: pz[i], depth: ry };
    }
    projected.sort((a: any, b: any) => a.depth - b.depth);

    for (const p of projected) {
      const sx = (p.sx - fCx) * scale + w / 2;
      const sy = (p.sy - fCy) * scale + h / 2;
      const t = (p.z - minZ) / rangeZ;
      const r = Math.round(60 + t * 100);
      const g = Math.round(160 + t * 95);
      const b = Math.round(255 - t * 120);
      ctx.fillStyle = `rgb(${r}, ${g}, ${b})`;
      ctx.beginPath();
      ctx.arc(sx, sy, 2.5, 0, Math.PI * 2);
      ctx.fill();
    }

    drawAxes(ctx, w, h, cosYaw, sinYaw);
  }

  function drawTableGrid(
    ctx: CanvasRenderingContext2D, w: number, h: number,
    info: ScaleInfo, cosYaw: number, sinYaw: number,
    fCx: number, fCy: number,
  ) {
    const { scale, cz, minZ } = info;
    const dzGrid = minZ - cz;
    const syGrid = -dzGrid * COS_PITCH;
    const gridSize = 30;
    const gridCount = 4;
    ctx.strokeStyle = "rgba(80, 80, 80, 0.35)";
    ctx.lineWidth = 0.5;

    for (let gi = -gridCount; gi <= gridCount; gi++) {
      const v = gi * gridSize;
      for (let dir = 0; dir < 2; dir++) {
        ctx.beginPath();
        for (let gj = -gridCount; gj <= gridCount; gj++) {
          const gv = gj * gridSize;
          const lx = dir === 0 ? gv : v;
          const ly = dir === 0 ? v : gv;
          const rx = lx * cosYaw - ly * sinYaw;
          const ry = lx * sinYaw + ly * cosYaw;
          const sx = (rx - fCx) * scale + w / 2;
          const sy2 = (syGrid + ry * SIN_PITCH - fCy) * scale + h / 2;
          if (gj === -gridCount) ctx.moveTo(sx, sy2); else ctx.lineTo(sx, sy2);
        }
        ctx.stroke();
      }
    }
    ctx.lineWidth = 1;
  }

  function drawCupRings(
    ctx: CanvasRenderingContext2D, w: number, h: number,
    info: ScaleInfo, cosYaw: number, sinYaw: number,
    fCx: number, fCy: number,
  ) {
    const { scale, minZ, cz } = info;
    const segments = 48;
    const profileAngles = 8;

    // Draw vertical profile lines connecting rings (wine glass silhouette)
    ctx.strokeStyle = "rgba(232,160,245,0.25)";
    ctx.lineWidth = 0.8;
    ctx.setLineDash([2, 4]);
    for (let ai = 0; ai < profileAngles; ai++) {
      const angle = (ai / profileAngles) * Math.PI * 2;
      ctx.beginPath();
      for (const ring of RINGS) {
        const ringZ = minZ + ring.heightFromFloor;
        const dzRing = ringZ - cz;
        const lx = Math.cos(angle) * ring.radius;
        const ly = Math.sin(angle) * ring.radius;
        const rx = lx * cosYaw - ly * sinYaw;
        const ry = lx * sinYaw + ly * cosYaw;
        const sx = (rx - fCx) * scale + w / 2;
        const sy = (-dzRing * COS_PITCH + ry * SIN_PITCH - fCy) * scale + h / 2;
        if (ring === RINGS[0]) ctx.moveTo(sx, sy); else ctx.lineTo(sx, sy);
      }
      ctx.stroke();
    }
    ctx.setLineDash([]);

    // Draw ring circles
    for (const ring of RINGS) {
      const ringZ = minZ + ring.heightFromFloor;
      const dzRing = ringZ - cz;
      ctx.strokeStyle = ring.color;
      ctx.lineWidth = 1.5;
      ctx.setLineDash([4, 3]);
      ctx.beginPath();
      for (let ci = 0; ci <= segments; ci++) {
        const angle = (ci / segments) * Math.PI * 2;
        const lx = Math.cos(angle) * ring.radius;
        const ly = Math.sin(angle) * ring.radius;
        const rx = lx * cosYaw - ly * sinYaw;
        const ry = lx * sinYaw + ly * cosYaw;
        const sx = (rx - fCx) * scale + w / 2;
        const sy = (-dzRing * COS_PITCH + ry * SIN_PITCH - fCy) * scale + h / 2;
        if (ci === 0) ctx.moveTo(sx, sy); else ctx.lineTo(sx, sy);
      }
      ctx.stroke();
      ctx.setLineDash([]);

      ctx.font = "9px IBM Plex Mono";
      ctx.fillStyle = ring.color;
      const labelX = (ring.radius * cosYaw - fCx) * scale + w / 2 + 6;
      const labelY = (-dzRing * COS_PITCH + ring.radius * sinYaw * SIN_PITCH - fCy) * scale + h / 2;
      ctx.fillText(`${ring.label} ø${Math.round(ring.radius * 2)}`, labelX, labelY - 3);
    }
  }

  function drawCenterLine(
    ctx: CanvasRenderingContext2D, w: number, h: number,
    info: ScaleInfo, fCx: number, fCy: number,
  ) {
    const { scale, minZ, cz } = info;
    ctx.strokeStyle = "rgba(232,160,245,0.4)";
    ctx.lineWidth = 1;
    ctx.setLineDash([3, 3]);
    const dzBot = minZ - cz;
    const dzTop = (minZ + CUP_HEIGHT) - cz;
    const syBot = (-dzBot * COS_PITCH - fCy) * scale + h / 2;
    const syTop = (-dzTop * COS_PITCH - fCy) * scale + h / 2;
    const sxCenter = (0 - fCx) * scale + w / 2;
    ctx.beginPath();
    ctx.moveTo(sxCenter, syBot);
    ctx.lineTo(sxCenter, syTop);
    ctx.stroke();
    ctx.setLineDash([]);
  }

  function drawAxes(ctx: CanvasRenderingContext2D, w: number, h: number, cosYaw: number, sinYaw: number) {
    const axLen = 20, ox = PAD, oy = h - PAD;
    ctx.lineWidth = 1;
    ctx.strokeStyle = "#fa4d56";
    ctx.beginPath(); ctx.moveTo(ox, oy); ctx.lineTo(ox + axLen * cosYaw, oy + axLen * SIN_PITCH * sinYaw); ctx.stroke();
    ctx.strokeStyle = "#42be65";
    ctx.beginPath(); ctx.moveTo(ox, oy); ctx.lineTo(ox - axLen * sinYaw, oy + axLen * SIN_PITCH * cosYaw); ctx.stroke();
    ctx.strokeStyle = "#4589ff";
    ctx.beginPath(); ctx.moveTo(ox, oy); ctx.lineTo(ox, oy - axLen * COS_PITCH); ctx.stroke();
    ctx.font = "9px IBM Plex Mono";
    ctx.fillStyle = "#fa4d56"; ctx.fillText("X", ox + axLen * cosYaw + 4, oy + axLen * SIN_PITCH * sinYaw);
    ctx.fillStyle = "#42be65"; ctx.fillText("Y", ox - axLen * sinYaw - 8, oy + axLen * SIN_PITCH * cosYaw);
    ctx.fillStyle = "#4589ff"; ctx.fillText("Z", ox + 4, oy - axLen * COS_PITCH - 2);
  }

  function renderCameraOverlay(
    canvas: HTMLCanvasElement | undefined,
    obj: SegmentedObject,
    camImage: ImageBitmap | null,
    intrinsics: CamIntrinsics | null,
    transformedPts: { x: number[]; y: number[]; z: number[] } | undefined,
    camStatusLines: string[],
    label: string,
  ) {
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const w = canvas.width, h = canvas.height;

    ctx.fillStyle = "#111";
    ctx.fillRect(0, 0, w, h);

    if (camImage) {
      ctx.drawImage(camImage, 0, 0, w, h);
    } else {
      // Show troubleshooting log
      ctx.fillStyle = "#777";
      ctx.font = "10px IBM Plex Mono";
      ctx.textAlign = "left";
      const lineH = 14;
      const startY = 20;
      ctx.fillStyle = "#aaa";
      ctx.fillText(`${label} — troubleshooting:`, 8, startY);
      ctx.fillStyle = "#666";
      for (let li = 0; li < camStatusLines.length; li++) {
        const line = camStatusLines[li];
        const color = line.startsWith("FAIL") ? "#fa4d56" : line.startsWith("OK") ? "#42be65" : "#888";
        ctx.fillStyle = color;
        ctx.fillText(line.slice(0, 60), 8, startY + (li + 1) * lineH);
      }
      ctx.textAlign = "start";
    }

    if (!transformedPts || !intrinsics) return;
    const { x: tx, y: ty, z: tz } = transformedPts;
    const { fx, fy, cx: pcx, cy: pcy, width: iw, height: ih } = intrinsics;
    if (fx === 0 || fy === 0) return;
    const scaleX = w / iw, scaleY = h / ih;

    ctx.globalAlpha = 0.7;
    for (let i = 0; i < tx.length; i++) {
      if (tz[i] <= 0) continue;
      const u = (fx * tx[i] / tz[i] + pcx) * scaleX;
      const v = (fy * ty[i] / tz[i] + pcy) * scaleY;
      if (u < 0 || u >= w || v < 0 || v >= h) continue;
      ctx.fillStyle = CUP_COLOR;
      ctx.beginPath();
      ctx.arc(u, v, 2, 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.globalAlpha = 1.0;
  }

  async function ensureCamClients() {
    if (!robotClient) return false;
    if (!leftCamClient) leftCamClient = new CameraClient(robotClient, "left-cam");
    if (!rightCamClient) rightCamClient = new CameraClient(robotClient, "right-cam");
    return true;
  }

  async function fetchIntrinsics() {
    if (intrinsicsFetched || !(await ensureCamClients())) return;
    try {
      const p = await leftCamClient!.getProperties();
      if (p.intrinsicParameters) {
        const ip = p.intrinsicParameters;
        leftIntrinsics = { width: ip.widthPx, height: ip.heightPx, fx: ip.focalXPx, fy: ip.focalYPx, cx: ip.centerXPx, cy: ip.centerYPx };
      }
    } catch (e) { console.warn("left-cam intrinsics:", e); }
    try {
      const p = await rightCamClient!.getProperties();
      if (p.intrinsicParameters) {
        const ip = p.intrinsicParameters;
        rightIntrinsics = { width: ip.widthPx, height: ip.heightPx, fx: ip.focalXPx, fy: ip.focalYPx, cx: ip.centerXPx, cy: ip.centerYPx };
      }
    } catch (e) { console.warn("right-cam intrinsics:", e); }
    intrinsicsFetched = true;
  }

  async function fetchOneCameraImage(client: CameraClient, label: string): Promise<{ image: ImageBitmap | null; status: string[] }> {
    const log: string[] = [];
    // 1) renderFrame("image/jpeg")
    try {
      const blob = await client.renderFrame("image/jpeg");
      log.push(`renderFrame(jpeg): ${blob.size} bytes`);
      if (blob.size > 0) {
        const bmp = await createImageBitmap(blob);
        log.push("OK - image decoded");
        console.log(`[cam] ${label}: renderFrame OK (${blob.size} bytes)`);
        return { image: bmp, status: log };
      }
      log.push("WARN: blob empty");
    } catch (e: any) {
      const msg = e?.message?.slice(0, 80) || "unknown error";
      log.push(`FAIL: ${msg}`);
      console.warn(`[cam] ${label}: renderFrame:`, msg);
    }
    // 2) getImage("image/jpeg") → ArrayBuffer → Blob
    try {
      const bytes = await client.getImage("image/jpeg");
      log.push(`getImage(jpeg): ${bytes.length} bytes`);
      const copy = new Uint8Array(bytes).buffer as ArrayBuffer;
      const blob = new Blob([copy], { type: "image/jpeg" });
      if (blob.size > 0) {
        const bmp = await createImageBitmap(blob);
        log.push("OK - image decoded");
        console.log(`[cam] ${label}: getImage OK (${bytes.length} bytes)`);
        return { image: bmp, status: log };
      }
    } catch (e: any) {
      const msg = e?.message?.slice(0, 80) || "unknown error";
      log.push(`FAIL: ${msg}`);
      console.warn(`[cam] ${label}: getImage(jpeg):`, msg);
    }
    // 3) getImage() no mime → try as PNG
    try {
      const bytes = await client.getImage();
      log.push(`getImage(default): ${bytes.length} bytes`);
      const copy = new Uint8Array(bytes).buffer as ArrayBuffer;
      const blob = new Blob([copy], { type: "image/png" });
      if (blob.size > 0) {
        const bmp = await createImageBitmap(blob);
        log.push("OK - decoded as PNG");
        console.log(`[cam] ${label}: getImage(default) OK (${bytes.length} bytes)`);
        return { image: bmp, status: log };
      }
    } catch (e: any) {
      const msg = e?.message?.slice(0, 80) || "unknown error";
      log.push(`FAIL: ${msg}`);
      console.warn(`[cam] ${label}: getImage(default):`, msg);
    }
    return { image: null, status: log };
  }

  async function fetchCameraImages() {
    if (!(await ensureCamClients())) {
      leftCamStatus = ["No robot client available"];
      rightCamStatus = ["No robot client available"];
      return;
    }
    const left = await fetchOneCameraImage(leftCamClient!, "left-cam");
    leftCamImage = left.image;
    leftCamStatus = left.status;
    const right = await fetchOneCameraImage(rightCamClient!, "right-cam");
    rightCamImage = right.image;
    rightCamStatus = right.status;
  }

  async function fetchTransformedPoints() {
    if (!robotClient) return;
    for (const obj of objects) {
      if (!obj.rawPCD || obj.rawPCD.length === 0) continue;
      try {
        const pcd = await robotClient.transformPCD(obj.rawPCD, "cup-finder-segment", "left-cam");
        leftFramePoints.set(obj.index, parsePCD(pcd));
      } catch (_) {}
      try {
        const pcd = await robotClient.transformPCD(obj.rawPCD, "cup-finder-segment", "right-cam");
        rightFramePoints.set(obj.index, parsePCD(pcd));
      } catch (_) {}
    }
  }

  onMount(() => {
    animId = requestAnimationFrame(renderAllCanvases);
    fetchIntrinsics();
    fetchCameraImages();
    fetchTransformedPoints();
    camFetchInterval = setInterval(() => {
      fetchCameraImages();
      fetchTransformedPoints();
    }, 2000);
  });

  onDestroy(() => {
    if (animId !== null) cancelAnimationFrame(animId);
    if (camFetchInterval !== null) clearInterval(camFetchInterval);
  });

  function fmt(v: number): string {
    return v.toFixed(1);
  }
</script>

<div class="panel-container">
  <div class="panel-header">
    <span class="panel-title">Segmented Objects</span>
    <button class="close-btn" onclick={onClose} aria-label="Close panel">
      <svg viewBox="0 0 16 16" fill="currentColor" width="16" height="16">
        <path d="M12 4.7L11.3 4 8 7.3 4.7 4 4 4.7 7.3 8 4 11.3l.7.7L8 8.7l3.3 3.3.7-.7L8.7 8z"/>
      </svg>
    </button>
  </div>

  {#if objects.length === 0}
    <div class="empty-state">No objects detected</div>
  {:else}
    <div class="objects-list">
      {#each objects as obj, i (obj.index)}
        <div class="object-card">
          <div class="card-header">
            <span class="object-label">Object {obj.index}</span>
            <div class="header-meta">
              {#if obj.dims}
                <span class="dim-badge">{fmt(obj.dims.x)} × {fmt(obj.dims.y)} × {fmt(obj.dims.z)} mm</span>
              {/if}
              <span class="point-badge">{obj.totalPoints} pts</span>
            </div>
          </div>
          {#if obj.dims || obj.position}
            <div class="dims-row">
              {#if obj.dims}
                <div class="dim-group">
                  <span class="dim-title">Dimensions</span>
                  <span class="dim-val">x: {fmt(obj.dims.x)}mm</span>
                  <span class="dim-val">y: {fmt(obj.dims.y)}mm</span>
                  <span class="dim-val">z: {fmt(obj.dims.z)}mm</span>
                </div>
              {/if}
              {#if obj.position}
                <div class="dim-group">
                  <span class="dim-title">Position</span>
                  <span class="dim-val">x: {fmt(obj.position.x)}</span>
                  <span class="dim-val">y: {fmt(obj.position.y)}</span>
                  <span class="dim-val">z: {fmt(obj.position.z)}</span>
                </div>
              {/if}
            </div>
          {/if}
          <div class="card-body">
            <div class="pc-half">
              <canvas
                bind:this={canvasRefs[i]}
                width="500"
                height="500"
                class="pc-canvas"
              ></canvas>
            </div>
            <div class="cam-half">
              <div class="cam-wrapper">
                <span class="cam-label">Left Camera</span>
                <canvas
                  bind:this={leftOverlayRefs[i]}
                  width="640"
                  height="480"
                  class="cam-canvas"
                ></canvas>
              </div>
              <div class="cam-wrapper">
                <span class="cam-label">Right Camera</span>
                <canvas
                  bind:this={rightOverlayRefs[i]}
                  width="640"
                  height="480"
                  class="cam-canvas"
                ></canvas>
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
  .close-btn:hover { background: #333; color: #fff; }
  .empty-state {
    padding: 24px;
    text-align: center;
    color: #6f6f6f;
    font-size: 0.85rem;
  }
  .objects-list {
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 12px;
    overflow-y: auto;
    flex: 1;
    min-height: 0;
  }
  .object-card {
    background: #222;
    border-radius: 6px;
    border-top: 3px solid #4589ff;
    overflow: hidden;
    flex-shrink: 0;
  }
  .card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 12px;
    border-bottom: 1px solid #333;
  }
  .object-label {
    color: #e0e0e0;
    font-size: 0.8rem;
    font-weight: 600;
  }
  .header-meta {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .dim-badge {
    color: #a8a8a8;
    font-size: 0.7rem;
  }
  .point-badge {
    color: #6f6f6f;
    font-size: 0.7rem;
  }
  .dims-row {
    display: flex;
    gap: 24px;
    padding: 6px 12px;
    border-bottom: 1px solid #2a2a2a;
    background: #1e1e1e;
  }
  .dim-group {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .dim-title {
    color: #8d8d8d;
    font-size: 0.65rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin-right: 2px;
  }
  .dim-val {
    color: #c6c6c6;
    font-size: 0.7rem;
  }
  .card-body {
    display: flex;
    flex-direction: row;
    gap: 8px;
    padding: 8px;
  }
  .pc-half {
    flex: 0 0 auto;
    aspect-ratio: 1;
    width: 50%;
    max-width: 500px;
  }
  .pc-canvas {
    border-radius: 4px;
    display: block;
    width: 100%;
    height: 100%;
  }
  .cam-half {
    flex: 1 1 50%;
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-width: 0;
  }
  .cam-wrapper {
    position: relative;
    flex: 1;
    min-height: 0;
  }
  .cam-label {
    position: absolute;
    top: 4px;
    left: 6px;
    z-index: 2;
    font-size: 0.65rem;
    color: #c6c6c6;
    background: rgba(0,0,0,0.6);
    padding: 2px 6px;
    border-radius: 3px;
  }
  .cam-canvas {
    border-radius: 4px;
    display: block;
    width: 100%;
    height: 100%;
    object-fit: contain;
    background: #111;
  }
</style>
