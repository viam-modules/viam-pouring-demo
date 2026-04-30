"""Drive a Viam robot with a fine-tuned OpenVLA model.

Loop: image → predict 7-DoF action delta → compute new world-frame gripper pose
→ move via motion service → repeat.

Action format matches train_openvla.py:
  [Δx, Δy, Δz, Δroll, Δpitch, Δyaw, gripper]
  - position delta in same units as training (Viam returns mm)
  - rotation delta as RPY from quaternion delta
  - gripper dim is unused (we never trained it; ignored here)

Action token de-quantization uses action_stats.npz saved next to the checkpoint.

Env vars (required):
  VIAM_API_KEY, VIAM_API_KEY_ID

Usage:
  VIAM_API_KEY=... VIAM_API_KEY_ID=... \
  python infer_openvla.py \
      --model-path openvla-finetuned/epoch_5 \
      --host vinopart.abc123.viam.cloud \
      --camera right-cam \
      --gripper gripper-left \
      --instruction "grab the cup"
"""

import argparse
import asyncio
import io
import logging
import math
import os
import sys
import time
from pathlib import Path

import numpy as np
import torch
from PIL import Image
from transformers import AutoModelForVision2Seq, AutoProcessor

from viam.components.camera import Camera
from viam.components.gripper import Gripper
from viam.proto.common import Pose, PoseInFrame
from viam.robot.client import RobotClient
from viam.rpc.dial import Credentials, DialOptions
from viam.services.motion import MotionClient

# Reuse math helpers from the training script.
sys.path.insert(0, str(Path(__file__).parent))
from train_openvla import (  # noqa: E402
    ACTION_DIM,
    NUM_ACTION_BINS,
    _axis_angle_quat,
    _quat_inv,
    _quat_mul,
    ov_deg_to_quat,
)

os.environ.setdefault("PYTORCH_CUDA_ALLOC_CONF", "expandable_segments:True")
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(message)s",
    datefmt="%H:%M:%S",
)
log = logging.getLogger("vla-infer")


# -----------------------------------------------------------------------------
# Math: quat ↔ Viam orientation-vector form, RPY → quat
# -----------------------------------------------------------------------------

def quat_to_ov_deg(q):
    """Inverse of train_openvla.ov_deg_to_quat. Returns (ox, oy, oz, theta_deg)."""
    w, x, y, z = q
    # +Z axis under R: this is the OV's (ox, oy, oz)
    ox = 2 * (x * z + w * y)
    oy = 2 * (y * z - w * x)
    oz = 1 - 2 * (x * x + y * y)
    n = math.sqrt(ox * ox + oy * oy + oz * oz)
    if n > 1e-9:
        ox, oy, oz = ox / n, oy / n, oz / n
    lat = math.acos(max(-1.0, min(1.0, oz)))
    lon = math.atan2(oy, ox) if (1 - abs(oz)) > 1e-9 else 0.0
    qz1 = _axis_angle_quat([0, 0, 1], lon)
    qy = _axis_angle_quat([0, 1, 0], lat)
    q_residual = _quat_mul(_quat_inv(_quat_mul(qz1, qy)), q)
    theta = 2 * math.atan2(q_residual[3], q_residual[0])
    return ox, oy, oz, math.degrees(theta)


def rpy_to_quat(roll, pitch, yaw):
    """Tait-Bryan ZYX (yaw-pitch-roll), returns (w, x, y, z)."""
    cr, sr = math.cos(roll / 2), math.sin(roll / 2)
    cp, sp = math.cos(pitch / 2), math.sin(pitch / 2)
    cy, sy = math.cos(yaw / 2), math.sin(yaw / 2)
    return np.array([
        cr * cp * cy + sr * sp * sy,
        sr * cp * cy - cr * sp * sy,
        cr * sp * cy + sr * cp * sy,
        cr * cp * sy - sr * sp * cy,
    ])


# -----------------------------------------------------------------------------
# Model
# -----------------------------------------------------------------------------

def load_model(model_path: Path, device: str):
    log.info("loading processor + model from %s", model_path)
    t0 = time.time()
    processor = AutoProcessor.from_pretrained(str(model_path), trust_remote_code=True)
    model = AutoModelForVision2Seq.from_pretrained(
        str(model_path),
        trust_remote_code=True,
        torch_dtype=torch.bfloat16,
        low_cpu_mem_usage=True,
    )
    model.to(device)
    model.eval()
    log.info("model ready on %s in %.1fs", device, time.time() - t0)

    # action_stats.npz lives next to the model dir (training's --output-dir),
    # not inside each epoch_N/ checkpoint.
    stats_candidates = [
        model_path / "action_stats.npz",
        model_path.parent / "action_stats.npz",
    ]
    stats_path = next((p for p in stats_candidates if p.exists()), None)
    if stats_path is None:
        raise SystemExit(
            f"action_stats.npz not found near {model_path} (looked in {stats_candidates})"
        )
    log.info("loading action stats from %s", stats_path)
    stats = np.load(stats_path)
    q01 = stats["q01"].astype(np.float32)
    q99 = stats["q99"].astype(np.float32)
    log.info("action q01: %s", q01)
    log.info("action q99: %s", q99)
    return processor, model, q01, q99


