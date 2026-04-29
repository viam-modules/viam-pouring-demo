"""LoRA fine-tune OpenVLA-7B on capture-direct exports.

Input layout (produced by `vla capture-direct`):
  <data-root>/<session_id>/
    steps.jsonl                # one JSON per line
    images/step_NNNNNN.jpg

Each step:
  {
    "step_index": 0,
    "timestamp": "...",
    "is_first": bool, "is_last": bool, "is_terminal": bool,
    "image": "images/step_NNNNNN.jpg",
    "ee_pose": [x, y, z, ox, oy, oz, theta_deg],   # Viam orientation-vector form
    "language_instruction": "..."
  }

Action used for training: 7-DoF =
  (Δx, Δy, Δz, Δroll, Δpitch, Δyaw, gripper)
  - position delta in world frame, units as captured (mm by Viam default);
    q01/q99 normalization is scale-invariant so this is fine.
  - rotation delta as RPY from quaternion delta q1 * q0^-1.
  - gripper dim is constant 0 (we don't currently capture gripper state).
  - terminal step uses zero action.

Tokenization follows OpenVLA convention:
  q01/q99 → [-1, 1] → 256 bins → last 256 tokens of LLaMA-2 vocab.

Inference de-normalization needs the saved `action_stats.npz`.

Requirements:
  pip install torch transformers peft bitsandbytes accelerate \
              pillow tqdm timm "tokenizers>=0.15"

Usage:
  python train_openvla.py \
      --data-root openvla-export \
      --output-dir openvla-finetuned \
      --epochs 5 --batch-size 4 --load-4bit
"""

import argparse
import json
import math
from pathlib import Path

import numpy as np
import torch
from PIL import Image
from peft import LoraConfig, get_peft_model, prepare_model_for_kbit_training
from torch.utils.data import DataLoader, Dataset
from tqdm import tqdm
from transformers import (
    AutoModelForVision2Seq,
    AutoProcessor,
    get_linear_schedule_with_warmup,
)

NUM_ACTION_BINS = 256
ACTION_DIM = 7


# -----------------------------------------------------------------------------
# Viam orientation vector → quaternion (port of rdk spatialmath.OrientationVector)
# Convention: rotate by Z(lon), then Y(lat), then Z(theta), Hamilton (w, x, y, z).
# -----------------------------------------------------------------------------

def _axis_angle_quat(axis, angle):
    c = math.cos(angle / 2.0)
    s = math.sin(angle / 2.0)
    return np.array([c, axis[0] * s, axis[1] * s, axis[2] * s], dtype=np.float64)


def _quat_mul(a, b):
    w1, x1, y1, z1 = a
    w2, x2, y2, z2 = b
    return np.array([
        w1*w2 - x1*x2 - y1*y2 - z1*z2,
        w1*x2 + x1*w2 + y1*z2 - z1*y2,
        w1*y2 - x1*z2 + y1*w2 + z1*x2,
        w1*z2 + x1*y2 - y1*x2 + z1*w2,
    ], dtype=np.float64)


def _quat_inv(q):
    w, x, y, z = q
    return np.array([w, -x, -y, -z], dtype=np.float64)


def _quat_to_rpy(q):
    w, x, y, z = q
    sinr_cosp = 2 * (w * x + y * z)
    cosr_cosp = 1 - 2 * (x * x + y * y)
    roll = math.atan2(sinr_cosp, cosr_cosp)
    sinp = max(-1.0, min(1.0, 2 * (w * y - z * x)))
    pitch = math.asin(sinp)
    siny_cosp = 2 * (w * z + x * y)
    cosy_cosp = 1 - 2 * (y * y + z * z)
    yaw = math.atan2(siny_cosp, cosy_cosp)
    return np.array([roll, pitch, yaw], dtype=np.float64)


def ov_deg_to_quat(ee_pose):
    ox, oy, oz, theta_deg = ee_pose[3], ee_pose[4], ee_pose[5], ee_pose[6]
    n = math.sqrt(ox * ox + oy * oy + oz * oz)
    if n == 0.0:
        ox, oy, oz = 0.0, 0.0, 1.0
    else:
        ox, oy, oz = ox / n, oy / n, oz / n
    theta = math.radians(theta_deg)
    lat = math.acos(max(-1.0, min(1.0, oz)))
    lon = math.atan2(oy, ox) if (1 - abs(oz)) > 1e-9 else 0.0
    qz1 = _axis_angle_quat([0, 0, 1], lon)
    qy = _axis_angle_quat([0, 1, 0], lat)
    qz2 = _axis_angle_quat([0, 0, 1], theta)
    return _quat_mul(_quat_mul(qz1, qy), qz2)


