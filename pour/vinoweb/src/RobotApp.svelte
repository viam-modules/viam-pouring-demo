<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { useRobotClient } from "@viamrobotics/svelte-sdk";
  import { GenericServiceClient, ArmClient, VisionClient } from "@viamrobotics/sdk";
  import { Struct } from "@bufbuild/protobuf";
  import MainContent from "./lib/MainContent.svelte";
  import Status from "./lib/status.svelte";
  import type { SegmentedObject, Joint, CupDetectionMetrics } from "./lib/types.js";
  import { parseVisionCupObjects } from "./lib/parseVisionCups.js";

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
  let pollingHandle: ReturnType<typeof setInterval> | null = null;
  let pollingInterval = 250;
  let cupDetailLastFetch = 0;
  const cupDetailRefreshMs = 1000;

  let leftArm: ArmClient | null = null;
  let rightArm: ArmClient | null = null;

  // -- Vision (sam2 segmenters for still-image standby view) ---
  let visionClients: (VisionClient | null)[] = [null, null];
  let imagePollingHandle: ReturnType<typeof setInterval> | null = null;
  let imagePollingInterval = 1000; // ms; sam2 capture is relatively slow
  let imageCaptureInFlight = [false, false];
  // Per-pane failure tracking so a missing/disabled vision service doesn't
  // get hammered forever. After ERROR_THRESHOLD consecutive errors we throttle
  // retries to BACKOFF_MS; any successful call resets the counter.
  let consecutiveErrors = [0, 0];
  let nextRetryAt = [0, 0];
  const ERROR_THRESHOLD = 3;
  const BACKOFF_MS = 15000;

  async function captureStillImage(index: number) {
    const client = visionClients[index];
    if (!client) return;
    if (imageCaptureInFlight[index]) return;
    // Respect backoff window for a pane whose vision service keeps failing.
    if (
      consecutiveErrors[index] >= ERROR_THRESHOLD &&
      Date.now() < nextRetryAt[index]
    ) {
      return;
    }
    imageCaptureInFlight[index] = true;
    try {
      const result = await client.captureAllFromCamera(
        // The vision service config already specifies its camera_name; passing
        // an empty string lets the service use its own configured camera.
        "",
        {
          returnImage: true,
          // We ask for detections only so we can gate on their presence --
          // they're never drawn on the image.
          returnDetections: true,
          returnClassifications: false,
          returnObjectPointClouds: false,
        }
      );
      consecutiveErrors[index] = 0;
      const hasDetection = (result.detections?.length ?? 0) > 0;
      if (hasDetection && result.image) {
        const url = imageToDataUrl(result.image);
        if (url) {
          setStillImageUrl(index, url);
          return;
        }
      }
      // No detection (or no usable image) -> fall back to the live camera.
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
      // Fall back to the live camera stream whenever the vision call fails so
      // the user never sees a stale overlay when the service is down.
      if (stillImageUrls[index]) {
        setStillImageUrl(index, null);
      }
    } finally {
      imageCaptureInFlight[index] = false;
    }
  }

  $effect(() => {
    if (!robotClientStore) return;
    const robotClient = robotClientStore.current;
    if (robotClient && !pollingHandle) {
      if (!leftArm) leftArm = new ArmClient(robotClient, "left-arm");
      if (!rightArm) rightArm = new ArmClient(robotClient, "right-arm");
      if (!cartClient) cartClient = new GenericServiceClient(robotClient, "cart");
      if (!cupVisionClient) cupVisionClient = new VisionClient(robotClient, CUP_VISION_SERVICE);
      for (let i = 0; i < visionServiceNames.length; i++) {
        if (!visionClients[i]) {
          visionClients[i] = new VisionClient(
            robotClient,
            visionServiceNames[i]
          );
        }
      }

      // --- Still-image capture loop (when the demo isn't actively running) ---
      if (!imagePollingHandle) {
        imagePollingHandle = setInterval(() => {
          if (!showStillImages(status)) return;
          captureStillImage(0);
          captureStillImage(1);
        }, imagePollingInterval);
      }

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

        if (Date.now() - cupDetailLastFetch >= cupDetailRefreshMs) {
          try {
            const objects = await cupVisionClient!.getObjectPointClouds("");
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
            cupDetailLastFetch = Date.now();
          } catch (_) {}
        }

        if (leftArm && rightArm) {
          try { const lj = await leftArm.getJointPositions(); leftJoints = lj.values.map((position, index) => ({ index, position })); } catch (_) {}
          try { const rj = await rightArm.getJointPositions(); rightJoints = rj.values.map((position, index) => ({ index, position })); } catch (_) {}
        }
      }, pollingInterval);
    }

    return () => {
      if (pollingHandle) {
        clearInterval(pollingHandle);
        pollingHandle = null;
      }
      if (imagePollingHandle) {
        clearInterval(imagePollingHandle);
        imagePollingHandle = null;
      }
      for (let i = 0; i < stillImageUrls.length; i++) {
        if (stillImageUrls[i]) {
          URL.revokeObjectURL(stillImageUrls[i] as string);
          stillImageUrls[i] = null;
        }
      }
    };
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
