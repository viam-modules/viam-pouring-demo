<script lang="ts">
  import { fade, scale } from "svelte/transition";
  import type { Snippet } from "svelte";
  import DataPane from "./DataPane.svelte";
  import type { Joint } from "./types.js";
  import JointTable from "./JointTable.svelte";
  import CameraFeed from "./CameraFeed.svelte";

  interface CameraConfig {
    name: string;
    partID: string;
    label: string;
  }

  interface PaneData {
    joints: Joint[];
    tableTitle?: string;
    camera: CameraConfig;
  }

  interface Props {
    statusBar: Snippet;
    panes: PaneData[];
    status: string; // Add this line
  }

  let { statusBar, panes, status }: Props = $props(); // Destructure status from props
</script>

<main class="main-content">
  <header class="status-bar">
    {@render statusBar()}
  </header>

  <section class="content-panes">
    <div class="expand-pane">
      <DataPane mode={status === "picking" ? "embedded" : "side-by-side"}>
        {#snippet table()}
          <JointTable joints={panes[0].joints} />
        {/snippet}
        {#snippet camera()}
          <CameraFeed
            name={panes[0].camera.name}
            partID={panes[0].camera.partID}
            label={panes[0].camera.label}
            overlay={status === "picking" ? table : undefined}
          />
        {/snippet}
      </DataPane>
    </div>
    {#if status !== "picking"}
      <div class="expand-pane">
        <DataPane mode="side-by-side">
          {#snippet table()}
            <JointTable joints={panes[1].joints} />
          {/snippet}
          {#snippet camera()}
            <CameraFeed
              name={panes[1].camera.name}
              partID={panes[1].camera.partID}
              label={panes[1].camera.label}
            />
          {/snippet}
        </DataPane>
      </div>
    {/if}
  </section>
</main>

<style>
  .main-content {
    background-color: white;
    display: grid;
    gap: 10px;
    grid-template-rows: auto 1fr;
    height: 90%;
    width: 95%;
    border-radius: 12px;
    overflow: hidden;
    padding: 20px;
    margin: auto 0; /* Add vertical margin auto */
  }

  .status-bar {
    display: flex;
    align-items: center;
  }

  .content-panes {
    display: grid;
    grid-template-rows: 1fr 1fr;
    gap: 0;
    background-color: #ddd;
    min-height: 0;
    height: 100%;
    padding: 15px;
    border-radius: 16px;
    overflow: hidden;
  }

  .expand-pane {
    height: 100%;
    min-height: 0;
    overflow: hidden;
  }

  .expand-pane:only-child {
    grid-row: 1 / span 2;
  }
</style>
