<script lang="ts">
  import { Tag, InlineLoading } from "carbon-components-svelte";

  let {
    message = "SENSING...",
    status = "standby",
    objectCount = 0,
    onCupClick,
    cupPanelOpen = false,
  }: {
    message?: string;
    status?: string;
    objectCount?: number;
    onCupClick?: () => void;
    cupPanelOpen?: boolean;
  } = $props();

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

        <div class="right-content">
          <div class="detection-indicators">
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div class="detection-item cups-clickable" class:cups-active={cupPanelOpen} onclick={onCupClick}>
              <span class="detection-label">Objects</span>
              <span class="detection-count" class:valid={objectCount > 0} class:dimmed={objectCount === 0}>
                {objectCount}
              </span>
            </div>
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
    background-color: #161616;
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
    justify-content: space-between;
    gap: 12px;
    margin: 0;
    width: 100%;
  }

  .left-content {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .right-content {
    display: flex;
    align-items: center;
    gap: 16px;
    margin-left: auto;
  }

  .status-message {
    font-size: 1.2rem;
    color: #ffffff;
    letter-spacing: 0.05em;
  }

  .tag-wrapper :global(.bx--tag) {
    padding: 0 12px;
    height: 24px;
    line-height: 24px;
    font-weight: 600;
  }

  .detection-indicators {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 0.85rem;
  }

  .detection-item {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .cups-clickable {
    cursor: pointer;
    padding: 4px 8px;
    border-radius: 4px;
    transition: background 0.15s, border-color 0.15s;
    border-bottom: 2px solid transparent;
  }
  .cups-clickable:hover {
    background: #333;
  }
  .cups-active {
    background: #2a2a2a;
    border-bottom-color: #4589ff;
  }
  .cups-active .detection-label {
    color: #4589ff;
  }

  .detection-label {
    color: #a8a8a8;
    font-weight: 500;
    text-transform: uppercase;
    font-size: 0.75rem;
    letter-spacing: 0.05em;
  }

  .detection-count {
    display: flex;
    align-items: center;
    gap: 3px;
    font-weight: 600;
    font-size: 0.9rem;
    color: #c6c6c6;
  }

  .detection-count.valid {
    color: #42be65;
  }

  .detection-count.dimmed {
    color: #6f6f6f;
  }

  .loading-wrapper :global(.bx--inline-loading) {
    min-height: 24px;
  }
</style>
