<script lang="ts">
    import { onMount, onDestroy } from "svelte";
    import { useRobotClient } from "@viamrobotics/svelte-sdk";
    import { GenericServiceClient } from "@viamrobotics/sdk";
    import { ArmClient } from "@viamrobotics/sdk";
    import { Struct, type JsonValue } from "@bufbuild/protobuf";
    import MainContent from "./lib/MainContent.svelte";
    import Status from "./lib/status.svelte";
    import type { Joint } from "./lib/types.js";

    // --- Pouring status ---
    type StatusKey =
        | "standby"
        | "looking"
        | "picking"
        | "prepping"
        | "pouring"
        | "placing"
        | "waiting"
        | "manual mode";
    let status: StatusKey = $state("standby") as StatusKey;

    const statusMessages: Record<StatusKey, string> = {
        standby: "Ready to pour!",
        looking: "Please place your glass in the indicated area",
        picking: "Thank you, just a moment",
        prepping: "Preparing to pour...",
        pouring: "Pouring...",
        placing: "Placing glass down",
        waiting: "Please enjoy!",
        "manual mode": "Manual mode active",
    };

    // --- Keyboard controls for debugging ---
    function handleKeydown(event: KeyboardEvent) {
        const keys = Object.keys(statusMessages) as StatusKey[];
        const keyNum = parseInt(event.key);
        if (keyNum >= 1 && keyNum <= keys.length) {
            status = keys[keyNum - 1];
        }
    }

    onMount(() => {
        window.addEventListener("keydown", handleKeydown);
        return () => window.removeEventListener("keydown", handleKeydown);
    });

    // --- Generate initial joints ---
    function* jointGenerator() {
        for (let index = 0; index < 6; index++) {
            yield { index, position: 0 } as Joint;
        }
    }
    const initialJoints = Array.from(jointGenerator()) as Joint[];

    // --- $state-ful joint arrays ---
    let leftJoints = $state([...initialJoints]);
    let rightJoints = $state([...initialJoints]);

    // --- Define panes data ---
    let panesData = $state([
        {
            joints: leftJoints,
            tableTitle: "Left Arm",
            camera: {
                name: "cam-left",
                partID: "xxx",
                label: "Left Camera",
            },
        },
        {
            joints: rightJoints,
            tableTitle: "Right Arm",
            camera: {
                name: "cam-right",
                partID: "xxx",
                label: "Right Camera",
            },
        },
    ]);

    // --- Robot client and polling logic ---
    const robotClientStore = useRobotClient(() => "xxx");
    let generic: GenericServiceClient | null = null;
    let pollingHandle: ReturnType<typeof setInterval> | null = null;
    let pollingInterval = 250; // Polling interval in milliseconds

    // -- Robot Arms ---
    let leftArm: ArmClient | null = null;
    let rightArm: ArmClient | null = null;

    $effect(() => {
        const robotClient = robotClientStore.current;
        $inspect(robotClient, "robotClient");
        if (robotClient && !pollingHandle) {
            if (!leftArm) leftArm = new ArmClient(robotClient, "arm-left");
            if (!rightArm) rightArm = new ArmClient(robotClient, "arm-right");
            if (!generic) generic = new GenericServiceClient(robotClient, "cart");

            pollingHandle = setInterval(async () => {
                // --- Status ---
                try {
                    const result = await generic!.doCommand(
                        Struct.fromJson({ status: true }),
                    );
                    if (
                        result &&
                        typeof result === "object" &&
                        "status" in result &&
                        typeof (result as any).status === "string"
                    ) {
                        const statusStr = (result as any).status;
                        if (
                            (
                                Object.keys(statusMessages) as StatusKey[]
                            ).includes(statusStr as StatusKey)
                        ) {
                            status = statusStr as StatusKey;
                        }
                    }
                } catch (err) {
                    // Optionally handle status polling error
                }

                // --- Joint positions ---
                if (leftArm && rightArm) {
                    try {
                        const leftJoints = await leftArm.getJointPositions();
                        panesData[0].joints = leftJoints.values.map(
                            (position, index) => ({ index, position }),
                        );
                    } catch (err) {
                        // Optionally handle left arm error
                    }
                    try {
                        const rightJoints = await rightArm.getJointPositions();
                        panesData[1].joints = rightJoints.values.map(
                            (position, index) => ({ index, position }),
                        );
                    } catch (err) {
                        // Optionally handle right arm error
                    }
                    panesData = panesData; // triggers $state reactivity without remounting children
                }
            }, pollingInterval);
        }

        return () => {
            if (pollingHandle) {
                clearInterval(pollingHandle);
                pollingHandle = null;
            }
        };
    });
</script>

<div class="app-container">
    <aside class="sidebar"></aside>

    <MainContent
        panes={panesData}
        status={status}
    >
        {#snippet statusBar()}
            <Status message={statusMessages[status]} />
        {/snippet}
    </MainContent>
</div>

<style>
    .app-container {
        height: calc(100vh - 80px);
        width: 100%;
        max-width: 1920px;
        margin: 0 auto;
        display: grid;
        grid-template-columns: 700px 1fr;
        grid-template-rows: 1fr;
        overflow: hidden;
    }
    .sidebar {
        color: white;
        padding: 40px;
        overflow-y: auto;
    }
</style>
