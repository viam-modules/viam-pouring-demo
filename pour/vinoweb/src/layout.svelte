<script lang="ts">

 import { ViamProvider } from '@viamrobotics/svelte-sdk';
 import { CameraStream } from '@viamrobotics/svelte-sdk';

 import type { DialConf } from '@viamrobotics/sdk';
 import CameraFeed from './lib/CameraFeed.svelte';
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

  <div class="status-area">
      <Status name="xxx" display="connection to {host}"/>
    </div>

  <div class="main">
    <div class="camera-top">
      <CameraFeed
        name="cam-left"
        partID="xxx"
        displayName="Left Camera"
      />
    </div>
    <div class="camera-bottom">
      <CameraFeed
        name="cam-right"
        partID="xxx"
        displayName="Right Camera"
      />
    </div>
  </div>
</div>
</ViamProvider>


<style>

  :global(body) {
  background-image: url('assets/viam-winedemo-interface-dc-16x9-blank.png');
  background-size: cover;
  background-position: center;
  background-attachment: fixed;
}

  .layout {
    display: grid;
    grid-template-columns: 700px 1fr;
    grid-template-rows: 1fr 6fr;
    grid-template-areas:
      "sidebar status"
      "sidebar main";
    height: 1000px; /* Exact screen height - titlebar */
    width: 1700px;  /* Exact screen width - some padding */
    overflow: hidden;
  }

  .sidebar {
    padding: 1rem;
    box-sizing: border-box;
    grid-area: sidebar;
    border-right: 3px solid #777;
  }

  .main {
    grid-area: main;
    display: grid;
    grid-template-columns: 1fr 2fr;  
    grid-template-rows: 1fr 1fr;
  }

  .status-area {
    grid-area: status;
    color: white;
    padding: 1rem;
    box-sizing: border-box;
  }

  .camera-top, .camera-bottom {
    color: white;
    padding: 1rem;
    box-sizing: border-box;
    border-bottom: 1px solid #777;
    height: 400px;
  }
</style>
