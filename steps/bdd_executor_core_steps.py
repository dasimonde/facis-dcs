"""Expose reusable step definitions shipped by bdd-executor core."""

# Behave discovers step definitions by importing modules under features/steps.
# Importing this module registers canonical reusable steps from bdd-executor.
from eu.xfsc.bdd.core.steps import *  # noqa: F401,F403
