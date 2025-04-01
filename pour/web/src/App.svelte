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

<section
  class="relative rounded-xl w-fit h-fit max-h-[calc(100vh-100px)] overflow-hidden m-12 flex flex-col gap-4 bg-white p-8"
>
  <h1 class="text-2xl">Wine Pouring Demo</h1>
  <h2>Arrange your cups as-desired on the table.</h2>
  <h2>Ensure that they are detected in the camera view. Then press start.</h2>
  {#if pouringClient}
    <StartButton client={pouringClient} />
  {:else}
    <div>pouring service connecting...</div>
  {/if}

  <div class="flex gap-4">
    <div class="w-1/2">
      <h4>Camera</h4>
      {#if houghClient}
        <ImageDisplay client={houghClient} />
      {:else}
        <div>connecting...</div>
      {/if}
    </div>

    <div class="w-1/2">
      <h4>Weight Sensor</h4>
      {#if weightClient}
        <WeightSensor client={weightClient} />
      {:else}
        <div>connecting...</div>
      {/if}
      <p>
        The weight is used to calculate how much wine is left in the bottle.
        This is important for controlling the pouring angle and duration.
      </p>
    </div>
  </div>
</section>
