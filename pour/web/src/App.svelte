<script lang="ts">
 import * as VIAM from "@viamrobotics/sdk";
 import StartButton from "./start-button.svelte";
 import ImageDisplay from "./image-display.svelte";
 import WeightSensor from "./weight-sensor.svelte";
 
 let pouringClient: VIAM.GenericService | undefined = undefined;
 let houghClient: VIAM.VisionClient | undefined = undefined;
 let weightClient: VIAM.SensorClient | undefined = undefined;
 
 

 async function getConfig() {
   if (import.meta.env.VITE_HOST) {
     return {
       host: import.meta.env.VITE_HOST,
       payload: import.meta.env.VITE_PAYLOAD,
       authEntity: import.meta.env.VITE_KEY_ID
     };
   }

   const urlParams = new URLSearchParams(window.location.search);
   
   const host = urlParams.get("host");
   const payload = urlParams.get("payload");
   const authEntity = urlParams.get("authEntity");

   if (host) {
     return {
       host: host,
       payload: payload,
       authEntity: authEntity
     }
   }
   
   throw new Error("no connection config");
 }

 const main = async () => {

   var cfg = await getConfig();
   
   const machine = await VIAM.createRobotClient({
     host: cfg.host,
      credentials: {
        type: "api-key",
        payload: cfg.payload,
        authEntity: cfg.authEntity
      },
      signalingAddress: "https://app.viam.com:443",
    });


    pouringClient = new VIAM.GenericServiceClient(machine, "pouring-service");
    houghClient = new VIAM.VisionClient(machine, "circle-service");
    weightClient = new VIAM.SensorClient(machine, "sensor-1");

  };

  main().catch((error: unknown) => {
    console.error("encountered an error:", error);
  });
</script>

<section class="p-4 flex flex-col gap-4 w-full">
  <h1 class="text-2xl font-bold ">Wine Pouring Demo</h1>
  <p class="text-xl text-center">Arrange your cups as-desired on the table.<br/>
    Ensure that they are detected in the camera view. Then press start.</p>
  {#if pouringClient}
    <StartButton client={pouringClient} />
  {:else}
    <div>pouring service connecting...</div>
  {/if}
  <div class="flex grow w-full gap-4 ">
    <div class="bg-gradient-2 w-full rounded">
      {#if houghClient}
        <ImageDisplay client={houghClient} />
      {:else}
        <div>vision service connecting...</div>
      {/if}
    </div>
    <div class="bg-gradient-2 w-full rounded p-3 flex flex-col">
      {#if weightClient}
        <WeightSensor client={weightClient} />
        <p class="text-sm mt-auto">The weight is used to calculate how much wine is left in the bottle. This is important for controlling the pouring angle and duration.</p>
      {:else}
        <div>weight sensor connecting...</div>
      {/if}
    </div>
  </div>
</section>

<style>
  .bg-gradient-2 {
    background: linear-gradient(120deg, #3a4145 0%, #4a5152 100%);
  }
</style>
