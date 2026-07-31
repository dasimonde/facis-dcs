from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from assert_junit_scenarios import executed_scenario_count, main


class AssertJunitScenariosTest(unittest.TestCase):
    def test_empty_directory_fails(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            self.assertEqual(main(["assert_junit_scenarios.py", directory]), 1)

    def test_only_skipped_scenarios_fail(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, "skipped.xml").write_text(
                '<testsuite tests="1" skipped="1"><testcase name="skipped"><skipped/></testcase></testsuite>',
                encoding="utf-8",
            )
            self.assertEqual(executed_scenario_count(Path(directory)), 0)
            self.assertEqual(main(["assert_junit_scenarios.py", directory]), 1)

    def test_executed_scenario_passes(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, "executed.xml").write_text(
                '<testsuite tests="1"><testcase name="executed"/></testsuite>',
                encoding="utf-8",
            )
            self.assertEqual(executed_scenario_count(Path(directory)), 1)
            self.assertEqual(main(["assert_junit_scenarios.py", directory]), 0)


if __name__ == "__main__":
    unittest.main()
