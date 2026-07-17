package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/golang/geo/r3"
	"github.com/viam-labs/motion-tools/draw"
	"go.viam.com/rdk/motionplan/armplanning"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/spatialmath"
)

// snapshotStep is one entry in the -snapshots.json file consumed by vinoweb's
// planner debug stepper: a self-contained draw snapshot plus display metadata.
type snapshotStep struct {
	Label     string          `json:"label"`
	Colliding []string        `json:"colliding"`
	Snapshot  json.RawMessage `json:"snapshot"`
}

// writeSnapshotsFile renders each candidate as a draw.Snapshot the published
// motion-tools <Snapshot/> component can display directly: colliding frames red,
// obstacles uninvolved in any failure removed, goal poses drawn as axes helpers.
func writeSnapshotsFile(fileName string, req *armplanning.PlanRequest, steps []doctorStep) error {
	keep := map[string]bool{}
	for _, s := range steps {
		for _, frame := range s.colliding {
			if base, _, ok := strings.Cut(frame, ":"); ok {
				keep[base] = true
			}
			keep[frame] = true
		}
	}
	hideNonCollidingObstacles(req.FrameSystem, keep)

	out := []snapshotStep{}
	for i, s := range steps {
		snap := draw.NewSnapshot()

		if _, err := snap.DrawFrameSystemGeometries(draw.DrawFrameSystemGeometriesOptions{
			ID:          fmt.Sprintf("candidate-%d", i+1),
			FrameSystem: req.FrameSystem,
			Inputs:      s.inputs,
			Colors:      map[string]draw.Color{"world": draw.ColorFromHex("#4A90D9")},
		}); err != nil {
			return fmt.Errorf("drawing candidate %d: %w", i+1, err)
		}

		for _, goal := range req.Goals {
			for frameName, poseInFrame := range goal.Poses() {
				if _, err := snap.DrawFrame(draw.DrawFrameOptions{
					Name:   "goal:" + frameName,
					Parent: poseInFrame.Parent(),
					Pose:   poseInFrame.Pose(),
				}); err != nil {
					return fmt.Errorf("drawing goal %s: %w", frameName, err)
				}
			}
		}

		raw, err := snap.MarshalJSON()
		if err != nil {
			return err
		}
		raw, err = paintGeometries(raw, s.colliding)
		if err != nil {
			return err
		}
		raw, err = stripSceneCamera(raw)
		if err != nil {
			return err
		}
		out = append(out, snapshotStep{Label: s.label, Colliding: s.colliding, Snapshot: raw})
	}

	// The camera lives at file level (viewer meters, applied once per file load by
	// the stepper) rather than in the snapshots: the viewer re-applies any camera a
	// snapshot carries, which would reset the view every time a step is revisited.
	goal := goalPoint(req)
	data, err := json.Marshal(map[string]any{
		"steps": out,
		"camera": map[string]any{
			"position": []float64{(goal.X + 1600) / 1000, (goal.Y - 1600) / 1000, (goal.Z + 900) / 1000},
			"lookAt":   []float64{goal.X / 1000, goal.Y / 1000, goal.Z / 1000},
		},
	})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Clean(fileName), data, 0o600)
}

// collidingPairs enumerates robot-vs-obstacle geometry pairs intersecting at the
// given configuration, keyed "labelA|labelB" (sorted). Same-component and
// robot-vs-robot pairs are skipped: they include permanently touching neighbors
// (gripper on wrist) the planner permits via an allowed-collision list we don't have.
func collidingPairs(
	fs *referenceframe.FrameSystem, inputs referenceframe.FrameSystemInputs,
) (map[string][2]string, error) {
	frameMap, err := referenceframe.FrameSystemGeometries(fs, inputs)
	if err != nil {
		return nil, err
	}

	type worldGeometry struct {
		component string
		geometry  spatialmath.Geometry
	}
	all := []worldGeometry{}
	for component, geometriesInFrame := range frameMap {
		for _, g := range geometriesInFrame.Geometries() {
			all = append(all, worldGeometry{component, g})
		}
	}

	pairs := map[string][2]string{}
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			a, b := all[i], all[j]
			if strings.HasPrefix(a.component, "obstacle") == strings.HasPrefix(b.component, "obstacle") {
				continue
			}
			hit, _, err := a.geometry.CollidesWith(b.geometry, 0)
			if err != nil || !hit {
				continue
			}
			la, lb := a.geometry.Label(), b.geometry.Label()
			if lb < la {
				la, lb = lb, la
			}
			pairs[la+"|"+lb] = [2]string{la, lb}
		}
	}
	return pairs, nil
}

