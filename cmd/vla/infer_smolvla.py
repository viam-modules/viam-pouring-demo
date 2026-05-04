"""Drive a Viam robot with a fine-tuned SmolVLA policy.

Loop: read images + robot state → policy.select_action(...) → command joint
positions and gripper position → repeat.

Action format must match training (convert_to_lerobot.py):
  [j0, j1, j2, j3, j4, j5, gripper_norm]
  - j0..j5: target joint positions (radians)
  - gripper_norm: target gripper position in [0, 1]; multiplied by 840 for the
    raw xArm gripper position.

Env vars (required):
  VIAM_API_KEY, VIAM_API_KEY_ID

Usage:
  VIAM_API_KEY=... VIAM_API_KEY_ID=... \\
  python infer_smolvla.py \\
      --model-path outputs/smolvla/checkpoints/last/pretrained_model \\
      --host vinopart.abc123.viam.cloud \\
      --arm left-arm \\
      --gripper left-gripper \\
      --camera right-cam \\
      --camera left-cam \\
      --instruction "grab the black block and put it in the blue bowl"
"""

import argparse
import asyncio
import io
import logging
import os
import time
from pathlib import Path

import numpy as np
import torch
from PIL import Image

from viam.components.arm import Arm
from viam.components.camera import Camera
from viam.components.gripper import Gripper
from viam.proto.component.arm import JointPositions
from viam.robot.client import RobotClient

GRIPPER_RAW_MAX = 840.0

os.environ.setdefault("PYTORCH_CUDA_ALLOC_CONF", "expandable_segments:True")
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(message)s",
    datefmt="%H:%M:%S",
)
log = logging.getLogger("smolvla-infer")


def load_policy(model_path: Path, device: str):
    """Import lazily so the file imports cleanly without lerobot installed."""
    from lerobot.common.policies.smolvla.modeling_smolvla import SmolVLAPolicy

    log.info("loading SmolVLA policy from %s", model_path)
    t0 = time.time()
    policy = SmolVLAPolicy.from_pretrained(str(model_path))
    policy.to(device)
    policy.eval()
    log.info("policy ready on %s in %.1fs", device, time.time() - t0)
    return policy


def image_to_tensor(image: Image.Image, device: str) -> torch.Tensor:
    """PIL RGB → (1, 3, H, W) float tensor in [0, 1]."""
    arr = np.asarray(image.convert("RGB"), dtype=np.float32) / 255.0
    t = torch.from_numpy(arr).permute(2, 0, 1).unsqueeze(0)  # (1, 3, H, W)
    return t.to(device)


def build_state(joints: list[float], gripper_pos: int) -> np.ndarray:
    if len(joints) != 6:
        raise RuntimeError(f"expected 6 joints from arm, got {len(joints)}")
    return np.asarray(
        list(joints) + [float(gripper_pos) / GRIPPER_RAW_MAX],
        dtype=np.float32,
    )


async def gripper_position(gripper: Gripper) -> int:
    res = await gripper.do_command({"get": True})
    pos = res.get("pos")
    if pos is None:
        raise RuntimeError(f"gripper get returned no 'pos': {res}")
    return int(pos)


async def run(args):
    device = "cuda" if torch.cuda.is_available() else "cpu"
    if device == "cpu":
        log.warning("running inference on CPU — will be slow")

    policy = load_policy(Path(args.model_path), device)

    api_key = os.environ.get("VIAM_API_KEY")
    api_key_id = os.environ.get("VIAM_API_KEY_ID")
    if not api_key or not api_key_id:
        raise SystemExit("VIAM_API_KEY and VIAM_API_KEY_ID env vars are required")

    log.info("connecting to robot %s", args.host)
    opts = RobotClient.Options.with_api_key(api_key=api_key, api_key_id=api_key_id)
    robot = await RobotClient.at_address(args.host, opts)

    try:
        arm = Arm.from_robot(robot, args.arm)
        gripper = Gripper.from_robot(robot, args.gripper)
        cams = [Camera.from_robot(robot, name) for name in args.camera]
        log.info("connected: arm=%s gripper=%s cameras=%s",
                 args.arm, args.gripper, args.camera)

        steps = 1 if args.dry_run else args.max_steps
        if args.dry_run:
            log.info("DRY RUN: one step, no robot motion")

        for step in range(steps):
            t_step = time.time()

            # --- read observation ---
            t = time.time()
            joints = (await arm.get_joint_positions()).values
            grip = await gripper_position(gripper)
            state = build_state(list(joints), grip)
            log.debug("step %d: state %.2f / pose=%s grip=%d (%.2fs)",
                      step, state[-1], list(state[:6]), grip, time.time() - t)

            t = time.time()
            cam_tensors = {}
            for i, cam in enumerate(cams):
                named, _ = await cam.get_images()
                if not named:
                    raise RuntimeError(f"camera {args.camera[i]} returned no images")
                img = Image.open(io.BytesIO(named[0].data)).convert("RGB")
                cam_tensors[f"observation.images.cam_{i}"] = image_to_tensor(img, device)
            log.debug("step %d: cams in %.2fs", step, time.time() - t)

            observation = {
                "observation.state": torch.from_numpy(state).unsqueeze(0).to(device),
                "task": [args.instruction],
                **cam_tensors,
            }

            # --- predict ---
            t = time.time()
            with torch.inference_mode():
                action_t = policy.select_action(observation)
            action = action_t.squeeze(0).detach().cpu().numpy().astype(np.float32)
            log.info(
                "step %d action: joints=%s grip_norm=%.3f (infer %.2fs)",
                step,
                np.array2string(action[:6], precision=3, suppress_small=True),
                float(action[6]),
                time.time() - t,
            )

            target_joints = action[:6].tolist()
            target_grip = int(round(float(action[6]) * GRIPPER_RAW_MAX))
            target_grip = max(0, min(int(GRIPPER_RAW_MAX), target_grip))

            if args.dry_run:
                log.info("step %d: dry-run, skipping motion (total %.2fs)",
                         step, time.time() - t_step)
                continue

            # --- apply action ---
            t = time.time()
            await asyncio.gather(
                arm.move_to_joint_positions(JointPositions(values=target_joints)),
                gripper.do_command({"set": float(target_grip)}),
            )
            log.info("step %d: moved in %.2fs (total %.2fs)",
                     step, time.time() - t, time.time() - t_step)

            if args.sleep > 0:
                await asyncio.sleep(args.sleep)
    finally:
        await robot.close()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--model-path", required=True,
                    help="dir containing the fine-tuned SmolVLA checkpoint")
    ap.add_argument("--host", required=True, help="Viam robot FQDN")
    ap.add_argument("--arm", default="left-arm")
    ap.add_argument("--gripper", default="left-gripper")
    ap.add_argument("--camera", action="append", required=True,
                    help="camera name; pass multiple times for multi-cam (must match training)")
    ap.add_argument("--instruction", default="grab the cup")
    ap.add_argument("--max-steps", type=int, default=200)
    ap.add_argument("--sleep", type=float, default=0.0,
                    help="seconds to sleep between steps")
    ap.add_argument("--dry-run", action="store_true",
                    help="run one step, log predicted action, do NOT command the robot")
    args = ap.parse_args()
    asyncio.run(run(args))


if __name__ == "__main__":
    main()
