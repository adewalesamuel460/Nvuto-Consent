#!/usr/bin/env python3
"""Assert that required Go tests actually ran and passed.

usage: assert-go-test-json.py <go-test-json-file> <TestName>=<passes> [...]

Reads `go test -json` output and fails unless every named top-level test
passed exactly the expected number of times. Any skipped or failed top-level
test also fails the check, so a required integration job cannot go green by
silently skipping its database tests.
"""
import json
import sys


def main(argv):
    if len(argv) < 3:
        sys.exit(__doc__)
    path, expectations = argv[1], argv[2:]
    passes, problems = {}, []
    with open(path, encoding="utf-8") as stream:
        for number, line in enumerate(stream, 1):
            line = line.strip()
            if not line:
                continue
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                sys.exit(f"{path}:{number}: not go test -json output: {line[:120]}")
            test, action = event.get("Test"), event.get("Action")
            if not test or "/" in test:
                continue
            if action == "pass":
                passes[test] = passes.get(test, 0) + 1
            elif action in ("skip", "fail"):
                problems.append(f"{test}: {action}")
    for expectation in expectations:
        name, _, count = expectation.partition("=")
        if passes.get(name, 0) != int(count or 1):
            problems.append(f"{name}: expected {count or 1} passing run(s), saw {passes.get(name, 0)}")
    if problems:
        sys.exit("required test evidence missing:\n  " + "\n  ".join(problems))
    print("verified passing runs:", json.dumps(passes, sort_keys=True))


if __name__ == "__main__":
    main(sys.argv)