// detectCollisions returns the geometries newly in collision at `inputs` relative to
// the start configuration, plus whatever the planner's constraint string named (which
// covers robot-vs-robot reports). Baseline subtraction removes resting contacts —
// arm bases on the table, cable runs — that exist in every configuration and would
// otherwise highlight as noise; the constraint error alone only names the most
// frequent pair, so links like a forearm inside the table would go unmarked.
func detectCollisions(
	fs *referenceframe.FrameSystem,
	inputs referenceframe.FrameSystemInputs,
	baseline map[string][2]string,
	constraintPair []string,
) ([]string, error) {
	pairs, err := collidingPairs(fs, inputs)
	if err != nil {
		return nil, err
	}

	colliding := map[string]bool{}
	for _, frame := range constraintPair {
		colliding[frame] = true
	}
	for key, pair := range pairs {
		if baseline[key] != [2]string{} {
			continue
		}
		colliding[pair[0]] = true
		colliding[pair[1]] = true
	}

	names := make([]string, 0, len(colliding))
	for name := range colliding {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// componentBaseColor mirrors the viewer's live-machine resource colors
// (visualization src/lib/color.ts resourceColors, tailwind *-600).
func componentBaseColor(component string) [3]byte {
	switch {
	case strings.Contains(component, "gripper"):
		return [3]byte{0x08, 0x91, 0xB2} // cyan-600
	case strings.Contains(component, "arm"):
		return [3]byte{0xD9, 0x77, 0x06} // amber-600
	default:
		return [3]byte{0x4B, 0x55, 0x63} // gray-600, the viewer default
	}
}

// paintGeometries assigns each geometry its viewer-default component color and gives
// anything in collision a half-transparent red fill instead. Applied on the marshaled
// JSON because the draw package can only color per component, and geometry labels
// ("left-arm:wrist_link") match collision names exactly.
func paintGeometries(raw json.RawMessage, colliding []string) (json.RawMessage, error) {
	set := map[string]bool{}
	for _, frame := range colliding {
		set[frame] = true
	}
	red := base64.StdEncoding.EncodeToString([]byte{0xD4, 0x00, 0x00})
	redAlpha := base64.StdEncoding.EncodeToString([]byte{0x54})
	restingAlpha := base64.StdEncoding.EncodeToString([]byte{0x8C})

	var snap map[string]any
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, err
	}
	transforms, _ := snap["transforms"].([]any)
	for _, t := range transforms {
		tm, _ := t.(map[string]any)
		physical, _ := tm["physicalObject"].(map[string]any)
		label, _ := physical["label"].(string)
		meta, _ := tm["metadata"].(map[string]any)
		if meta == nil || label == "" {
			continue
		}
		if set[label] {
			meta["colors"] = red
			meta["opacities"] = redAlpha
			continue
		}
		component, _, _ := strings.Cut(label, ":")
		color := componentBaseColor(component)
		meta["colors"] = base64.StdEncoding.EncodeToString(color[:])
		meta["opacities"] = restingAlpha
	}
	return json.Marshal(snap)
}

// goalPoint returns the position of the first goal pose, the natural focus for a
// failed-IK scene; zero vector when the request carries no goal poses.
func goalPoint(req *armplanning.PlanRequest) r3.Vector {
	for _, goal := range req.Goals {
		for _, poseInFrame := range goal.Poses() {
			return poseInFrame.Pose().Point()
		}
	}
	return r3.Vector{}
}

// stripSceneCamera removes the camera the draw package stamps into every snapshot:
// the viewer re-applies any camera a snapshot carries, which would reset the view
// whenever a step is shown — including revisits. The stepper applies the file-level
// camera once instead.
func stripSceneCamera(raw json.RawMessage) (json.RawMessage, error) {
	var snap map[string]any
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, err
	}
	meta, _ := snap["sceneMetadata"].(map[string]any)
	if meta == nil {
		return raw, nil
	}
	delete(meta, "sceneCamera")
	return json.Marshal(snap)
}

// hideNonCollidingObstacles removes obstacle frames (demo convention: named
// "obstacle...") whose component is not involved in any collision, so the
// rendered scene only shows the geometry that matters to the failure.
func hideNonCollidingObstacles(fs *referenceframe.FrameSystem, keep map[string]bool) {
	for _, name := range fs.FrameNames() {
		base, _, _ := strings.Cut(name, ":")
		base = strings.TrimSuffix(base, "_origin")
		if !strings.HasPrefix(base, "obstacle") || keep[base] {
			continue
		}
		if f := fs.Frame(name); f != nil {
			fs.RemoveFrame(f)
		}
	}
}
