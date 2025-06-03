<script lang="ts">
    import JointTable from "./lib/JointTable.svelte";
    import CameraFeed from "./lib/CameraFeed.svelte";
    import { ViamProvider } from "@viamrobotics/svelte-sdk";
    import DataPane from "./lib/DataPane.svelte";

    import type { Joint } from "./lib/types.js";
    import MainContent from "./lib/MainContent.svelte";

    let { host, credentials, children } = $props();

    const dialConfigs: Record<string, DialConf> = {
        xxx: {
            host: host,
            credentials: credentials,
            signalingAddress: "https://app.viam.com:443",
            disableSessions: false,
        },
    };

    const sampleJoints: Joint[] = [
        { index: 0, position: -64.83 },
        { index: 1, position: -108.82 },
        { index: 2, position: -32.63 },
        { index: 3, position: 348.78 },
        { index: 4, position: 110.8 },
        { index: 5, position: -184.56 },
    ];

    const mockCamera = {
        name: "cam-left",
        partID: "xxx",
        label: "Camera 1",
    };

    const panesData = [
        {
            joints: sampleJoints,
            tableTitle: "Robot Arm 1",
            camera: {
                name: "cam-left",
                partID: "xxx",
                label: "Camera 1",
            },
        },
        {
            joints: sampleJoints,
            tableTitle: "Robot Arm 2",
            camera: {
                name: "cam-right",
                partID: "xxx",
                label: "Camera 2",
            },
        },
    ];
</script>

<div class="preview-container">
    <h1>Component Preview</h1>
    <ViamProvider {dialConfigs}>
        <section>
            <h2>MainContent</h2>
            <div style="height: 600px;">
                <MainContent panes={panesData}>
                    {#snippet statusBar()}
                        <h2>Status Banner</h2>
                    {/snippet}
                </MainContent>
            </div>
        </section>

        <section>
            <h2>DataPane</h2>
            <div style="height: 400px;">
                <DataPane>
                    {#snippet table()}
                        <JointTable joints={sampleJoints} title="Robot Arm" />
                    {/snippet}

                    {#snippet camera()}
                        <CameraFeed
                            name={mockCamera.name}
                            partID={mockCamera.partID}
                            label={mockCamera.label}
                        />
                    {/snippet}
                </DataPane>
            </div>
        </section>
    </ViamProvider>
</div>

<style>
    .preview-container {
        padding: 20px;
        max-width: 800px;
        margin: 0 auto;
    }

    section {
        margin-bottom: 40px;
        padding: 20px;
        border: 1px solid #eee;
        border-radius: 8px;
    }

    h1,
    h2 {
        margin-bottom: 16px;
    }
</style>
