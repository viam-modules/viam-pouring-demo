<script lang="ts">
  // This code must be run in a browser environment.
  import * as VIAM from "@viamrobotics/sdk";

  const main = async () => {
    const host = "xarm-lab-main.2d8xnaiovq.viam.cloud";

    const machine = await VIAM.createRobotClient({
      host,
      credentials: {
        type: "api-key",
        payload: "iw1n36ua8pznnkd4mr7zn24y7y4l50d9",
        authEntity: "46025557-9b2f-47e6-9b06-adbd15fce33c",
      },
      signalingAddress: "https://app.viam.com:443",
    });

    console.log("Resources:");
    console.log(await machine.resourceNames());

    // arm
    const armClient = new VIAM.ArmClient(machine, "arm");
    const armReturnValue = await armClient.getEndPosition();
    console.log("arm getEndPosition return value:", armReturnValue);
  };

  main().catch((error: unknown) => {
    console.error("encountered an error:", error);
  });
</script>

<svelte:head>
  <title>Pouring Demo UI</title>
  <meta name="description" content="Pouring Demo UI" />
</svelte:head>

<section></section>

<style>
  section {
    display: flex;
    flex-direction: column;
    justify-content: center;
    align-items: center;
    flex: 0.6;
  }

  h1 {
    width: 100%;
  }

  .welcome {
    display: block;
    position: relative;
    width: 100%;
    height: 0;
    padding: 0 0 calc(100% * 495 / 2048) 0;
  }

  .welcome img {
    position: absolute;
    width: 100%;
    height: 100%;
    top: 0;
    display: block;
  }
</style>
