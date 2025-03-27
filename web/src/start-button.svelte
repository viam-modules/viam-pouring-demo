<script lang="ts">
  import { Struct, type GenericService } from "@viamrobotics/sdk";
  import StatusReading from "./status-reading.svelte";
  export let client: GenericService;
  let isRunning = false;
  const onClick = async () => {
    try {
      isRunning = true;
      const res = await client.doCommand(new Struct({}));
      console.log(res);
    } catch (error) {
      console.error(error);
    } finally {
      isRunning = false;
    }
  };
</script>

<div class="flex flex-col gap-4 bg-gradient-2 p-4 rounded h-[100px]">
{#if isRunning}
    <div class="relative w-full">
        <div class="absolute top-2 right-2 w-8 h-8 bg-red-600 rounded-full animate-pulse"></div>
        <StatusReading {client} />
    </div>
{:else}
<button 
    on:click={onClick}
    class="bg-[#547aa5] text-white h-full p-3 text-3xl rounded w-full transition-colors" 
>
    Start Pouring
</button>
{/if}
</div>

<style>
  .bg-gradient-2 {
    background: linear-gradient(135deg, #3a4142 100%, #4a5152 100%);
  }
</style>