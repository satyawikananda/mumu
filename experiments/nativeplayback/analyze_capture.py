"""Measure this spike's synthetic pattern in a process-filtered capture (NumPy + FFmpeg)."""
import argparse
import hashlib
import json
import pathlib
import subprocess

import numpy as np


RATE = 60
WIDTH, HEIGHT = 320, 208


def intervals(mask, rate, offset=0):
    edges = np.diff(np.r_[False, mask, False].astype(np.int8))
    return [[round(start / rate + offset, 6), round(end / rate + offset, 6)]
            for start, end in zip(np.flatnonzero(edges == 1), np.flatnonzero(edges == -1))]


def video_region(frame):
    values = frame.astype(np.int16)
    saturated = (values.max(2) - values.min(2) > 90) & (values.max(2) > 120)
    spans = intervals(saturated.sum(1) > 45, 1)
    if not spans:
        return None
    top, bottom = map(int, max(spans, key=lambda span: span[1] - span[0]))
    if bottom - top < 20:
        return None
    columns = np.flatnonzero(saturated[top:bottom].sum(0) > (bottom - top) * 0.5)
    if len(columns) < 45:
        return None
    left, right = int(columns[0]), int(columns[-1]) + 1
    return frame[top:bottom, left:right]


def video_metrics(movie):
    process = subprocess.Popen([
        "ffmpeg", "-v", "error", "-i", str(movie), "-map", "0:v:0",
        "-t", "120", "-vf", f"fps={RATE}:start_time=0,scale={WIDTH}:{HEIGHT}",
        "-f", "rawvideo", "-pix_fmt", "rgb24", "-",
    ], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    visible, flashes, repeated = [], [], []
    previous = None
    frame_size = WIDTH * HEIGHT * 3
    try:
        while True:
            raw = process.stdout.read(frame_size)
            if not raw:
                break
            if len(raw) != frame_size:
                raise RuntimeError("truncated decoded video frame")
            frame = np.frombuffer(raw, dtype=np.uint8).reshape(HEIGHT, WIDTH, 3)
            region = video_region(frame)
            visible.append(region is not None)
            if region is None:
                flashes.append(False)
                repeated.append(False)
                previous = None
                continue
            height, width = region.shape[:2]
            patch = region[2:max(3, int(width * 0.09)), 2:max(3, int(width * 0.09))]
            flashes.append(float((patch.min(2) > 205).mean()) > 0.75)
            rows = np.linspace(0, height - 1, 36).astype(int)
            columns = np.linspace(0, width - 1, 64).astype(int)
            thumbnail = region[np.ix_(rows, columns)].astype(np.int16)
            repeated.append(previous is not None and float(np.abs(thumbnail - previous).mean()) < 0.4)
            previous = thumbnail
    finally:
        process.stdout.close()
        error = process.stderr.read().decode()
        process.stderr.close()
        if process.wait(timeout=10):
            raise RuntimeError(error)
    if not visible or sum(flashes) < 2:
        raise RuntimeError("capture lacks the visible synthetic flash pattern")
    return {
        "duration_seconds": len(visible) / RATE,
        "flash_intervals": intervals(flashes, RATE),
        "video_absent_intervals": intervals(~np.asarray(visible), RATE),
        "repeated_image_intervals_over_100ms": [
            span for span in intervals(repeated, RATE) if span[1] - span[0] >= 0.1
        ],
    }


def analyze(movie):
    info = json.loads(subprocess.run([
        "ffprobe", "-v", "error", "-show_streams", "-of", "json", str(movie),
    ], capture_output=True, check=True, timeout=15).stdout)
    audio_stream = next(stream for stream in info["streams"] if stream["codec_type"] == "audio")
    audio = subprocess.run([
        "ffmpeg", "-v", "error", "-i", str(movie), "-map", "0:a:0", "-t", "120",
        "-ac", "1", "-ar", "8000", "-f", "f32le", "-",
    ], capture_output=True, check=True, timeout=30).stdout
    samples = np.frombuffer(audio, dtype="<f4")
    windows = samples[:len(samples) // 80 * 80].reshape(-1, 80)
    rms = np.sqrt(np.mean(windows ** 2, axis=1))
    baseline = float(np.median(rms[rms > 1e-6]))
    if not np.isfinite(baseline) or baseline <= 1e-6:
        raise RuntimeError("capture has no measurable continuous tone")
    offset = float(audio_stream.get("start_time", 0))
    spectrum = np.abs(np.fft.rfft(windows * np.hanning(80), axis=1))
    tone_band = spectrum[:, 1:4].sum(1)
    beep_band = spectrum[:, 7:11].sum(1)
    beeps = intervals((beep_band > tone_band * 2) & (rms > baseline * 0.15), 100, offset)
    video = video_metrics(movie)
    pairs = []
    for flash, _ in video["flash_intervals"]:
        if not beeps:
            break
        beep = min((span[0] for span in beeps), key=lambda time: abs(time - flash))
        if abs(beep - flash) < 0.25:
            pairs.append({"flash_seconds": flash, "beep_seconds": beep,
                          "audio_minus_video_ms": round((beep - flash) * 1000, 3)})
    if len(pairs) < 3:
        raise RuntimeError("fewer than three paired flash/beep observations")
    with movie.open("rb") as source:
        digest = hashlib.file_digest(source, "sha256").hexdigest()
    return {
        "capture_sha256": digest,
        "video_sampling_hz": RATE, "audio_rms_window_ms": 10,
        "continuous_tone_rms": baseline,
        "beep_intervals": beeps,
        "low_audio_intervals": intervals(rms < baseline * 0.15, 100, offset),
        **video, "paired_sync": pairs,
        "limitations": [
            "Capture path includes OS/compositor/codec latency; physical speaker/display latency is unmeasured.",
            "Low audio includes deliberate mute and startup/shutdown; correlate with playback.log.",
            "Video absence includes deliberate hide/closure and capture failure; it is not automatically a player blackout.",
            "Repeated-image detection is specific to this moving synthetic fixture and includes normal frame duplication.",
        ],
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("movie", type=pathlib.Path)
    parser.add_argument("output", type=pathlib.Path)
    args = parser.parse_args()
    result = analyze(args.movie)
    with args.output.open("x") as output:
        json.dump(result, output, indent=2)
        output.write("\n")
    print(f"paired flash/beep observations={len(result['paired_sync'])}; evidence={args.output}")


if __name__ == "__main__":
    main()
