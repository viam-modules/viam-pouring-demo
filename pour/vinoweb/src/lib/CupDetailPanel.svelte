<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { CameraStream } from "@viamrobotics/svelte-sdk";
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

  const RINGS = [
    { label: "Base",  circumference: 200, heightFromFloor: 0,          color: "rgba(232,160,245,0.6)" },
    { label: "Belly", circumference: 270, heightFromFloor: 45,         color: "rgba(213,128,232,0.7)" },
    { label: "Rim",   circumference: 220, heightFromFloor: CUP_HEIGHT, color: "rgba(240,192,255,0.6)" },
  ].map(r => ({ ...r, radius: r.circumference / (2 * Math.PI) }));

  // RealSense D400 series: the color sensor is physically offset from the
  // depth sensor origin. The frame system transform targets the depth frame,
  // but getProperties() returns color intrinsics and CameraStream shows the
  // color image. This offset corrects for that mismatch (mm, in camera-local
  // coords: +X right, +Y down, +Z forward).
  const DEPTH_TO_COLOR_OFFSET = { x: 0, y: 0, z: 0 };

  const ROT_SPEED = 0.3;
  const PITCH = (Math.PI / 180) * 30;
  const COS_PITCH = Math.cos(PITCH);
  const SIN_PITCH = Math.sin(PITCH);
  const PAD_CSS = 20;

  let canvasRefs: (HTMLCanvasElement | undefined)[] = $state([]);
  let animId: number | null = null;

  // --- Camera overlay state ---
  interface CamIntrinsics { fx: number; fy: number; cx: number; cy: number; w: number; h: number; }
  interface CamProjection {
    points: { u: number; v: number; r: number; g: number; b: number }[];
    rings: { color: string; pts: { u: number; v: number }[]; label: string }[];
    centerLine: { u1: number; v1: number; u2: number; v2: number } | null;
  }

  let leftIntrinsics: CamIntrinsics | null = $state(null);
  let rightIntrinsics: CamIntrinsics | null = $state(null);
  let intrinsicsFetched = false;
  let leftOverlayRefs: (HTMLCanvasElement | undefined)[] = $state([]);
  let rightOverlayRefs: (HTMLCanvasElement | undefined)[] = $state([]);
  let leftProjections: (CamProjection | null)[] = $state([]);
  let rightProjections: (CamProjection | null)[] = $state([]);
  let projectionVersion = $state(0);

  function transformPointToColor(R: number[][], t: number[], wx: number, wy: number, wz: number): [number, number, number] {
    return [
      R[0][0]*wx + R[0][1]*wy + R[0][2]*wz + t[0] - DEPTH_TO_COLOR_OFFSET.x,
      R[1][0]*wx + R[1][1]*wy + R[1][2]*wz + t[1] - DEPTH_TO_COLOR_OFFSET.y,
      R[2][0]*wx + R[2][1]*wy + R[2][2]*wz + t[2] - DEPTH_TO_COLOR_OFFSET.z,
    ];
  }

  function projectPoint(cx: number, cy: number, cz: number, intr: CamIntrinsics): [number, number] | null {
    if (cz <= 0.1) return null;
    return [intr.fx * cx / cz + intr.cx, intr.fy * cy / cz + intr.cy];
  }

  async function getWorldToCamTransform(camName: string): Promise<{ R: number[][]; t: number[] } | null> {
    if (!robotClient) return null;
    const svc = (robotClient as any).robotService;
    const D = 1000;
    const mkPose = (x: number, y: number, z: number) => ({
      source: { referenceFrame: "world", pose: { x, y, z, oX: 0, oY: 0, oZ: 1, theta: 0 } },
      destination: camName,
    });
    const [r0, r1, r2, r3] = await Promise.all([
      svc.transformPose(mkPose(0, 0, 0)),
      svc.transformPose(mkPose(D, 0, 0)),
      svc.transformPose(mkPose(0, D, 0)),
      svc.transformPose(mkPose(0, 0, D)),
    ]);
    const p0 = r0.pose?.pose, p1 = r1.pose?.pose, p2 = r2.pose?.pose, p3 = r3.pose?.pose;
    if (!p0 || !p1 || !p2 || !p3) return null;
    const t = [p0.x, p0.y, p0.z];
    const R = [
      [(p1.x - p0.x) / D, (p2.x - p0.x) / D, (p3.x - p0.x) / D],
      [(p1.y - p0.y) / D, (p2.y - p0.y) / D, (p3.y - p0.y) / D],
      [(p1.z - p0.z) / D, (p2.z - p0.z) / D, (p3.z - p0.z) / D],
    ];
    return { R, t };
  }

  async function fetchIntrinsics() {
    if (intrinsicsFetched || !robotClient) return;
    intrinsicsFetched = true;
    for (const [name, setter] of [["left-cam", (v: CamIntrinsics) => leftIntrinsics = v], ["right-cam", (v: CamIntrinsics) => rightIntrinsics = v]] as const) {
      try {
        const cam = new CameraClient(robotClient, name);
        const props = await cam.getProperties();
        const ip = props.intrinsicParameters;
        if (ip && ip.focalXPx > 0) {
          (setter as (v: CamIntrinsics) => void)({ fx: ip.focalXPx, fy: ip.focalYPx, cx: ip.centerXPx, cy: ip.centerYPx, w: ip.widthPx, h: ip.heightPx });
          console.log(`[cam-overlay] ${name} intrinsics: ${ip.widthPx}x${ip.heightPx} fx=${ip.focalXPx.toFixed(1)}`);
        }
      } catch (e) {
        console.warn(`[cam-overlay] ${name}: getProperties failed:`, e);
      }
    }
  }

  async function fetchTransformAndProject(camName: string, intr: CamIntrinsics): Promise<(CamProjection | null)[]> {
    if (!robotClient) return objects.map(() => null);
    try {
      const xform = await getWorldToCamTransform(camName);
      if (!xform) return objects.map(() => null);
      const { R, t } = xform;

      return objects.map(obj => {
        if (!obj || obj.points_x.length === 0) return null;
        const px = obj.points_x, py = obj.points_y, pz = obj.points_z;
        const minZ = Math.min(...pz);
        const maxZ = Math.max(...pz);
        const pcHeight = (maxZ - minZ) || 1;
        const rangeZ = pcHeight;

        // Project point cloud
        const points: CamProjection["points"] = [];
        for (let i = 0; i < px.length; i++) {
          const [cx, cy, cz] = transformPointToColor(R, t, px[i], py[i], pz[i]);
          const uv = projectPoint(cx, cy, cz, intr);
          if (!uv) continue;
          const tz = (pz[i] - minZ) / rangeZ;
          points.push({
            u: uv[0], v: uv[1],
            r: Math.round(60 + tz * 100),
            g: Math.round(160 + tz * 95),
            b: Math.round(255 - tz * 120),
          });
        }

        // Bounding-box midpoint to match backend MetaData.Center()
        const minX = Math.min(...px), maxX = Math.max(...px);
        const minY = Math.min(...py), maxY = Math.max(...py);
        const centX = (minX + maxX) / 2;
        const centY = (minY + maxY) / 2;

        // Project cup rings
        const rings: CamProjection["rings"] = [];
        const segments = 32;
        for (const ring of RINGS) {
          const ringZ = minZ + (ring.heightFromFloor / CUP_HEIGHT) * pcHeight;
          const pts: { u: number; v: number }[] = [];
          for (let ci = 0; ci <= segments; ci++) {
            const angle = (ci / segments) * Math.PI * 2;
            const wx = Math.cos(angle) * ring.radius;
            const wy = Math.sin(angle) * ring.radius;
            const [ccx, ccy, ccz] = transformPointToColor(R, t, centX + wx, centY + wy, ringZ);
            const uv = projectPoint(ccx, ccy, ccz, intr);
            if (uv) pts.push({ u: uv[0], v: uv[1] });
          }
          rings.push({ color: ring.color, pts, label: ring.label });
        }

        // Project center line
        let centerLine: CamProjection["centerLine"] = null;
        const sx = centX, sy = centY;
        const [bx, by, bz] = transformPointToColor(R, t, sx, sy, minZ);
        const [tx2, ty2, tz2] = transformPointToColor(R, t, sx, sy, maxZ);
        const uvBot = projectPoint(bx, by, bz, intr);
        const uvTop = projectPoint(tx2, ty2, tz2, intr);
        if (uvBot && uvTop) centerLine = { u1: uvBot[0], v1: uvBot[1], u2: uvTop[0], v2: uvTop[1] };

        return { points, rings, centerLine };
      });
    } catch (e) {
      console.warn(`[cam-overlay] ${camName}: transformPose failed:`, e);
      return objects.map(() => null);
    }
  }

  async function refreshProjections() {
    if (!robotClient) return;
    await fetchIntrinsics();
    if (leftIntrinsics) {
      leftProjections = await fetchTransformAndProject("left-cam", leftIntrinsics);
    }
    if (rightIntrinsics) {
      rightProjections = await fetchTransformAndProject("right-cam", rightIntrinsics);
    }
    projectionVersion++;
  }

  // Refresh projections when objects change
  let lastObjRef: SegmentedObject[] | null = null;
  $effect(() => {
    if (objects !== lastObjRef && robotClient) {
      lastObjRef = objects;
      refreshProjections();
    }
  });

  function renderCamOverlay(
    canvas: HTMLCanvasElement,
    proj: CamProjection | null,
    intr: CamIntrinsics,
  ) {
    if (canvas.width !== intr.w || canvas.height !== intr.h) {
      canvas.width = intr.w;
      canvas.height = intr.h;
    }
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    ctx.clearRect(0, 0, intr.w, intr.h);
    if (!proj) return;

    // Draw rings
    for (const ring of proj.rings) {
      if (ring.pts.length < 2) continue;
      ctx.strokeStyle = ring.color;
      ctx.lineWidth = 2;
      ctx.setLineDash([6, 4]);
      ctx.beginPath();
      ctx.moveTo(ring.pts[0].u, ring.pts[0].v);
      for (let i = 1; i < ring.pts.length; i++) ctx.lineTo(ring.pts[i].u, ring.pts[i].v);
      ctx.stroke();
      ctx.setLineDash([]);
    }

    // Draw center line
    if (proj.centerLine) {
      const cl = proj.centerLine;
      ctx.strokeStyle = "rgba(232,160,245,0.5)";
      ctx.lineWidth = 1.5;
      ctx.setLineDash([4, 4]);
      ctx.beginPath();
      ctx.moveTo(cl.u1, cl.v1);
      ctx.lineTo(cl.u2, cl.v2);
      ctx.stroke();
      ctx.setLineDash([]);
    }

    // Draw points (subsample for performance — draw every 3rd point)
    for (let i = 0; i < proj.points.length; i += 3) {
      const p = proj.points[i];
      ctx.fillStyle = `rgba(${p.r}, ${p.g}, ${p.b}, 0.7)`;
      ctx.fillRect(p.u - 1.5, p.v - 1.5, 3, 3);
    }
  }

  // --- 3D PCD canvas rendering (unchanged) ---
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

  interface ScaleInfo {
    scale: number; minZ: number; maxZ: number; rangeZ: number;
    cx: number; cy: number; cz: number;
    projCx: number; projCy: number;
  }
  let scaleCache = new Map<string, ScaleInfo>();

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

  function renderAllCanvases() {
    const yaw = (performance.now() / 1000) * ROT_SPEED;
    const cosYaw = Math.cos(yaw);
    const sinYaw = Math.sin(yaw);

    for (let i = 0; i < objects.length; i++) {
      const canvas = canvasRefs[i];
      if (!canvas) continue;
      syncCanvasSize(canvas);
      const ctx = canvas.getContext("2d");
      if (!ctx) continue;
      const w = canvas.width, h = canvas.height;
      const obj = objects[i];
      if (!obj || obj.points_x.length === 0) {
        ctx.fillStyle = "#1a1a2e";
        ctx.fillRect(0, 0, w, h);
        ctx.fillStyle = "#525252";
        ctx.font = "12px IBM Plex Mono, monospace";
        ctx.textAlign = "center";
        ctx.fillText("No points", w / 2, h / 2);
        continue;
      }

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

      // Render camera overlays (static, not spinning)
      if (leftIntrinsics && leftOverlayRefs[i] && leftProjections[i]) {
        renderCamOverlay(leftOverlayRefs[i]!, leftProjections[i], leftIntrinsics);
      }
      if (rightIntrinsics && rightOverlayRefs[i] && rightProjections[i]) {
        renderCamOverlay(rightOverlayRefs[i]!, rightProjections[i], rightIntrinsics);
      }
    }
    animId = requestAnimationFrame(renderAllCanvases);
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

  onMount(() => {
    animId = requestAnimationFrame(renderAllCanvases);
  });

  onDestroy(() => {
    if (animId !== null) cancelAnimationFrame(animId);
  });

  function fmt(v: number): string {
    return v.toFixed(1);
  }

  const PART_ID = "xxx";
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
                class="pc-canvas"
              ></canvas>
            </div>
            <div class="cam-half">
              <div class="cam-wrapper">
                <span class="cam-label">Left Camera</span>
                <CameraStream name="left-cam" partID={PART_ID} />
                <canvas bind:this={leftOverlayRefs[i]} class="cam-overlay"></canvas>
              </div>
              <div class="cam-wrapper">
                <span class="cam-label">Right Camera</span>
                <CameraStream name="right-cam" partID={PART_ID} />
                <canvas bind:this={rightOverlayRefs[i]} class="cam-overlay"></canvas>
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
    flex: 0 0 350px;
    width: 350px;
    height: 350px;
  }
  .pc-canvas {
    border-radius: 4px;
    display: block;
    width: 350px;
    height: 350px;
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
    border-radius: 4px;
    overflow: hidden;
    background: #111;
  }
  .cam-wrapper :global(video) {
    width: 100%;
    height: 100%;
    object-fit: contain;
  }
  .cam-overlay {
    position: absolute;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    pointer-events: none;
    z-index: 1;
    object-fit: contain;
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
    pointer-events: none;
  }
</style>
