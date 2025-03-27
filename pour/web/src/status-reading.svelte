<script lang="ts">
  import { Struct, type GenericService } from "@viamrobotics/sdk";
  import { onDestroy, onMount } from "svelte";

  export let client: GenericService;
  let status: string | undefined = undefined;
  let interval: number | undefined = undefined;
  type Status = {
    status: string;
  };
  const updateStatus = async () => {
    try {
      const res = await client.doCommand(Struct.fromJson({status: "get"}));
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
</script>

<p class="text-xl font-mono">Status: <br/>{status}</p>