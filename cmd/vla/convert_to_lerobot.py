"""Convert capture-direct `steps.jsonl` episodes to LeRobot v2 dataset format.

Input layout (produced by `vla capture-direct`):
  <data-root>/<session_id>/
    steps.jsonl                # one JSON per line
    images_0/step_NNNNNN.jpg   # primary camera
    images_1/step_NNNNNN.jpg   # optional second camera
    images_2/step_NNNNNN.jpg   # optional third camera

Each step (current schema):
  {
    "step_index": int,
    "timestamp": ISO8601,
    "is_first": bool, "is_last": bool, "is_terminal": bool,
    "image":   "images_0/step_NNNNNN.jpg",
    "image_1": "images_1/step_NNNNNN.jpg",     # optional
    "image_2": "images_2/step_NNNNNN.jpg",     # optional
    "ee_pose": [x, y, z, ox, oy, oz, theta_deg],
    "joint_positions": [j0..j5],
    "gripper_position": int (0..840),
    "is_holding": bool,
    "language_instruction": str,
  }

Output (LeRobot v2.0):
  <output-dir>/
    meta/info.json
    meta/episodes.jsonl
    meta/tasks.jsonl
    meta/stats.json
    data/chunk-000/episode_000000.parquet
    ...

State / action representation:
  observation.state = [j0..j5, gripper_norm]                      (7-D float32)
  action            = state at next step (absolute target)        (7-D float32)
  gripper_norm      = gripper_position / 840.0  (in [0, 1])
  Terminal frames use action = current state (no movement).

Camera features are stored as JPEG-encoded `image` features in the parquet.
"""

import argparse
import io
import json
import logging
from pathlib import Path

import numpy as np
import pandas as pd
import pyarrow as pa
import pyarrow.parquet as pq
from PIL import Image

GRIPPER_RAW_MAX = 840.0
STATE_DIM = 7  # 6 joint angles + 1 gripper position

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("convert")


def normalize_gripper(pos: int) -> float:
    return float(pos) / GRIPPER_RAW_MAX


def step_state(step: dict) -> np.ndarray:
    joints = step.get("joint_positions") or []
    if len(joints) != 6:
        raise ValueError(
            f"step {step.get('step_index')} has {len(joints)} joint_positions, expected 6"
        )
    if "gripper_position" not in step:
        raise ValueError(f"step {step.get('step_index')} missing gripper_position")
    return np.asarray(
        list(joints) + [normalize_gripper(step["gripper_position"])],
        dtype=np.float32,
    )


def load_episode(sess_dir: Path):
    """Returns (steps, image_keys) where steps is a list of dicts and image_keys
    is the sorted list of camera field names present (e.g. ["image", "image_1"])."""
    jl = sess_dir / "steps.jsonl"
    if not jl.exists():
        return None, None
    steps = [json.loads(line) for line in jl.read_text().splitlines() if line.strip()]
    if len(steps) < 2:
        return None, None
    image_keys = sorted(k for k in steps[0].keys() if k == "image" or k.startswith("image_"))
    return steps, image_keys


def encode_jpeg(path: Path) -> bytes:
    """Re-encode any image to JPEG bytes for consistent storage."""
    img = Image.open(path).convert("RGB")
    buf = io.BytesIO()
    img.save(buf, format="JPEG", quality=92)
    return buf.getvalue()


def cam_field_name(image_key: str) -> str:
    """Map "image"->"observation.images.cam_0", "image_1"->"observation.images.cam_1"."""
    if image_key == "image":
        return "observation.images.cam_0"
    suffix = image_key.split("_", 1)[1]
    return f"observation.images.cam_{suffix}"


def build_frames(steps, sess_dir: Path, image_keys, episode_index: int,
                 task_index: int, global_index_start: int):
    """Returns (rows, num_frames). rows is a list of dicts ready for a DataFrame."""
    states = np.stack([step_state(s) for s in steps])  # (N, 7)
    actions = np.empty_like(states)
    actions[:-1] = states[1:]
    actions[-1] = states[-1]  # terminal: no movement

    rows = []
    t0 = None
    for i, s in enumerate(steps):
        # parse ISO timestamp to seconds-since-episode-start
        ts = pd.Timestamp(s["timestamp"]).timestamp()
        if t0 is None:
            t0 = ts
        row = {
            "observation.state": states[i].tolist(),
            "action": actions[i].tolist(),
            "timestamp": float(ts - t0),
            "frame_index": i,
            "episode_index": episode_index,
            "index": global_index_start + i,
            "task_index": task_index,
            "next.done": bool(s.get("is_last", i == len(steps) - 1)),
            "next.reward": 0.0,
        }
        for k in image_keys:
            rel = s.get(k)
            if not rel:
                raise ValueError(f"episode {sess_dir.name} step {i} missing {k}")
            img_bytes = encode_jpeg(sess_dir / rel)
            row[cam_field_name(k)] = {"bytes": img_bytes, "path": None}
        rows.append(row)
    return rows, len(steps)


def write_parquet(rows, parquet_path: Path, image_fields):
    df = pd.DataFrame(rows)
    table = pa.Table.from_pandas(df, preserve_index=False)
    parquet_path.parent.mkdir(parents=True, exist_ok=True)
    pq.write_table(table, parquet_path, compression="snappy")


