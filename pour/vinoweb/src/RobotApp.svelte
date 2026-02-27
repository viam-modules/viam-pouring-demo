<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { useRobotClient } from "@viamrobotics/svelte-sdk";
  import { GenericServiceClient, VisionClient, ArmClient } from "@viamrobotics/sdk";
  import { Struct } from "@bufbuild/protobuf";
  import MainContent from "./lib/MainContent.svelte";
  import Status from "./lib/status.svelte";
  import CupDetailPanel from "./lib/CupDetailPanel.svelte";
  import type { SegmentedObject } from "./lib/CupDetailPanel.svelte";
  import { parsePCD } from "./lib/parsePCD.js";
  import type { Joint } from "./lib/types.js";

  type StatusKey =
    | "standby"
    | "looking"
    | "picking"
    | "prepping"
    | "pouring"
    | "placing"
    | "waiting"
    | "manual mode";
  let status: StatusKey = $state("standby") as StatusKey;

  let objectCount = $state(0);
  let segmentedObjects: SegmentedObject[] = $state([]);

  let cupPanelOpen = $state(false);
  function toggleCupPanel() { cupPanelOpen = !cupPanelOpen; }

  const statusMessages: Record<StatusKey, string> = {
    standby: "Ready to pour!",
    looking: "Please place your glass in the indicated area",
    picking: "Thank you, just a moment",
    prepping: "Preparing to pour...",
    pouring: "Pouring...",
    placing: "Placing glass down",
    waiting: "Please enjoy!",
    "manual mode": "Manual mode active",
  };

  function handleKeydown(event: KeyboardEvent) {
    const keys = Object.keys(statusMessages) as StatusKey[];
    const keyNum = parseInt(event.key);
    if (keyNum >= 1 && keyNum <= keys.length) status = keys[keyNum - 1];
  }

  onMount(() => {
    window.addEventListener("keydown", handleKeydown);
    return () => window.removeEventListener("keydown", handleKeydown);
  });

  function* jointGenerator() {
    for (let index = 0; index < 6; index++) yield { index, position: 0 } as Joint;
  }
  const initialJoints = Array.from(jointGenerator()) as Joint[];
  let leftJoints = $state([...initialJoints]);
  let rightJoints = $state([...initialJoints]);

  let panesData = $state([
    { joints: leftJoints, tableTitle: "Left Arm", camera: { name: "left-cam", partID: "xxx", label: "Left Camera" } },
    { joints: rightJoints, tableTitle: "Right Arm", camera: { name: "right-cam", partID: "xxx", label: "Right Camera" } },
  ]);

  const robotClientStore = useRobotClient(() => "xxx");
  let cartClient: GenericServiceClient | null = null;
  let visionClient: VisionClient | null = null;
  let pollingHandle: ReturnType<typeof setInterval> | null = null;
  let pollingInterval = 250;
  let visionLastFetch = 0;
  const visionRefreshMs = 1000;

  let leftArm: ArmClient | null = null;
  let rightArm: ArmClient | null = null;

  $effect(() => {
    const robotClient = robotClientStore.current;
    $inspect(robotClient, "robotClient");
    if (robotClient && !pollingHandle) {
      if (!leftArm) leftArm = new ArmClient(robotClient, "left-arm");
      if (!rightArm) rightArm = new ArmClient(robotClient, "right-arm");
      if (!cartClient) cartClient = new GenericServiceClient(robotClient, "cart");
      if (!visionClient) visionClient = new VisionClient(robotClient, "cup-finder-segment");

      pollingHandle = setInterval(async () => {
        try {
          const result = await cartClient!.doCommand(Struct.fromJson({ status: true }));
          if (result && typeof result === "object") {
            const r = result as any;
            if ("status" in r && typeof r.status === "string") {
              const s = r.status;
              if ((Object.keys(statusMessages) as StatusKey[]).includes(s as StatusKey)) status = s as StatusKey;
            }
          }
        } catch (_) {}

        if (Date.now() - visionLastFetch >= visionRefreshMs) {
          try {
            const pcos = await visionClient!.getObjectPointClouds("");
            objectCount = pcos.length;
            segmentedObjects = pcos.map((pco, idx) => {
              const pc = parsePCD(pco.pointCloud);

              // Unit detection: if points look like meters (max abs < 10), convert to mm
              let maxAbs = 0;
              for (let k = 0; k < pc.x.length; k++) {
                const ax = Math.abs(pc.x[k]), ay = Math.abs(pc.y[k]), az = Math.abs(pc.z[k]);
                if (ax > maxAbs) maxAbs = ax;
                if (ay > maxAbs) maxAbs = ay;
                if (az > maxAbs) maxAbs = az;
              }
              const unitScale = maxAbs < 10 ? 1000 : 1;
              if (unitScale !== 1) {
                for (let k = 0; k < pc.x.length; k++) {
                  pc.x[k] *= unitScale;
                  pc.y[k] *= unitScale;
                  pc.z[k] *= unitScale;
                }
                console.log(`[vision] Object ${idx}: PCD in meters, converted to mm (scale=${unitScale}, maxAbs=${maxAbs.toFixed(4)})`);
              }

              if (idx === 0 && pc.x.length > 0) {
                let sx = 0, sy = 0, sz = 0;
                for (let k = 0; k < pc.x.length; k++) { sx += pc.x[k]; sy += pc.y[k]; sz += pc.z[k]; }
                sx /= pc.x.length; sy /= pc.x.length; sz /= pc.x.length;
                console.log(`[vision] Object 0 centroid: (${sx.toFixed(1)}, ${sy.toFixed(1)}, ${sz.toFixed(1)}) mm, ${pc.x.length} pts`);
              }

              let dims: { x: number; y: number; z: number } | undefined;
              let position: { x: number; y: number; z: number } | undefined;
              const geo = pco.geometries?.geometries?.[0];
              if (geo) {
                if (geo.center) {
                  position = { x: geo.center.x, y: geo.center.y, z: geo.center.z };
                }
                if (geo.geometryType.case === "box" && geo.geometryType.value.dimsMm) {
                  const d = geo.geometryType.value.dimsMm;
                  dims = { x: d.x, y: d.y, z: d.z };
                }
              }
              return {
                index: idx,
                totalPoints: pc.x.length,
                points_x: pc.x, points_y: pc.y, points_z: pc.z,
                rawPCD: pco.pointCloud,
                dims,
                position,
              };
            });
            visionLastFetch = Date.now();
          } catch (_) {}
        }

        if (leftArm && rightArm) {
          try { const lj = await leftArm.getJointPositions(); panesData[0].joints = lj.values.map((position, index) => ({ index, position })); } catch (_) {}
          try { const rj = await rightArm.getJointPositions(); panesData[1].joints = rj.values.map((position, index) => ({ index, position })); } catch (_) {}
          panesData = panesData;
        }
      }, pollingInterval);
    }
    return () => { if (pollingHandle) { clearInterval(pollingHandle); pollingHandle = null; } };
  });
</script>

<div class="app-container">
  <aside class="sidebar"></aside>
  <MainContent panes={panesData} {status} {cupPanelOpen}>
    {#snippet statusBar()}
      <Status message={statusMessages[status]} {objectCount} onCupClick={toggleCupPanel} {cupPanelOpen} />
    {/snippet}
    {#snippet detailPanel()}
      {#if cupPanelOpen}
        <CupDetailPanel
          objects={segmentedObjects}
          robotClient={robotClientStore.current ?? null}
          onClose={() => cupPanelOpen = false}
        />
      {/if}
    {/snippet}
  </MainContent>
</div>

<style>
  .app-container {
    height: calc(100vh - 80px);
    width: 100%;
    max-width: 1920px;
    margin: 0 auto;
    display: grid;
    grid-template-columns: 34.4% 65.6%;
    grid-template-rows: 1fr;
    overflow: hidden;
  }
  .sidebar { color: white; padding: 40px; overflow-y: auto; }
</style>
