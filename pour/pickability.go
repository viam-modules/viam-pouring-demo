package pour

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/golang/geo/r3"

	"go.viam.com/rdk/motionplan"
	"go.viam.com/rdk/motionplan/armplanning"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/spatialmath"

	"github.com/erh/vmodutils/touch"
)

// CupApproachChoices is the fan of approach orientations that Touch() tries
// when reaching for a detected cup. Kept here as a package-level variable so
// the heatmap script and the live Touch() flow stay in lockstep — if you add
// or reorder orientations, both code paths pick up the change automatically.
var CupApproachChoices = []*spatialmath.OrientationVectorDegrees{
	{OX: 1, Theta: 180},
	{OY: 1, Theta: 180},
	{OX: .5, OY: 1, Theta: 180},
	{OX: 1, OY: 1, Theta: 180},
	{OX: 1, OY: -1, Theta: 180},
	{OY: -1, Theta: 180},
	{OX: -.5, OY: -1, Theta: 180},
}

// PickPlanAttempt records the planner-only result for a single approach
// orientation. The Plan fields are kept for callers that want to inspect
// trajectories; MarshalJSON intentionally drops them to keep sidecar JSON
// compact.
type PickPlanAttempt struct {
	Orientation *spatialmath.OrientationVectorDegrees
	Approach    motionplan.Plan
	ApproachErr error
	Pickup      motionplan.Plan
	PickupErr   error
}

// Succeeded reports whether the gripper can both approach and linearly pick
// up the cup with this orientation.
func (a PickPlanAttempt) Succeeded() bool {
	return a.ApproachErr == nil && a.PickupErr == nil
}

// MarshalJSON emits a compact form suitable for the heatmap sidecar.
func (a PickPlanAttempt) MarshalJSON() ([]byte, error) {
	out := struct {
		Orientation *spatialmath.OrientationVectorDegrees `json:"orientation"`
		ApproachErr *string                               `json:"approach_err"`
		PickupErr   *string                               `json:"pickup_err"`
	}{Orientation: a.Orientation}
	if a.ApproachErr != nil {
		s := a.ApproachErr.Error()
		out.ApproachErr = &s
	}
	if a.PickupErr != nil {
		s := a.PickupErr.Error()
		out.PickupErr = &s
	}
	return json.Marshal(out)
}

// PickPlanResult is the aggregate "can the cup arm pick a cup standing here?"
// answer for a single grid cell.
type PickPlanResult struct {
	CupCenter r3.Vector
	Pickable  bool
	Attempts  []PickPlanAttempt
}

// BuildPickFrameSystem fetches the live frame system config and assembles a
// FrameSystem suitable for repeated PlanPickAt calls. The frame system is the
// expensive, network-bound piece of pickability planning, so callers that run
// many planning attempts (e.g. the cup-heatmap sweep) should call this once at
// startup and reuse the result rather than refetching per cell — a dropped
// connection mid-sweep otherwise turns every remaining cell into an error.
func (vc *VinoCart) BuildPickFrameSystem(ctx context.Context) (*referenceframe.FrameSystem, error) {
	fsCfg, err := vc.c.Rfs.FrameSystemConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch frame system config: %w", err)
	}
	fs, err := referenceframe.NewFrameSystem("cup-pickability", fsCfg.Parts, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build frame system: %w", err)
	}
	return fs, nil
}

