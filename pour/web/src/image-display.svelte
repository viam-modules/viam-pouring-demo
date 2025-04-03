<script lang="ts">
  import type { VisionClient } from "@viamrobotics/sdk";
  import { onMount, onDestroy } from "svelte";

  export let client: VisionClient;
  let imageUrl: string | undefined = undefined;
  let interval: number;

  async function updateImage() {
    try {
      const res = await client.captureAllFromCamera(
        "wine-pouring-camera-main:realsense",
        {
          returnImage: true,
          returnClassifications: false,
          returnDetections: false,
          returnObjectPointClouds: false,
        }
      );

      if (res.image?.image) {
        const base64String = btoa(
          String.fromCharCode.apply(null, Array.from(res.image.image))
        );
        imageUrl = `data:image/jpeg;base64,${base64String}`;
      }
    } catch (error) {
      console.error("Failed to capture image:", error);
    }
  }

  onMount(() => {
    updateImage();
    interval = setInterval(updateImage, 1000);
  });

  onDestroy(() => {
    if (interval) clearInterval(interval);
  });
</script>

<img class="w-full h-full rounded-md" alt="vision" src={imageUrl} />
<span
  class="text-sm absolute top-3 right-4 text-[#00fb7a] shadow-sm whitespace-nowrap"
  >LIVE FEED</span
>
