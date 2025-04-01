<script lang="ts">
  import * as VIAM from "@viamrobotics/sdk";

  import { Struct, type GenericService } from "@viamrobotics/sdk";
  import StatusReading from "./status-reading.svelte";
  export let client: GenericService;

  let isRunning = false;

  async function onClick(params) {
    if (!params) {
      params = {};
    }
    params["do-pour"] = true;

    console.log("pout do command", params);

    try {
      isRunning = true;
      var x = VIAM.Struct.fromJson(params);
      const res = await client.doCommand(x);
      console.log(res);
    } catch (error) {
      console.error(error);
    } finally {
      isRunning = false;
    }
  }
</script>

<div class="text-md">
  <StatusReading {client} />

  <div class="flex gap-4">
    <div class="grow">
      <button
        class="bg-gray-9 border border-gray-9 px-4 py-2 text-white"
        on:click={() => {
          onClick({});
        }}
      >
        Start Pouring from scale
      </button>
      <button
        class="bg-gray-9 border border-gray-9 px-4 py-2 text-white"
        on:click={() => {
          onClick({ far: true });
        }}
      >
        Start Pouring from far bottle
      </button>
      <button
        class="bg-gray-9 border border-gray-9 px-4 py-2 text-white"
        on:click={() => {
          onClick({ mid: true });
        }}
      >
        Start Pouring from middle bottle
      </button>
    </div>

    {#if isRunning}
      <p
        class="flex border rounded-2xl px-4 py-1 text-md border-success-medium bg-success-light text-success-dark"
      >
        Running
      </p>
    {:else}
      <p
        class="absolute right-4 top-4 self-center border rounded-lg px-4 py-1 text-md border-info-medium bg-info-light text-info-dark"
      >
        Stopped
      </p>
    {/if}
  </div>
</div>
