<script lang="ts">
  import './motion-tools.css'

  import { ViamAppProvider, ViamProvider } from '@viamrobotics/svelte-sdk'
  import { Visualizer } from '@viamrobotics/motion-tools'
  import { MotionPlanReplayer } from '@viamrobotics/motion-tools/plugins'
</script>

<!-- Self-contained: plan-file debugging needs no machine connection, so the
     providers get no dial configs / credentials and the visualizer no partID. -->
<div class="planner-debug">
  <ViamProvider dialConfigs={{}}>
    <ViamAppProvider
      serviceHost="https://app.viam.com"
      credentials={{ type: "api-key", payload: "", authEntity: "" }}
    >
      <Visualizer>
        <MotionPlanReplayer />
      </Visualizer>
    </ViamAppProvider>
  </ViamProvider>
</div>

<style>
  .planner-debug {
    position: fixed;
    inset: 0;
    background: white;
  }
</style>
