export interface ParsedPointCloud {
  x: number[];
  y: number[];
  z: number[];
}

/** RDK / Viam PCD coordinates are meters; viewer uses millimeters. */
const METERS_TO_MM = 1000;

function parsePcdHeader(data: Uint8Array): {
  headerText: string;
  dataOffset: number;
  dataFormat: "ascii" | "binary";
} | null {
  // Scan a reasonable prefix for the DATA marker (headers are small).
  const scanLen = Math.min(data.length, 8192);
  const prefix = new TextDecoder("latin1").decode(data.slice(0, scanLen));
  const match = prefix.match(/\nDATA\s+(ascii|binary)\s*\r?\n/i);
  if (!match || match.index == null) {
    // Some producers omit the leading newline on the first line after VERSION.
    const match2 = prefix.match(/^DATA\s+(ascii|binary)\s*\r?\n/im);
    if (!match2 || match2.index == null) return null;
    const dataFormat = match2[1].toLowerCase() === "ascii" ? "ascii" : "binary";
    const dataOffset = match2.index + match2[0].length;
    return { headerText: prefix.slice(0, match2.index), dataOffset, dataFormat };
  }
  const dataFormat = match[1].toLowerCase() === "ascii" ? "ascii" : "binary";
  const dataOffset = match.index + match[0].length;
  return { headerText: prefix.slice(0, match.index), dataOffset, dataFormat };
}

export function parsePCD(data: Uint8Array): ParsedPointCloud {
  const result: ParsedPointCloud = { x: [], y: [], z: [] };
  if (!data || data.length === 0) return result;

  const header = parsePcdHeader(data);
  if (!header) {
    console.warn("[parsePCD] no DATA ascii/binary header found; len=", data.length);
    return result;
  }

  const lines = header.headerText.split(/\r?\n/);
  let fields: string[] = [];
  let sizes: number[] = [];
  let types: string[] = [];
  let pointCount = 0;

  for (const line of lines) {
    const parts = line.trim().split(/\s+/);
    const key = (parts[0] || "").toUpperCase();
    if (key === "FIELDS") fields = parts.slice(1).map((f) => f.toLowerCase());
    else if (key === "SIZE") sizes = parts.slice(1).map(Number);
    else if (key === "TYPE") types = parts.slice(1).map((t) => t.toUpperCase());
    else if (key === "POINTS") pointCount = parseInt(parts[1], 10) || 0;
    else if (key === "WIDTH" && pointCount === 0) pointCount = parseInt(parts[1], 10) || 0;
  }

  const xIdx = fields.indexOf("x");
  const yIdx = fields.indexOf("y");
  const zIdx = fields.indexOf("z");
  if (xIdx === -1 || yIdx === -1 || zIdx === -1) {
    console.warn("[parsePCD] missing x/y/z fields:", fields);
    return result;
  }

  if (header.dataFormat === "ascii") {
    const text = new TextDecoder().decode(data.slice(header.dataOffset));
    const pointLines = text.trim().split(/\r?\n/);
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

  // Default float sizes if SIZE is missing.
  if (sizes.length < fields.length) {
    sizes = fields.map(() => 4);
  }

  const binaryData = data.slice(header.dataOffset);
  // Copy into a fresh ArrayBuffer so DataView is always on a contiguous ArrayBuffer
  // (not ArrayBufferLike / SharedArrayBuffer from some RPC stack views).
  const copy = new Uint8Array(binaryData.byteLength);
  copy.set(binaryData);
  const view = new DataView(copy.buffer);

  const fieldOffsets: number[] = [];
  let offset = 0;
  for (let i = 0; i < fields.length; i++) {
    fieldOffsets.push(offset);
    offset += sizes[i] || 4;
  }
  const stride = offset;
  if (stride <= 0) return result;

  const readFloat = (base: number, fieldIdx: number): number => {
    const off = base + fieldOffsets[fieldIdx];
    const size = sizes[fieldIdx] || 4;
    const typ = types[fieldIdx] || "F";
    if (size === 8 || typ === "D") return view.getFloat64(off, true);
    return view.getFloat32(off, true);
  };

  const maxPts =
    pointCount > 0
      ? Math.min(pointCount, Math.floor(copy.length / stride))
      : Math.floor(copy.length / stride);

  for (let i = 0; i < maxPts; i++) {
    const base = i * stride;
    if (base + stride > copy.length) break;
    const px = readFloat(base, xIdx);
    const py = readFloat(base, yIdx);
    const pz = readFloat(base, zIdx);
    if (Number.isFinite(px) && Number.isFinite(py) && Number.isFinite(pz)) {
      result.x.push(px * METERS_TO_MM);
      result.y.push(py * METERS_TO_MM);
      result.z.push(pz * METERS_TO_MM);
    }
  }

  return result;
}
