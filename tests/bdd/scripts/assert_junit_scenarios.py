#!/usr/bin/env python3
"""Fail when a Behave JUnit directory proves that no scenario ran."""

from __future__ import annotations

import sys
import xml.etree.ElementTree as ET
from pathlib import Path


def executed_scenario_count(report_directory: Path) -> int:
    executed = 0
    for report in sorted(report_directory.glob("*.xml")):
        root = ET.parse(report).getroot()
        for testcase in root.iter("testcase"):
            if testcase.find("skipped") is None:
                executed += 1
    return executed


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print("usage: assert_junit_scenarios.py <junit-directory>", file=sys.stderr)
        return 2

    report_directory = Path(argv[1])
    try:
        executed = executed_scenario_count(report_directory)
    except (OSError, ET.ParseError) as error:
        print(f"Could not read Behave JUnit reports: {error}", file=sys.stderr)
        return 1

    if executed == 0:
        print(
            f"BDD selection executed zero scenarios; refusing a false-green run ({report_directory}).",
            file=sys.stderr,
        )
        return 1

    print(f"Verified {executed} executed BDD scenario(s) in {report_directory}.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
