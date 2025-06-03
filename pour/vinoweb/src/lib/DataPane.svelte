<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    table: Snippet;
    camera: Snippet;
  }

  let { table, camera }: Props = $props();
</script>

<div class="pane">
  <div class="data-table-container">
    {@render table()}
  </div>
  <div class="camera-feed-container">
    {@render camera()}
  </div>
</div>

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
</style>
