import type { PointCloudObject } from "@viamrobotics/sdk";
import { parsePCD } from "./parsePCD.js";
import type { CupDetectionMetrics, SegmentedObject } from "./types.js";

export const CUP_DETECTION_META_LABEL = "__cup_detection_meta__";

export interface CupDetectionSummary {
  cupHeightMm: number;
  cupWidthMm: number;
  toleranceMm: number;
  objectCount: number;
  validCups: number;
  invalidCups: number;
}

export interface ParsedCupDetection {
  summary: CupDetectionSummary;
  cups: SegmentedObject[];
  bestCup: SegmentedObject | null;
  metrics: CupDetectionMetrics | null;
}

type Vector3Like = { x?: number; y?: number; z?: number };
type GeometryLike = {
  label?: string;
  center?: Vector3Like;
  /** @bufbuild/protobuf oneof */
  geometryType?: { case?: string; value?: { dimsMm?: Vector3Like } };
  box?: { dimsMm?: Vector3Like };
};

function num(v: unknown): number {
  const n = Number(v);
  return Number.isFinite(n) ? n : 0;
}

function firstGeometry(obj: PointCloudObject): GeometryLike | undefined {
  return obj.geometries?.geometries?.[0] as GeometryLike | undefined;
}

function objectLabel(obj: PointCloudObject): string {
  return firstGeometry(obj)?.label ?? "";
}

function isMetaLabel(label: string): boolean {
  return label === CUP_DETECTION_META_LABEL || label.startsWith(CUP_DETECTION_META_LABEL);
}

function boxDims(geom: GeometryLike): { x: number; y: number; z: number } | null {
  const gt = geom.geometryType;
  if (gt?.case === "box" && gt.value?.dimsMm) {
    const d = gt.value.dimsMm;
    return { x: num(d.x), y: num(d.y), z: num(d.z) };
  }
  const legacy = geom.box?.dimsMm;
  if (legacy) {
    return { x: num(legacy.x), y: num(legacy.y), z: num(legacy.z) };
  }
  return null;
}

function centerPoint(geom: GeometryLike): { x: number; y: number; z: number } | null {
  const c = geom.center;
  if (!c) return null;
  return { x: num(c.x), y: num(c.y), z: num(c.z) };
}

function pointCloudBytes(obj: PointCloudObject): Uint8Array | null {
  const raw = (obj as { pointCloud?: Uint8Array; point_cloud?: Uint8Array }).pointCloud
    ?? (obj as { point_cloud?: Uint8Array }).point_cloud;
  if (!raw || raw.length === 0) return null;
  return raw;
}

function parseMetaSummary(obj: PointCloudObject): CupDetectionSummary | null {
  const geom = firstGeometry(obj);
  if (!geom || !isMetaLabel(objectLabel(obj))) return null;

  const dims = boxDims(geom);
  const center = centerPoint(geom);
  if (!dims || !center) return null;

  return {
    cupHeightMm: dims.x,
    cupWidthMm: dims.y,
    toleranceMm: dims.z,
    objectCount: center.x,
    validCups: center.y,
    invalidCups: center.z,
  };
}

function boundsFromPoints(x: number[], y: number[], z: number[]) {
  let minX = Infinity;
  let maxX = -Infinity;
  let minY = Infinity;
  let maxY = -Infinity;
  let maxZ = -Infinity;
  for (let i = 0; i < x.length; i++) {
    minX = Math.min(minX, x[i]);
    maxX = Math.max(maxX, x[i]);
    minY = Math.min(minY, y[i]);
    maxY = Math.max(maxY, y[i]);
    maxZ = Math.max(maxZ, z[i]);
  }
  return { minX, maxX, minY, maxY, maxZ };
}

function analyzeCup(
  x: number[],
  y: number[],
  z: number[],
  expectedHeight: number,
  expectedWidth: number,
  toleranceMm: number,
): CupDetectionMetrics {
  const b = boundsFromPoints(x, y, z);
  const observedHeight = b.maxZ;
  const observedWidth = (b.maxY - b.minY + (b.maxX - b.minX)) / 2;
  const heightDelta = Math.abs(observedHeight - expectedHeight);
  const widthDelta = Math.abs(expectedWidth - observedWidth);
  const heightPass = heightDelta <= toleranceMm;
  const widthPass = widthDelta <= toleranceMm;
  return {
    valid: heightPass && widthPass,
    expectedHeight,
    observedHeight,
    heightDelta,
    heightPass,
    expectedWidth,
    observedWidth,
    widthDelta,
    widthPass,
    toleranceMm,
  };
}

function cupFromObject(obj: PointCloudObject, index: number): SegmentedObject | null {
  const label = objectLabel(obj);
  if (!label || isMetaLabel(label)) return null;

  const pc = pointCloudBytes(obj);
  if (!pc) return null;

  const parsed = parsePCD(pc);
  if (parsed.x.length === 0) return null;

  const valid = label === "cup_valid";

  return {
    index,
    totalPoints: parsed.x.length,
    points_x: parsed.x,
    points_y: parsed.y,
    points_z: parsed.z,
    valid,
    rawPCD: pc,
  };
}

export function cupMetricsFromCup(
  cup: SegmentedObject,
  summary: CupDetectionSummary,
): CupDetectionMetrics {
  return analyzeCup(
    cup.points_x,
    cup.points_y,
    cup.points_z,
    summary.cupHeightMm,
    summary.cupWidthMm,
    summary.toleranceMm,
  );
}

/** @deprecated use cupMetricsFromCup */
export function cupMetricsFromDetail(c: Record<string, unknown>): CupDetectionMetrics {
  return {
    valid: !!c.valid,
    expectedHeight: num(c.expected_height),
    observedHeight: num(c.height),
    heightDelta: num(c.height_delta),
    heightPass: !!c.height_pass,
    expectedWidth: num(c.expected_width),
    observedWidth: num(c.width),
    widthDelta: num(c.width_delta),
    widthPass: !!c.width_pass,
    toleranceMm: num(c.good_delta),
  };
}

export function parseVisionCupObjects(objects: PointCloudObject[]): ParsedCupDetection {
  const summary: CupDetectionSummary = {
    cupHeightMm: 0,
    cupWidthMm: 0,
    toleranceMm: 25,
    objectCount: 0,
    validCups: 0,
    invalidCups: 0,
  };

  const cups: SegmentedObject[] = [];
  let cupIndex = 0;

  for (const obj of objects) {
    const label = objectLabel(obj);
    if (isMetaLabel(label)) {
      const meta = parseMetaSummary(obj);
      if (meta) Object.assign(summary, meta);
      continue;
    }

    const cup = cupFromObject(obj, cupIndex);
    if (!cup) continue;
    cups.push(cup);
    cupIndex++;
  }

  if (summary.objectCount === 0) {
    summary.objectCount = cups.length;
    summary.invalidCups = cups.filter((c) => !c.valid).length;
    summary.validCups = cups.length - summary.invalidCups;
  }

  const bestCup = cups.find((c) => c.valid) ?? cups[0] ?? null;

  return {
    summary,
    cups,
    bestCup,
    metrics: bestCup ? cupMetricsFromCup(bestCup, summary) : null,
  };
}
