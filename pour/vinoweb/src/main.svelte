<script lang="ts">
  import { onMount } from "svelte";
  import { ViamProvider } from "@viamrobotics/svelte-sdk";
  import type { DialConf } from "@viamrobotics/sdk";
  import MainContent from "./lib/MainContent.svelte";
  import type { Joint } from "./lib/types.js";
  import Status from "./lib/Status.svelte";

  let { host, credentials } = $props();

  const dialConfigs: Record<string, DialConf> = {
    xxx: {
      host: host,
      credentials: credentials,
      signalingAddress: "https://app.viam.com:443",
      disableSessions: false,
    },
  };

  // Your actual robot data
  const panesData = [
    {
      joints: [
        { index: 0, position: -64.83 },
        { index: 1, position: -108.82 },
        { index: 2, position: -32.63 },
        { index: 3, position: 348.78 },
        { index: 4, position: 110.80 },
        { index: 5, position: -184.56 }
      ] as Joint[],
      tableTitle: "Left Arm",
      camera: {
        name: "cam-left",
        partID: "xxx",
        label: "Left Camera"
      }
    },
    {
      joints: [
        { index: 0, position: -64.83 },
        { index: 1, position: -108.82 },
        { index: 2, position: -32.63 },
        { index: 3, position: 348.78 },
        { index: 4, position: 110.80 },
        { index: 5, position: -184.56 }
      ] as Joint[],
      tableTitle: "Right Arm", 
      camera: {
        name: "cam-right",
        partID: "xxx",
        label: "Right Camera"
      }
    }
  ];




  
  // --- Status state and mapping ---
  type StatusKey = "standby" | "picking" | "prepping" | "pouring" | "placing" | "waiting";
  let status: StatusKey = $state("standby");

  const statusMessages: Record<StatusKey, string> = {
    standby: "Please place your glass in the indicated area",
    picking: "Thank you, just a moment",
    prepping: "Preparing to pour...",
    pouring: "Pouring...",
    placing: "Placing glass down",
    waiting: "Please enjoy!",
  };

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
  <div class="app-container">
    <aside class="sidebar">
    </aside>

    <MainContent panes={panesData}>
      {#snippet statusBar()}
        <Status message={statusMessages[status]} />
      {/snippet}
    </MainContent>
  </div>
</ViamProvider>

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
