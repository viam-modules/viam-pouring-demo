export interface Joint {
  index: number;
  position: number;
}

export interface SegmentedObject {
  index: number;
  totalPoints: number;
  points_x: number[];
  points_y: number[];
  points_z: number[];
  rawPCD: Uint8Array;
  dims?: { x: number; y: number; z: number };
  position?: { x: number; y: number; z: number };
}

export const CUP_HEIGHT = 109;

export const RINGS = [
  { label: "Base",  circumference: 200, heightFromFloor: 0,          color: "rgba(232,160,245,0.6)" },
  { label: "Belly", circumference: 270, heightFromFloor: 45,         color: "rgba(213,128,232,0.7)" },
  { label: "Rim",   circumference: 220, heightFromFloor: CUP_HEIGHT, color: "rgba(240,192,255,0.6)" },
].map(r => ({ ...r, radius: r.circumference / (2 * Math.PI) }));
