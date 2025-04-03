<script lang="ts">
  import * as VIAM from "@viamrobotics/sdk";
  import { getCookie } from "typescript-cookie";

  import StartButton from "./start-button.svelte";
  import ImageDisplay from "./image-display.svelte";
  import WeightSensor from "./weight-sensor.svelte";

  let pouringClient: VIAM.GenericService | undefined = undefined;
  let houghClient: VIAM.VisionClient | undefined = undefined;
  let weightClient: VIAM.SensorClient | undefined = undefined;

  function getUrlOrCookies(n, def) {
    const urlParams = new URLSearchParams(window.location.search);

    var x = urlParams.get(n);
    if (x && x.length > 0) {
      return x;
    }

    x = getCookie(n);
    if (x && x.length > 0) {
      return x;
    }

    return def;
  }

  async function getConfig() {
    if (import.meta.env.VITE_HOST) {
      return {
        host: import.meta.env.VITE_HOST,
        payload: import.meta.env.VITE_PAYLOAD,
        authEntity: import.meta.env.VITE_KEY_ID,
      };
    }

    var host = getUrlOrCookies("host");
    var payload = getUrlOrCookies("payload");
    var authEntity = getUrlOrCookies("authEntity");

    if (host) {
      return {
        host: host,
        payload: payload,
        authEntity: authEntity,
      };
    }

    throw new Error("no connection config");
  }

  const main = async () => {
    var cfg = await getConfig();

    const machine = await VIAM.createRobotClient({
      host: cfg.host,
      credentials: {
        type: "api-key",
        payload: cfg.payload,
        authEntity: cfg.authEntity,
      },
      signalingAddress: "https://app.viam.com:443",
    });

    pouringClient = new VIAM.GenericServiceClient(
      machine,
      getUrlOrCookies("my_service", "pouring-service")
    );
    houghClient = new VIAM.VisionClient(
      machine,
      getUrlOrCookies("circle_detection_service", "circle-service")
    );
    weightClient = new VIAM.SensorClient(
      machine,
      getUrlOrCookies("weight_sensor_name", "scale1")
    );
  };

  main().catch((error: unknown) => {
    console.error("encountered an error:", error);
  });
</script>

<div class="flex flex-col gap-12 w-full">
  <section
    class="w-screen-md lg:w-lg flex gap-16 relative rounded-xl bg-white px-14 py-12 mt-8 mx-8"
  >
    <div class="flex flex-col gap-4 self-center">
      <h1 class="text-4xl font-light">Red or white wine?</h1>
      <h2 class="-mt-2 mb-4 text-4xl font-medium">
        Poured by AI, picked by you.
      </h2>
      <ol class="mb-4 text-xl leading-8 list-decimal pl-5 font-light">
        <li>Grab a glass.</li>
        <li>Place on table in view of robot camera.</li>
        <li>Select a wine below and enjoy!</li>
      </ol>
      {#if pouringClient}
        <StartButton client={pouringClient} />
      {:else}
        <p class="font-light text-lg">Pouring service connecting...</p>
      {/if}
      <p class="font-light text-lg">Share on social and tag @viamrobotics</p>
    </div>
    <p></p>
    <div class="relative w-[441px] h-[400px]">
      {#if houghClient}
        <ImageDisplay client={houghClient} />
      {:else}
        <p class="font-light text-lg">connecting...</p>
      {/if}
    </div>
    <!-- <h4>Weight Sensor</h4>
      {#if weightClient}
        <WeightSensor client={weightClient} />
      {:else}
        <div>connecting...</div>
      {/if}
      <p>
        The weight is used to calculate how much wine is left in the bottle.
        This is important for controlling the pouring angle and duration.
      </p> -->
  </section>
  <div class="center">
    <img
      src="/static/viam-winedemo-interface-lockup.png"
      alt="Viam Wine Demo Interface"
      class="mx-auto w-auto mt-8 max-w-[600px]"
    />
  </div>
</div>
