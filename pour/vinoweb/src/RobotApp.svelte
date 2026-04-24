<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { useRobotClient } from "@viamrobotics/svelte-sdk";
  import { GenericServiceClient } from "@viamrobotics/sdk";
  import { ArmClient } from "@viamrobotics/sdk";
  import { VisionClient } from "@viamrobotics/sdk";
  import { Struct, type JsonValue } from "@bufbuild/protobuf";
  import MainContent from "./lib/MainContent.svelte";
  import Status from "./lib/status.svelte";
  import type { Joint } from "./lib/types.js";

  // --- Pouring status ---
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

  // --- Keyboard controls for debugging ---
  function handleKeydown(event: KeyboardEvent) {
    const keys = Object.keys(statusMessages) as StatusKey[];
    const keyNum = parseInt(event.key);
    if (keyNum >= 1 && keyNum <= keys.length) {
      status = keys[keyNum - 1];
    }
  }

  onMount(() => {
    window.addEventListener("keydown", handleKeydown);
    return () => window.removeEventListener("keydown", handleKeydown);
  });

  // --- Generate initial joints ---
  function* jointGenerator() {
    for (let index = 0; index < 6; index++) {
      yield { index, position: 0 } as Joint;
    }
  }
  const initialJoints = Array.from(jointGenerator()) as Joint[];

  // --- $state-ful joint arrays ---
  let leftJoints = $state([...initialJoints]);
  let rightJoints = $state([...initialJoints]);

  // --- Define panes data ---
  let panesData = $state([
    {
      joints: leftJoints,
      tableTitle: "Left Arm",
      camera: {
        name: "left-cam",
        partID: "xxx",
        label: "Left Camera",
      },
      stillImageUrl: null as string | null,
    },
    {
      joints: rightJoints,
      tableTitle: "Right Arm",
      camera: {
        name: "right-cam",
        partID: "xxx",
        label: "Right Camera",
      },
      stillImageUrl: null as string | null,
    },
  ]);

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
    const prev = panesData[index].stillImageUrl;
    panesData[index].stillImageUrl = url;
    if (prev) URL.revokeObjectURL(prev);
  }

  // --- Robot client and polling logic ---
  const robotClientStore = useRobotClient(() => "xxx");
  let generic: GenericServiceClient | null = null;
  let pollingHandle: ReturnType<typeof setInterval> | null = null;
  let pollingInterval = 250; // Polling interval in milliseconds

  // -- Robot Arms ---
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
          panesData = panesData;
          return;
        }
      }
      // No detection (or no usable image) -> fall back to the live camera.
      if (panesData[index].stillImageUrl) {
        setStillImageUrl(index, null);
        panesData = panesData;
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
      if (panesData[index].stillImageUrl) {
        setStillImageUrl(index, null);
        panesData = panesData;
      }
    } finally {
      imageCaptureInFlight[index] = false;
    }
  }

  $effect(() => {
    const robotClient = robotClientStore.current;
    $inspect(robotClient, "robotClient");
    if (robotClient && !pollingHandle) {
      if (!leftArm) leftArm = new ArmClient(robotClient, "left-arm");
      if (!rightArm) rightArm = new ArmClient(robotClient, "right-arm");
      if (!generic) generic = new GenericServiceClient(robotClient, "cart");
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
        // --- Status ---
        try {
          const result = await generic!.doCommand(
            Struct.fromJson({ status: true })
          );
          if (
            result &&
            typeof result === "object" &&
            "status" in result &&
            typeof (result as any).status === "string"
          ) {
            const statusStr = (result as any).status;
            if (
              (Object.keys(statusMessages) as StatusKey[]).includes(
                statusStr as StatusKey
              )
            ) {
              status = statusStr as StatusKey;
            }
          }
        } catch (err) {
          // Optionally handle status polling error
        }

        // --- Joint positions ---
        if (leftArm && rightArm) {
          try {
            const leftJoints = await leftArm.getJointPositions();
            panesData[0].joints = leftJoints.values.map((position, index) => ({
              index,
              position,
            }));
          } catch (err) {
            // Optionally handle left arm error
          }
          try {
            const rightJoints = await rightArm.getJointPositions();
            panesData[1].joints = rightJoints.values.map((position, index) => ({
              index,
              position,
            }));
          } catch (err) {
            // Optionally handle right arm error
          }
          panesData = panesData; // triggers $state reactivity without remounting children
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
      for (let i = 0; i < panesData.length; i++) {
        if (panesData[i].stillImageUrl) {
          URL.revokeObjectURL(panesData[i].stillImageUrl as string);
          panesData[i].stillImageUrl = null;
        }
      }
    };
  });

  // Drop the still images as soon as the demo begins running so the live
  // camera streams take over immediately.
  $effect(() => {
    if (!showStillImages(status)) {
      for (let i = 0; i < panesData.length; i++) {
        if (panesData[i].stillImageUrl) {
          setStillImageUrl(i, null);
        }
      }
    }
  });
</script>

<div class="app-container">
  <aside class="sidebar"></aside>

  <MainContent panes={panesData} {status}>
    {#snippet statusBar()}
      <Status message={statusMessages[status]} />
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
  .sidebar {
    color: white;
    padding: 40px;
    overflow-y: auto;
  }
</style>
