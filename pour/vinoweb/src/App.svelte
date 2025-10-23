<script lang="ts">
  import { getCookie, setCookie } from "typescript-cookie";
  import { ViamProvider } from "@viamrobotics/svelte-sdk";

  import logo from "./assets/viam.svg";
  import Main from "./main.svelte";

  let myState = $state({ error: "", machineId: "", host: "", credentials: {} });

  function getHostAndCredentials() {
    const parts = window.location.pathname.split("/");
    if (parts && parts.length >= 3 && parts[1] == "machine") {
      const machineId = parts[2];
      myState.machineId = machineId;
      const cookieValue = getCookie(machineId);
      if (cookieValue) {
        const parsedValue = JSON.parse(cookieValue);
        return [
          parsedValue.hostname,
          {
            type: "api-key",
            payload: parsedValue.key,
            authEntity: parsedValue.id,
          },
        ];
      }
    }

    const defaultCookie = getCookie("default-host");
    if (defaultCookie && defaultCookie != "") {
      const parsedDefault = JSON.parse(defaultCookie);
      return [
        parsedDefault.hostname,
        {
          type: "api-key",
          payload: parsedDefault.key,
          authEntity: parsedDefault.id,
        },
      ];
    }

    const host = getCookie("host");
    const keyId = getCookie("api-key-id");
    const key = getCookie("api-key");

    if (host && host != "") {
      return [host, { type: "api-key", payload: key, authEntity: keyId }];
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
    const host =
      (document.getElementById("in_host") as HTMLInputElement)?.value || "";
    const id =
      (document.getElementById("in_id") as HTMLInputElement)?.value || "";
    const key =
      (document.getElementById("in_key") as HTMLInputElement)?.value || "";

    if (host == "") {
      myState.error = "need a host";
      return;
    }
    if (id == "") {
      myState.error = "need an id";
      return;
    }
    if (key == "") {
      myState.error = "need a key";
      return;
    }

    setCookie(
      myState.machineId,
      JSON.stringify({ hostname: host, key: key, id: id })
    );
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
    <div class="credentials-form">
      <p class="form-text">No host found, want to specify a default?</p>
      <div class="form-group">
        <label for="in_host">Host:</label>
        <input id="in_host" type="text" />
      </div>
      <div class="form-group">
        <label for="in_id">Key Id:</label>
        <input id="in_id" type="text" />
      </div>
      <div class="form-group">
        <label for="in_key">Key:</label>
        <input id="in_key" type="password" />
      </div>
      <button onclick={saveHostInfo} class="save-button">Save</button>
    </div>
  {/if}
</main>

<style>
  .credentials-form {
    background-color: rgba(255, 255, 255, 0.95);
    padding: 30px;
    border-radius: 10px;
    box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3);
    max-width: 400px;
    margin: 0 auto;
  }

  .form-text {
    color: #333;
    font-size: 18px;
    margin-bottom: 20px;
    font-weight: 500;
  }

  .form-group {
    margin-bottom: 15px;
  }

  .form-group label {
    display: block;
    color: #333;
    font-weight: 500;
    margin-bottom: 5px;
  }

  .form-group input {
    width: 100%;
    padding: 10px;
    border: 2px solid #ddd;
    border-radius: 5px;
    font-size: 16px;
    background-color: white;
    color: #333;
  }

  .form-group input:focus {
    outline: none;
    border-color: #007bff;
    box-shadow: 0 0 5px rgba(0, 123, 255, 0.3);
  }

  .save-button {
    background-color: #007bff;
    color: white;
    padding: 12px 24px;
    border: none;
    border-radius: 5px;
    font-size: 16px;
    font-weight: 500;
    cursor: pointer;
    width: 100%;
    transition: background-color 0.2s;
  }

  .save-button:hover {
    background-color: #0056b3;
  }

  .save-button:active {
    background-color: #004085;
  }
</style>
