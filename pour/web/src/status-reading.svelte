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
</script>

<div
  class="inline-flex mb-4 rounded border py-1.5 pl-2.5 pr-2 text-md hover:bg-gray-8 hover:text-gray-1
  {status
    ? 'border-success-medium bg-success-light text-success-dark'
    : 'border-warning-medium bg-warning-light text-warning-dark'}"
>
  Status: {status ? status : "unknown"}
</div>
