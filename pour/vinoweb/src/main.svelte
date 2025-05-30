<script lang="ts">

 import { ViamProvider } from '@viamrobotics/svelte-sdk';
 import Stream from './lib/stream.svelte'; // TODO - use from svelte-sdk when we can

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
  <Stream name="cam-left" partID="xxx" />
  <Stream name="cam-right" partID="xxx" />
  <Status name="xxx" display="connection to {host}"/>
  {@render children?.()}
</ViamProvider>

