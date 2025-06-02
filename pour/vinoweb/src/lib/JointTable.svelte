<script lang="ts">
  export let joints: { index: number, position: number }[] = [
    { index: 0, position: -64.83 },
    { index: 1, position: -108.82 },
    { index: 2, position: -32.63 },
    { index: 3, position: 348.78 },
    { index: 4, position: 110.80 },
    { index: 5, position: -184.56 }
  ];
</script>

<table class="joint-table">
  <thead>
    <tr>
      <th>Joint</th>
      <th>Position (°)</th>
    </tr>
  </thead>
  <tbody>
    {#each joints as joint}
      <tr>
        <td>{joint.index}</td>
        <td>{joint.position.toFixed(2)}</td>
      </tr>
    {/each}
  </tbody>
</table>

<style>
  .joint-table {
    border-collapse: collapse;
    width: 100%;
    background: #fff;
    border-radius: 8px;
    overflow: hidden;
    font-family: inherit;
    font-size: 1.1rem;
    box-shadow: 0 2px 8px rgba(0,0,0,0.08);
  }
  .joint-table th, .joint-table td {
    padding: 8px 16px;
    text-align: left;
  }
  .joint-table th {
    background: #f5f5f5;
    font-weight: 600;
  }
  .joint-table tr:not(:last-child) td {
    border-bottom: 1px solid #eee;
  }
</style>