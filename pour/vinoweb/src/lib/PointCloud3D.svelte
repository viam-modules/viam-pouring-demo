<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import * as THREE from "three";
  import { CUP_HEIGHT, RINGS, type SegmentedObject } from "./types.js";

  let { objects = [] }: { objects: SegmentedObject[] } = $props();

  let containerRef: HTMLDivElement | undefined = $state();
  let renderer: THREE.WebGLRenderer | null = null;
  let scene: THREE.Scene;
  let camera: THREE.PerspectiveCamera;
  let animId: number | null = null;
  let pointsMesh: THREE.Points | null = null;
  let ringLines: THREE.LineLoop[] = [];
  let profileLines: THREE.Line[] = [];
  let centerLine: THREE.Line | null = null;
  let gridHelper: THREE.GridHelper | null = null;
  let lastObjKey = "";

  const BG_COLOR = 0x1a1a2e;
  const ROT_SPEED = 0.3;

  function buildScene() {
    scene = new THREE.Scene();
    scene.background = new THREE.Color(BG_COLOR);

    camera = new THREE.PerspectiveCamera(40, 1, 0.1, 10000);
    camera.position.set(0, 120, 200);
    camera.lookAt(0, 40, 0);

    const axes = new THREE.AxesHelper(25);
    scene.add(axes);
  }

  function objKey(obj: SegmentedObject): string {
    const px = obj.points_x;
    return `${obj.index}:${px.length}:${px[0]}:${px[px.length - 1]}`;
  }

  function clearObject() {
    if (pointsMesh) { scene.remove(pointsMesh); pointsMesh.geometry.dispose(); (pointsMesh.material as THREE.Material).dispose(); pointsMesh = null; }
    for (const l of ringLines) { scene.remove(l); l.geometry.dispose(); (l.material as THREE.Material).dispose(); }
    ringLines = [];
    for (const l of profileLines) { scene.remove(l); l.geometry.dispose(); (l.material as THREE.Material).dispose(); }
    profileLines = [];
    if (centerLine) { scene.remove(centerLine); centerLine.geometry.dispose(); (centerLine.material as THREE.Material).dispose(); centerLine = null; }
    if (gridHelper) { scene.remove(gridHelper); gridHelper = null; }
  }

  function updateObject(obj: SegmentedObject) {
    clearObject();
    const px = obj.points_x, py = obj.points_y, pz = obj.points_z;
    const n = px.length;
    if (n === 0) return;

    let minZ = Infinity, maxZ = -Infinity;
    let cx = 0, cy = 0;
    for (let i = 0; i < n; i++) {
      cx += px[i]; cy += py[i];
      if (pz[i] < minZ) minZ = pz[i];
      if (pz[i] > maxZ) maxZ = pz[i];
    }
    cx /= n; cy /= n;
    const pcHeight = (maxZ - minZ) || 1;

    // Point cloud: center XY at origin, Z starts at 0
    const positions = new Float32Array(n * 3);
    const colors = new Float32Array(n * 3);
    for (let i = 0; i < n; i++) {
      positions[i * 3] = px[i] - cx;
      positions[i * 3 + 1] = pz[i] - minZ;
      positions[i * 3 + 2] = py[i] - cy;
      const t = (pz[i] - minZ) / pcHeight;
      colors[i * 3] = (60 + t * 100) / 255;
      colors[i * 3 + 1] = (160 + t * 95) / 255;
      colors[i * 3 + 2] = (255 - t * 120) / 255;
    }
    const geom = new THREE.BufferGeometry();
    geom.setAttribute("position", new THREE.BufferAttribute(positions, 3));
    geom.setAttribute("color", new THREE.BufferAttribute(colors, 3));
    pointsMesh = new THREE.Points(geom, new THREE.PointsMaterial({ size: 4, vertexColors: true, sizeAttenuation: false }));
    scene.add(pointsMesh);

    // Cup rings
    const segments = 64;
    for (const ring of RINGS) {
      const ringY = (ring.heightFromFloor / CUP_HEIGHT) * pcHeight;
      const pts: THREE.Vector3[] = [];
      for (let i = 0; i <= segments; i++) {
        const a = (i / segments) * Math.PI * 2;
        pts.push(new THREE.Vector3(Math.cos(a) * ring.radius, ringY, Math.sin(a) * ring.radius));
      }
      const lineGeom = new THREE.BufferGeometry().setFromPoints(pts);
      const c = new THREE.Color(ring.color);
      const line = new THREE.LineLoop(lineGeom, new THREE.LineDashedMaterial({ color: c, dashSize: 3, gapSize: 2, transparent: true, opacity: 0.7 }));
      line.computeLineDistances();
      scene.add(line);
      ringLines.push(line);
    }

    // Profile lines connecting rings
    const profileCount = 8;
    for (let ai = 0; ai < profileCount; ai++) {
      const a = (ai / profileCount) * Math.PI * 2;
      const pts: THREE.Vector3[] = RINGS.map(ring => {
        const ringY = (ring.heightFromFloor / CUP_HEIGHT) * pcHeight;
        return new THREE.Vector3(Math.cos(a) * ring.radius, ringY, Math.sin(a) * ring.radius);
      });
      const lineGeom = new THREE.BufferGeometry().setFromPoints(pts);
      const line = new THREE.Line(lineGeom, new THREE.LineDashedMaterial({ color: 0xe8a0f5, dashSize: 2, gapSize: 3, transparent: true, opacity: 0.25 }));
      line.computeLineDistances();
      scene.add(line);
      profileLines.push(line);
    }

    // Center line
    const clGeom = new THREE.BufferGeometry().setFromPoints([new THREE.Vector3(0, 0, 0), new THREE.Vector3(0, pcHeight, 0)]);
    centerLine = new THREE.Line(clGeom, new THREE.LineDashedMaterial({ color: 0xe8a0f5, dashSize: 2, gapSize: 2, transparent: true, opacity: 0.4 }));
    centerLine.computeLineDistances();
    scene.add(centerLine);

    // Grid at floor
    gridHelper = new THREE.GridHelper(240, 8, 0x505050, 0x505050);
    (gridHelper.material as THREE.Material).transparent = true;
    (gridHelper.material as THREE.Material).opacity = 0.35;
    scene.add(gridHelper);

    // Fit camera to bounding sphere of all content with padding
    const bbox = new THREE.Box3();
    if (pointsMesh) bbox.expandByObject(pointsMesh);
    for (const l of ringLines) bbox.expandByObject(l);
    if (centerLine) bbox.expandByObject(centerLine);
    if (gridHelper) bbox.expandByObject(gridHelper);

    const sphere = new THREE.Sphere();
    bbox.getBoundingSphere(sphere);
    const fov = camera.fov * (Math.PI / 180);
    const padding = 1.35;
    const dist = (sphere.radius * padding) / Math.sin(fov / 2);
    const center = sphere.center;
    camera.position.set(center.x, center.y + dist * 0.35, center.z + dist * 0.85);
    camera.lookAt(center);
  }

  function animate() {
    if (!renderer || !containerRef) { animId = requestAnimationFrame(animate); return; }

    const obj = objects.length > 0 ? objects[0] : null;
    const key = obj ? objKey(obj) : "";
    if (key !== lastObjKey) {
      lastObjKey = key;
      if (obj && obj.points_x.length > 0) updateObject(obj);
      else clearObject();
    }

    // Rotate the whole scene around Y
    const t = performance.now() / 1000;
    scene.rotation.y = t * ROT_SPEED;

    // Handle resize
    const w = containerRef.clientWidth, h = containerRef.clientHeight;
    if (renderer.domElement.width !== w * devicePixelRatio || renderer.domElement.height !== h * devicePixelRatio) {
      renderer.setSize(w, h);
      camera.aspect = w / h;
      camera.updateProjectionMatrix();
    }

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
    if (renderer) { renderer.dispose(); renderer.domElement.remove(); }
  });
</script>

<div bind:this={containerRef} class="pcd-container"></div>

<style>
  .pcd-container {
    width: 100%;
    height: 100%;
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
