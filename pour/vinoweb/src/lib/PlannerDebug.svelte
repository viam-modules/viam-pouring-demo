<script lang="ts">
  import './motion-tools.css'

  import { ViamAppProvider, ViamProvider } from '@viamrobotics/svelte-sdk'
  import { Visualizer } from '@viamrobotics/motion-tools'
  import { Snapshot, SnapshotProto } from '@viamrobotics/motion-tools/lib'

  interface StepJson {
    label: string
    colliding: string[]
    snapshot: unknown
  }

  interface FileCamera {
    position: [number, number, number]
    lookAt: [number, number, number]
  }

  interface Step {
    label: string
    colliding: string[]
    proto: SnapshotProto
  }

  let steps = $state<Step[]>([])
  let index = $state(0)
  let loadError = $state('')
  let fileInput: HTMLInputElement | undefined = $state()
  // Applied by the Visualizer only when the value changes — i.e. once per file
  // load — so stepping (including back to step 1) never resets the view.
  let cameraPose = $state<FileCamera | undefined>()

  const current = $derived(steps[index])

  const onFileChange = async (e: Event) => {
    const file = (e.currentTarget as HTMLInputElement).files?.[0]
    if (!file) return
    try {
      const parsed = JSON.parse(await file.text()) as { steps: StepJson[]; camera?: FileCamera }
      if (!Array.isArray(parsed.steps) || parsed.steps.length === 0) {
        throw new Error('file has no steps — expected a plan-doctor -snapshots.json')
      }
      steps = parsed.steps.map((s) => ({
        label: s.label,
        colliding: s.colliding ?? [],
        proto: SnapshotProto.fromJson(s.snapshot as Parameters<typeof SnapshotProto.fromJson>[0]),
      }))
      index = 0
      loadError = ''
      if (parsed.camera) cameraPose = parsed.camera
    } catch (err) {
      steps = []
      loadError = err instanceof Error ? err.message : 'failed to parse snapshots file'
    }
    if (fileInput) fileInput.value = ''
  }
</script>

<!-- Self-contained: snapshot debugging needs no machine connection, so the
     providers get no dial configs / credentials and the visualizer no partID. -->
<div class="planner-debug">
  <ViamProvider dialConfigs={{}}>
    <ViamAppProvider
      serviceHost="https://app.viam.com"
      credentials={{ type: "api-key", payload: "", authEntity: "" }}
    >
      <Visualizer {cameraPose}>
        {#if current}
          <Snapshot snapshot={current.proto} />
        {/if}
      </Visualizer>
    </ViamAppProvider>
  </ViamProvider>

  <div class="stepper">
    <div class="stepper-title">Failed IK candidates</div>

    {#if steps.length > 0}
      <div class="stepper-controls">
        <button onclick={() => (index = Math.max(0, index - 1))} disabled={index === 0}>‹</button>
        <span class="stepper-count">{index + 1} / {steps.length}</span>
        <button
          onclick={() => (index = Math.min(steps.length - 1, index + 1))}
          disabled={index === steps.length - 1}>›</button
        >
      </div>
      {#if current}
        <div class="stepper-label">{current.label}</div>
      {/if}
    {:else}
      <div class="stepper-hint">Upload a plan-doctor <code>-snapshots.json</code> file</div>
    {/if}

    {#if loadError}
      <div class="stepper-error">{loadError}</div>
    {/if}

    <input bind:this={fileInput} type="file" accept=".json" onchange={onFileChange} />
  </div>
</div>

<style>
  .planner-debug {
    position: fixed;
    inset: 0;
    background: white;
  }
  .stepper {
    position: absolute;
    top: 12px;
    right: 12px;
    width: 300px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 12px;
    background: rgba(255, 255, 255, 0.95);
    border: 1px solid #d7d7d9;
    border-radius: 6px;
    font-family: system-ui;
    font-size: 12px;
    color: #333;
  }
  .stepper-title {
    font-size: 14px;
    font-weight: 600;
  }
  .stepper-controls {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .stepper-controls button {
    width: 28px;
    height: 24px;
    border: 1px solid #d7d7d9;
    border-radius: 4px;
    background: white;
    cursor: pointer;
  }
  .stepper-controls button:disabled {
    opacity: 0.4;
    cursor: default;
  }
  .stepper-count {
    font-variant-numeric: tabular-nums;
  }
  .stepper-label {
    color: #b91c1c;
    line-height: 1.35;
  }
  .stepper-hint {
    color: #666;
  }
  .stepper-error {
    color: #b91c1c;
  }
</style>
