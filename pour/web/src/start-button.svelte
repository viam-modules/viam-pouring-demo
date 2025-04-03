<script lang="ts">
  import * as VIAM from "@viamrobotics/sdk";

  import { Struct, type GenericService } from "@viamrobotics/sdk";
  import StatusReading from "./status-reading.svelte";
  export let client: GenericService;

  let isRunning = false;
  let buttonClicked: "far" | "mid" | undefined = undefined;

  async function onClick(params) {
    if (!params) {
      params = {};
    }
    params["do-pour"] = true;

    console.log("pout do command", params);

    try {
      isRunning = true;
      buttonClicked = params.far ? "far" : "mid";
      var x = VIAM.Struct.fromJson(params);
      const res = await client.doCommand(x);
      console.log(res);
    } catch (error) {
      console.error(error);
    } finally {
      isRunning = false;
    }
  }

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

  let status: string | undefined = undefined;
</script>

<div class="text-md">
  <div class="mb-4 flex gap-4">
    <div class="grow flex gap-4">
      <!-- <button
        class="bg-gray-9 border border-gray-9 px-4 py-2 text-white"
        on:click={() => {
          onClick({});
        }}
      >
        Start Pouring from scale
      </button> -->
      <button
        class="bg-[#fffef7] border border-[#ffd800] transition-all hover:rounded-3xl px-16 py-3"
        on:click={() => {
          onClick({ far: true });
        }}
      >
        {#if !isRunning}
          WHITE WINE
        {:else if buttonClicked === "far"}
          <StatusReading {client} />
        {/if}
      </button>
      <button
        class="bg-[#fcf2f6] border border-[#b90045] transition-all hover:rounded-3xl px-16 py-3"
        on:click={() => {
          onClick({ mid: true });
        }}
      >
        {#if !isRunning}
          RED WINE
        {:else if buttonClicked === "mid"}
          <StatusReading {client} />
        {/if}
      </button>
    </div>

    <!-- {#if isRunning}
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
    {/if} -->
  </div>
</div>
