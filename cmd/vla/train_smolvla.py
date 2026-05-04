"""Fine-tune SmolVLA on a LeRobot v2 dataset produced by convert_to_lerobot.py.

Wraps `lerobot.scripts.train` with sensible defaults for SmolVLA fine-tuning
on a small, single-arm dataset.

Requirements:
  pip install lerobot[smolvla]
  # or, from source:
  # pip install "lerobot @ git+https://github.com/huggingface/lerobot.git"

Usage:
  python train_smolvla.py \
      --dataset-dir lerobot-dataset \
      --output-dir outputs/smolvla \
      --steps 20000

The dataset must be on disk in LeRobot v2 layout (see convert_to_lerobot.py).
LeRobot's training script expects a `repo_id` — for local datasets, it reads
from `LEROBOT_HOME/<repo_id>` by default. We pass `--dataset.root` to point
at our local path so the repo_id is just a label.
"""

import argparse
import os
import subprocess
import sys
from pathlib import Path


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--dataset-dir", required=True,
                    help="local LeRobot v2 dataset dir (output of convert_to_lerobot.py)")
    ap.add_argument("--output-dir", default="outputs/smolvla",
                    help="checkpoint + log output dir")
    ap.add_argument("--policy-path", default="lerobot/smolvla_base",
                    help="HF repo id or local path to base SmolVLA checkpoint")
    ap.add_argument("--repo-id", default="local/pouring-demo",
                    help="logical dataset name (any label, dataset is loaded from --dataset-dir)")
    ap.add_argument("--steps", type=int, default=20000)
    ap.add_argument("--batch-size", type=int, default=8)
    ap.add_argument("--lr", type=float, default=1e-4)
    ap.add_argument("--save-freq", type=int, default=2000)
    ap.add_argument("--num-workers", type=int, default=4)
    ap.add_argument("--device", default="cuda")
    ap.add_argument("--wandb", action="store_true")
    args, passthrough = ap.parse_known_args()

    dataset_dir = Path(args.dataset_dir).resolve()
    if not (dataset_dir / "meta" / "info.json").exists():
        raise SystemExit(f"{dataset_dir}/meta/info.json not found — is this a LeRobot v2 dataset?")

    cmd = [
        sys.executable, "-m", "lerobot.scripts.train",
        f"--policy.path={args.policy_path}",
        f"--dataset.repo_id={args.repo_id}",
        f"--dataset.root={dataset_dir}",
        f"--output_dir={args.output_dir}",
        f"--steps={args.steps}",
        f"--batch_size={args.batch_size}",
        f"--policy.optimizer_lr={args.lr}",
        f"--save_freq={args.save_freq}",
        f"--num_workers={args.num_workers}",
        f"--policy.device={args.device}",
        f"--wandb.enable={'true' if args.wandb else 'false'}",
    ]
    cmd.extend(passthrough)

    print("running:", " ".join(cmd))
    env = os.environ.copy()
    env.setdefault("PYTORCH_CUDA_ALLOC_CONF", "expandable_segments:True")
    sys.exit(subprocess.call(cmd, env=env))


if __name__ == "__main__":
    main()
