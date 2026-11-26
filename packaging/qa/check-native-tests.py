#!/usr/bin/env python3
"""Require executed native tests, not a green process consisting of skips."""
import json
import sys

required = set(sys.argv[2:])
passed = set()
with open(sys.argv[1], encoding="utf-8") as source:
    for line in source:
        event = json.loads(line)
        name = event.get("Test", "")
        if event["Action"] == "fail":
            raise SystemExit("native test failed: " + name)
        if event["Action"] == "skip" and name in required:
            raise SystemExit("required native test skipped: " + name)
        if event["Action"] == "pass" and name in required:
            passed.add(name)
if missing := required - passed:
    raise SystemExit("native tests did not execute: " + ", ".join(sorted(missing)))
print("Executed native tests: " + ", ".join(sorted(passed)))
