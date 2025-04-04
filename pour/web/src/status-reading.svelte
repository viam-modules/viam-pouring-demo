<script lang="ts">
  import { Struct, type GenericService } from "@viamrobotics/sdk";
  import { onDestroy, onMount } from "svelte";

  export let client: GenericService;

  let status: string | undefined;
  let interval: number | undefined = undefined;
  type Status = {
    status: string;
  };

  const displayStates: Record<string, string[]> = {
    THINKING: [
      "found the positions of the cups, will do planning now",
      "done with prep planning",
      "planned cup",
    ],
    POURING: ["DONE CONSTRUCTING PLANS -- EXECUTING NOW", "success"],
    "CHEERS!": ["done running the demo"],
    "UH OH!": ["error"],
  };

  let displayStatus: string | undefined = undefined;

  const updateStatus = async () => {
    try {
      const res = await client.doCommand(Struct.fromJson({ status: "get" }));
      const statusRet = res as Status;
      status = statusRet.status;
    } catch (error) {
      console.error(error);
    }
  };
  onMount(() => {
    updateStatus();
    interval = setInterval(updateStatus, 1000);
  });
  onDestroy(() => {
    if (interval) {
      clearInterval(interval);
    }
  });

  $: displayStatus = (() => {
    for (const key in displayStates) {
      if (displayStates.hasOwnProperty(key)) {
        const values = displayStates[key];
        if (values.some((val) => status?.includes(val))) {
          return key;
        }
      }
    }
    // Return "THINKING" if we have a status but it doesn't match any in the map
    return "THINKING";
  })();
</script>

{displayStatus}
