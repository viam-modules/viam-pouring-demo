<script lang="ts">
  import { CameraStream } from "@viamrobotics/svelte-sdk";
  import type { Snippet } from "svelte";

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
  <div class="camera-label right">{label}</div>
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
    right: 16px;
  }

  .overlay.left {
    position: absolute;
    top: 16px;
    left: 16px;
    z-index: 20;
    /* Cool transparent glassmorphism effect */
    background: rgba(24, 28, 31, 0.45);
    border-radius: 12px;
    padding: 12px 18px;
    color: #fff;
    min-width: 180px;
    max-width: 260px;
    pointer-events: none;
    box-shadow:
      0 2px 16px 0 rgba(0,0,0,0.18),
      0 0 0 1.5px #39FF14;
    backdrop-filter: blur(8px) saturate(1.2);
    border: 1.5px solid #39FF14;
    /* Optional: subtle gradient overlay */
    background-image: linear-gradient(120deg, rgba(57,255,20,0.08) 0%, rgba(24,28,31,0.45) 100%);
  }

  /* Optional: style tables inside overlay for extra clarity */
  .overlay.left :global(table) {
    width: 100%;
    border-collapse: collapse;
    background: transparent;
  }
  .overlay.left :global(th),
  .overlay.left :global(td) {
    background: transparent;
    color: #39FF14;
    border-bottom: 1px solid rgba(57,255,20,0.18);
    padding: 4px 8px;
    font-family: "Share Tech Mono", "Fira Mono", "Consolas", monospace;
    font-size: 1rem;
    text-shadow: 0 0 2px #39FF14, 0 0 6px #39FF14;
  }
  .overlay.left :global(th) {
    font-weight: 700;
    border-bottom: 2px solid #39FF14;
  }
</style>
