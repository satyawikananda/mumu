"""Sample system-wide Apple GPU counters; never attribute them to one process."""
import argparse
import json
import pathlib
import plistlib
import subprocess
import time


def sample():
    result = subprocess.run(
        ["ioreg", "-a", "-r", "-c", "AGXAccelerator", "-d", "1"],
        capture_output=True, check=True, timeout=5,
    )
    devices = plistlib.loads(result.stdout)
    statistics = [device["PerformanceStatistics"] for device in devices
                  if "PerformanceStatistics" in device]
    if len(statistics) != 1:
        raise RuntimeError(f"expected one Apple GPU, found {len(statistics)}")
    record = {"unix_seconds": time.time(), "scope": "whole-system"}
    for name in ("Device Utilization %", "Renderer Utilization %", "Tiler Utilization %"):
        value = statistics[0][name]
        if not isinstance(value, (int, float)) or not 0 <= value <= 100:
            raise RuntimeError(f"invalid GPU utilization: {name}={value!r}")
        record[name] = value
    return record


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("output", type=pathlib.Path)
    parser.add_argument("--seconds", type=int, default=45)
    args = parser.parse_args()
    if not 1 <= args.seconds <= 120:
        parser.error("--seconds must be 1..120")
    args.output.parent.mkdir(parents=True, exist_ok=True)
    # Exclusive creation prevents replacing an earlier measurement accidentally.
    with args.output.open("x") as output:
        for _ in range(args.seconds):
            start = time.monotonic()
            output.write(json.dumps(sample()) + "\n")
            output.flush()
            time.sleep(max(0, 1 - (time.monotonic() - start)))


if __name__ == "__main__":
    main()