def compute_action(ee_t, ee_t1):
    p0 = np.asarray(ee_t[:3], dtype=np.float64)
    p1 = np.asarray(ee_t1[:3], dtype=np.float64)
    q0 = ov_deg_to_quat(ee_t)
    q1 = ov_deg_to_quat(ee_t1)
    dpos = p1 - p0
    dq = _quat_mul(q1, _quat_inv(q0))
    drpy = _quat_to_rpy(dq)
    return np.concatenate([dpos, drpy, [0.0]]).astype(np.float32)


# -----------------------------------------------------------------------------
# Dataset
# -----------------------------------------------------------------------------

def load_episodes(data_root: Path):
    episodes = []
    for sess in sorted(p for p in data_root.iterdir() if p.is_dir()):
        jl = sess / "steps.jsonl"
        if not jl.exists():
            continue
        steps = [json.loads(line) for line in jl.read_text().splitlines() if line.strip()]
        if len(steps) < 2:
            continue
        episodes.append((sess, steps))
    return episodes


def build_steps(episodes):
    out = []
    for sess, steps in episodes:
        for i, s in enumerate(steps):
            if not s.get("ee_pose"):
                continue
            if i + 1 < len(steps) and steps[i + 1].get("ee_pose"):
                action = compute_action(s["ee_pose"], steps[i + 1]["ee_pose"])
            else:
                action = np.zeros(ACTION_DIM, dtype=np.float32)
            out.append({
                "image_path": str(sess / s["image"]),
                "instruction": s.get("language_instruction", ""),
                "action": action,
            })
    return out


def compute_action_stats(steps):
    actions = np.stack([s["action"] for s in steps])
    q01 = np.quantile(actions, 0.01, axis=0).astype(np.float32)
    q99 = np.quantile(actions, 0.99, axis=0).astype(np.float32)
    # Avoid degenerate dims (all-zero, e.g. gripper) collapsing to NaN later.
    span = q99 - q01
    q99 = np.where(span < 1e-6, q01 + 1.0, q99)
    return q01, q99


def discretize(action, q01, q99):
    norm = 2.0 * (action - q01) / (q99 - q01) - 1.0
    norm = np.clip(norm, -1.0, 1.0)
    edges = np.linspace(-1.0, 1.0, NUM_ACTION_BINS + 1)[:-1]
    return (np.digitize(norm, edges) - 1).astype(np.int64)  # 0..255


class OpenVLADataset(Dataset):
    def __init__(self, steps, q01, q99, processor):
        self.steps = steps
        self.q01 = q01
        self.q99 = q99
        self.processor = processor
        tok = processor.tokenizer
        # OpenVLA's pretrained tokenizer reserves the last 256 IDs as action tokens.
        self.action_token_offset = tok.vocab_size - NUM_ACTION_BINS
        self.eos_id = tok.eos_token_id

    def __len__(self):
        return len(self.steps)

    def __getitem__(self, i):
        s = self.steps[i]
        image = Image.open(s["image_path"]).convert("RGB")
        prompt = f"In: What action should the robot take to {s['instruction'].lower()}?\nOut:"

        bins = discretize(s["action"], self.q01, self.q99)
        action_token_ids = bins + self.action_token_offset

        inputs = self.processor(prompt, image)
        prompt_ids = inputs["input_ids"][0]
        action_ids = torch.tensor(action_token_ids, dtype=torch.long)
        eos = torch.tensor([self.eos_id], dtype=torch.long)

        input_ids = torch.cat([prompt_ids, action_ids, eos])
        labels = input_ids.clone()
        labels[: prompt_ids.numel()] = -100  # mask prompt; train only on action+EOS
        attention_mask = torch.ones_like(input_ids)

        return {
            "pixel_values": inputs["pixel_values"][0],
            "input_ids": input_ids,
            "attention_mask": attention_mask,
            "labels": labels,
        }


