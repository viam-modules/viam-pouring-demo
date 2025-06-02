<script lang="ts">
    import { CameraStream } from '@viamrobotics/svelte-sdk';

	let { name, partID, label } = $props();
</script>

<div class="camera-feed-container">
    <span class="camera-label">{label}</span>
    <CameraStream {partID} {name} />
</div>

<style>
    .camera-feed-container {
        position: relative;
        display: inline-block;
        height: 300px; /* Set desired height for aspect ratio control */
    }
    .camera-label {
        position: absolute;
        top: 8px;
        left: 8px;
        background: rgba(0,0,0,0.6);
        color: #fff;
        padding: 4px 12px;
        border-radius: 4px;
        font-size: 1rem;
        z-index: 2;
        pointer-events: none;
    }
    :global(.camera-feed-container video) {
        height: 100%;
        width: auto;
        object-fit: contain;
        display: block;
        max-width: 100%;
        border-radius: 8px;
    }
</style>