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
</script>

<div class="camera-feed">
  <CameraStream {name} {partID} />
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
