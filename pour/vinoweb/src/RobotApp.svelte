<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { useRobotClient } from "@viamrobotics/svelte-sdk";
  import { GenericServiceClient, ArmClient, VisionClient } from "@viamrobotics/sdk";
  import { Struct } from "@bufbuild/protobuf";
  import MainContent from "./lib/MainContent.svelte";
  import Status from "./lib/status.svelte";
  import type { SegmentedObject, Joint, CupDetectionMetrics } from "./lib/types.js";
  import { parseVisionCupObjects } from "./lib/parseVisionCups.js";
  import { acquireVisionPoll } from "./lib/visionPoll.js";
  import { timeVisionCall } from "./lib/visionLatency.js";

  const CUP_VISION_SERVICE = "cup-detection";

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

  // Statuses where the pouring demo is actively running. In any other status
  // (standby, waiting, manual mode) we show the SAM segmenter still images
  // instead of the live camera streams.
  const demoRunningStatuses = new Set<StatusKey>([
    "looking",
    "picking",
    "prepping",
    "pouring",
    "placing",
  ]);
  const showStillImages = (s: StatusKey) => !demoRunningStatuses.has(s);

  // Only poll cup PCD while the debug/detection UI is shown (matches MainContent).
  const cupDetectionStatuses = new Set<StatusKey>(["manual mode", "standby", "looking"]);

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

  // SAM still-image URLs for left/right camera panes
  let stillImageUrls = $state<[string | null, string | null]>([null, null]);

  // --- Vision services for standby still images (sam2 segmenters) ---
  const visionServiceNames = ["sam2-segmenter-left", "sam2-segmenter-right"];
  // Format enum values from viam.component.camera.v1.Format
  const FORMAT_JPEG = 3;
  const FORMAT_PNG = 4;
  function imageToDataUrl(image: {
    format: number;
    image: Uint8Array;
  }): string | null {
    if (!image.image || image.image.length === 0) return null;
    const mime =
      image.format === FORMAT_PNG ? "image/png" : "image/jpeg";
    // Copy into a fresh ArrayBuffer to satisfy strict BlobPart typing
    // (the proto-generated Uint8Array has an ArrayBufferLike backing store).
    const buf = new Uint8Array(image.image.byteLength);
    buf.set(image.image);
    const blob = new Blob([buf.buffer], { type: mime });
    return URL.createObjectURL(blob);
  }
  function setStillImageUrl(index: number, url: string | null) {
    const prev = stillImageUrls[index];
    stillImageUrls[index] = url;
    stillImageUrls = stillImageUrls;
    if (prev) URL.revokeObjectURL(prev);
  }

  const robotClientStore = useRobotClient(() => "xxx");
  let cartClient: GenericServiceClient | null = null;
  let cupVisionClient: VisionClient | null = null;
  let lightPollingHandle: ReturnType<typeof setInterval> | null = null;
  const lightPollingIntervalMs = 250;

  // Independent poll cadences (do not share one loop).
  const stillCooldownMs = 300;
  const cupCooldownMs = 1500;

  let leftArm: ArmClient | null = null;
  let rightArm: ArmClient | null = null;

  let visionClients: (VisionClient | null)[] = [null, null];
  let consecutiveErrors = [0, 0];
  let nextRetryAt = [0, 0];
  const ERROR_THRESHOLD = 3;
  const BACKOFF_MS = 15000;

  let releaseVisionPoll: (() => void) | null = null;

  async function captureStillImage(index: number) {
    const client = visionClients[index];
    if (!client) return;
    if (
      consecutiveErrors[index] >= ERROR_THRESHOLD &&
      Date.now() < nextRetryAt[index]
    ) {
      return;
    }
    const label = `sam-still ${visionServiceNames[index]}`;
    try {
      const result = await timeVisionCall(label, () =>
        client.captureAllFromCamera("", {
          returnImage: true,
          returnDetections: true,
          returnClassifications: false,
          returnObjectPointClouds: false,
        })
      , (r) => `detections=${r.detections?.length ?? 0} hasImage=${!!r.image}`);
      consecutiveErrors[index] = 0;
      const hasDetection = (result.detections?.length ?? 0) > 0;
      if (hasDetection && result.image) {
        const url = imageToDataUrl(result.image);
        if (url) {
          setStillImageUrl(index, url);
          return;
        }
      }
      if (stillImageUrls[index]) {
        setStillImageUrl(index, null);
      }
    } catch (err) {
      consecutiveErrors[index] += 1;
      nextRetryAt[index] = Date.now() + BACKOFF_MS;
      if (consecutiveErrors[index] === 1 || consecutiveErrors[index] === ERROR_THRESHOLD) {
        console.warn(
          `[${visionServiceNames[index]}] captureAllFromCamera failed (errors=${consecutiveErrors[index]}):`,
          err
        );
      }
      if (stillImageUrls[index]) {
        setStillImageUrl(index, null);
      }
    }
  }

  async function fetchCupPointClouds() {
    if (!cupVisionClient) return;
    try {
      const objects = await timeVisionCall(
        "cup-detection getObjectPointClouds",
        () => cupVisionClient!.getObjectPointClouds(""),
        (objs) => `objects=${objs?.length ?? 0}`
      );
      const parsed = parseVisionCupObjects(objects);
      cupHeightMm = parsed.summary.cupHeightMm;
      cupWidthMm = parsed.summary.cupWidthMm;
      objectCount = parsed.summary.objectCount;

      cupDetectionMetrics = parsed.metrics;
      if (parsed.cups.length === 0) {
        segmentedObjects = [];
      } else {
        const best = parsed.cups.find((c) => c.valid) ?? parsed.cups[0];
        segmentedObjects = [best];
      }
      console.info(
        `[latency] cup-detection parse  cups=${parsed.cups.length} points=${parsed.bestCup?.totalPoints ?? 0} ` +
          `valid=${parsed.summary.validCups} heightMm=${Math.round(cupHeightMm)} widthMm=${Math.round(cupWidthMm)} ` +
          `rawObjs=${objects?.length ?? 0}`
      );
    } catch (_) {
      // Latency FAIL line already logged by timeVisionCall; keep last PCD on screen.
    }
  }

  function wireClients(robotClient: NonNullable<typeof robotClientStore.current>) {
    if (!leftArm) leftArm = new ArmClient(robotClient, "left-arm");
    if (!rightArm) rightArm = new ArmClient(robotClient, "right-arm");
    if (!cartClient) cartClient = new GenericServiceClient(robotClient, "cart");
    if (!cupVisionClient) cupVisionClient = new VisionClient(robotClient, CUP_VISION_SERVICE);
    for (let i = 0; i < visionServiceNames.length; i++) {
      if (!visionClients[i]) {
        visionClients[i] = new VisionClient(robotClient, visionServiceNames[i]);
      }
    }

    if (!releaseVisionPoll) {
      releaseVisionPoll = acquireVisionPoll({
        shouldPollCup: () => cupDetectionStatuses.has(status),
        shouldPollStills: () => showStillImages(status),
        fetchCup: fetchCupPointClouds,
        captureStill: captureStillImage,
        stillCooldownMs,
        cupCooldownMs,
      });
    }

    if (!lightPollingHandle) {
      lightPollingHandle = setInterval(async () => {
        try {
          const result = await cartClient!.doCommand(Struct.fromJson({ status: true }));
          if (result && typeof result === "object") {
            const r = result as any;
            if ("status" in r && typeof r.status === "string") {
              const s = r.status;
              if ((Object.keys(statusMessages) as StatusKey[]).includes(s as StatusKey)) {
                status = s as StatusKey;
              }
            }
          }
        } catch (_) {}

        if (leftArm && rightArm) {
          try {
            const lj = await leftArm.getJointPositions();
            leftJoints = lj.values.map((position, index) => ({ index, position }));
          } catch (_) {}
          try {
            const rj = await rightArm.getJointPositions();
            rightJoints = rj.values.map((position, index) => ({ index, position }));
          } catch (_) {}
        }
      }, lightPollingIntervalMs);
    }
  }

  // Only react to robot client identity becoming available — do not restart
  // vision polls on every store tick.
  $effect(() => {
    if (!robotClientStore) return;
    const robotClient = robotClientStore.current;
    if (robotClient) {
      wireClients(robotClient);
    }
  });

  onDestroy(() => {
    if (releaseVisionPoll) {
      releaseVisionPoll();
      releaseVisionPoll = null;
    }
    if (lightPollingHandle) {
      clearInterval(lightPollingHandle);
      lightPollingHandle = null;
    }
    for (let i = 0; i < stillImageUrls.length; i++) {
      if (stillImageUrls[i]) {
        URL.revokeObjectURL(stillImageUrls[i] as string);
        stillImageUrls[i] = null;
      }
    }
  });

  // Drop the still images as soon as the demo begins running so the live
  // camera streams take over immediately.
  $effect(() => {
    if (!showStillImages(status)) {
      for (let i = 0; i < stillImageUrls.length; i++) {
        if (stillImageUrls[i]) {
          setStillImageUrl(i, null);
        }
      }
    }
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
    leftStillImageUrl={stillImageUrls[0]}
    rightStillImageUrl={stillImageUrls[1]}
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
