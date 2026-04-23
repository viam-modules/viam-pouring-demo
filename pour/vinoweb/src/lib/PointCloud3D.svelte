<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import * as THREE from "three";
  import type { SegmentedObject } from "./types.js";
  import { PC_INVALID_T0, PC_INVALID_T1, PC_VALID_T0, PC_VALID_T1 } from "./pcGradientColors.js";

  let {
    objects = [],
    cupHeightMm = 0,
    cupWidthMm = 0,
  }: {
    objects: SegmentedObject[];
    cupHeightMm?: number;
    cupWidthMm?: number;
  } = $props();

  let containerRef: HTMLDivElement | undefined = $state();
  let renderer: THREE.WebGLRenderer | null = null;
  let scene: THREE.Scene;
  let camera: THREE.PerspectiveCamera;
  let animId: number | null = null;
  let pointsMesh: THREE.Points | null = null;
  /** Dashed purple outline + grid + center line — only rebuilt when cup config changes */
  let cupVisualsGroup: THREE.Group | null = null;
  let lastObjKey = "";
  let lastCupKey = "";

  const BG_COLOR = 0x1a1a2e;
  const ROT_SPEED = 0.3;
  /** Purple dashed UX (matches center line family) */
  const CUP_LINE_COLOR = 0xa855f7;

  function buildScene() {
    scene = new THREE.Scene();
    scene.background = new THREE.Color(BG_COLOR);
    camera = new THREE.PerspectiveCamera(40, 1, 0.1, 10000);
    camera.position.set(0, 120, 200);
    camera.lookAt(0, 40, 0);
    scene.add(new THREE.AxesHelper(25));
  }

  function objKey(obj: SegmentedObject): string {
    const px = obj.points_x;
    return `${obj.index}:${px.length}:${px[0]}:${px[px.length - 1]}:${obj.valid}`;
  }

  function clearPointsOnly() {
    if (pointsMesh) {
      scene.remove(pointsMesh);
      pointsMesh.geometry.dispose();
      (pointsMesh.material as THREE.Material).dispose();
      pointsMesh = null;
    }
  }

  function clearCupVisuals() {
    if (!cupVisualsGroup) return;
    scene.remove(cupVisualsGroup);
    cupVisualsGroup.traverse((o) => {
      const g = o as THREE.Object3D & { geometry?: THREE.BufferGeometry; material?: THREE.Material | THREE.Material[] };
      if (g.geometry) g.geometry.dispose();
      if (g.material) {
        if (Array.isArray(g.material)) g.material.forEach((m) => m.dispose());
        else g.material.dispose();
      }
    });
    cupVisualsGroup = null;
  }

  /** Top/bottom circles + vertical edges, all dashed purple */
  function buildCupDashedOutline(radius: number, height: number): THREE.Group {
    const group = new THREE.Group();
    const ringSeg = 48;
    const dashMat = new THREE.LineDashedMaterial({
      color: CUP_LINE_COLOR,
      dashSize: 4,
      gapSize: 2,
      transparent: true,
      opacity: 0.85,
    });

    function ringPoints(y: number): THREE.Vector3[] {
      const pts: THREE.Vector3[] = [];
      for (let i = 0; i < ringSeg; i++) {
        const a = (i / ringSeg) * Math.PI * 2;
        pts.push(new THREE.Vector3(Math.cos(a) * radius, y, Math.sin(a) * radius));
      }
      return pts;
    }

    for (const y of [0, height]) {
      const geo = new THREE.BufferGeometry().setFromPoints(ringPoints(y));
      const loop = new THREE.LineLoop(geo, dashMat.clone());
      loop.computeLineDistances();
      group.add(loop);
    }

    const vertMat = dashMat.clone();
    const verts: THREE.Vector3[] = [];
    for (let i = 0; i < 4; i++) {
      const a = (i / 4) * Math.PI * 2;
      const x = Math.cos(a) * radius;
      const z = Math.sin(a) * radius;
      verts.push(new THREE.Vector3(x, 0, z), new THREE.Vector3(x, height, z));
    }
    const vGeo = new THREE.BufferGeometry().setFromPoints(verts);
    const verticals = new THREE.LineSegments(vGeo, vertMat);
    verticals.computeLineDistances();
    group.add(verticals);

    return group;
  }

  function fitCameraToCup(cupH: number, cupW: number) {
    const radius = cupW / 2;
    const center = new THREE.Vector3(0, cupH / 2, 0);
    const boundR = Math.sqrt(radius * radius + (cupH / 2) * (cupH / 2));
    const padding = 1.2;
    const vFov = camera.fov * (Math.PI / 180);
    const dist = (boundR * padding) / Math.sin(vFov / 2);
    camera.position.set(center.x, center.y + dist * 0.35, center.z + dist * 0.82);
    camera.lookAt(center);
  }

  /**
   * Cup outline, center dashed line, grid — only when cup_height / cup_width change.
   * Camera framing uses cup dimensions only (never point cloud).
   */
  function refreshCupVisualsAndCamera() {
    clearCupVisuals();

    const cupH = cupHeightMm;
    const cupW = cupWidthMm;
    const radius = cupW > 0 ? cupW / 2 : 0;

    if (cupH <= 0 || radius <= 0) return;

    const group = new THREE.Group();
    cupVisualsGroup = group;

    group.add(buildCupDashedOutline(radius, cupH));

    const clGeom = new THREE.BufferGeometry().setFromPoints([
      new THREE.Vector3(0, 0, 0),
      new THREE.Vector3(0, cupH, 0),
    ]);
    const centerLine = new THREE.Line(
      clGeom,
      new THREE.LineDashedMaterial({
        color: CUP_LINE_COLOR,
        dashSize: 3,
        gapSize: 2,
        transparent: true,
        opacity: 0.55,
      }),
    );
    centerLine.computeLineDistances();
    group.add(centerLine);

    const gridSize = Math.max(120, cupW * 2.5);
    const gridHelper = new THREE.GridHelper(gridSize, 6, 0x505050, 0x505050);
    const ghMat = gridHelper.material as THREE.Material | THREE.Material[];
    if (Array.isArray(ghMat)) {
      for (const m of ghMat) {
        m.transparent = true;
        m.opacity = 0.35;
      }
    } else {
      ghMat.transparent = true;
      ghMat.opacity = 0.35;
    }
    group.add(gridHelper);

    scene.add(group);
    fitCameraToCup(cupH, cupW);
  }

  /** Point cloud only — does not touch cup outline or camera */
  function refreshPointCloudOnly() {
    clearPointsOnly();

    const obj = objects.length > 0 ? objects[0] : null;
    if (!obj || obj.points_x.length === 0) return;

    const px = obj.points_x,
      py = obj.points_y,
      pz = obj.points_z;
    const n = px.length;

    let minZ = Infinity,
      maxZ = -Infinity;
    let cx = 0,
      cy = 0;
    for (let i = 0; i < n; i++) {
      cx += px[i];
      cy += py[i];
      if (pz[i] < minZ) minZ = pz[i];
      if (pz[i] > maxZ) maxZ = pz[i];
    }
    cx /= n;
    cy /= n;
    const pcHeight = maxZ - minZ || 1;
    const valid = obj.valid !== false;

    const positions = new Float32Array(n * 3);
    const colors = new Float32Array(n * 3);
    for (let i = 0; i < n; i++) {
      positions[i * 3] = px[i] - cx;
      positions[i * 3 + 1] = pz[i] - minZ;
      positions[i * 3 + 2] = py[i] - cy;
      const t = (pz[i] - minZ) / pcHeight;
      if (valid) {
        colors[i * 3] = (PC_VALID_T0.r + t * (PC_VALID_T1.r - PC_VALID_T0.r)) / 255;
        colors[i * 3 + 1] = (PC_VALID_T0.g + t * (PC_VALID_T1.g - PC_VALID_T0.g)) / 255;
        colors[i * 3 + 2] = (PC_VALID_T0.b + t * (PC_VALID_T1.b - PC_VALID_T0.b)) / 255;
      } else {
        colors[i * 3] = (PC_INVALID_T0.r + t * (PC_INVALID_T1.r - PC_INVALID_T0.r)) / 255;
        colors[i * 3 + 1] = (PC_INVALID_T0.g + t * (PC_INVALID_T1.g - PC_INVALID_T0.g)) / 255;
        colors[i * 3 + 2] = (PC_INVALID_T0.b + t * (PC_INVALID_T1.b - PC_INVALID_T0.b)) / 255;
      }
    }
    const geom = new THREE.BufferGeometry();
    geom.setAttribute("position", new THREE.BufferAttribute(positions, 3));
    geom.setAttribute("color", new THREE.BufferAttribute(colors, 3));
    pointsMesh = new THREE.Points(
      geom,
      new THREE.PointsMaterial({ size: 4, vertexColors: true, sizeAttenuation: false }),
    );
    scene.add(pointsMesh);
  }

  function applySquareViewport() {
    if (!renderer || !containerRef) return;
    const w = containerRef.clientWidth;
    const h = containerRef.clientHeight;
    const dpr = renderer.getPixelRatio();
    const wPx = Math.floor(w * dpr);
    const hPx = Math.floor(h * dpr);
    const sidePx = Math.min(wPx, hPx);
    const xPx = Math.floor((wPx - sidePx) / 2);
    const yPx = Math.floor((hPx - sidePx) / 2);
    renderer.setScissorTest(true);
    renderer.setScissor(xPx, yPx, sidePx, sidePx);
    renderer.setViewport(xPx, yPx, sidePx, sidePx);
    camera.aspect = 1;
    camera.updateProjectionMatrix();
  }

  function animate() {
    if (!renderer || !containerRef) {
      animId = requestAnimationFrame(animate);
      return;
    }

    const obj = objects.length > 0 ? objects[0] : null;
    const objK = obj && obj.points_x.length > 0 ? objKey(obj) : "";
    const cupK = `${cupHeightMm}|${cupWidthMm}`;

    if (cupK !== lastCupKey) {
      lastCupKey = cupK;
      refreshCupVisualsAndCamera();
    }

    if (objK !== lastObjKey) {
      lastObjKey = objK;
      refreshPointCloudOnly();
    }

    scene.rotation.y = (performance.now() / 1000) * ROT_SPEED;

    const w = containerRef.clientWidth;
    const h = containerRef.clientHeight;
    const dpr = renderer.getPixelRatio();
    if (
      renderer.domElement.width !== w * dpr ||
      renderer.domElement.height !== h * dpr
    ) {
      renderer.setSize(w, h);
    }
    renderer.setScissorTest(false);
    renderer.setViewport(0, 0, Math.floor(w * dpr), Math.floor(h * dpr));
    renderer.clear();

    applySquareViewport();

    renderer.render(scene, camera);
    animId = requestAnimationFrame(animate);
  }

  onMount(() => {
    if (!containerRef) return;
    renderer = new THREE.WebGLRenderer({ antialias: true });
    renderer.setPixelRatio(window.devicePixelRatio);
    renderer.setSize(containerRef.clientWidth, containerRef.clientHeight);
    containerRef.appendChild(renderer.domElement);
    buildScene();
    animId = requestAnimationFrame(animate);
  });

  onDestroy(() => {
    if (animId !== null) cancelAnimationFrame(animId);
    if (renderer) {
      renderer.dispose();
      renderer.domElement.remove();
    }
  });
</script>

<div bind:this={containerRef} class="pcd-container"></div>

<style>
  .pcd-container {
    width: 100%;
    height: 100%;
    min-height: 0;
    border-radius: 8px;
    overflow: hidden;
    background: #1a1a2e;
  }

  .pcd-container :global(canvas) {
    display: block;
    width: 100%;
    height: 100%;
  }
</style>
