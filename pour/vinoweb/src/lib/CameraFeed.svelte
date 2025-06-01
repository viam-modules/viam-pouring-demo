<script lang="ts">
import { CameraStream } from '@viamrobotics/svelte-sdk';

type SizeMode = 'fitHeight' | 'fitWidth' | 'fill';

interface Props {
  partID: string;
  name: string;
  displayName?: string;
    sizeMode?: SizeMode;
}

let { partID, name, displayName = name, sizeMode }: Props = $props();

let containerStyle = $derived.by(() => {
  let styles = [];
  if (sizeMode === 'fitHeight') {
    styles.push('height: 100%;');
  } else if (sizeMode === 'fitWidth') {
    styles.push('width: 100%; height: auto;');
  } else if (sizeMode === 'fill') {
    styles.push('width: 100%; height: 100%;');
  } else {
    styles.push('width: 100%; height: 100%;'); // Default to fill
  }
  return styles.join(' ');
});

</script>

<div class="camera-feed">
  <div class="camera-overlay">
    <span class="camera-label">{displayName}</span>
  </div>

  <div class="video-container" style={containerStyle}>
    <CameraStream 
      {partID} 
      {name}
      class="camera-video"
    />
  </div>
</div>

<style>
  .camera-feed {
    position: relative;
    width: 100%;
    height: 100%;
    background-color: #000;
    border-radius: 4px;
    overflow: hidden;
  }

  :global(.camera-video) {
    width: auto;
    height: auto;
    object-fit: contain;
  }

    .video-container {
        height: 432px;
        aspect-ratio: 16 / 9; /* Maintain a 16:9 aspect ratio */
    }

  .camera-overlay {
    position: absolute;
    top: 8px;
    left: 8px;
    z-index: 10;
    background-color: rgba(0, 0, 0, 0.7);
    color: white;
    padding: 4px 8px;
    border-radius: 4px;
    font-size: 0.875rem;
    font-weight: 500;
  }

  .camera-label {
    text-shadow: 1px 1px 2px rgba(0, 0, 0, 0.8);
  }
</style>