def compute_stats(all_states, all_actions):
    """Per-feature mean/std/min/max for state and action."""
    def stats(arr):
        return {
            "mean": arr.mean(axis=0).tolist(),
            "std": (arr.std(axis=0) + 1e-8).tolist(),
            "min": arr.min(axis=0).tolist(),
            "max": arr.max(axis=0).tolist(),
            "count": int(arr.shape[0]),
        }
    return {
        "observation.state": stats(np.stack(all_states)),
        "action": stats(np.stack(all_actions)),
    }


def write_meta(out_dir: Path, episodes_meta, tasks, fps, image_keys, stats,
               total_frames, image_shape):
    meta = out_dir / "meta"
    meta.mkdir(parents=True, exist_ok=True)

    with (meta / "tasks.jsonl").open("w") as f:
        for ti, task in enumerate(tasks):
            f.write(json.dumps({"task_index": ti, "task": task}) + "\n")

    with (meta / "episodes.jsonl").open("w") as f:
        for em in episodes_meta:
            f.write(json.dumps(em) + "\n")

    features = {
        "observation.state": {
            "dtype": "float32",
            "shape": [STATE_DIM],
            "names": ["j0", "j1", "j2", "j3", "j4", "j5", "gripper"],
        },
        "action": {
            "dtype": "float32",
            "shape": [STATE_DIM],
            "names": ["j0", "j1", "j2", "j3", "j4", "j5", "gripper"],
        },
        "timestamp": {"dtype": "float32", "shape": [1], "names": None},
        "frame_index": {"dtype": "int64", "shape": [1], "names": None},
        "episode_index": {"dtype": "int64", "shape": [1], "names": None},
        "index": {"dtype": "int64", "shape": [1], "names": None},
        "task_index": {"dtype": "int64", "shape": [1], "names": None},
        "next.done": {"dtype": "bool", "shape": [1], "names": None},
        "next.reward": {"dtype": "float32", "shape": [1], "names": None},
    }
    for k in image_keys:
        features[cam_field_name(k)] = {
            "dtype": "image",
            "shape": list(image_shape),
            "names": ["height", "width", "channel"],
        }

    info = {
        "codebase_version": "v2.0",
        "robot_type": "xarm",
        "total_episodes": len(episodes_meta),
        "total_frames": total_frames,
        "total_tasks": len(tasks),
        "total_videos": 0,
        "total_chunks": 1,
        "chunks_size": 1000,
        "fps": fps,
        "splits": {"train": f"0:{len(episodes_meta)}"},
        "data_path": "data/chunk-{episode_chunk:03d}/episode_{episode_index:06d}.parquet",
        "video_path": None,
        "features": features,
    }
    (meta / "info.json").write_text(json.dumps(info, indent=2))
    (meta / "stats.json").write_text(json.dumps(stats, indent=2))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--data-root", required=True, help="dir containing <session>/steps.jsonl")
    ap.add_argument("--output-dir", required=True, help="LeRobot v2 dataset output dir")
    ap.add_argument("--fps", type=int, default=10)
    args = ap.parse_args()

    data_root = Path(args.data_root)
    out_dir = Path(args.output_dir)
    out_dir.mkdir(parents=True, exist_ok=True)

    sessions = sorted(p for p in data_root.iterdir() if p.is_dir())
    log.info("found %d session dirs in %s", len(sessions), data_root)

    tasks = []
    task_to_index = {}
    episodes_meta = []
    all_states, all_actions = [], []
    total_frames = 0
    image_keys_global = None
    image_shape = None

    for ep_idx, sess in enumerate(sessions):
        steps, image_keys = load_episode(sess)
        if steps is None:
            log.warning("skipping %s (no/empty steps.jsonl)", sess.name)
            continue
        if image_keys_global is None:
            image_keys_global = image_keys
            sample_img = Image.open(sess / steps[0][image_keys[0]])
            w, h = sample_img.size
            image_shape = (h, w, 3)
        elif image_keys != image_keys_global:
            raise ValueError(
                f"episode {sess.name} cameras {image_keys} differ from first {image_keys_global}"
            )

        task = steps[0].get("language_instruction", "")
        if task not in task_to_index:
            task_to_index[task] = len(tasks)
            tasks.append(task)
        ti = task_to_index[task]

        rows, n_frames = build_frames(steps, sess, image_keys, ep_idx, ti, total_frames)
        for r in rows:
            all_states.append(np.asarray(r["observation.state"], dtype=np.float32))
            all_actions.append(np.asarray(r["action"], dtype=np.float32))

        parquet_path = out_dir / "data" / "chunk-000" / f"episode_{ep_idx:06d}.parquet"
        write_parquet(rows, parquet_path, image_keys)
        log.info("wrote %s (%d frames, task=%r)", parquet_path.name, n_frames, task)

        episodes_meta.append({
            "episode_index": ep_idx,
            "tasks": [task],
            "length": n_frames,
        })
        total_frames += n_frames

    if not episodes_meta:
        raise SystemExit("no episodes converted")

    stats = compute_stats(all_states, all_actions)
    write_meta(out_dir, episodes_meta, tasks, args.fps,
               image_keys_global, stats, total_frames, image_shape)
    log.info("done: %d episodes, %d frames, %d tasks → %s",
             len(episodes_meta), total_frames, len(tasks), out_dir)


if __name__ == "__main__":
    main()
