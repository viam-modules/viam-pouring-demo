<script lang="ts">
  import { onMount } from "svelte";
  import { ViamProvider, useRobotClient } from "@viamrobotics/svelte-sdk";
  import type { DialConf } from "@viamrobotics/sdk";
  import { GenericComponentClient } from "@viamrobotics/sdk";
  import MainContent from "./lib/MainContent.svelte";
  import type { Joint } from "./lib/types.js";
  import Status from "./lib/status.svelte";
  import RobotApp from "./RobotApp.svelte";

  let { host, credentials } = $props();

  const dialConfigs: Record<string, DialConf> = {
    xxx: {
      host: host,
      credentials: credentials,
      signalingAddress: "https://app.viam.com:443",
      disableSessions: false,
    },
  };

  // Get the robot client for partID "xxx"
  const robotClientStore = useRobotClient(() => "xxx");
  // let generic: GenericComponentClient | null = null;

  $effect(() => {
    const robotClient = robotClientStore.current;
    if (robotClient) {
      // generic = new GenericComponentClient(robotClient, "cart");
    }
  });

  const panesData = [
    {
      joints: [
        { index: 0, position: -64.83 },
        { index: 1, position: -108.82 },
        { index: 2, position: -32.63 },
        { index: 3, position: 348.78 },
        { index: 4, position: 110.8 },
        { index: 5, position: -184.56 },
      ] as Joint[],
      tableTitle: "Left Arm",
      camera: {
        name: "cam-left",
        partID: "xxx",
        label: "Left Camera",
      },
    },
    {
      joints: [
        { index: 0, position: -64.83 },
        { index: 1, position: -108.82 },
        { index: 2, position: -32.63 },
        { index: 3, position: 348.78 },
        { index: 4, position: 110.8 },
        { index: 5, position: -184.56 },
      ] as Joint[],
      tableTitle: "Right Arm",
      camera: {
        name: "cam-right",
        partID: "xxx",
        label: "Right Camera",
      },
    },
  ];

  // --- Status state and mapping ---
  type StatusKey =
    | "standby"
    | "picking"
    | "prepping"
    | "pouring"
    | "placing"
    | "waiting";
  let status: StatusKey = $state("standby");

  const statusMessages: Record<StatusKey, string> = {
    standby: "Please place your glass in the indicated area",
    picking: "Thank you, just a moment",
    prepping: "Preparing to pour...",
    pouring: "Pouring...",
    placing: "Placing glass down",
    waiting: "Please enjoy!",
  };

  // --- DEBUG: Keydown handler to change status ---
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
</script>

<ViamProvider {dialConfigs}>
  <RobotApp />
</ViamProvider>
