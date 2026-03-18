<script lang="ts">
  import { CameraClient } from "@viamrobotics/sdk";
  import type { RobotClient } from "@viamrobotics/sdk";
  import CameraFeed from "./CameraFeed.svelte";
  import { CUP_HEIGHT, RINGS, type SegmentedObject } from "./types.js";

  let {
    camName = "",
    label = "",
    objects = [],
    robotClient = null,
  }: {
    camName: string;
    label: string;
    objects: SegmentedObject[];
    robotClient: RobotClient | null;
  } = $props();

  const PART_ID = "xxx";

  const DEPTH_TO_COLOR_OFFSET = { x: 0, y: 0, z: 0 };

  interface CamIntrinsics { fx: number; fy: number; cx: number; cy: number; w: number; h: number; }
  interface CamProjection {
    points: { u: number; v: number; r: number; g: number; b: number }[];
    rings: { color: string; pts: { u: number; v: number }[]; label: string }[];
    centerLine: { u1: number; v1: number; u2: number; v2: number } | null;
  }

  let overlayCanvas: HTMLCanvasElement | undefined = $state();
  let intrinsics: CamIntrinsics | null = $state(null);
  let intrinsicsFetched = false;
  let projections: (CamProjection | null)[] = $state([]);
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

  async function getWorldToCamTransform(): Promise<{ R: number[][]; t: number[] } | null> {
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
    try {
      const cam = new CameraClient(robotClient, camName);
      const props = await cam.getProperties();
      const ip = props.intrinsicParameters;
      if (ip && ip.focalXPx > 0) {
        intrinsics = { fx: ip.focalXPx, fy: ip.focalYPx, cx: ip.centerXPx, cy: ip.centerYPx, w: ip.widthPx, h: ip.heightPx };
        console.log(`[cam-overlay] ${camName} intrinsics: ${ip.widthPx}x${ip.heightPx} fx=${ip.focalXPx.toFixed(1)}`);
      }
    } catch (e) {
      console.warn(`[cam-overlay] ${camName}: getProperties failed:`, e);
    }
  }

  async function fetchTransformAndProject(): Promise<(CamProjection | null)[]> {
    if (!robotClient || !intrinsics) return objects.map(() => null);
    const intr = intrinsics;
    try {
      const xform = await getWorldToCamTransform();
      if (!xform) return objects.map(() => null);
      const { R, t } = xform;

      return objects.map(obj => {
        if (!obj || obj.points_x.length === 0) return null;
        const px = obj.points_x, py = obj.points_y, pz = obj.points_z;
        const minZ = Math.min(...pz);
        const maxZ = Math.max(...pz);
        const pcHeight = (maxZ - minZ) || 1;

        const points: CamProjection["points"] = [];
        for (let i = 0; i < px.length; i++) {
          const [cx, cy, cz] = transformPointToColor(R, t, px[i], py[i], pz[i]);
          const uv = projectPoint(cx, cy, cz, intr);
          if (!uv) continue;
          const tz = (pz[i] - minZ) / pcHeight;
          points.push({
            u: uv[0], v: uv[1],
            r: Math.round(60 + tz * 100),
            g: Math.round(160 + tz * 95),
            b: Math.round(255 - tz * 120),
          });
        }

        const minX = Math.min(...px), maxX = Math.max(...px);
        const minY = Math.min(...py), maxY = Math.max(...py);
        const centX = (minX + maxX) / 2;
        const centY = (minY + maxY) / 2;

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

        let centerLine: CamProjection["centerLine"] = null;
        const [bx, by, bz] = transformPointToColor(R, t, centX, centY, minZ);
        const [tx2, ty2, tz2] = transformPointToColor(R, t, centX, centY, maxZ);
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
    if (intrinsics) {
      projections = await fetchTransformAndProject();
    }
    projectionVersion++;
  }

  let lastObjRef: SegmentedObject[] | null = null;
  $effect(() => {
    if (objects !== lastObjRef && robotClient) {
      lastObjRef = objects;
      refreshProjections();
    }
  });

  $effect(() => {
    void projectionVersion;
    if (overlayCanvas && intrinsics) {
      renderAllOverlays();
    }
  });

  function renderAllOverlays() {
    if (!overlayCanvas || !intrinsics) return;
    const canvas = overlayCanvas;
    if (canvas.width !== intrinsics.w || canvas.height !== intrinsics.h) {
      canvas.width = intrinsics.w;
      canvas.height = intrinsics.h;
    }
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    ctx.clearRect(0, 0, intrinsics.w, intrinsics.h);
    for (const proj of projections) {
      if (!proj) continue;
      renderCamOverlay(ctx, proj, intrinsics);
    }
  }

  function renderCamOverlay(ctx: CanvasRenderingContext2D, proj: CamProjection, intr: CamIntrinsics) {
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

    for (let i = 0; i < proj.points.length; i += 3) {
      const p = proj.points[i];
      ctx.fillStyle = `rgba(${p.r}, ${p.g}, ${p.b}, 0.7)`;
      ctx.fillRect(p.u - 1.5, p.v - 1.5, 3, 3);
    }
  }
</script>

<CameraFeed name={camName} partID={PART_ID} {label}>
  {#snippet fullOverlay()}
    <canvas bind:this={overlayCanvas} class="cam-overlay"></canvas>
  {/snippet}
</CameraFeed>

<style>
  .cam-overlay {
    position: absolute;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    pointer-events: none;
    z-index: 5;
    object-fit: contain;
  }
</style>
