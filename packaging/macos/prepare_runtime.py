#!/usr/bin/env python3
"""Collect the libmpv runtime closure and rewrite install names.

The build prefix of the LGPL runtime lives outside the app. This script copies
every non-system dependency into one flat directory, points each install name
and dependency at @loader_path, and re-signs the copies ad hoc. The result is
the Contents/Resources/libmpv payload that mygo build copies and signs with the
app.

Usage:
  prepare_runtime.py --source build/runtime/lib --dest resources/darwin-arm64/libmpv
"""

from __future__ import annotations

import argparse
import os
import shutil
import subprocess
import sys
from pathlib import Path

SYSTEM_PREFIXES = ("/usr/lib/", "/System/", "/Library/Frameworks/")


def run(*args: str) -> str:
    result = subprocess.run(args, check=True, capture_output=True, text=True)
    return result.stdout


def dependencies(path: Path) -> list[str]:
    """Return the libraries path loads, as otool -L reports them."""
    lines = run("otool", "-L", str(path)).splitlines()[1:]
    return [line.strip().split(" (")[0] for line in lines]


def is_system(path: str) -> bool:
    return path.startswith(SYSTEM_PREFIXES)


def collect(root: Path, entry: Path) -> dict[str, Path]:
    """Map each referenced basename to the file it names under root."""
    found: dict[str, Path] = {}
    queue = [entry]

    while queue:
        current = queue.pop()
        name = current.name
        if name in found:
            continue
        found[name] = current

        for dependency in dependencies(current):
            if is_system(dependency):
                continue
            # Dependencies of a self-contained build point inside its prefix.
            # Anything else is a build-host leak that must fail the gate.
            candidate = root / Path(dependency).name
            if not candidate.exists():
                raise SystemExit(
                    f"{current.name} depends on {dependency}, which is outside the runtime"
                )
            queue.append(candidate)

    return found


def rewrite(path: Path, names: set[str]) -> None:
    subprocess.run(
        ["install_name_tool", "-id", f"@loader_path/{path.name}", str(path)],
        check=True,
    )
    for dependency in dependencies(path):
        if is_system(dependency):
            continue
        name = Path(dependency).name
        if name not in names:
            raise SystemExit(f"{path.name} still depends on {dependency}")
        subprocess.run(
            [
                "install_name_tool",
                "-change",
                dependency,
                f"@loader_path/{name}",
                str(path),
            ],
            check=True,
        )


def sign(path: Path) -> None:
    subprocess.run(
        ["codesign", "--force", "--sign", "-", str(path)],
        check=True,
        capture_output=True,
    )


def verify(dest: Path) -> None:
    for path in sorted(dest.iterdir()):
        for dependency in dependencies(path):
            if is_system(dependency):
                continue
            if not dependency.startswith("@loader_path/"):
                raise SystemExit(f"{path.name} keeps {dependency}")
            if not (dest / Path(dependency).name).exists():
                raise SystemExit(f"{path.name} misses {dependency}")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", required=True, type=Path)
    parser.add_argument("--dest", required=True, type=Path)
    args = parser.parse_args()

    entry = args.source / "libmpv.2.dylib"
    if not entry.exists():
        raise SystemExit(f"missing {entry}")

    closure = collect(args.source, entry)
    args.dest.mkdir(parents=True, exist_ok=True)

    for name, source in closure.items():
        target = args.dest / name
        # Copy the file the dependency names, not the symlink beside it, so
        # the payload has one real file per referenced install name.
        shutil.copyfile(source, target)
        target.chmod(0o755)

    names = set(closure)
    for path in sorted(args.dest.iterdir()):
        rewrite(path, names)
        sign(path)

    verify(args.dest)

    total = sum(path.stat().st_size for path in args.dest.iterdir())
    print(f"prepared {len(names)} libraries, {total / 1024 / 1024:.1f} MiB")
    return 0


if __name__ == "__main__":
    sys.exit(main())
