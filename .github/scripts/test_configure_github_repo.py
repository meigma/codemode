#!/usr/bin/env python3
"""Tests for configure_github_repo.py."""

from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path
from typing import Any


SCRIPT_PATH = Path(__file__).with_name("configure_github_repo.py")
SPEC = importlib.util.spec_from_file_location("configure_github_repo", SCRIPT_PATH)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError(f"Unable to load {SCRIPT_PATH}")

configure = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = configure
SPEC.loader.exec_module(configure)


class FakeGitHubApi:
    def __init__(self, pages_site: dict[str, Any] | None = None) -> None:
        self.pages_site = pages_site
        self.created_pages: list[dict[str, Any]] = []
        self.updated_pages: list[dict[str, Any]] = []

    def get_repository(self, owner: str, repo: str) -> dict[str, Any]:
        return {"private": False}

    def list_rulesets(self, owner: str, repo: str) -> list[dict[str, Any]]:
        return []

    def get_pages_site(self, owner: str, repo: str) -> dict[str, Any] | None:
        return self.pages_site

    def create_pages_site(self, owner: str, repo: str, payload: dict[str, Any]) -> dict[str, Any]:
        self.created_pages.append(payload)
        self.pages_site = {"build_type": payload["build_type"]}
        return self.pages_site

    def update_pages_site(self, owner: str, repo: str, payload: dict[str, Any]) -> None:
        self.updated_pages.append(payload)
        if self.pages_site is None:
            self.pages_site = {}
        self.pages_site.update(payload)


def base_config(pages: dict[str, Any]) -> dict[str, Any]:
    return {
        "repository": {},
        "security": {},
        "pages": pages,
        "rulesets": {},
        "unsupported": {},
    }


def pages_change(plan: Any) -> Any:
    matches = [change for change in plan.changes if change.key == "pages.site"]
    if len(matches) != 1:
        raise AssertionError(f"Expected exactly one pages.site change, got {len(matches)}")
    return matches[0]


class ConfigureGitHubRepoPagesTest(unittest.TestCase):
    def test_manifest_accepts_pages_table(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            manifest = Path(tmpdir) / "repository-settings.toml"
            manifest.write_text(
                """
[pages]
build_type = "workflow"
https_enforced = true
""",
                encoding="utf-8",
            )

            config = configure.load_manifest(manifest)

        self.assertEqual(config["pages"]["build_type"], "workflow")
        self.assertTrue(config["pages"]["https_enforced"])

    def test_manifest_rejects_invalid_pages_build_type(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            manifest = Path(tmpdir) / "repository-settings.toml"
            manifest.write_text(
                """
[pages]
build_type = "branch"
""",
                encoding="utf-8",
            )

            with self.assertRaisesRegex(configure.ConfigError, "pages.build_type"):
                configure.load_manifest(manifest)

    def test_plan_creates_workflow_pages_site(self) -> None:
        api = FakeGitHubApi()

        plan = configure.build_plan(
            api,
            "meigma",
            "template-go",
            base_config({"build_type": "workflow", "https_enforced": True}),
            mode="plan",
            hostname="github.com",
        )

        change = pages_change(plan)
        self.assertEqual(change.operation["kind"], "create_pages")
        self.assertEqual(change.operation["create_payload"], {"build_type": "workflow"})
        self.assertEqual(change.operation["update_payload"], {"https_enforced": True})

    def test_plan_updates_existing_pages_site(self) -> None:
        api = FakeGitHubApi(pages_site={"build_type": "legacy", "https_enforced": False})

        plan = configure.build_plan(
            api,
            "meigma",
            "template-go",
            base_config({"build_type": "workflow", "https_enforced": True}),
            mode="plan",
            hostname="github.com",
        )

        change = pages_change(plan)
        self.assertEqual(change.operation["kind"], "update_pages")
        self.assertEqual(change.operation["payload"], {"build_type": "workflow", "https_enforced": True})

    def test_apply_create_pages_runs_follow_up_update(self) -> None:
        api = FakeGitHubApi()
        plan = configure.PlanResult(
            repo="meigma/template-go",
            hostname="github.com",
            mode="apply",
            changes=[
                configure.PlannedChange(
                    key="pages.site",
                    description="Create GitHub Pages site",
                    current=None,
                    desired={"build_type": "workflow", "https_enforced": True},
                    operation={
                        "kind": "create_pages",
                        "create_payload": {"build_type": "workflow"},
                        "update_payload": {"https_enforced": True},
                    },
                )
            ],
            unsupported=[],
            warnings=[],
        )

        applied = configure.apply_plan(api, "meigma", "template-go", plan)

        self.assertEqual(applied, ["Create GitHub Pages site"])
        self.assertEqual(api.created_pages, [{"build_type": "workflow"}])
        self.assertEqual(api.updated_pages, [{"https_enforced": True}])


if __name__ == "__main__":
    unittest.main()
