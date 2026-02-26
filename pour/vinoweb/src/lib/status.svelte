<script lang="ts">
  import { Tag, InlineLoading } from "carbon-components-svelte";

  interface DetectionInfo {
    total_cup_objects: number;
    valid_cups: number;
    invalid_cups: number;
    bottles: number;
  }

  let {
    message = "SENSING...",
    status = "standby",
    detection = { total_cup_objects: 0, valid_cups: 0, invalid_cups: 0, bottles: 0 },
  }: { message?: string; status?: string; detection?: DetectionInfo } = $props();

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
            <div class="detection-item">
              <span class="detection-label">Cups</span>
              <span class="detection-count valid">
                <svg class="check-icon" viewBox="0 0 16 16" fill="currentColor">
                  <path d="M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1zm3.646 5.354l-4 4a.5.5 0 0 1-.707 0l-2-2a.5.5 0 1 1 .707-.708L7.293 9.293l3.646-3.647a.5.5 0 0 1 .707.708z"/>
                </svg>
                {detection.valid_cups}
              </span>
              {#if detection.invalid_cups > 0}
                <span class="detection-count invalid">
                  <svg class="x-icon" viewBox="0 0 16 16" fill="currentColor">
                    <path d="M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1zm2.854 9.146a.5.5 0 0 1-.708.708L8 8.707l-2.146 2.147a.5.5 0 0 1-.708-.708L7.293 8 5.146 5.854a.5.5 0 1 1 .708-.708L8 7.293l2.146-2.147a.5.5 0 0 1 .708.708L8.707 8l2.147 2.146z"/>
                  </svg>
                  {detection.invalid_cups}
                </span>
              {/if}
            </div>

            <span class="detection-divider">|</span>

            <div class="detection-item">
              <span class="detection-label">Bottles</span>
              <span class="detection-count" class:valid={detection.bottles > 0} class:dimmed={detection.bottles === 0}>
                {#if detection.bottles > 0}
                  <svg class="check-icon" viewBox="0 0 16 16" fill="currentColor">
                    <path d="M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1zm3.646 5.354l-4 4a.5.5 0 0 1-.707 0l-2-2a.5.5 0 1 1 .707-.708L7.293 9.293l3.646-3.647a.5.5 0 0 1 .707.708z"/>
                  </svg>
                {/if}
                {detection.bottles}
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

  .detection-count.invalid {
    color: #fa4d56;
  }

  .detection-count.dimmed {
    color: #6f6f6f;
  }

  .check-icon {
    width: 14px;
    height: 14px;
  }

  .x-icon {
    width: 14px;
    height: 14px;
  }

  .detection-divider {
    color: #525252;
    font-size: 0.85rem;
  }

  .loading-wrapper :global(.bx--inline-loading) {
    min-height: 24px;
  }
</style>
