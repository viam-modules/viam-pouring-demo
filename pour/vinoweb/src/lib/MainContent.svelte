<script lang="ts">
  import type { Snippet } from "svelte";
  import type { RobotClient } from "@viamrobotics/sdk";
  import PointCloud3D from "./PointCloud3D.svelte";
  import CameraWithOverlay from "./CameraWithOverlay.svelte";
  import type { Joint, SegmentedObject } from "./types.js";

  interface Props {
    statusBar: Snippet;
    segmentedObjects: SegmentedObject[];
    robotClient: RobotClient | null;
    leftJoints: Joint[];
    rightJoints: Joint[];
    status: string;
  }

  let { statusBar, segmentedObjects, robotClient, leftJoints, rightJoints, status }: Props = $props();
</script>

<main class="main-content">
  <header class="status-bar">
    {@render statusBar()}
  </header>

  <section class="content-area">
    <div class="left-col">
      <div class="pcd-area">
        <PointCloud3D objects={segmentedObjects} />
      </div>
      <div class="table-area">
        <table class="joint-table">
          <thead>
            <tr>
              <th>Joint</th>
              <th>Left Arm (°)</th>
              <th>Right Arm (°)</th>
            </tr>
          </thead>
          <tbody>
            {#each leftJoints as _, i}
              <tr>
                <td class="joint-idx">{i}</td>
                <td>{leftJoints[i]?.position.toFixed(2) ?? "—"}</td>
                <td>{rightJoints[i]?.position.toFixed(2) ?? "—"}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>

    <div class="right-col">
      <div class="cam-area">
        <CameraWithOverlay camName="left-cam" label="Left Camera" objects={segmentedObjects} {robotClient} />
      </div>
      <div class="cam-area">
        <CameraWithOverlay camName="right-cam" label="Right Camera" objects={segmentedObjects} {robotClient} />
      </div>
    </div>
  </section>
</main>

<style>
  .main-content {
    background-color: white;
    display: flex;
    flex-direction: column;
    gap: 10px;
    height: 90%;
    width: 95%;
    border-radius: 12px;
    overflow: hidden;
    padding: 20px;
    margin: auto 0;
  }

  .status-bar {
    display: flex;
    align-items: center;
    flex-shrink: 0;
  }

  .content-area {
    display: flex;
    gap: 12px;
    flex: 1;
    min-height: 0;
    background: #ddd;
    border-radius: 16px;
    padding: 15px;
    overflow: hidden;
  }

  .left-col {
    width: 35%;
    display: flex;
    flex-direction: column;
    gap: 10px;
    min-height: 0;
  }

  .pcd-area {
    flex: 1;
    min-height: 0;
    border-radius: 8px;
    overflow: hidden;
  }

  .table-area {
    flex-shrink: 0;
  }

  .joint-table {
    border-collapse: collapse;
    width: 100%;
    background: #fff;
    border-radius: 8px;
    overflow: hidden;
    font-family: "IBM Plex Mono", monospace;
    font-size: 0.8rem;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  }

  .joint-table th,
  .joint-table td {
    padding: 5px 10px;
    text-align: left;
  }

  .joint-table th {
    background: #f5f5f5;
    font-weight: 600;
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: #555;
  }

  .joint-table tr:not(:last-child) td {
    border-bottom: 1px solid #eee;
  }

  .joint-table .joint-idx {
    font-weight: 600;
    color: #888;
  }

  .right-col {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 10px;
    min-height: 0;
  }

  .cam-area {
    flex: 1;
    min-height: 0;
    display: flex;
    justify-content: center;
    align-items: center;
  }
</style>
