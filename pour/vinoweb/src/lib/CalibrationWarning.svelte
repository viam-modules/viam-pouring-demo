<script lang="ts">
  import { tick } from "svelte";

  let {
    show = false,
    message = "",
  }: { show: boolean; message: string } = $props();

  let dismissed = $state(false);
  let lastMessage = $state("");
  let modalEl: HTMLDivElement | null = $state(null);

  // Re-show if the error message changes (new calibration failure)
  $effect(() => {
    if (message && message !== lastMessage) {
      dismissed = false;
      lastMessage = message;
    }
  });

  const visible = $derived(show && !dismissed);

  // Focus modal when it becomes visible so keyboard/screen readers work
  $effect(() => {
    if (visible && modalEl) {
      tick().then(() => modalEl?.focus());
    }
  });

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") dismissed = true;
  }
</script>

{#if visible}
  <div class="backdrop-wrap">
    <button class="backdrop" onclick={() => (dismissed = true)} aria-label="Dismiss calibration warning"></button>
    <div
      class="modal"
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="cal-title"
      tabindex="-1"
      bind:this={modalEl}
      onkeydown={handleKeydown}
    >
      <div class="modal-icon">⚠</div>
      <h2 id="cal-title">Calibration Warning</h2>
      <p class="modal-message">
        {message || "April tag alignment check failed. Please reposition the arms."}
      </p>
      <p class="modal-hint">The robot will not operate until the arms are realigned.</p>
      <button class="dismiss-btn" onclick={() => (dismissed = true)}>
        Acknowledge &amp; Dismiss
      </button>
    </div>
  </div>
{/if}

<style>
  /* Carbon warning: $support-warning = #f1c21b (yellow-30) */
  .backdrop-wrap {
    position: fixed;
    inset: 0;
    z-index: 200;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .backdrop {
    position: absolute;
    inset: 0;
    background: rgba(0, 0, 0, 0.82);
    border: none;
    cursor: default;
  }

  .modal {
    position: relative;
    z-index: 1;
    background: #1c1c1c;
    border: 1px solid #f1c21b;
    border-radius: 12px;
    padding: 40px 48px;
    max-width: 520px;
    width: 90%;
    text-align: center;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
    transform: translateY(-1rem);
    outline: none;
  }

  .modal-icon {
    font-size: 2.5rem;
    color: #f1c21b;
    margin-bottom: 12px;
    line-height: 1;
  }

  h2 {
    color: #f4f4f4;
    font-size: 1.25rem;
    font-weight: 600;
    margin: 0 0 12px;
  }

  .modal-message {
    color: #c6c6c6;
    font-size: 0.925rem;
    line-height: 1.5;
    margin: 0 0 8px;
  }

  .modal-hint {
    color: #6f6f6f;
    font-size: 0.825rem;
    margin: 0 0 28px;
  }

  .dismiss-btn {
    background: #f1c21b;
    color: #161616;
    border: none;
    border-radius: 6px;
    padding: 10px 28px;
    font-size: 0.925rem;
    font-weight: 600;
    cursor: pointer;
    transition: background 0.15s;
  }

  .dismiss-btn:hover {
    background: #d2a106;
  }
</style>
