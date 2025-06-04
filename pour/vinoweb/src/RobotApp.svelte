<script lang="ts">
    import { onMount, onDestroy } from "svelte";
    import { useRobotClient } from "@viamrobotics/svelte-sdk";
    import { GenericServiceClient } from "@viamrobotics/sdk";
    import { ArmClient } from "@viamrobotics/sdk";
    import { Struct, type JsonValue } from "@bufbuild/protobuf";
    import MainContent from "./lib/MainContent.svelte";
    import Status from "./lib/status.svelte";
    import type { Joint } from "./lib/types.js";

    // --- Generate initial joints ---
    function* jointGenerator() {
        for (let index = 0; index < 6; index++) {
            yield { index, position: 0 } as Joint;
        }
    }
    const initialJoints = Array.from(jointGenerator()) as Joint[];

    // --- Define panes data ---
    let panesData = $state([
        {
            joints: Array.from(initialJoints) as Joint[],
            tableTitle: "Left Arm",
            camera: {
                name: "cam-left",
                partID: "xxx",
                label: "Left Camera",
            },
        },
        {
            joints: Array.from(initialJoints) as Joint[],
            tableTitle: "Right Arm",
            camera: {
                name: "cam-right",
                partID: "xxx",
                label: "Right Camera",
            },
        },
    ]);

    // --- Pouring status ---
    type StatusKey =
        | "standby"
        | "picking"
        | "prepping"
        | "pouring"
        | "placing"
        | "waiting"
        | "manual mode";
    let status: StatusKey = $state("standby");

    const statusMessages: Record<StatusKey, string> = {
        standby: "Please place your glass in the indicated area",
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

    // --- Robot client and polling logic ---
    const robotClientStore = useRobotClient(() => "xxx");
    let generic: GenericServiceClient | null = null;
    let pollingInterval: ReturnType<typeof setInterval> | null = null;

    // -- Robot Arms ---
    let leftArm: ArmClient | null = null;
    let rightArm: ArmClient | null = null;

    $effect(() => {
        const robotClient = robotClientStore.current;
        if (robotClient && !pollingInterval) {
            leftArm = new ArmClient(robotClient, "arm-left");
            rightArm = new ArmClient(robotClient, "arm-right");

            (async () => {
                try {
                    generic = new GenericServiceClient(robotClient, "cart");

                    pollingInterval = setInterval(async () => {
                        try {
                            // --- Status polling ---
                            const result = await generic!.doCommand(
                                Struct.fromJson({ status: true }),
                            );
                            let resultObj: any = {};
                            if (result instanceof Struct) {
                                resultObj = result.toJson();
                            } else if (
                                typeof result === "object" &&
                                result !== null
                            ) {
                                resultObj = result;
                            }
                            if (
                                resultObj &&
                                typeof resultObj.status === "string"
                            ) {
                                if (
                                    (
                                        Object.keys(
                                            statusMessages,
                                        ) as StatusKey[]
                                    ).includes(resultObj.status as StatusKey)
                                ) {
                                    status = resultObj.status as StatusKey;
                                }
                            }

                            // --- Joint polling ---
                            if (leftArm && rightArm) {
                                const leftJoints = await leftArm.getJointPositions();
                                const rightJoints = await rightArm.getJointPositions();
                                panesData[0].joints = leftJoints.values.map(
                                    (position, index) => ({ index, position }),
                                );
                                panesData[1].joints = rightJoints.values.map(
                                    (position, index) => ({ index, position }),
                                );
                                panesData = panesData; // triggers $state reactivity without remounting children
                            }
                        } catch (err) {
                            // Optionally handle error
                        }
                    }, 250);
                } catch (err) {
                    // Optionally handle error
                }
            })();
        }

        return () => {
            if (pollingInterval) {
                clearInterval(pollingInterval);
                pollingInterval = null;
            }
        };
    });
</script>

<div class="app-container">
    <aside class="sidebar"></aside>

    <MainContent panes={panesData}>
        {#snippet statusBar()}
            <Status message={statusMessages[status]} />
        {/snippet}
    </MainContent>
</div>

<style>
    .app-container {
        height: calc(100vh - 80px);
        width: 100%;
        max-width: calc(1920px - 80px);
        margin: 0 auto;
        display: grid;
        grid-template-columns: 700px 1fr;
        grid-template-rows: 1fr;
    }
    .sidebar {
        color: white;
        padding: 40px;
        overflow-y: auto;
    }
</style>
