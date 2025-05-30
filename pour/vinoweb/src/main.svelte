<script lang="ts">

 import { ViamProvider } from '@viamrobotics/svelte-sdk';
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
  <CameraStream name="cam-left" partID="xxx" />
  <CameraStream name="cam-right" partID="xxx" />
  <Status name="xxx" display="connection to {host}"/>
  {@render children?.()}
</ViamProvider>

