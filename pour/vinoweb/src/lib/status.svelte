<script lang="ts">
  // import { useConnectionStatus } from '@viamrobotics/svelte-sdk';

  let { message = "SENSING..." } = $props();

  // var status = useConnectionStatus(() => name);
</script>

<div class="status-message">
  {message}
  <!-- <h5>{display} status: {status.current}</h5> -->
</div>

<style>
  .status-message {
    font-family: 'Share Tech Mono', 'Fira Mono', 'Consolas', monospace;
    font-size: 3rem;
    color: #00ffea;
    background: #181c1f;
    padding: 2rem 0;
    text-align: center;
    border-radius: 12px;
    letter-spacing: 0.1em;
    box-shadow: 0 2px 16px rgba(0,255,234,0.08);
    text-transform: uppercase;
    width: 100%;
    user-select: none;
  }
</style>
