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
    }

    let { statusBar, panes }: Props = $props();
</script>

<main class="main-content">
    <header class="status-bar">
        {@render statusBar()}
    </header>

    <section class="content-panes">
        {#each panes as pane, i}
            <div
                class="expand-pane"
                transition:scale={{ duration: 350 }}
            >
                <DataPane
                    mode={panes.length === 1 ? "embedded" : "side-by-side"}
                >
                    {#snippet table()}
                        <JointTable joints={pane.joints} />
                    {/snippet}
                    {#snippet camera()}
                        <CameraFeed
                            name={pane.camera.name}
                            partID={pane.camera.partID}
                            label={pane.camera.label}
                            overlay={panes.length === 1 ? table : undefined}
                        />
                    {/snippet}
                </DataPane>
            </div>
        {/each}
    </section>
</main>

<style>
  .main-content {
    background-color: white;
    display: grid;
    gap: 10px;
    grid-template-rows: 170px 1fr;
    height: 100%;
    border-radius: 12px;
    overflow: hidden;
    padding: 20px;
  }

  .status-bar {
    display: flex;
    align-items: center;
  }

  .content-panes {
    display: grid;
    grid-template-rows: 1fr 1fr;
    gap: 10px;
    background-color: #ddd;
    min-height: 0;
    padding: 5px;
    border-radius: 16px;
  }

  .expand-pane:only-child {
    grid-row: 1 / span 2;
  }
</style>
