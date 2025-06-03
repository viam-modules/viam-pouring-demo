<script lang="ts">
  import type { Joint } from '../lib/types.js';

  interface Props {
    joints?: Joint[];
    title?: string;
    indexLabel?: string;
    positionLabel?: string;
    positionUnit?: string;
    precision?: number;
  }

  let { 
    joints = [
      { index: 0, position: -64.83 },
      { index: 1, position: -108.82 },
      { index: 2, position: -32.63 },
      { index: 3, position: 348.78 },
      { index: 4, position: 110.80 },
      { index: 5, position: -184.56 }
    ],
    title,
    indexLabel = "Joint",
    positionLabel = "Position",
    positionUnit = "°",
    precision = 2
  }: Props = $props();
</script>

<div class="joint-table-wrapper">
  {#if title}
    <h3 class="table-title">{title}</h3>
  {/if}

  <table class="joint-table">
    <thead>
      <tr>
        <th>{indexLabel}</th>
        <th>{positionLabel} ({positionUnit})</th>
      </tr>
    </thead>
    <tbody>
      {#each joints as joint}
        <tr>
          <td>{joint.index}</td>
          <td>{joint.position.toFixed(precision)}</td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
  .joint-table-wrapper {
    width: 100%;
  }

  .table-title {
    margin-bottom: 8px;
    font-size: 1rem;
    font-weight: 600;
    color: #333;
  }

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

  .joint-table th, 
  .joint-table td {
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
