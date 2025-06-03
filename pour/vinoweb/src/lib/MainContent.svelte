<script lang="ts">
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
        {#each panes as pane}
            <DataPane>
                {#snippet table()}
                    <JointTable joints={pane.joints} title={pane.tableTitle} />
                {/snippet}

                {#snippet camera()}
                    <CameraFeed
                        name={pane.camera.name}
                        partID={pane.camera.partID}
                        label={pane.camera.label}
                    />
                {/snippet}
            </DataPane>
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
</style>