def collate(batch):
    max_len = max(b["input_ids"].numel() for b in batch)
    out = {"pixel_values": torch.stack([b["pixel_values"] for b in batch])}
    for k in ("input_ids", "attention_mask", "labels"):
        padded = []
        for b in batch:
            x = b[k]
            pad_len = max_len - x.numel()
            if pad_len == 0:
                padded.append(x)
                continue
            pad_val = -100 if k == "labels" else (0 if k == "attention_mask" else 0)
            padded.append(torch.cat([x, torch.full((pad_len,), pad_val, dtype=x.dtype)]))
        out[k] = torch.stack(padded)
    return out


# -----------------------------------------------------------------------------
# Train
# -----------------------------------------------------------------------------

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--data-root", required=True)
    ap.add_argument("--output-dir", required=True)
    ap.add_argument("--model-id", default="openvla/openvla-7b")
    ap.add_argument("--epochs", type=int, default=5)
    ap.add_argument("--batch-size", type=int, default=4)
    ap.add_argument("--lr", type=float, default=2e-4)
    ap.add_argument("--lora-rank", type=int, default=32)
    ap.add_argument("--load-4bit", action="store_true")
    ap.add_argument("--num-workers", type=int, default=2)
    args = ap.parse_args()

    data_root = Path(args.data_root)
    out_dir = Path(args.output_dir)
    out_dir.mkdir(parents=True, exist_ok=True)

    print(f"loading episodes from {data_root}")
    episodes = load_episodes(data_root)
    steps = build_steps(episodes)
    print(f"  episodes={len(episodes)} steps={len(steps)}")
    if not steps:
        raise SystemExit("no steps to train on")

    q01, q99 = compute_action_stats(steps)
    print(f"action q01: {q01}")
    print(f"action q99: {q99}")
    np.savez(out_dir / "action_stats.npz", q01=q01, q99=q99)

    print("loading processor + model")
    processor = AutoProcessor.from_pretrained(args.model_id, trust_remote_code=True)

    model_kwargs = dict(
        trust_remote_code=True,
        torch_dtype=torch.bfloat16,
        low_cpu_mem_usage=True,
    )
    if args.load_4bit:
        from transformers import BitsAndBytesConfig
        model_kwargs["quantization_config"] = BitsAndBytesConfig(
            load_in_4bit=True,
            bnb_4bit_compute_dtype=torch.bfloat16,
            bnb_4bit_quant_type="nf4",
            bnb_4bit_use_double_quant=True,
        )

    model = AutoModelForVision2Seq.from_pretrained(args.model_id, **model_kwargs)
    if args.load_4bit:
        model = prepare_model_for_kbit_training(model)

    lora = LoraConfig(
        r=args.lora_rank,
        lora_alpha=args.lora_rank * 2,
        target_modules="all-linear",
        lora_dropout=0.0,
        bias="none",
        task_type="CAUSAL_LM",
    )
    model = get_peft_model(model, lora)
    model.print_trainable_parameters()

    dataset = OpenVLADataset(steps, q01, q99, processor)
    loader = DataLoader(
        dataset,
        batch_size=args.batch_size,
        shuffle=True,
        collate_fn=collate,
        num_workers=args.num_workers,
    )

    device = "cuda" if torch.cuda.is_available() else "cpu"
    if not args.load_4bit:
        model.to(device)

    optim = torch.optim.AdamW(
        filter(lambda p: p.requires_grad, model.parameters()), lr=args.lr
    )
    total_steps = args.epochs * max(1, len(loader))
    sched = get_linear_schedule_with_warmup(
        optim,
        num_warmup_steps=max(1, total_steps // 20),
        num_training_steps=total_steps,
    )

    model.train()
    for epoch in range(args.epochs):
        bar = tqdm(loader, desc=f"epoch {epoch+1}/{args.epochs}")
        for batch in bar:
            batch = {k: v.to(device) for k, v in batch.items()}
            batch["pixel_values"] = batch["pixel_values"].to(torch.bfloat16)
            out = model(**batch)
            loss = out.loss
            optim.zero_grad()
            loss.backward()
            optim.step()
            sched.step()
            bar.set_postfix(loss=float(loss))

        ckpt = out_dir / f"epoch_{epoch+1}"
        model.save_pretrained(ckpt)
        processor.save_pretrained(ckpt)
        print(f"saved {ckpt}")


if __name__ == "__main__":
    main()
