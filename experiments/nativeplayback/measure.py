"""Bounded process sampler for the native spike (standard library only)."""
import csv
import argparse
import json
import pathlib
import subprocess
import sys
import time

from observe_gpu import sample as sample_gpu


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary")
    parser.add_argument("source")
    parser.add_argument("output", type=pathlib.Path)
    parser.add_argument("--capture", help="Path to the compiled capture.swift diagnostic recorder")
    parser.add_argument("--gpu", action="store_true", help="Include whole-system GPU samples and a 6-second baseline")
    args = parser.parse_args()
    binary, source, output = args.binary, args.source, args.output
    if any((output / name).exists() for name in ("playback.log", "process.csv", "gpu.jsonl", "capture.mov")):
        parser.error("output directory already contains measurement files; choose a new directory")
    output.mkdir(parents=True, exist_ok=True)
    capture = None
    with (output / "playback.log").open("w") as log, \
            (output / "process.csv").open("w", newline="") as data, \
            (output / "gpu.jsonl").open("x") as gpu_data, \
            (output / "capture.log").open("w") as capture_log:
        def record_gpu(phase):
            if args.gpu:
                gpu_data.write(json.dumps({**sample_gpu(), "phase": phase}) + "\n")
                gpu_data.flush()

        if args.gpu:
            for _ in range(6):
                record_gpu("prelaunch")
                time.sleep(1)
        process = subprocess.Popen(
            [binary, "-source", source, "-exercise", "-duration", "36s"],
            stdout=log, stderr=log,
        )
        writer = csv.writer(data)
        writer.writerow(["elapsed_seconds", "cpu_percent", "rss_kib"])
        start = time.monotonic()
        try:
            if args.capture:
                capture = subprocess.Popen(
                    [args.capture, str(process.pid), str(output / "capture.mov"), "27"],
                    stdout=capture_log, stderr=capture_log,
                )
            while process.poll() is None and time.monotonic() - start < 75:
                sample = subprocess.run(
                    ["ps", "-p", str(process.pid), "-o", "%cpu=,rss="],
                    capture_output=True, text=True, timeout=3, check=False,
                ).stdout.split()
                if len(sample) == 2:
                    writer.writerow([round(time.monotonic() - start, 3), *sample])
                    data.flush()
                record_gpu("exercise")
                time.sleep(0.5)
        finally:
            if process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
            if capture is not None and capture.poll() is None:
                try:
                    capture.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    capture.terminate()
                    try:
                        capture.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        capture.kill()
                        capture.wait(timeout=5)
        result = process.wait(timeout=5)
        if args.gpu:
            for _ in range(3):
                record_gpu("postexit")
                time.sleep(1)
        summary = {"app_exit": result, "capture_exit": capture.returncode if capture else None}
        (output / "run.json").write_text(json.dumps(summary, indent=2) + "\n")
        print(f"spike exit={result}; evidence={output}")
        if capture is not None and capture.returncode:
            return 1
        return result


if __name__ == "__main__":
    sys.exit(main())
