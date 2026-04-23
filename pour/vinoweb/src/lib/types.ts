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
  rawPCD?: Uint8Array;
  dims?: { x: number; y: number; z: number };
  position?: { x: number; y: number; z: number };
  valid?: boolean;
}

/** Matches `getCupDetails` / AnalyzeObject fields from the cart service */
export interface CupDetectionMetrics {
  valid: boolean;
  expectedHeight: number;
  observedHeight: number;
  heightDelta: number;
  heightPass: boolean;
  expectedWidth: number;
  observedWidth: number;
  widthDelta: number;
  widthPass: boolean;
  toleranceMm: number;
}
