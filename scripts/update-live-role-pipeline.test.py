#!/usr/bin/env python3
"""The checked-in Role Pipeline example and the live updater must not drift."""
import importlib.util
import unittest
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "update-live-role-pipeline.py"
EXAMPLE = ROOT / "docs" / "examples" / "role-pipeline.workflow.yml"


def load_updater():
    spec = importlib.util.spec_from_file_location("update_live_role_pipeline", SCRIPT)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class RolePipelineParityTest(unittest.TestCase):
    def setUp(self) -> None:
        self.updater = load_updater()
        example = yaml.safe_load(EXAMPLE.read_text())
        self.steps = {step["name"]: step for step in example["workflows"][0]["steps"]}

    def test_example_prompts_equal_updater_prompts(self) -> None:
        expected = {
            "Spike": self.updater.SPIKE_PROMPT,
            "Architect": self.updater.ARCHITECT_PROMPT,
            "Implement": self.updater.IMPLEMENT_PROMPT,
            "Review": self.updater.REVIEW_PROMPT,
        }
        for name, prompt in expected.items():
            with self.subTest(step=name):
                self.assertEqual(self.steps[name]["prompt"].strip(), prompt.strip())

    def test_example_steps_match_expected_structure(self) -> None:
        self.assertEqual(list(self.steps), self.updater.EXPECTED_STEPS)

    def test_current_prompt_is_a_known_baseline_and_older_ones_stay_known(self) -> None:
        baselines = self.updater.KNOWN_PRIOR_BASELINES["Architect"]["prompts"]
        self.assertEqual(baselines[0], self.updater.ARCHITECT_PROMPT.strip())
        self.assertEqual(len(set(baselines)), len(baselines))
        self.assertIn(self.updater.ARCHITECT_PROMPT_MINOR_APPROVAL.strip(), baselines)
        self.assertIn(self.updater.ARCHITECT_PROMPT_ENROLL_ONLY.strip(), baselines)

    def test_design_and_plan_have_distinct_roles(self) -> None:
        prompt = self.updater.ARCHITECT_PROMPT
        self.assertIn("DESIGN AND PLAN ARTIFACTS", prompt)
        self.assertIn("PREFLIGHT BEFORE THE FIRST HANDOFF", prompt)
        self.assertIn("Leave out increment tables, command checklists and delivery", prompt)
        self.assertIn("Do not\n  restate contract prose from the design.", prompt)


if __name__ == "__main__":
    unittest.main()
