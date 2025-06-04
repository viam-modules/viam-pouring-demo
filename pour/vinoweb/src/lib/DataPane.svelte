<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    table: Snippet;
    camera: Snippet;
    mode?: "side-by-side" | "embedded";
  }

  let { table, camera, mode = "side-by-side" }: Props = $props();
</script>

{#if mode === "side-by-side"}
  <div class="pane">
    <div class="data-table-container">
      {@render table()}
    </div>
    <div class="camera-feed-container">
      {@render camera()}
    </div>
  </div>
{:else if mode === "embedded"}
  <div class="pane embedded">
    <div class="camera-feed-container embedded">
      {@render camera()}
    </div>
  </div>
{/if}

<style>
  .pane {
    background-color: #ddd;
    padding: 20px;
    overflow: hidden;
    display: flex;
    gap: 20px;
    align-items: center;
    min-height: 0;
    height: 100%;
  }

  .pane.embedded {
    flex-direction: column;
    padding: 0;
    gap: 0;
    background: transparent;
  }

  .data-table-container {
    flex-shrink: 0;
    width: 220px;
    display: flex;
    align-items: flex-start;
  }

  .camera-feed-container {
    flex: 1;
    display: flex;
    justify-content: center;
    align-items: center;
    height: 100%;
    min-height: 0;
    max-height: 100%;
    overflow: hidden;
  }

  .camera-feed-container.embedded {
    width: 100%;
    height: 100%;
    padding: 0;
  }
</style>
