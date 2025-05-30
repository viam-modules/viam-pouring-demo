<script lang="ts">

 import { useConnectionStatus } from '@viamrobotics/svelte-sdk';

 let { name, display } = $props();
 
 var status = useConnectionStatus(() => name);

</script>
<h5>{display} status: {status.current}</h5>
