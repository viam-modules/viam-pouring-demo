export interface ParsedPointCloud {
  x: number[];
  y: number[];
  z: number[];
}

export function parsePCD(data: Uint8Array): ParsedPointCloud {
  const result: ParsedPointCloud = { x: [], y: [], z: [] };

  const text = new TextDecoder().decode(data);
  const headerEnd = text.indexOf("\nDATA ");
  if (headerEnd === -1) return result;

  const header = text.slice(0, headerEnd);
  const dataLine = text.slice(headerEnd + 1, text.indexOf("\n", headerEnd + 1));
  const dataFormat = dataLine.replace("DATA ", "").trim();

  const lines = header.split("\n");
  let fields: string[] = [];
  let sizes: number[] = [];
  let types: string[] = [];
  let pointCount = 0;

  for (const line of lines) {
    const parts = line.trim().split(/\s+/);
    const key = parts[0];
    if (key === "FIELDS") fields = parts.slice(1);
    else if (key === "SIZE") sizes = parts.slice(1).map(Number);
    else if (key === "TYPE") types = parts.slice(1).map((s) => s);
    else if (key === "POINTS") pointCount = parseInt(parts[1], 10);
    else if (key === "WIDTH" && pointCount === 0) pointCount = parseInt(parts[1], 10);
  }

  const xIdx = fields.indexOf("x");
  const yIdx = fields.indexOf("y");
  const zIdx = fields.indexOf("z");
  if (xIdx === -1 || yIdx === -1 || zIdx === -1) return result;

  if (dataFormat === "ascii") {
    const dataStart = text.indexOf("\n", headerEnd + 1) + 1;
    const pointLines = text.slice(dataStart).trim().split("\n");
    for (const pl of pointLines) {
      const vals = pl.trim().split(/\s+/).map(Number);
      if (vals.length > Math.max(xIdx, yIdx, zIdx)) {
        result.x.push(vals[xIdx]);
        result.y.push(vals[yIdx]);
        result.z.push(vals[zIdx]);
      }
    }
  } else {
    const dataLineEnd = text.indexOf("\n", headerEnd + 1) + 1;
    const headerBytes = new TextEncoder().encode(text.slice(0, dataLineEnd)).length;
    const binaryData = data.slice(headerBytes);
    const stride = sizes.reduce((a, b) => a + b, 0);

    let fieldOffsets: number[] = [];
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
        result.x.push(px);
        result.y.push(py);
        result.z.push(pz);
      }
    }
  }

  return result;
}
