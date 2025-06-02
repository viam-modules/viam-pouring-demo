<script lang="ts">

 import { ViamProvider } from '@viamrobotics/svelte-sdk';
 import CameraFeed from './lib/camera-feed.svelte';
 import { CameraStream } from '@viamrobotics/svelte-sdk';
 import JointTable from './lib/JointTable.svelte';

 import type { DialConf } from '@viamrobotics/sdk';
 import Status from "./lib/status.svelte"

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
    </div>
    <div class="status">
      <Status name="xxx" display="connection to {host}"/>
    </div>
    <div class = "main">
      <div class="camera-box">
        <div class="data">
          <JointTable />
        </div>
        <CameraFeed name="cam-left" partID="xxx" label="Left Camera" />
      </div>
      <div class="camera-box">
        <div class="data">
          <JointTable />
        </div>
        <CameraFeed name="cam-right" partID="xxx" label="Right Camera" />
      </div>
    </div>
  </div>
  {@render children?.()}
</ViamProvider>

<style>
  .layout {
    box-sizing: border-box;
    margin-right: 20px;
    margin-bottom: 20px;
    display: grid;
    gap: 20px;
    height: 100vh;
    grid-template-areas:
      "sidebar status"
      "sidebar main";
    grid-template-columns: 600px 1fr;
    grid-template-rows: 150px 1fr;
    background: url('/src/assets/viam-winedemo-interface-dc-16x9-blank.png') center center / cover no-repeat;
  }
  .sidebar {
    padding: 20px;
    box-shadow: 2px 0 5px rgba(0,0,0,0.1);
    grid-area: sidebar;
  }
  .status {
    height: 150px;
    margin: 2rem;
    /* box-shadow: -2px 0 5px rgba(0,0,0,0.1); */
    grid-area: status;
  }
  .main {
    grid-area: main;
    display: grid;
    gap: 20px;
    grid-template-rows: 1fr 1fr;
    height: 100%;
    min-height: 0;
    min-width: 0;
    overflow: hidden; /* Prevent overflow */
    min-height: 0; /* Allow shrinking */
    background-color: lightgrey;
    border-radius: 8px;
  }
  .camera-box {
    padding: 20px;
    display: grid;
    grid-template-rows: 1fr 1fr;
    grid-template-columns: auto 1fr;
    height: 100%;
    overflow: hidden; /* Prevent overflow */
  }
  
  .camera-border {
    border: 2px solid #fff;
    border-radius: 8px;
    background: rgba(0,0,0,0.2);
    box-shadow: 0 2px 12px 0 rgba(0,0,0,0.25);
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .data {
    display: flex;
    justify-content: center;
    padding: 10px;
    background: rgba(255, 255, 255, 0.8);
    border-radius: 8px;
    box-shadow: 0 2px 12px rgba(0,0,0,0.1);
    overflow-y: auto; /* Allow scrolling if content overflows */
  }
</style>