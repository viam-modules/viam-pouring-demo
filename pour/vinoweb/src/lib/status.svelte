<script lang="ts">
  // import { useConnectionStatus } from '@viamrobotics/svelte-sdk';
  import { Tag, InlineLoading } from "carbon-components-svelte";

  let { message = "SENSING...", status = "standby" } = $props();

  // Define status types using valid Carbon tag colors
  const statusTypes: Record<
    string,
    {
      type:
        | "red"
        | "magenta"
        | "purple"
        | "blue"
        | "cyan"
        | "teal"
        | "green"
        | "gray"
        | "cool-gray"
        | "warm-gray"
        | "high-contrast"
        | "outline";
      icon?: string;
      loading?: boolean;
    }
  > = {
    standby: { type: "green", icon: "checkmark--filled" },
    looking: { type: "purple", icon: "search" },
    picking: { type: "blue", loading: true },
    prepping: { type: "blue", loading: true },
    pouring: { type: "teal", loading: true },
    placing: { type: "teal", loading: true },
    waiting: { type: "green", icon: "checkmark--filled" },
    "manual mode": { type: "magenta", icon: "settings" },
    error: { type: "red", icon: "warning--filled" },
  };

  $effect(() => {
    console.log(`Status changed to: ${status}`);
  });

  const currentStatusType = () => {
    return statusTypes[status] || { type: "cool-gray", icon: "undefined" };
  };
</script>

<div class="status-container">
  <div class="status-terminal">
    <div class="terminal-body">
      <div class="status-display">
        <div class="left-content">
          <div class="tag-wrapper">
            <Tag type={currentStatusType().type}>{status.toUpperCase()}</Tag>
          </div>

          <span class="status-message">{message}</span>
        </div>

        {#if currentStatusType().loading}
          <div class="loading-wrapper">
            <InlineLoading status="active" description="" />
          </div>
        {/if}
      </div>
    </div>
  </div>
</div>

<style>
  .status-container {
    width: 100%;
    display: flex;
    justify-content: center;
    align-items: center;
    padding: 10px 0;
  }

  .status-terminal {
    width: 100%;
    background-color: #161616; /* Carbon's g100 theme background */
    border-radius: 8px;
    overflow: hidden;
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.2);
    border: 1px solid #393939;
  }

  .terminal-body {
    padding: 16px;
    font-family: "IBM Plex Mono", monospace;
  }

  .status-display {
    display: flex;
    align-items: center;
    justify-content: space-between; /* Push items to edges */
    gap: 12px;
    margin: 0;
    width: 100%;
  }

  .left-content {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .status-message {
    font-size: 1.2rem;
    color: #ffffff;
    letter-spacing: 0.05em;
  }

  /* Wrapper classes to style Carbon components */
  .tag-wrapper :global(.bx--tag) {
    padding: 0 12px;
    height: 24px;
    line-height: 24px;
    font-weight: 600;
  }

  /* Make loading indicator take minimal space */
  .loading-wrapper {
    margin-left: auto; /* This pushes it to the right */
  }

  .loading-wrapper :global(.bx--inline-loading) {
    min-height: 24px;
  }
</style>
