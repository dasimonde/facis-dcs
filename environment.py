"""Behave environment hooks for DCS BDD tests."""

import os
import sys
from pathlib import Path


SKIP_TAGS = {"skip", "skipped"}


def _normalize_tag(tag):
	value = str(tag).strip().lower()
	if value.startswith("@"):
		value = value[1:]
	return value


def _has_skip_tag(tags):
	return any(_normalize_tag(tag) in SKIP_TAGS for tag in (tags or ()))


def _iter_feature_scenarios(feature):
	# Includes scenarios generated from outlines when supported by Behave.
	if hasattr(feature, "walk_scenarios"):
		yield from feature.walk_scenarios()
	else:
		yield from feature.scenarios


def before_feature(context, feature):
	if not _has_skip_tag(getattr(feature, "tags", ())):
		return
	for scenario in _iter_feature_scenarios(feature):
		scenario.skip('Skipped by feature tag "@skip"')


def _scenario_has_skip_tag(scenario):
	tags = []
	tags.extend(getattr(scenario, "effective_tags", ()) or ())
	tags.extend(getattr(scenario, "tags", ()) or ())
	feature = getattr(scenario, "feature", None)
	if feature is not None:
		tags.extend(getattr(feature, "tags", ()) or ())
	return _has_skip_tag(tags)


def before_scenario(context, scenario):
	if _scenario_has_skip_tag(scenario):
		scenario.skip('Skipped by scenario tag "@skip"')


def before_all(context):
	steps_dir = Path(__file__).resolve().parent / "steps"
	steps_dir_str = str(steps_dir)
	if steps_dir_str not in sys.path:
		sys.path.insert(0, steps_dir_str)

	# Shared request defaults for step definitions.
	context.base_url = os.getenv("BDD_DCS_BASE_URL", "http://127.0.0.1:8991").rstrip("/")
	context.http_timeout_seconds = float(os.getenv("BDD_HTTP_TIMEOUT_SECONDS", "20"))
	context.aliases = {}
