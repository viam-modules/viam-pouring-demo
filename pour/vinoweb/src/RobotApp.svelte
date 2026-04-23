<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { useRobotClient } from "@viamrobotics/svelte-sdk";
  import { GenericServiceClient, ArmClient } from "@viamrobotics/sdk";
  import { Struct } from "@bufbuild/protobuf";
  import MainContent from "./lib/MainContent.svelte";
  import Status from "./lib/status.svelte";
  import type { SegmentedObject, Joint, CupDetectionMetrics } from "./lib/types.js";

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
  let cupHeightMm = $state(0);
  let cupWidthMm = $state(0);
  let cupDetectionMetrics = $state<CupDetectionMetrics | null>(null);

  function cupMetricsFromApi(c: Record<string, unknown>): CupDetectionMetrics {
    const num = (k: string) => {
      const v = Number(c[k]);
      return Number.isFinite(v) ? v : 0;
    };
    return {
      valid: !!c.valid,
      expectedHeight: num("expected_height"),
      observedHeight: num("height"),
      heightDelta: num("height_delta"),
      heightPass: !!c.height_pass,
      expectedWidth: num("expected_width"),
      observedWidth: num("width"),
      widthDelta: num("width_delta"),
      widthPass: !!c.width_pass,
      toleranceMm: num("good_delta"),
    };
  }

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

  const robotClientStore = useRobotClient(() => "xxx");
  let cartClient: GenericServiceClient | null = null;
  let pollingHandle: ReturnType<typeof setInterval> | null = null;
  let pollingInterval = 250;
  let cupDetailLastFetch = 0;
  const cupDetailRefreshMs = 1000;

  let leftArm: ArmClient | null = null;
  let rightArm: ArmClient | null = null;

  $effect(() => {
    const robotClient = robotClientStore.current;
    $inspect(robotClient, "robotClient");
    if (robotClient && !pollingHandle) {
      if (!leftArm) leftArm = new ArmClient(robotClient, "left-arm");
      if (!rightArm) rightArm = new ArmClient(robotClient, "right-arm");
      if (!cartClient) cartClient = new GenericServiceClient(robotClient, "cart");

      pollingHandle = setInterval(async () => {
        try {
          const result = await cartClient!.doCommand(Struct.fromJson({ status: true }));
          if (result && typeof result === "object") {
            const r = result as any;
            if ("status" in r && typeof r.status === "string") {
              const s = r.status;
              if ((Object.keys(statusMessages) as StatusKey[]).includes(s as StatusKey)) status = s as StatusKey;
            }
            const ch = Number(r.cup_height);
            const cw = Number(r.cup_width);
            if (!Number.isNaN(ch) && ch > 0) cupHeightMm = ch;
            if (!Number.isNaN(cw) && cw > 0) cupWidthMm = cw;
          }
        } catch (_) {}

        if (Date.now() - cupDetailLastFetch >= cupDetailRefreshMs) {
          try {
            const result = await cartClient!.doCommand(Struct.fromJson({ cup_details: true }));
            const cups = (result as any)?.cups as any[] ?? [];
            objectCount = cups.length;

            // Pick the first valid cup, or first cup if none are valid
            const bestCup = cups.find((c: any) => c.valid) ?? cups[0];
            if (bestCup) {
              cupDetectionMetrics = cupMetricsFromApi(bestCup as Record<string, unknown>);
              segmentedObjects = [{
                index: bestCup.index,
                totalPoints: bestCup.total_points,
                points_x: bestCup.points_x,
                points_y: bestCup.points_y,
                points_z: bestCup.points_z,
                valid: bestCup.valid,
              }];
            } else {
              cupDetectionMetrics = null;
              segmentedObjects = [];
            }
            cupDetailLastFetch = Date.now();
          } catch (_) {}
        }

        if (leftArm && rightArm) {
          try { const lj = await leftArm.getJointPositions(); leftJoints = lj.values.map((position, index) => ({ index, position })); } catch (_) {}
          try { const rj = await rightArm.getJointPositions(); rightJoints = rj.values.map((position, index) => ({ index, position })); } catch (_) {}
        }
      }, pollingInterval);
    }
    return () => { if (pollingHandle) { clearInterval(pollingHandle); pollingHandle = null; } };
  });
</script>

<div class="app-container">
  <aside class="sidebar"></aside>
  <MainContent
    {segmentedObjects}
    {leftJoints}
    {rightJoints}
    {status}
    {cupHeightMm}
    {cupWidthMm}
    {cupDetectionMetrics}
  >
    {#snippet statusBar()}
      <Status message={statusMessages[status]} {objectCount} />
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
