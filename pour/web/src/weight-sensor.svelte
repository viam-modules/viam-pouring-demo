<script lang="ts">
  import { SensorClient } from "@viamrobotics/sdk";
  import { onDestroy, onMount } from "svelte";
  console.log("weight sensor");
  export let client: SensorClient;
  let weight: number | undefined = undefined;
  let interval: number | undefined = undefined;
  type Reading = {
    mass_kg: number;
  };
  const updateWeight = async () => {
    try {
      const res = await client.getReadings({});
      const reading = res as Reading;
      weight = reading?.mass_kg;
    } catch (error) {
      console.error(error);
    }
  };
  onMount(() => {
    updateWeight();
    interval = setInterval(updateWeight, 1000);
  });
  onDestroy(() => {
    if (interval) {
      clearInterval(interval);
    }
  });
</script>

<p class="text-xl font-mono">Detected Bottle Weight: <br/>{weight?.toPrecision(3)} kg</p>
