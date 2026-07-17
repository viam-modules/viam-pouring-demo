// plan-doctor re-runs a dumped motion-plan request in-process to recover the IK candidate
// configurations the planner rejected — data that exists at failure time but is never
// serialized into the dump. It writes a new plan file whose "trajectory" is those failed
// candidates, so the motion-tools visualizer can scrub through them against the goal.
//
// Usage: go run ./cmd/plan-doctor [-max-per-type N] [-out FILE] <plan-...-err-....json>
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/motionplan/armplanning"
	"go.viam.com/rdk/referenceframe"
)

var collisionPairRe = regexp.MustCompile(`violation between (\S+) and (\S+) geometries`)

func main() {
	if err := realMain(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func realMain() error {
	maxPerType := flag.Int("max-per-type", 25, "max candidate configurations to keep per failure type")
	out := flag.String("out", "", "output file (default: <input>-ik-failures.json)")
	debug := flag.Bool("debug", false, "enable debug logging")
	flag.Parse()

	if flag.NArg() != 1 {
		return fmt.Errorf("usage: plan-doctor [flags] <plan.json>")
	}
	in := flag.Arg(0)

	logger := logging.NewLogger("plan-doctor")
	if *debug {
		logger.SetLevel(logging.DEBUG)
	} else {
		logger.SetLevel(logging.WARN)
	}

	req, err := armplanning.ReadRequestFromFile(in)
	if err != nil {
		return fmt.Errorf("reading plan request: %w", err)
	}

	rawRequest, err := firstJSONObject(in)
	if err != nil {
		return err
	}

	outFile := *out
	if outFile == "" {
		outFile = strings.TrimSuffix(in, ".json") + "-ik-failures.json"
	}

	fmt.Printf("re-planning %s ...\n", filepath.Base(in))
	plan, _, planErr := armplanning.PlanMotion(context.Background(), logger, req)

	if planErr == nil {
		fmt.Printf("re-plan SUCCEEDED (%d steps) — the original failure did not reproduce.\n", len(plan.Trajectory()))
		fmt.Println("IK sampling is randomized; borderline problems can go either way. Re-run to try again,")
		outFile = strings.TrimSuffix(in, ".json") + "-replan-ok.json"
		if err := req.WriteRequestAndResponseToFile(outFile, plan); err != nil {
			return err
		}
		fmt.Printf("wrote successful plan to %s\n", outFile)
		return nil
	}

	var ikErr *armplanning.IkConstraintError
	if !errors.As(planErr, &ikErr) {
		return fmt.Errorf("re-plan failed with a non-IK-constraint error, nothing to visualize: %w", planErr)
	}

	type failureGroup struct {
		constraint string
		candidates []*referenceframe.LinearInputs
	}
	groups := make([]failureGroup, 0, len(ikErr.FailuresByType))
	for constraint, candidates := range ikErr.FailuresByType {
		groups = append(groups, failureGroup{constraint, candidates})
	}
	sort.Slice(groups, func(i, j int) bool { return len(groups[i].candidates) > len(groups[j].candidates) })

	// Contacts already present at the start state (arm bases on the table, cable
	// runs) are planner-approved; only collisions beyond that baseline get marked.
	baselinePairs := map[string][2]string{}
	if req.StartState != nil && req.StartState.Configuration() != nil {
		if pairs, berr := collidingPairs(req.FrameSystem, req.StartState.Configuration()); berr == nil {
			baselinePairs = pairs
		}
	}

	steps := []doctorStep{}
	fmt.Printf("re-plan failed as expected: %d rejected IK candidates across %d constraint(s)\n", ikErr.Count, len(groups))
	for _, g := range groups {
		kept := min(len(g.candidates), *maxPerType)
		fmt.Printf("  %5.1f%%  (%d, keeping %d)  %s\n",
			100*float64(len(g.candidates))/float64(ikErr.Count), len(g.candidates), kept, g.constraint)

		collidingFrames := []string{}
		if m := collisionPairRe.FindStringSubmatch(g.constraint); m != nil {
			collidingFrames = []string{m[1], m[2]}
		}
		distinct := dedupeCandidates(g.candidates, collidingComponents(collidingFrames))
		fmt.Printf("         %d distinct configuration(s) after dedupe\n", len(distinct))
		for _, candidate := range distinct[:min(len(distinct), kept)] {
			colliding := collidingFrames
			if detected, derr := detectCollisions(req.FrameSystem, candidate, baselinePairs, collidingFrames); derr == nil {
				colliding = detected
			}
			steps = append(steps, doctorStep{inputs: candidate, colliding: colliding, label: g.constraint})
		}
	}

	trajectory := []referenceframe.FrameSystemInputs{}
	stepCollisions := [][]string{}
	for _, s := range steps {
		trajectory = append(trajectory, s.inputs)
		stepCollisions = append(stepCollisions, s.colliding)
	}
	if err := writeViewableFile(outFile, rawRequest, trajectory, stepCollisions); err != nil {
		return err
	}
	fmt.Printf("wrote %d candidates to %s\n", len(trajectory), outFile)

	snapshotsFile := strings.TrimSuffix(in, ".json") + "-snapshots.json"
	if err := writeSnapshotsFile(snapshotsFile, req, steps); err != nil {
		return err
	}
	fmt.Printf("wrote per-candidate snapshots to %s\n", snapshotsFile)
	return nil
}

type doctorStep struct {
	inputs    referenceframe.FrameSystemInputs
	colliding []string
	label     string
}

// collidingComponents extracts component names ("left-arm") from colliding frame
// names ("left-arm:wrist_link"); frames without a component prefix are dropped.
func collidingComponents(collidingFrames []string) []string {
	components := []string{}
	for _, frame := range collidingFrames {
		if name, _, ok := strings.Cut(frame, ":"); ok {
			components = append(components, name)
		}
	}
	return components
}

// dedupeCandidates drops configurations that repeat the pose of the components involved
// in the failure: IK seeds routinely converge to the same solution, joint angles that
// differ by 2π are the same physical pose, and variation in an uninvolved component (the
// other arm) doesn't change what the failure looks like. keyComponents narrows the key
// to the involved components; when empty, every component participates.
func dedupeCandidates(
	candidates []*referenceframe.LinearInputs, keyComponents []string,
) []referenceframe.FrameSystemInputs {
	const twoPi = 2 * math.Pi
	keyed := map[string]bool{}
	for _, name := range keyComponents {
		keyed[name] = true
	}

	seen := map[string]bool{}
	distinct := []referenceframe.FrameSystemInputs{}
	for _, candidate := range candidates {
		fsi := candidate.ToFrameSystemInputs()

		names := make([]string, 0, len(fsi))
		for name := range fsi {
			if len(keyed) == 0 || keyed[name] {
				names = append(names, name)
			}
		}
		sort.Strings(names)

		var key strings.Builder
		for _, name := range names {
			key.WriteString(name)
			for _, v := range fsi[name] {
				wrapped := math.Mod(math.Mod(v, twoPi)+twoPi, twoPi)
				fmt.Fprintf(&key, ":%.2f", wrapped)
			}
		}

		if seen[key.String()] {
			continue
		}
		seen[key.String()] = true
		distinct = append(distinct, fsi)
	}
	return distinct
}

// firstJSONObject returns the request chunk byte-for-byte, so the output file's frame system
// is exactly what the robot dumped rather than a lossy read-then-remarshal round trip.
func firstJSONObject(fileName string) (json.RawMessage, error) {
	f, err := os.Open(filepath.Clean(fileName))
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var raw json.RawMessage
	if err := json.NewDecoder(f).Decode(&raw); err != nil {
		return nil, fmt.Errorf("reading request chunk: %w", err)
	}
	return raw, nil
}

func writeViewableFile(
	fileName string, rawRequest json.RawMessage, trajectory []referenceframe.FrameSystemInputs, stepCollisions [][]string,
) error {
	fakePlan, err := json.Marshal(map[string]any{"trajectory": trajectory, "step_collisions": stepCollisions})
	if err != nil {
		return err
	}

	f, err := os.OpenFile(filepath.Clean(fileName), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	if _, err := f.Write(rawRequest); err != nil {
		return err
	}
	_, err = f.Write(fakePlan)
	return err
}
