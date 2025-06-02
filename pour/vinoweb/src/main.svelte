<script lang="ts">

 import { ViamProvider } from '@viamrobotics/svelte-sdk';
 import CameraFeed from './lib/camera-feed.svelte';
 import { CameraStream } from '@viamrobotics/svelte-sdk';

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
      <div class=camera-box>
        <div class="camera-border">
          <CameraFeed name="cam-left" partID="xxx" label="Left Camera" />
        </div>
      </div>
      <div class=camera-box>
        <div class="camera-border">
          <CameraFeed name="cam-right" partID="xxx" label="Right Camera" />
        </div>
      </div>
    </div>
  </div>
  {@render children?.()}
</ViamProvider>

<style>
  .layout {
    display: grid;
    height: 100vh;
    grid-template-areas:
      "sidebar status"
      "sidebar main";
    grid-template-columns: 600px 1fr;
    background: url('/src/assets/viam-winedemo-interface-dc-16x9-blank.png') center center / cover no-repeat;
  }
  .sidebar {
    /* background-color: #f0f0f0; */
    padding: 20px;
    box-shadow: 2px 0 5px rgba(0,0,0,0.1);
    grid-area: sidebar;
  }
  .status {
    height: 150px;
    /* background-color: #e0e0e0; */
    box-shadow: -2px 0 5px rgba(0,0,0,0.1);
    grid-area: status;
    /* border: red 1px solid; */
  }
  .main {
    /* background-color: #fff; */
    /* border: blue 1px solid; */
    grid-area: main;
    display: grid;
    grid-template-rows: 1fr 1fr;
  }
  .camera-box {
    padding: 20px;
    height: 300px;
    display: flex;
    align-items: center;
    justify-content: center;
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
</style>