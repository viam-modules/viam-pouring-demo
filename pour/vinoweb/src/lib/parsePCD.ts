export interface ParsedPointCloud {
  x: number[];
  y: number[];
  z: number[];
}

/** RDK writes PCD coordinates in meters; internal vision math uses millimeters. */
const METERS_TO_MM = 1000;

function indexOfSubarray(haystack: Uint8Array, needle: Uint8Array): number {
  if (needle.length === 0) return 0;
  for (let i = 0; i <= haystack.length - needle.length; i++) {
    let match = true;
    for (let j = 0; j < needle.length; j++) {
      if (haystack[i + j] !== needle[j]) {
        match = false;
        break;
      }
    }
    if (match) return i;
  }
  return -1;
}

function parsePcdHeader(data: Uint8Array): {
  headerText: string;
  dataOffset: number;
  dataFormat: "ascii" | "binary";
} | null {
  const binaryMarker = new TextEncoder().encode("\nDATA binary\n");
  const asciiMarker = new TextEncoder().encode("\nDATA ascii\n");

  let markerIdx = indexOfSubarray(data, binaryMarker);
  let dataFormat: "ascii" | "binary" = "binary";
  if (markerIdx === -1) {
    markerIdx = indexOfSubarray(data, asciiMarker);
    dataFormat = "ascii";
  }
  if (markerIdx === -1) return null;

  const dataOffset = markerIdx + (dataFormat === "binary" ? binaryMarker.length : asciiMarker.length);
  const headerText = new TextDecoder().decode(data.slice(0, markerIdx));
  return { headerText, dataOffset, dataFormat };
}

export function parsePCD(data: Uint8Array): ParsedPointCloud {
  const result: ParsedPointCloud = { x: [], y: [], z: [] };
  const header = parsePcdHeader(data);
  if (!header) return result;

  const lines = header.headerText.split("\n");
  let fields: string[] = [];
  let sizes: number[] = [];
  let pointCount = 0;

  for (const line of lines) {
    const parts = line.trim().split(/\s+/);
    const key = parts[0];
    if (key === "FIELDS") fields = parts.slice(1);
    else if (key === "SIZE") sizes = parts.slice(1).map(Number);
    else if (key === "POINTS") pointCount = parseInt(parts[1], 10);
    else if (key === "WIDTH" && pointCount === 0) pointCount = parseInt(parts[1], 10);
  }

  const xIdx = fields.indexOf("x");
  const yIdx = fields.indexOf("y");
  const zIdx = fields.indexOf("z");
  if (xIdx === -1 || yIdx === -1 || zIdx === -1) return result;

  if (header.dataFormat === "ascii") {
    const text = new TextDecoder().decode(data.slice(header.dataOffset));
    const pointLines = text.trim().split("\n");
    for (const pl of pointLines) {
      const vals = pl.trim().split(/\s+/).map(Number);
      if (vals.length > Math.max(xIdx, yIdx, zIdx)) {
        result.x.push(vals[xIdx] * METERS_TO_MM);
        result.y.push(vals[yIdx] * METERS_TO_MM);
        result.z.push(vals[zIdx] * METERS_TO_MM);
      }
    }
    return result;
  }

  const binaryData = data.slice(header.dataOffset);
  const stride = sizes.reduce((a, b) => a + b, 0);
  if (stride <= 0) return result;

  const fieldOffsets: number[] = [];
  let offset = 0;
  for (const s of sizes) {
    fieldOffsets.push(offset);
    offset += s;
  }

  const view = new DataView(binaryData.buffer, binaryData.byteOffset, binaryData.byteLength);

  for (let i = 0; i < pointCount && (i + 1) * stride <= binaryData.length; i++) {
    const base = i * stride;
    const px = view.getFloat32(base + fieldOffsets[xIdx], true);
    const py = view.getFloat32(base + fieldOffsets[yIdx], true);
    const pz = view.getFloat32(base + fieldOffsets[zIdx], true);

    if (isFinite(px) && isFinite(py) && isFinite(pz)) {
      result.x.push(px * METERS_TO_MM);
      result.y.push(py * METERS_TO_MM);
      result.z.push(pz * METERS_TO_MM);
    }
  }

  return result;
}
