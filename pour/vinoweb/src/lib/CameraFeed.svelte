<script lang="ts">
  import { CameraStream } from "@viamrobotics/svelte-sdk";

  interface Props {
    name: string;
    partID: string;
    label: string;
  }

  let { name, partID, label }: Props = $props();
</script>

<div class="camera-feed">
  <CameraStream {name} {partID} />
  <div class="camera-label">{label}</div>
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
  }

  .camera-feed :global(video) {
    max-height: 100%;
    max-width: 100%;
    width: auto;
    height: auto;
    object-fit: contain;
  }

  .camera-label {
    position: absolute;
    top: 16px;
    left: 16px;
    background: rgba(24, 28, 31, 0.92);
    color: #39FF14;
    padding: 10px 20px;
    border-radius: 8px;
    font-size: 1.3rem;
    font-family: "Share Tech Mono", "Fira Mono", "Consolas", monospace;
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    border: 2px solid #39FF14;
    box-shadow:
      0 0 2px #39FF14,
      0 0 8px #39FF1455;
    text-shadow:
      0 0 2px #39FF14,
      0 0 6px #39FF14;
    z-index: 10;
    user-select: none;
    pointer-events: none;
  }
</style>
