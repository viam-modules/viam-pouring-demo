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

</script>

<ViamProvider {dialConfigs}>
  <RobotApp />
</ViamProvider>
