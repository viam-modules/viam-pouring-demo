<script lang="ts">
  import { CameraStream } from "@viamrobotics/svelte-sdk";
  import type { Snippet } from "svelte";
  import { Tag } from "carbon-components-svelte";

  interface Props {
    name: string;
    partID: string;
    label: string;
    overlay?: Snippet;
  }

  let { name, partID, label, overlay }: Props = $props();

  let streamKey = $state(0);
  let reconnecting = $state(false);
  let containerRef: HTMLDivElement | undefined = $state();

  const HEALTH_CHECK_MS = 3000;
  const RECONNECT_DELAY_MS = 2000;

  $effect(() => {
    void streamKey;

    let healthTimer: ReturnType<typeof setInterval> | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let trackListeners: { track: MediaStreamTrack; handler: () => void }[] = [];
    let settled = false;

    const mountDelay = setTimeout(() => {
      settled = true;
      if (!containerRef) return;
      const video = containerRef.querySelector("video");
      if (!video) return;

      function isStreamDead(): boolean {
        if (!video!.srcObject || !(video!.srcObject instanceof MediaStream)) {
          return false;
        }
        const tracks = video!.srcObject.getTracks();
        return tracks.length > 0 && tracks.every((t) => t.readyState === "ended");
      }

      function doReconnect() {
        if (reconnecting) return;
        reconnecting = true;
        cleanupTrackListeners();
        if (healthTimer) clearInterval(healthTimer);
        healthTimer = null;

        reconnectTimer = setTimeout(() => {
          streamKey++;
          reconnecting = false;
        }, RECONNECT_DELAY_MS);
      }

      function attachTrackListeners() {
        cleanupTrackListeners();
        if (!video!.srcObject || !(video!.srcObject instanceof MediaStream)) return;
        for (const track of video!.srcObject.getTracks()) {
          const handler = () => doReconnect();
          track.addEventListener("ended", handler);
          trackListeners.push({ track, handler });
        }
      }

      function cleanupTrackListeners() {
        for (const { track, handler } of trackListeners) {
          track.removeEventListener("ended", handler);
        }
        trackListeners = [];
      }

      healthTimer = setInterval(() => {
        if (reconnecting) return;
        if (!video!.srcObject || !(video!.srcObject instanceof MediaStream)) return;

        attachTrackListeners();

        if (isStreamDead()) {
          doReconnect();
        }
      }, HEALTH_CHECK_MS);
    }, 1000);

    return () => {
      clearTimeout(mountDelay);
      if (healthTimer) clearInterval(healthTimer);
      if (reconnectTimer) clearTimeout(reconnectTimer);
      for (const { track, handler } of trackListeners) {
        track.removeEventListener("ended", handler);
      }
    };
  });
</script>

<div class="camera-feed" bind:this={containerRef}>
  {#key streamKey}
    <CameraStream {name} {partID} />
  {/key}

  {#if reconnecting}
    <div class="reconnect-overlay">
      <div class="reconnect-spinner"></div>
      <span class="reconnect-text">Reconnecting...</span>
    </div>
  {/if}

  {#if overlay}
    <div class="overlay left">
      {@render overlay()}
    </div>
  {/if}
  <div class="camera-label">
    <Tag type="blue" size="sm">{label}</Tag>
  </div>
</div>

<style>
  .camera-feed {
    position: relative;
    height: 100%;
    aspect-ratio: 16/9;
    border-radius: 12px;
    overflow: hidden;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
    background: #000;
    display: flex;
    justify-content: center;
    align-items: center;
    max-width: 100%;
  }

  .camera-feed :global(video) {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .camera-label {
    position: absolute;
    top: 16px;
    right: 16px;
    z-index: 10;
    user-select: none;
    pointer-events: none;
  }

  .camera-label :global(.bx--tag) {
    font-size: 0.875rem;
    letter-spacing: 0.16px;
    font-weight: 600;
    padding: 0 12px;
    height: 24px;
    line-height: 24px;
    background-color: rgba(180, 37, 244, 0.85);
    color: #ffffff;
    border: none;
    border-radius: 15px;
  }

  .reconnect-overlay {
    position: absolute;
    inset: 0;
    z-index: 15;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 12px;
    background: rgba(0, 0, 0, 0.7);
    backdrop-filter: blur(4px);
  }

  .reconnect-spinner {
    width: 28px;
    height: 28px;
    border: 3px solid rgba(255, 255, 255, 0.2);
    border-top-color: #4589ff;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .reconnect-text {
    color: #c6c6c6;
    font-family: "IBM Plex Mono", monospace;
    font-size: 0.8rem;
    font-weight: 500;
    letter-spacing: 0.05em;
  }

  .overlay.left {
    position: absolute;
    top: 16px;
    left: 16px;
    z-index: 20;
    background: rgba(22, 22, 22, 0.75);
    border-radius: 4px;
    padding: 16px;
    color: #f4f4f4;
    min-width: 180px;
    max-width: 260px;
    pointer-events: none;
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.2);
    backdrop-filter: blur(8px);
    border: 1px solid #4589ff;
  }

  .overlay.left :global(table) {
    width: 100%;
    border-collapse: collapse;
    background: transparent;
  }

  .overlay.left :global(th),
  .overlay.left :global(td) {
    background: transparent;
    color: #f4f4f4;
    border-bottom: 1px solid #393939;
    padding: 8px 16px 8px 0;
    font-family: "IBM Plex Mono", monospace;
    font-size: 0.875rem;
    line-height: 1.125rem;
  }

  .overlay.left :global(th) {
    font-weight: 600;
    border-bottom: 2px solid #4589ff;
    color: #78a9ff;
  }
</style>