// PlanPickAt mirrors the approach + pickup sequence in Touch() but using
// armplanning.PlanMotion (no arm movement). Returns Pickable=true iff at
// least one orientation in CupApproachChoices plans both the approach and
// the linear pickup successfully, matching Touch()'s "first that works"
// behaviour.
//
// fs must be obtained from BuildPickFrameSystem (or be otherwise equivalent
// to the live frame system); PlanPickAt deliberately does not fetch it so
// the per-cell hot path makes no RPC calls. startInputs must contain joint
// configurations for every actuated arm in the frame system.
func (vc *VinoCart) PlanPickAt(
	ctx context.Context,
	fs *referenceframe.FrameSystem,
	cupCenter r3.Vector,
	startInputs referenceframe.FrameSystemInputs,
) (*PickPlanResult, error) {
	if fs == nil {
		return nil, fmt.Errorf("PlanPickAt: fs is nil (call BuildPickFrameSystem first)")
	}
	cupGeom, err := spatialmath.NewBox(
		spatialmath.NewPoseFromPoint(cupCenter),
		r3.Vector{X: vc.conf.cupWidth(), Y: vc.conf.cupWidth(), Z: vc.conf.CupHeight},
		"cup",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build synthetic cup geometry: %w", err)
	}
	worldState, err := referenceframe.NewWorldState(
		[]*referenceframe.GeometriesInFrame{
			referenceframe.NewGeometriesInFrame("world", []spatialmath.Geometry{cupGeom}),
		},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build world state: %w", err)
	}

	result := &PickPlanResult{
		CupCenter: cupCenter,
		Attempts:  make([]PickPlanAttempt, 0, len(CupApproachChoices)),
	}

	for _, o := range CupApproachChoices {
		attempt := PickPlanAttempt{Orientation: o}

		approachReq := &armplanning.PlanRequest{
			FrameSystem: fs,
			Goals: []*armplanning.PlanState{
				armplanning.NewPlanState(referenceframe.FrameSystemPoses{
					vc.conf.GripperName: vc.approachPoseForCup(cupCenter, 100, o),
				}, nil),
			},
			StartState: armplanning.NewPlanState(nil, startInputs),
			WorldState: worldState,
		}
		attempt.Approach, _, attempt.ApproachErr = armplanning.PlanMotion(ctx, vc.logger, approachReq)

		if attempt.ApproachErr == nil {
			// Pickup mirrors Touch() / moveWithLinearConstraint: the cup is intentionally
			// dropped from the world state so the gripper can descend into the cup volume.
			pickupReq := &armplanning.PlanRequest{
				FrameSystem: fs,
				Goals: []*armplanning.PlanState{
					armplanning.NewPlanState(referenceframe.FrameSystemPoses{
						vc.conf.GripperName: vc.approachPoseForCup(cupCenter, gripperToCupCenterHack, o),
					}, nil),
				},
				StartState:  armplanning.NewPlanState(nil, lastInputsFromPlan(attempt.Approach, startInputs)),
				Constraints: &LinearConstraint,
			}
			attempt.Pickup, _, attempt.PickupErr = armplanning.PlanMotion(ctx, vc.logger, pickupReq)
		}

		result.Attempts = append(result.Attempts, attempt)

		if attempt.Succeeded() {
			result.Pickable = true
			break
		}
	}

	return result, nil
}

// approachPoseForCup mirrors VinoCart.getApproachPoint but takes a
// pre-computed cup center vector (we don't have a viz.Object in planner-only
// mode).
func (vc *VinoCart) approachPoseForCup(
	cupCenter r3.Vector,
	deltaLinear float64,
	o *spatialmath.OrientationVectorDegrees,
) *referenceframe.PoseInFrame {
	p := touch.GetApproachPoint(cupCenter, deltaLinear, o)
	p.Z = vc.conf.CupHeight - vc.conf.cupGripHeightOffset()
	return referenceframe.NewPoseInFrame("world", spatialmath.NewPose(p, o))
}

// lastInputsFromPlan returns the configuration at the end of a plan merged
// with start, so frames that the plan didn't touch keep their original values.
func lastInputsFromPlan(
	plan motionplan.Plan,
	start referenceframe.FrameSystemInputs,
) referenceframe.FrameSystemInputs {
	out := referenceframe.FrameSystemInputs{}
	for k, v := range start {
		out[k] = v
	}
	if plan == nil {
		return out
	}
	traj := plan.Trajectory()
	if len(traj) == 0 {
		return out
	}
	last := traj[len(traj)-1]
	for k, v := range last {
		out[k] = v
	}
	return out
}
