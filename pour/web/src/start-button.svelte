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
 };
</script>

<div class="">
  <div class="">
    <div class=""></div>
    <StatusReading {client} />
  </div>
  <button on:click={() => {onClick({})}}>
    Start Pouring from scale
  </button>
  <button on:click={() => {onClick({"far" : true})}}>
    Start Pouring from far bottle
  </button>
  <button on:click={() => {onClick({"mid" : true})}}>
    Start Pouring from middle bottle
  </button>

  <span>Is Running: {isRunning}</span>
</div>


