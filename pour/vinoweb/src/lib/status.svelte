<script lang="ts">
  // import { useConnectionStatus } from '@viamrobotics/svelte-sdk';

  let { message = "SENSING..." } = $props();

  // var status = useConnectionStatus(() => name);
</script>

<div class="status-container">
  <div class="status-message">
    {message}
  </div>
</div>

<style>
  .status-container {
    width: 100%;
    height: 80px;
    display: flex;
    justify-content: center;
    align-items: center;
    padding: 10px 0;
    box-shadow: 0 2px 8px rgba(0, 255, 234, 0.04);
  }

  .status-message {
    display: flex;
    align-items: center;
    justify-content: center;
    font-family: "Share Tech Mono", "Fira Mono", "Consolas", monospace;
    font-size: 1.5rem;
    color: #39FF14;
    background: #181c1f;
    text-align: center;
    border-radius: 12px;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    width: 100%;
    height: 100%;
    user-select: none;
    /* Softer CRT glow effect */
    text-shadow:
      0 0 2px #39FF14,
      0 0 6px #39FF14;
    border: 2px solid #39FF14;
    box-shadow: 0 0 8px #39FF1433;
    padding: 10px 20px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
</style>
