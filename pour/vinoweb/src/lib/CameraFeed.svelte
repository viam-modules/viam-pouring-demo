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
    top: 12px;
    left: 12px;
    background: rgba(0, 0, 0, 0.7);
    color: white;
    padding: 6px 12px;
    border-radius: 6px;
    font-size: 0.9rem;
    font-weight: 500;
    backdrop-filter: blur(4px);
    z-index: 10;
  }
</style>
