<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { CUP_HEIGHT, RINGS, type SegmentedObject } from "./types.js";

  let { objects = [] }: { objects: SegmentedObject[] } = $props();

  const ROT_SPEED = 0.3;
  const PITCH = (Math.PI / 180) * 30;
  const COS_PITCH = Math.cos(PITCH);
  const SIN_PITCH = Math.sin(PITCH);
  const PAD_CSS = 20;

  let canvasRef: HTMLCanvasElement | undefined = $state();
  let animId: number | null = null;

  interface ScaleInfo {
    scale: number; minZ: number; maxZ: number; rangeZ: number;
    cx: number; cy: number; cz: number;
    projCx: number; projCy: number;
  }
  let scaleCache = new Map<string, ScaleInfo>();

  function syncCanvasSize(canvas: HTMLCanvasElement) {
    const rect = canvas.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    const pw = Math.round(rect.width * dpr);
    const ph = Math.round(rect.height * dpr);
    if (canvas.width !== pw || canvas.height !== ph) {
      canvas.width = pw;
      canvas.height = ph;
      scaleCache.clear();
    }
  }

  function scaleKey(obj: SegmentedObject, w: number, h: number): string {
    const px = obj.points_x, py = obj.points_y;
    return `${obj.index}:${px.length}:${px[0]}:${py[0]}:${px[px.length - 1]}:${w}x${h}`;
  }

  function scaledRingZ(ring: typeof RINGS[number], minZ: number, pcHeight: number): number {
    return minZ + (ring.heightFromFloor / CUP_HEIGHT) * pcHeight;
  }

  function computeScale(obj: SegmentedObject, w: number, h: number, pad: number): ScaleInfo {
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
    const pcHeight = (maxZ - minZ) || 1;

    let projMinX = Infinity, projMaxX = -Infinity;
    let projMinY = Infinity, projMaxY = -Infinity;

    for (let a = 0; a < Math.PI * 2; a += Math.PI / 8) {
      const ca = Math.cos(a), sa = Math.sin(a);

      for (let j = 0; j < n; j++) {
        const dx = px[j] - cx, dy = py[j] - cy, dz = pz[j] - cz;
        const hx = dx * ca - dy * sa;
        const hy = -dz * COS_PITCH + (dx * sa + dy * ca) * SIN_PITCH;
        if (hx < projMinX) projMinX = hx;
        if (hx > projMaxX) projMaxX = hx;
        if (hy < projMinY) projMinY = hy;
        if (hy > projMaxY) projMaxY = hy;
      }

      for (const ring of RINGS) {
        const ringZv = scaledRingZ(ring, minZ, pcHeight);
        const dzRing = ringZv - cz;
        for (let ri = 0; ri < 8; ri++) {
          const ra = (ri / 8) * Math.PI * 2;
          const lx = Math.cos(ra) * ring.radius;
          const ly = Math.sin(ra) * ring.radius;
          const hx = lx * ca - ly * sa;
          const hy = -dzRing * COS_PITCH + (lx * sa + ly * ca) * SIN_PITCH;
          if (hx < projMinX) projMinX = hx;
          if (hx > projMaxX) projMaxX = hx;
          if (hy < projMinY) projMinY = hy;
          if (hy > projMaxY) projMaxY = hy;
        }
      }

      const dzBot = minZ - cz;
      const dzTop = maxZ - cz;
      const hyBot = -dzBot * COS_PITCH;
      const hyTop = -dzTop * COS_PITCH;
      if (hyBot < projMinY) projMinY = hyBot;
      if (hyBot > projMaxY) projMaxY = hyBot;
      if (hyTop < projMinY) projMinY = hyTop;
      if (hyTop > projMaxY) projMaxY = hyTop;
    }

    const projCxv = (projMinX + projMaxX) / 2;
    const projCyv = (projMinY + projMaxY) / 2;
    const halfW = Math.max(projMaxX - projCxv, projCxv - projMinX) || 1;
    const halfH = Math.max(projMaxY - projCyv, projCyv - projMinY) || 1;
    const scale = Math.min((w / 2 - pad) / halfW, (h / 2 - pad) / halfH);
    return { scale, minZ, maxZ, rangeZ: pcHeight, cx, cy, cz, projCx: projCxv, projCy: projCyv };
  }

  function renderCanvas() {
    if (!canvasRef) { animId = requestAnimationFrame(renderCanvas); return; }
    syncCanvasSize(canvasRef);
    const ctx = canvasRef.getContext("2d");
    if (!ctx) { animId = requestAnimationFrame(renderCanvas); return; }
    const w = canvasRef.width, h = canvasRef.height;

    const obj = objects.length > 0 ? objects[0] : null;
    if (!obj || obj.points_x.length === 0) {
      ctx.fillStyle = "#1a1a2e";
      ctx.fillRect(0, 0, w, h);
      ctx.fillStyle = "#525252";
      const dpr = window.devicePixelRatio || 1;
      ctx.font = `${Math.round(13 * dpr)}px IBM Plex Mono, monospace`;
      ctx.textAlign = "center";
      ctx.fillText("No objects detected", w / 2, h / 2);
      animId = requestAnimationFrame(renderCanvas);
      return;
    }

    const yaw = (performance.now() / 1000) * ROT_SPEED;
    const cosYaw = Math.cos(yaw);
    const sinYaw = Math.sin(yaw);

    const dpr = window.devicePixelRatio || 1;
    const pad = PAD_CSS * dpr;
    const key = scaleKey(obj, w, h);
    let info = scaleCache.get(key);
    if (!info) {
      info = computeScale(obj, w, h, pad);
      scaleCache.set(key, info);
      if (scaleCache.size > 20) scaleCache.delete(scaleCache.keys().next().value!);
    }

    render3DView(ctx, w, h, obj, info, cosYaw, sinYaw);
    animId = requestAnimationFrame(renderCanvas);
  }

  function render3DView(
    ctx: CanvasRenderingContext2D, w: number, h: number,
    obj: SegmentedObject, info: ScaleInfo,
    cosYaw: number, sinYaw: number,
  ) {
    const { scale, minZ, rangeZ, cx, cy, cz, projCx: pCx, projCy: pCy } = info;
    const px = obj.points_x, py = obj.points_y, pz = obj.points_z;

    ctx.fillStyle = "#1a1a2e";
    ctx.fillRect(0, 0, w, h);

    drawTableGrid(ctx, w, h, info, cosYaw, sinYaw, pCx, pCy);
    drawCupRings(ctx, w, h, info, cosYaw, sinYaw, pCx, pCy);
    drawCenterLine(ctx, w, h, info, pCx, pCy);

    const projected = new Array(px.length);
    for (let i = 0; i < px.length; i++) {
      const dx = px[i] - cx, dy = py[i] - cy, dz = pz[i] - cz;
      const rx = dx * cosYaw - dy * sinYaw;
      const ry = dx * sinYaw + dy * cosYaw;
      projected[i] = { sx: rx, sy: -dz * COS_PITCH + ry * SIN_PITCH, z: pz[i], depth: ry };
    }
    projected.sort((a: any, b: any) => a.depth - b.depth);

    for (const p of projected) {
      const sx = (p.sx - pCx) * scale + w / 2;
      const sy = (p.sy - pCy) * scale + h / 2;
      const tz = (p.z - minZ) / rangeZ;
      const r = Math.round(60 + tz * 100);
      const g = Math.round(160 + tz * 95);
      const b = Math.round(255 - tz * 120);
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
    const { scale, minZ, maxZ, cz } = info;
    const pcHeight = maxZ - minZ;
    const segments = 48;
    const profileAngles = 8;

    ctx.strokeStyle = "rgba(232,160,245,0.25)";
    ctx.lineWidth = 0.8;
    ctx.setLineDash([2, 4]);
    for (let ai = 0; ai < profileAngles; ai++) {
      const angle = (ai / profileAngles) * Math.PI * 2;
      ctx.beginPath();
      for (const ring of RINGS) {
        const ringZ = scaledRingZ(ring, minZ, pcHeight);
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

    for (const ring of RINGS) {
      const ringZ = scaledRingZ(ring, minZ, pcHeight);
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
    const { scale, minZ, maxZ, cz } = info;
    ctx.strokeStyle = "rgba(232,160,245,0.4)";
    ctx.lineWidth = 1;
    ctx.setLineDash([3, 3]);
    const dzBot = minZ - cz;
    const dzTop = maxZ - cz;
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
    const dpr = window.devicePixelRatio || 1;
    const pad = PAD_CSS * dpr;
    const axLen = 20 * dpr, ox = pad, oy = h - pad;
    ctx.lineWidth = 1 * dpr;
    ctx.strokeStyle = "#fa4d56";
    ctx.beginPath(); ctx.moveTo(ox, oy); ctx.lineTo(ox + axLen * cosYaw, oy + axLen * SIN_PITCH * sinYaw); ctx.stroke();
    ctx.strokeStyle = "#42be65";
    ctx.beginPath(); ctx.moveTo(ox, oy); ctx.lineTo(ox - axLen * sinYaw, oy + axLen * SIN_PITCH * cosYaw); ctx.stroke();
    ctx.strokeStyle = "#4589ff";
    ctx.beginPath(); ctx.moveTo(ox, oy); ctx.lineTo(ox, oy - axLen * COS_PITCH); ctx.stroke();
    const fontSize = Math.round(9 * dpr);
    ctx.font = `${fontSize}px IBM Plex Mono`;
    ctx.fillStyle = "#fa4d56"; ctx.fillText("X", ox + axLen * cosYaw + 4 * dpr, oy + axLen * SIN_PITCH * sinYaw);
    ctx.fillStyle = "#42be65"; ctx.fillText("Y", ox - axLen * sinYaw - 8 * dpr, oy + axLen * SIN_PITCH * cosYaw);
    ctx.fillStyle = "#4589ff"; ctx.fillText("Z", ox + 4 * dpr, oy - axLen * COS_PITCH - 2 * dpr);
  }

  onMount(() => { animId = requestAnimationFrame(renderCanvas); });
  onDestroy(() => { if (animId !== null) cancelAnimationFrame(animId); });
</script>

<canvas bind:this={canvasRef} class="pcd-canvas"></canvas>

<style>
  .pcd-canvas {
    width: 100%;
    height: 100%;
    display: block;
    border-radius: 8px;
    background: #1a1a2e;
  }
</style>
