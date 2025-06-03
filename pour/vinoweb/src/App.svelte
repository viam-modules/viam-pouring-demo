<script lang="ts">
 import { getCookie, setCookie } from 'typescript-cookie'
 import { ViamProvider } from '@viamrobotics/svelte-sdk';
 
 import logo from './assets/viam.svg'
 import Main from "./main.svelte"


 let myState = $state({error : ""});

 function getHostAndCredentials() {
   var parts = window.location.pathname.split("/");
   if (parts && parts.length >= 3 && parts[1] == "machine") {
     var machineId = parts[2];
     myState.machineId = machineId;
     var x = getCookie(machineId);
     if (x) {
       var x = JSON.parse(x);
       return [x.hostname, {type: 'api-key', payload: x.key, authEntity: x.id}];
     }
   }

   var x = getCookie("default-host");
   if (x && x != "") {
     var x = JSON.parse(x);
     return [x.hostname, {type: 'api-key', payload: x.key, authEntity: x.id}];
   }

   var host = getCookie("host");
   var keyId = getCookie("api-key-id");
   var key = getCookie("api-key");

   if (host && host != "") {
     return [host, {type: 'api-key', payload: key, authEntity: keyId}];     
   }
   
   return ["", {}];
 }
 
 function init() {
   const [host, credentials] = getHostAndCredentials();
   if (host != "") {
     myState.host = host;
     myState.credentials = credentials;
   }
 }
 
 function saveHostInfo() {
   if (!myState.machineId) {
     throw "neeed a machineId";
   }
   var host = document.getElementById("in_host").value;
   var id = document.getElementById("in_id").value;
   var key = document.getElementById("in_key").value;

   if (host == "") {
     myState.error = "need a host";
     return;
   }
   if (id == "") {
     myState.error = "need an id";
     return;
   }
   if (key == "") {
     myState.error = "need a key"
     return;
   }

   setCookie(myState.machineId, JSON.stringify({hostname: host, key: key, id: id}))

 }

 init();
</script>

<main>
  {#if myState.error}
    <h1 style="color: red;">
      {myState.error}
    </h1>
  {/if}
      
  {#if myState.host}
    <Main host={myState.host} credentials={myState.credentials} />
    <!-- <ComponentPreview host={myState.host} credentials={myState.credentials} /> -->
  {:else}
    No host found, want to specify a default?<br>
    Host: <input id="in_host"><br>
    Key Id: <input id="in_id"><br>
    Key: <input id="in_key"><br>
    <button onclick="{saveHostInfo}">Save</button>
  {/if}


</main>

<style>
</style>
