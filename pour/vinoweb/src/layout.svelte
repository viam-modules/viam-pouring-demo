<script lang="ts">

 import { ViamProvider } from '@viamrobotics/svelte-sdk';
 import { CameraStream } from '@viamrobotics/svelte-sdk';

 import type { DialConf } from '@viamrobotics/sdk';
 import CameraFeed from './lib/CameraFeed.svelte';
 import CameraFeedA from './lib/CameraFeedA.svelte';
 import Status from './lib/status.svelte';

 let { host, credentials, children } = $props();

 const dialConfigs: Record<string, DialConf> = {
   "xxx": {
     host: host,
     credentials: credentials,
     signalingAddress: 'https://app.viam.com:443',
     disableSessions: false,
   },
 };
 
</script>

<ViamProvider {dialConfigs}>
<div class="layout">
  <div class="sidebar">
    <h3>Development Area</h3>
    <p>640px wide (1/3 of 1920px)</p>
    {@render children?.()}
  </div>

  <div class="main">
    <div class="status-area">
      <Status name="xxx" display="connection to {host}"/>
      <p>216px tall (20% of 1080px)</p>
    </div>

    <div class="camera-top">
      <CameraFeed
        name="cam-left"
        partID="xxx"
        displayName="Left Camera"
        sizeMode="fitHeight"
      />
    </div>

    <div class="camera-bottom">
      <CameraStream name="cam-right" partID="xxx" class="camera-video"/>
    </div>
  </div>
</div>
</ViamProvider>


<style>
  .layout {
    display: grid;
    grid-template-columns: 640px 1280px; /* Exact pixel widths */
    height: 1080px; /* Exact screen height */
    width: 1920px;  /* Exact screen width */
    overflow: hidden;
  }

  .sidebar {
    background-color: #333;
    color: white;
    padding: 1rem;
    box-sizing: border-box;
  }

  .main {
    display: grid;
    grid-template-rows: 216px 432px 432px; /* Exact pixel heights */
    background-color: #444;
  }

  .status-area {
    background-color: #555;
    color: white;
    padding: 1rem;
    box-sizing: border-box;
  }

  .camera-top, .camera-bottom {
    background-color: #666;
    color: white;
    padding: 1rem;
    box-sizing: border-box;
    border-bottom: 1px solid #777;
  }

  .camera-video {
    width: 100%;
    height: 100%;
    object-fit: cover;
    max-width: 100%;
    max-height: 100%;
  }
</style>