def predict_action(model, processor, image, instruction, q01, q99, device):
    prompt = f"In: What action should the robot take to {instruction.lower()}?\nOut:"
    inputs = processor(prompt, image)
    pixel_values = inputs["pixel_values"].to(device)
    try:
        vision_dtype = next(model.vision_backbone.parameters()).dtype
    except (AttributeError, StopIteration):
        vision_dtype = torch.bfloat16
    pixel_values = pixel_values.to(vision_dtype)
    input_ids = inputs["input_ids"].to(device)

    with torch.inference_mode():
        out = model.generate(
            input_ids=input_ids,
            pixel_values=pixel_values,
            max_new_tokens=ACTION_DIM,
            do_sample=False,
        )
    action_ids = out[0, -ACTION_DIM:].detach().cpu().numpy()
    offset = processor.tokenizer.vocab_size - NUM_ACTION_BINS
    bins = np.clip(action_ids - offset, 0, NUM_ACTION_BINS - 1)
    norm = -1.0 + 2.0 * (bins.astype(np.float32) + 0.5) / NUM_ACTION_BINS
    return q01 + (norm + 1.0) / 2.0 * (q99 - q01)


# -----------------------------------------------------------------------------
# Robot loop
# -----------------------------------------------------------------------------

async def run(args):
    device = "cuda" if torch.cuda.is_available() else "cpu"
    if device == "cpu":
        log.warning("running inference on CPU — will be very slow")

    model_path = Path(args.model_path)
    processor, model, q01, q99 = load_model(model_path, device)

    api_key = os.environ.get("VIAM_API_KEY")
    api_key_id = os.environ.get("VIAM_API_KEY_ID")
    if not api_key or not api_key_id:
        raise SystemExit("VIAM_API_KEY and VIAM_API_KEY_ID env vars are required")

    log.info("connecting to robot %s", args.host)
    opts = RobotClient.Options.with_api_key(api_key=api_key, api_key_id=api_key_id)
    robot = await RobotClient.at_address(args.host, opts)

    try:
        cam = Camera.from_robot(robot, args.camera)
        motion = MotionClient.from_robot(robot, args.motion)
        # motion.get_pose looks up the frame by name in the frame system.
        # Frames are keyed by bare component name, not the canonical
        # "rdk:component:gripper/..." form. Use the bare name here, but pass
        # the full resource-name proto string to motion.move (which uses it
        # as a resource lookup, not a frame lookup).
        gripper_frame = args.gripper
        gripper_resource = str(Gripper.get_resource_name(args.gripper))

        steps = 1 if args.dry_run else args.max_steps
        if args.dry_run:
            log.info("DRY RUN: one step, no motion.move call")
        for step in range(steps):
            t_step = time.time()

            t = time.time()
            named_images, _ = await cam.get_images()
            if not named_images:
                raise RuntimeError(f"camera {args.camera} returned no images")
            ni = named_images[0]
            image = Image.open(io.BytesIO(ni.data)).convert("RGB")
            log.debug("step %d: image fetched in %.2fs (mime=%s)",
                      step, time.time() - t, getattr(ni, "mime_type", "?"))

            t = time.time()
            pif = await motion.get_pose(
                component_name=gripper_frame, destination_frame="world"
            )
            cur = pif.pose
            log.debug(
                "step %d: cur pose (%.1f, %.1f, %.1f / ov %.3f %.3f %.3f / θ %.1f) in %.2fs",
                step, cur.x, cur.y, cur.z, cur.o_x, cur.o_y, cur.o_z, cur.theta,
                time.time() - t,
            )

            t = time.time()
            action = predict_action(
                model, processor, image, args.instruction, q01, q99, device
            )
            log.info(
                "step %d action: dxyz=[%.2f %.2f %.2f] drpy=[%.3f %.3f %.3f] g=%.2f (infer %.2fs)",
                step, action[0], action[1], action[2],
                action[3], action[4], action[5], action[6],
                time.time() - t,
            )

            cur_q = ov_deg_to_quat(
                [cur.x, cur.y, cur.z, cur.o_x, cur.o_y, cur.o_z, cur.theta]
            )
            dq = rpy_to_quat(action[3], action[4], action[5])
            new_q = _quat_mul(dq, cur_q)
            new_ov = quat_to_ov_deg(new_q)

            target = Pose(
                x=cur.x + float(action[0]),
                y=cur.y + float(action[1]),
                z=cur.z + float(action[2]),
                o_x=new_ov[0], o_y=new_ov[1], o_z=new_ov[2], theta=new_ov[3],
            )
            log.info(
                "step %d target: (%.1f, %.1f, %.1f / ov %.3f %.3f %.3f / θ %.1f)",
                step, target.x, target.y, target.z,
                target.o_x, target.o_y, target.o_z, target.theta,
            )

            if args.dry_run:
                log.info("step %d: dry-run, skipping motion.move (total step %.2fs)",
                         step, time.time() - t_step)
            else:
                t = time.time()
                await motion.move(
                    component_name=gripper_resource,
                    destination=PoseInFrame(reference_frame="world", pose=target),
                )
                log.info("step %d: moved in %.2fs (total step %.2fs)",
                         step, time.time() - t, time.time() - t_step)

            if args.sleep > 0:
                await asyncio.sleep(args.sleep)
    finally:
        await robot.close()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--model-path", default="openvla-finetuned/epoch_5",
                    help="dir containing the trained checkpoint + action_stats.npz")
    ap.add_argument("--host", required=True, help="Viam robot FQDN")
    ap.add_argument("--camera", default="right-cam")
    ap.add_argument("--gripper", default="left-gripper")
    ap.add_argument("--motion", default="builtin",
                    help="motion service resource name")
    ap.add_argument("--instruction", default="grab the cup")
    ap.add_argument("--max-steps", type=int, default=200)
    ap.add_argument("--sleep", type=float, default=0.0,
                    help="seconds to sleep between steps")
    ap.add_argument("--dry-run", action="store_true",
                    help="run one step, log predicted action and target pose, "
                         "do NOT call motion.move")
    args = ap.parse_args()
    asyncio.run(run(args))


if __name__ == "__main__":
    main()
