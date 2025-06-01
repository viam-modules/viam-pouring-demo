<script lang="ts">
 import { getCookie, setCookie } from 'typescript-cookie'
 import { ViamProvider } from '@viamrobotics/svelte-sdk';
 
 import logo from './assets/viam.svg'
 import Main from "./main.svelte"
 import Layout from "./layout.svelte"


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
  {#if myState.host}
    <Layout host={myState.host} credentials={myState.credentials} />
  {:else}
    No host found, want to specify a default?<br>
    Host: <input id="in_host"><br>
    Key Id: <input id="in_id"><br>
    Key: <input id="in_key"><br>
    <button on:click="{saveHostInfo}">Save</button>
  {/if}
  
</main>

<style>
  .logo {
    height: 6em;
    padding: 1.5em;
    will-change: filter;
    transition: filter 300ms;
  }
  .logo:hover {
    filter: drop-shadow(0 0 2em #646cffaa);
  }
</style>
