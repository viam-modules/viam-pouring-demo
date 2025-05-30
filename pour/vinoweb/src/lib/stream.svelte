<script lang="ts">
 import { createStreamClient } from '@viamrobotics/svelte-sdk';

 interface Props {
   partID: string;
   name: string;
 }

 let { partID, name }: Props = $props();

 let element: HTMLVideoElement;

 const client = createStreamClient(
   () => partID,
   () => name
 );

 $effect(() => {
   element.srcObject = client.mediaStream;
 });

 $effect(() => {
   const [firstResolution] = client.resolutions ?? [];

   console.log(firstResolution);

   if (firstResolution) {
     client.setResolution(firstResolution);
   }
 });
</script>

<video
  muted
  autoplay
  controls={false}
  playsinline
  bind:this={element}
></video>
