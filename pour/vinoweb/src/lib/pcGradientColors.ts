/**
 * Point cloud vertex gradient (PointCloud3D.refreshPointCloudOnly).
 * t=0 = base of cup along height; t=1 = top.
 * Valid ramp: Carbon green palette (same family as `Tag type="green"` in white theme:
 * green-60 → tag fill green).
 */
export const PC_VALID_T0 = { r: 25, g: 128, b: 56 } as const; // #198038 green-60
export const PC_VALID_T1 = { r: 167, g: 240, b: 186 } as const; // #a7f0ba tag background

export const PC_INVALID_T0 = { r: 255, g: 218, b: 3 } as const;
export const PC_INVALID_T1 = { r: 254, g: 127, b: 133 } as const;

/** Valid pill: Carbon green tag fill (`bx--tag--green` in white.css). */
export function pcValidPillRgba(alpha = 0.92) {
  const { r, g, b } = PC_VALID_T1;
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}

export function pcInvalidPillRgba(alpha = 0.92) {
  const { r, g, b } = PC_INVALID_T1;
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}
