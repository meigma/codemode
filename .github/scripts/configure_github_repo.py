#!/usr/bin/env python3
"""Configure GitHub repository settings from a TOML manifest.

Usage:
  uv run .github/scripts/configure_github_repo.py plan --repo OWNER/REPO
  uv run .github/scripts/configure_github_repo.py apply --repo OWNER/REPO
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import tomllib
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any


DEFAULT_CONFIG_PATH = Path(".github/repository-settings.toml")
API_VERSION = "2022-11-28"
DEFAULT_HOSTNAME = "github.com"
ADMIN_REPOSITORY_ROLE_ID = 5

SUPPORTED_REPOSITORY_FIELDS = {
    "default_branch": "default_branch",
    "is_template": "is_template",
    "allow_auto_merge": "allow_auto_merge",
    "allow_update_branch": "allow_update_branch",
    "delete_branch_on_merge": "delete_branch_on_merge",
    "allow_merge_commit": "allow_merge_commit",
    "allow_squash_merge": "allow_squash_merge",
    "allow_rebase_merge": "allow_rebase_merge",
    "has_discussions": "has_discussions",
    "has_issues": "has_issues",
    "has_projects": "has_projects",
    "has_wiki": "has_wiki",
    "web_commit_signoff_required": "web_commit_signoff_required",
    "squash_merge_commit_title": "squash_merge_commit_title",
    "squash_merge_commit_message": "squash_merge_commit_message",
}

SUPPORTED_STATUS_CHECK_KEYS = {
    "enabled",
    "strict_required_status_checks_policy",
    "do_not_enforce_on_create",
    "contexts",
}
SUPPORTED_PAGES_FIELDS = {
    "build_type",
    "https_enforced",
}
SUPPORTED_PAGES_BUILD_TYPES = {"legacy", "workflow"}

UNSUPPORTED_REASONS = {
    "preserve_repository": "GitHub Archive Program enrollment is not exposed through a documented repository REST API.",
    "code_quality_preview": "The Code quality preview screen does not have a documented repository-level public REST API.",
    "automatic_dependency_submission": "Automatic dependency submission is not exposed through a documented repository REST API.",
    "dependabot_malware_alerts": "Dependabot malware alerts are not exposed through a documented repository REST API.",
    "prevent_direct_alert_dismissals": "Direct alert dismissal controls are not exposed through a documented repository REST API.",
    "grouped_security_updates": "Grouped security updates are not exposed through a documented repository REST API.",
    "dependabot_version_updates": "Dependabot version updates are configured through dependabot.yml/rules, not a simple repository REST toggle.",
    "commit_comments_on_individual_commits": "The commit comments visibility toggle is not exposed through a documented repository REST API.",
    "pull_request_creation_permissions": "Pull request creation permissions are not exposed through a documented repository REST API.",
}


class ConfigError(RuntimeError):
    """Raised when the manifest is invalid."""


class ApiError(RuntimeError):
    """Raised when a GitHub API call fails."""

    def __init__(self, method: str, path: str, status: int, message: str) -> None:
        super().__init__(f"{method} {path} failed with HTTP {status}: {message}")
        self.method = method
        self.path = path
        self.status = status
        self.message = message


@dataclass
class UnsupportedSetting:
    key: str
    desired: Any
    reason: str


@dataclass
class PlannedChange:
    key: str
    description: str
    current: Any
    desired: Any
    operation: dict[str, Any]


@dataclass
class PlanResult:
    repo: str
    hostname: str
    mode: str
    changes: list[PlannedChange]
    unsupported: list[UnsupportedSetting]
    warnings: list[str]

    def to_json(self) -> dict[str, Any]:
        return {
            "repo": self.repo,
            "hostname": self.hostname,
            "mode": self.mode,
            "changes": [asdict(change) for change in self.changes],
            "unsupported": [asdict(item) for item in self.unsupported],
            "warnings": self.warnings,
        }


class GitHubApi:
    """Minimal GitHub API client for repository configuration."""

    def __init__(self, token: str, hostname: str = DEFAULT_HOSTNAME) -> None:
        self.token = token
        self.hostname = hostname
        if hostname == DEFAULT_HOSTNAME:
            self.base_url = "https://api.github.com"
        else:
            self.base_url = f"https://{hostname}/api/v3"
        self._app_slug_cache: dict[tuple[str, str], int] = {}

    def get_repository(self, owner: str, repo: str) -> dict[str, Any]:
        return self._request_json("GET", f"/repos/{owner}/{repo}")

    def update_repository(self, owner: str, repo: str, payload: dict[str, Any]) -> dict[str, Any]:
        return self._request_json("PATCH", f"/repos/{owner}/{repo}", payload)

    def get_toggle_state(self, owner: str, repo: str, toggle_name: str) -> bool:
        path = self._toggle_path(owner, repo, toggle_name)
        try:
            status, data = self._request("GET", path)
        except ApiError as exc:
            if toggle_name == "private_vulnerability_reporting" and exc.status == 422:
                raise ApiError(exc.method, exc.path, exc.status, "private vulnerability reporting is unavailable for this repository") from exc
            if exc.status == 404:
                return False
            raise

        if toggle_name == "vulnerability_alerts":
            return status == 204

        if toggle_name in {"immutable_releases", "automated_security_fixes", "private_vulnerability_reporting"}:
            return bool(data.get("enabled"))

        raise ConfigError(f"Unknown toggle {toggle_name!r}")

    def set_toggle_state(self, owner: str, repo: str, toggle_name: str, enabled: bool) -> None:
        path = self._toggle_path(owner, repo, toggle_name)
        method = "PUT" if enabled else "DELETE"
        self._request(method, path, expected_statuses={204})

    def list_rulesets(self, owner: str, repo: str) -> list[dict[str, Any]]:
        return self._request_json("GET", f"/repos/{owner}/{repo}/rulesets?includes_parents=false&per_page=100")

    def get_ruleset(self, owner: str, repo: str, ruleset_id: int) -> dict[str, Any]:
        return self._request_json("GET", f"/repos/{owner}/{repo}/rulesets/{ruleset_id}?includes_parents=false")

    def create_ruleset(self, owner: str, repo: str, payload: dict[str, Any]) -> dict[str, Any]:
        return self._request_json("POST", f"/repos/{owner}/{repo}/rulesets", payload, expected_statuses={201})

    def update_ruleset(self, owner: str, repo: str, ruleset_id: int, payload: dict[str, Any]) -> dict[str, Any]:
        return self._request_json("PATCH", f"/repos/{owner}/{repo}/rulesets/{ruleset_id}", payload)

    def get_pages_site(self, owner: str, repo: str) -> dict[str, Any] | None:
        try:
            return self._request_json("GET", f"/repos/{owner}/{repo}/pages")
        except ApiError as exc:
            if exc.status == 404:
                return None
            raise

    def create_pages_site(self, owner: str, repo: str, payload: dict[str, Any]) -> dict[str, Any]:
        return self._request_json("POST", f"/repos/{owner}/{repo}/pages", payload, expected_statuses={201})

    def update_pages_site(self, owner: str, repo: str, payload: dict[str, Any]) -> None:
        self._request("PUT", f"/repos/{owner}/{repo}/pages", payload, expected_statuses={204})

    def resolve_app_actor_id(self, owner: str, repo: str, slug: str) -> int:
        cache_key = (self.hostname, slug)
        if cache_key in self._app_slug_cache:
            return self._app_slug_cache[cache_key]

        app = self._request_json("GET", f"/apps/{slug}")
        app_id = app.get("id")
        if app_id is None:
            raise ConfigError(f"Unable to resolve GitHub App slug {slug!r} for {owner}/{repo}")

        self._app_slug_cache[cache_key] = int(app_id)
        return int(app_id)

    def _toggle_path(self, owner: str, repo: str, toggle_name: str) -> str:
        suffixes = {
            "immutable_releases": "immutable-releases",
            "private_vulnerability_reporting": "private-vulnerability-reporting",
            "vulnerability_alerts": "vulnerability-alerts",
            "automated_security_fixes": "automated-security-fixes",
        }
        return f"/repos/{owner}/{repo}/{suffixes[toggle_name]}"

    def _request_json(
        self,
        method: str,
        path: str,
        payload: dict[str, Any] | None = None,
        expected_statuses: set[int] | None = None,
    ) -> Any:
        _, data = self._request(method, path, payload, expected_statuses)
        return data

    def _paginate(self, path: str, array_key: str | None = None) -> list[dict[str, Any]]:
        items: list[dict[str, Any]] = []
        next_path: str | None = path
        while next_path:
            status, data, headers = self._request_raw("GET", next_path, expected_statuses={200})
            if status != 200:
                raise ApiError("GET", next_path, status, "unexpected response while paginating")

            if array_key:
                page_items = data.get(array_key, [])
            elif isinstance(data, list):
                page_items = data
            else:
                raise ApiError("GET", next_path, status, "unexpected pagination payload")

            items.extend(page_items)
            next_path = self._parse_next_link(headers.get("Link"))
        return items

    def _request(
        self,
        method: str,
        path: str,
        payload: dict[str, Any] | None = None,
        expected_statuses: set[int] | None = None,
    ) -> tuple[int, Any]:
        status, data, _ = self._request_raw(method, path, payload, expected_statuses)
        return status, data

    def _request_raw(
        self,
        method: str,
        path: str,
        payload: dict[str, Any] | None = None,
        expected_statuses: set[int] | None = None,
    ) -> tuple[int, Any, dict[str, str]]:
        if expected_statuses is None:
            expected_statuses = {200, 201, 204}

        url = urllib.parse.urljoin(f"{self.base_url}/", path.lstrip("/"))
        headers = {
            "Accept": "application/vnd.github+json",
            "Authorization": f"Bearer {self.token}",
            "User-Agent": "configure-github-repo",
            "X-GitHub-Api-Version": API_VERSION,
        }

        body: bytes | None = None
        if payload is not None:
            body = json.dumps(payload).encode("utf-8")
            headers["Content-Type"] = "application/json"

        request = urllib.request.Request(url, data=body, headers=headers, method=method)

        try:
            with urllib.request.urlopen(request) as response:
                status = response.getcode()
                raw = response.read()
                response_headers = dict(response.headers.items())
        except urllib.error.HTTPError as exc:
            raw = exc.read()
            message = raw.decode("utf-8", errors="replace").strip()
            try:
                parsed = json.loads(message)
                if isinstance(parsed, dict) and parsed.get("message"):
                    message = parsed["message"]
            except json.JSONDecodeError:
                pass
            raise ApiError(method, path, exc.code, message or "request failed") from exc
        except urllib.error.URLError as exc:
            raise ApiError(method, path, 0, str(exc.reason)) from exc

        if status not in expected_statuses:
            raise ApiError(method, path, status, "unexpected status code")

        if status == 204 or not raw:
            return status, {}, response_headers

        content_type = response_headers.get("Content-Type", "")
        if "json" in content_type:
            return status, json.loads(raw.decode("utf-8")), response_headers
        return status, raw.decode("utf-8"), response_headers

    @staticmethod
    def _parse_next_link(link_header: str | None) -> str | None:
        if not link_header:
            return None
        for part in link_header.split(","):
            section = part.strip()
            if 'rel="next"' not in section:
                continue
            if not section.startswith("<") or ">" not in section:
                continue
            url = section[1 : section.index(">")]
            parsed = urllib.parse.urlparse(url)
            if parsed.netloc and parsed.scheme:
                path = parsed.path
                if parsed.query:
                    path = f"{path}?{parsed.query}"
                return path
            return url
        return None


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Configure GitHub repository settings from a TOML manifest."
    )
    parser.add_argument("mode", choices=["plan", "apply"], help="Preview or apply changes")
    parser.add_argument("--repo", required=True, help="Target repository in OWNER/REPO format")
    parser.add_argument(
        "--config",
        default=str(DEFAULT_CONFIG_PATH),
        help=f"Path to repository settings manifest (default: {DEFAULT_CONFIG_PATH})",
    )
    parser.add_argument(
        "--hostname",
        default=os.environ.get("GH_HOST", DEFAULT_HOSTNAME),
        help=f"GitHub hostname (default: {DEFAULT_HOSTNAME})",
    )
    parser.add_argument("--json-report", help="Write a machine-readable JSON report to this path")
    return parser.parse_args(argv)


def load_manifest(path: Path) -> dict[str, Any]:
    if not path.exists():
        raise ConfigError(f"Manifest file not found: {path}")

    with path.open("rb") as handle:
        data = tomllib.load(handle)

    require_mapping(data, "manifest")
    validate_top_level_keys(data, {"repository", "security", "pages", "rulesets", "unsupported"}, "manifest")

    repository = data.get("repository", {})
    security = data.get("security", {})
    pages = data.get("pages", {})
    rulesets = data.get("rulesets", {})
    unsupported = data.get("unsupported", {})

    require_mapping(repository, "repository")
    require_mapping(security, "security")
    require_mapping(pages, "pages")
    require_mapping(rulesets, "rulesets")
    require_mapping(unsupported, "unsupported")

    validate_top_level_keys(
        repository,
        set(SUPPORTED_REPOSITORY_FIELDS) | {"immutable_releases"},
        "repository",
    )
    validate_top_level_keys(
        security,
        {
            "advanced_security",
            "dependency_graph",
            "vulnerability_alerts",
            "private_vulnerability_reporting",
            "automated_security_fixes",
        },
        "security",
    )
    validate_top_level_keys(pages, SUPPORTED_PAGES_FIELDS, "pages")
    validate_pages_table(pages)
    validate_top_level_keys(rulesets, {"branch_default", "tags_default"}, "rulesets")

    branch_ruleset = rulesets.get("branch_default", {})
    tags_ruleset = rulesets.get("tags_default", {})
    require_mapping(branch_ruleset, "rulesets.branch_default")
    require_mapping(tags_ruleset, "rulesets.tags_default")

    validate_ruleset_table(branch_ruleset, "rulesets.branch_default", allow_pull_request=True)
    validate_ruleset_table(tags_ruleset, "rulesets.tags_default", allow_pull_request=False)

    return data


def require_mapping(value: Any, name: str) -> None:
    if not isinstance(value, dict):
        raise ConfigError(f"{name} must be a TOML table")


def validate_top_level_keys(data: dict[str, Any], allowed: set[str], name: str) -> None:
    extra = sorted(set(data) - allowed)
    if extra:
        raise ConfigError(f"Unsupported keys in {name}: {', '.join(extra)}")


def validate_ruleset_table(data: dict[str, Any], name: str, allow_pull_request: bool) -> None:
    allowed = {
        "name",
        "enforcement",
        "bypass",
        "restrict_creations",
        "restrict_updates",
        "update_allows_fetch_and_merge",
        "restrict_deletions",
        "require_linear_history",
        "require_signed_commits",
        "block_force_pushes",
        "status_checks",
    }
    if allow_pull_request:
        allowed |= {"require_pull_request", "pull_request"}

    validate_top_level_keys(data, allowed, name)

    bypass = data.get("bypass", [])
    if bypass is not None and not isinstance(bypass, list):
        raise ConfigError(f"{name}.bypass must be an array of inline tables")
    for idx, actor in enumerate(bypass):
        if not isinstance(actor, dict):
            raise ConfigError(f"{name}.bypass[{idx}] must be an inline table")

    if allow_pull_request:
        pull_request = data.get("pull_request", {})
        require_mapping(pull_request, f"{name}.pull_request")
        validate_top_level_keys(
            pull_request,
            {
                "allowed_merge_methods",
                "dismiss_stale_reviews_on_push",
                "require_code_owner_review",
                "require_last_push_approval",
                "required_approving_review_count",
                "required_review_thread_resolution",
            },
            f"{name}.pull_request",
        )

    status_checks = data.get("status_checks", {})
    require_mapping(status_checks, f"{name}.status_checks")
    validate_top_level_keys(status_checks, SUPPORTED_STATUS_CHECK_KEYS, f"{name}.status_checks")


def validate_pages_table(data: dict[str, Any]) -> None:
    build_type = data.get("build_type")
    if build_type is not None and build_type not in SUPPORTED_PAGES_BUILD_TYPES:
        allowed = ", ".join(sorted(SUPPORTED_PAGES_BUILD_TYPES))
        raise ConfigError(f"pages.build_type must be one of: {allowed}")


def resolve_token() -> str:
    for env_name in ("GH_TOKEN", "GITHUB_TOKEN"):
        token = os.environ.get(env_name)
        if token:
            return token.strip()

    result = subprocess.run(
        ["gh", "auth", "token"],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode == 0 and result.stdout.strip():
        return result.stdout.strip()

    raise ConfigError(
        "No GitHub token available. Set GH_TOKEN/GITHUB_TOKEN or authenticate with `gh auth login`."
    )


def split_repo(repo: str) -> tuple[str, str]:
    if repo.count("/") != 1:
        raise ConfigError(f"--repo must be OWNER/REPO, got {repo!r}")
    owner, name = repo.split("/", 1)
    if not owner or not name:
        raise ConfigError(f"--repo must be OWNER/REPO, got {repo!r}")
    return owner, name


def build_plan(api: GitHubApi, owner: str, repo: str, config: dict[str, Any], mode: str, hostname: str) -> PlanResult:
    changes: list[PlannedChange] = []
    unsupported: list[UnsupportedSetting] = []
    warnings: list[str] = []

    current_repo = api.get_repository(owner, repo)
    is_private_repo = bool(current_repo.get("private"))

    desired_repo_patch = build_repository_patch(config.get("repository", {}))
    current_repo_subset = normalize_repository_state(current_repo, desired_repo_patch.keys())
    repo_patch = {
        key: value
        for key, value in desired_repo_patch.items()
        if current_repo_subset.get(key) != value
    }
    if repo_patch:
        changes.append(
            PlannedChange(
                key="repository.settings",
                description="Update general repository settings",
                current=current_repo_subset,
                desired={**current_repo_subset, **repo_patch},
                operation={"kind": "patch_repository", "payload": repo_patch},
            )
        )

    security_cfg = config.get("security", {})
    dependency_graph_value = coerce_dependency_graph_setting(security_cfg)
    toggle_desired = {
        "immutable_releases": config.get("repository", {}).get("immutable_releases"),
        "private_vulnerability_reporting": security_cfg.get("private_vulnerability_reporting"),
        "vulnerability_alerts": dependency_graph_value,
        "automated_security_fixes": security_cfg.get("automated_security_fixes"),
    }

    advanced_security = security_cfg.get("advanced_security")
    if advanced_security is not None:
        if not is_private_repo:
            if not bool(advanced_security):
                unsupported.append(
                    UnsupportedSetting(
                        key="security.advanced_security",
                        desired=bool(advanced_security),
                        reason="GitHub Advanced Security is always available on public repositories and cannot be disabled through the repository REST API.",
                    )
                )
        else:
            current_status = (((current_repo.get("security_and_analysis") or {}).get("advanced_security") or {}).get("status"))
            current_enabled = current_status == "enabled"
            if current_enabled != bool(advanced_security):
                changes.append(
                    PlannedChange(
                        key="security.advanced_security",
                        description="Update GitHub Advanced Security",
                        current=current_enabled,
                        desired=bool(advanced_security),
                        operation={
                            "kind": "patch_repository",
                            "payload": {
                                "security_and_analysis": {
                                    "advanced_security": {
                                        "status": "enabled" if advanced_security else "disabled"
                                    }
                                }
                            },
                        },
                    )
                )

    if dependency_graph_value is not None:
        desired_label = "dependency graph and vulnerability alerts"
        current_enabled = api.get_toggle_state(owner, repo, "vulnerability_alerts")
        if current_enabled != dependency_graph_value:
            changes.append(
                PlannedChange(
                    key="security.vulnerability_alerts",
                    description=f"Update {desired_label}",
                    current=current_enabled,
                    desired=dependency_graph_value,
                    operation={
                        "kind": "toggle",
                        "toggle": "vulnerability_alerts",
                        "enabled": dependency_graph_value,
                    },
                )
            )

    for toggle_name, desired_value in toggle_desired.items():
        if toggle_name == "vulnerability_alerts" or desired_value is None:
            continue
        current_enabled = api.get_toggle_state(owner, repo, toggle_name)
        if current_enabled != bool(desired_value):
            changes.append(
                PlannedChange(
                    key=f"security.{toggle_name}",
                    description=f"Update {toggle_name.replace('_', ' ')}",
                    current=current_enabled,
                    desired=bool(desired_value),
                    operation={
                        "kind": "toggle",
                        "toggle": toggle_name,
                        "enabled": bool(desired_value),
                    },
                )
            )

    pages_config = config.get("pages", {})
    if pages_config:
        changes.extend(plan_pages_changes(api, owner, repo, pages_config))

    unsupported.extend(extract_unsupported_settings(config))

    desired_branch_ruleset, branch_unsupported, branch_rule_types = build_ruleset_payload(
        api,
        owner,
        repo,
        config.get("rulesets", {}).get("branch_default", {}),
        target="branch",
        repo_config=config.get("repository", {}),
    )
    desired_tag_ruleset, tag_unsupported, tag_rule_types = build_ruleset_payload(
        api,
        owner,
        repo,
        config.get("rulesets", {}).get("tags_default", {}),
        target="tag",
        repo_config=config.get("repository", {}),
    )
    unsupported.extend(branch_unsupported)
    unsupported.extend(tag_unsupported)

    ruleset_details = get_managed_rulesets(api, owner, repo, [desired_branch_ruleset, desired_tag_ruleset])
    changes.extend(
        plan_ruleset_changes(
            current_rulesets=ruleset_details,
            desired_rulesets=[
                (desired_branch_ruleset, branch_rule_types),
                (desired_tag_ruleset, tag_rule_types),
            ],
        )
    )

    return PlanResult(
        repo=f"{owner}/{repo}",
        hostname=hostname,
        mode=mode,
        changes=changes,
        unsupported=unsupported,
        warnings=warnings,
    )


def build_repository_patch(repository_config: dict[str, Any]) -> dict[str, Any]:
    payload: dict[str, Any] = {}
    for manifest_key, api_key in SUPPORTED_REPOSITORY_FIELDS.items():
        if manifest_key in repository_config:
            payload[api_key] = repository_config[manifest_key]
    return payload


def normalize_repository_state(repo_data: dict[str, Any], keys: Any) -> dict[str, Any]:
    normalized: dict[str, Any] = {}
    for key in keys:
        normalized[key] = repo_data.get(key)
    return normalized


def coerce_dependency_graph_setting(security_config: dict[str, Any]) -> bool | None:
    dependency_graph = security_config.get("dependency_graph")
    vulnerability_alerts = security_config.get("vulnerability_alerts")

    if dependency_graph is None and vulnerability_alerts is None:
        return None

    if dependency_graph is not None and vulnerability_alerts is not None and dependency_graph != vulnerability_alerts:
        raise ConfigError(
            "security.dependency_graph and security.vulnerability_alerts must match because GitHub manages them through the same repository toggle"
        )

    return bool(vulnerability_alerts if vulnerability_alerts is not None else dependency_graph)


def plan_pages_changes(api: GitHubApi, owner: str, repo: str, pages_config: dict[str, Any]) -> list[PlannedChange]:
    desired = build_pages_state(pages_config)
    current = api.get_pages_site(owner, repo)

    if current is None:
        update_payload = {
            key: value
            for key, value in desired.items()
            if key not in {"build_type"}
        }
        return [
            PlannedChange(
                key="pages.site",
                description="Create GitHub Pages site",
                current=None,
                desired=desired,
                operation={
                    "kind": "create_pages",
                    "create_payload": {"build_type": desired["build_type"]},
                    "update_payload": update_payload,
                },
            )
        ]

    current_normalized = normalize_pages_state(current, desired.keys())
    if current_normalized == desired:
        return []

    return [
        PlannedChange(
            key="pages.site",
            description="Update GitHub Pages site",
            current=current_normalized,
            desired=desired,
            operation={"kind": "update_pages", "payload": desired},
        )
    ]


def build_pages_state(pages_config: dict[str, Any]) -> dict[str, Any]:
    return {
        "build_type": pages_config.get("build_type", "workflow"),
        **(
            {"https_enforced": bool(pages_config["https_enforced"])}
            if "https_enforced" in pages_config
            else {}
        ),
    }


def normalize_pages_state(current: dict[str, Any], keys: Any) -> dict[str, Any]:
    return {key: current.get(key) for key in keys}


def extract_unsupported_settings(config: dict[str, Any]) -> list[UnsupportedSetting]:
    items: list[UnsupportedSetting] = []
    for key, desired in config.get("unsupported", {}).items():
        reason = UNSUPPORTED_REASONS.get(key, "This setting is not applied by the v1 repository configurator.")
        items.append(UnsupportedSetting(key=key, desired=desired, reason=reason))
    return items


def build_ruleset_payload(
    api: GitHubApi,
    owner: str,
    repo: str,
    ruleset_config: dict[str, Any],
    target: str,
    repo_config: dict[str, Any],
) -> tuple[dict[str, Any], list[UnsupportedSetting], set[str]]:
    unsupported: list[UnsupportedSetting] = []
    bypass_actors = resolve_bypass_actors(api, owner, repo, ruleset_config.get("bypass", []))

    rule_types_under_management: set[str] = {
        "creation",
        "update",
        "deletion",
        "required_linear_history",
        "required_signatures",
        "non_fast_forward",
    }
    if target == "branch":
        rule_types_under_management.add("pull_request")
    rule_types_under_management.add("required_status_checks")

    rules: list[dict[str, Any]] = []
    if ruleset_config.get("restrict_creations", False):
        rules.append({"type": "creation"})
    if ruleset_config.get("restrict_updates", False):
        rules.append(
            {
                "type": "update",
                "parameters": {
                    "update_allows_fetch_and_merge": bool(
                        ruleset_config.get("update_allows_fetch_and_merge", False)
                    )
                },
            }
        )
    if ruleset_config.get("restrict_deletions", False):
        rules.append({"type": "deletion"})
    if ruleset_config.get("require_linear_history", False):
        rules.append({"type": "required_linear_history"})
    if ruleset_config.get("require_signed_commits", False):
        rules.append({"type": "required_signatures"})
    if ruleset_config.get("block_force_pushes", False):
        rules.append({"type": "non_fast_forward"})

    if target == "branch":
        if ruleset_config.get("require_pull_request", False):
            pull_request_config = ruleset_config.get("pull_request", {})
            allowed_merge_methods = pull_request_config.get("allowed_merge_methods")
            if not allowed_merge_methods:
                allowed_merge_methods = derive_allowed_merge_methods(repo_config)
            if not allowed_merge_methods:
                raise ConfigError(
                    "rulesets.branch_default.pull_request.allowed_merge_methods is empty and could not be derived from repository merge settings"
                )
            rules.append(
                {
                    "type": "pull_request",
                    "parameters": {
                        "allowed_merge_methods": allowed_merge_methods,
                        "dismiss_stale_reviews_on_push": bool(
                            pull_request_config.get("dismiss_stale_reviews_on_push", False)
                        ),
                        "require_code_owner_review": bool(
                            pull_request_config.get("require_code_owner_review", False)
                        ),
                        "require_last_push_approval": bool(
                            pull_request_config.get("require_last_push_approval", False)
                        ),
                        "required_approving_review_count": int(
                            pull_request_config.get("required_approving_review_count", 0)
                        ),
                        "required_review_thread_resolution": bool(
                            pull_request_config.get("required_review_thread_resolution", False)
                        ),
                    },
                }
            )

    status_checks_config = ruleset_config.get("status_checks", {})
    if status_checks_config.get("enabled", False):
        contexts = normalize_status_checks(status_checks_config.get("contexts", []))
        if contexts:
            rules.append(
                {
                    "type": "required_status_checks",
                    "parameters": {
                        "do_not_enforce_on_create": bool(
                            status_checks_config.get("do_not_enforce_on_create", False)
                        ),
                        "required_status_checks": contexts,
                        "strict_required_status_checks_policy": bool(
                            status_checks_config.get("strict_required_status_checks_policy", False)
                        ),
                    },
                }
            )
        else:
            unsupported.append(
                UnsupportedSetting(
                    key=f"rulesets.{ 'branch_default' if target == 'branch' else 'tags_default' }.status_checks",
                    desired=status_checks_config,
                    reason="Status checks are enabled in the manifest but no check contexts were configured, so the rule cannot be applied safely.",
                )
            )
            rule_types_under_management.remove("required_status_checks")

    include = ["~DEFAULT_BRANCH"] if target == "branch" else ["~ALL"]
    payload = {
        "name": ruleset_config.get("name", "Default branch" if target == "branch" else "Default tags"),
        "target": target,
        "enforcement": ruleset_config.get("enforcement", "active"),
        "bypass_actors": bypass_actors,
        "conditions": {"ref_name": {"include": include, "exclude": []}},
        "rules": rules,
    }
    return payload, unsupported, rule_types_under_management


def derive_allowed_merge_methods(repository_config: dict[str, Any]) -> list[str]:
    allowed: list[str] = []
    if repository_config.get("allow_merge_commit"):
        allowed.append("merge")
    if repository_config.get("allow_squash_merge"):
        allowed.append("squash")
    if repository_config.get("allow_rebase_merge"):
        allowed.append("rebase")
    return allowed


def normalize_status_checks(values: list[Any]) -> list[dict[str, Any]]:
    normalized: list[dict[str, Any]] = []
    if not isinstance(values, list):
        raise ConfigError("status_checks.contexts must be an array")
    for idx, value in enumerate(values):
        if isinstance(value, str):
            normalized.append({"context": value})
        elif isinstance(value, dict):
            if "context" not in value:
                raise ConfigError(f"status_checks.contexts[{idx}] must include a context")
            item = {"context": value["context"]}
            if "integration_id" in value and value["integration_id"] is not None:
                item["integration_id"] = int(value["integration_id"])
            normalized.append(item)
        else:
            raise ConfigError("status_checks.contexts entries must be strings or inline tables")
    normalized.sort(key=lambda item: (item["context"], item.get("integration_id", -1)))
    return normalized


def resolve_bypass_actors(
    api: GitHubApi,
    owner: str,
    repo: str,
    bypass_config: list[dict[str, Any]],
) -> list[dict[str, Any]]:
    actors: list[dict[str, Any]] = []
    for actor in bypass_config:
        actor_type = actor.get("type")
        bypass_mode = actor.get("mode", "always")
        if actor_type == "repository_admin_role":
            actors.append(
                {
                    "actor_type": "RepositoryRole",
                    "actor_id": ADMIN_REPOSITORY_ROLE_ID,
                    "bypass_mode": bypass_mode,
                }
            )
        elif actor_type == "organization_admin":
            actors.append(
                {
                    "actor_type": "OrganizationAdmin",
                    "actor_id": 1,
                    "bypass_mode": bypass_mode,
                }
            )
        elif actor_type == "app":
            slug = actor.get("slug")
            if not slug:
                raise ConfigError("App bypass actors must define a slug")
            actors.append(
                {
                    "actor_type": "Integration",
                    "actor_id": api.resolve_app_actor_id(owner, repo, slug),
                    "bypass_mode": bypass_mode,
                }
            )
        elif actor_type == "integration":
            actor_id = actor.get("actor_id")
            if actor_id is None:
                raise ConfigError("integration bypass actors must define actor_id")
            actors.append(
                {
                    "actor_type": "Integration",
                    "actor_id": int(actor_id),
                    "bypass_mode": bypass_mode,
                }
            )
        else:
            raise ConfigError(f"Unsupported bypass actor type: {actor_type!r}")

    actors.sort(key=lambda item: (item["actor_type"], item.get("actor_id", -1), item["bypass_mode"]))
    return actors


def get_managed_rulesets(
    api: GitHubApi,
    owner: str,
    repo: str,
    desired_rulesets: list[dict[str, Any]],
) -> dict[tuple[str, str], dict[str, Any]]:
    listed = api.list_rulesets(owner, repo)
    managed: dict[tuple[str, str], dict[str, Any]] = {}
    targets = {(item["name"], item["target"]) for item in desired_rulesets}

    for entry in listed:
        key = (entry.get("name"), entry.get("target"))
        if key not in targets:
            continue
        if entry.get("source_type") and entry.get("source_type") != "Repository":
            continue
        ruleset_id = entry.get("id")
        if ruleset_id is None:
            continue
        if key in managed:
            raise ConfigError(f"Multiple repository rulesets found for managed rule {key[0]!r} ({key[1]})")
        managed[key] = api.get_ruleset(owner, repo, int(ruleset_id))
    return managed


def plan_ruleset_changes(
    current_rulesets: dict[tuple[str, str], dict[str, Any]],
    desired_rulesets: list[tuple[dict[str, Any], set[str]]],
) -> list[PlannedChange]:
    changes: list[PlannedChange] = []
    for desired_payload, managed_rule_types in desired_rulesets:
        key = (desired_payload["name"], desired_payload["target"])
        current = current_rulesets.get(key)
        if current is None:
            changes.append(
                PlannedChange(
                    key=f"rulesets.{desired_payload['target']}.{desired_payload['name']}",
                    description=f"Create managed {desired_payload['target']} ruleset {desired_payload['name']!r}",
                    current=None,
                    desired=desired_payload,
                    operation={"kind": "create_ruleset", "payload": desired_payload},
                )
            )
            continue

        desired_normalized = normalize_ruleset_payload(desired_payload, managed_rule_types)
        current_normalized = normalize_ruleset_payload(current, managed_rule_types)
        if desired_normalized != current_normalized:
            changes.append(
                PlannedChange(
                    key=f"rulesets.{desired_payload['target']}.{desired_payload['name']}",
                    description=f"Update managed {desired_payload['target']} ruleset {desired_payload['name']!r}",
                    current=current_normalized,
                    desired=desired_normalized,
                    operation={
                        "kind": "update_ruleset",
                        "ruleset_id": current["id"],
                        "payload": desired_payload,
                    },
                )
            )
    return changes


def normalize_ruleset_payload(payload: dict[str, Any], managed_rule_types: set[str]) -> dict[str, Any]:
    normalized_rules = []
    for rule in payload.get("rules", []):
        rule_type = rule.get("type")
        if rule_type not in managed_rule_types:
            continue
        normalized_rules.append(normalize_rule(rule))
    normalized_rules.sort(key=lambda item: item["type"])

    normalized_actors = [
        {
            "actor_type": actor["actor_type"],
            "actor_id": actor.get("actor_id"),
            "bypass_mode": actor.get("bypass_mode", "always"),
        }
        for actor in payload.get("bypass_actors", [])
    ]
    normalized_actors.sort(key=lambda actor: (actor["actor_type"], actor.get("actor_id", -1), actor["bypass_mode"]))

    ref_name = (payload.get("conditions") or {}).get("ref_name") or {}
    return {
        "name": payload.get("name"),
        "target": payload.get("target"),
        "enforcement": payload.get("enforcement"),
        "bypass_actors": normalized_actors,
        "conditions": {
            "ref_name": {
                "include": sorted(ref_name.get("include", [])),
                "exclude": sorted(ref_name.get("exclude", [])),
            }
        },
        "rules": normalized_rules,
    }


def normalize_rule(rule: dict[str, Any]) -> dict[str, Any]:
    rule_type = rule.get("type")
    if rule_type in {"creation", "deletion", "required_linear_history", "required_signatures", "non_fast_forward"}:
        return {"type": rule_type}
    if rule_type == "update":
        params = rule.get("parameters") or {}
        return {
            "type": "update",
            "parameters": {
                "update_allows_fetch_and_merge": bool(params.get("update_allows_fetch_and_merge", False))
            },
        }
    if rule_type == "pull_request":
        params = rule.get("parameters") or {}
        return {
            "type": "pull_request",
            "parameters": {
                "allowed_merge_methods": sorted(params.get("allowed_merge_methods", [])),
                "dismiss_stale_reviews_on_push": bool(params.get("dismiss_stale_reviews_on_push", False)),
                "require_code_owner_review": bool(params.get("require_code_owner_review", False)),
                "require_last_push_approval": bool(params.get("require_last_push_approval", False)),
                "required_approving_review_count": int(params.get("required_approving_review_count", 0)),
                "required_review_thread_resolution": bool(params.get("required_review_thread_resolution", False)),
            },
        }
    if rule_type == "required_status_checks":
        params = rule.get("parameters") or {}
        checks = []
        for check in params.get("required_status_checks", []):
            item = {"context": check["context"]}
            if check.get("integration_id") is not None:
                item["integration_id"] = int(check["integration_id"])
            checks.append(item)
        checks.sort(key=lambda item: (item["context"], item.get("integration_id", -1)))
        return {
            "type": "required_status_checks",
            "parameters": {
                "do_not_enforce_on_create": bool(params.get("do_not_enforce_on_create", False)),
                "required_status_checks": checks,
                "strict_required_status_checks_policy": bool(
                    params.get("strict_required_status_checks_policy", False)
                ),
            },
        }
    return {"type": rule_type}


def apply_plan(api: GitHubApi, owner: str, repo: str, plan: PlanResult) -> list[str]:
    applied: list[str] = []
    for change in plan.changes:
        operation = change.operation
        kind = operation["kind"]
        if kind == "patch_repository":
            api.update_repository(owner, repo, operation["payload"])
        elif kind == "toggle":
            api.set_toggle_state(owner, repo, operation["toggle"], bool(operation["enabled"]))
        elif kind == "create_ruleset":
            api.create_ruleset(owner, repo, operation["payload"])
        elif kind == "update_ruleset":
            api.update_ruleset(owner, repo, int(operation["ruleset_id"]), operation["payload"])
        elif kind == "create_pages":
            api.create_pages_site(owner, repo, operation["create_payload"])
            if operation.get("update_payload"):
                api.update_pages_site(owner, repo, operation["update_payload"])
        elif kind == "update_pages":
            api.update_pages_site(owner, repo, operation["payload"])
        else:
            raise ConfigError(f"Unsupported operation kind: {kind}")
        applied.append(change.description)
    return applied


def print_report(plan: PlanResult, applied: list[str] | None = None) -> None:
    if plan.changes:
        header = "Applied changes" if applied is not None else "Planned changes"
        print(header + ":")
        for change in plan.changes:
            print(f"- {change.description}")
    else:
        print("No supported changes are required.")

    if applied is not None and applied:
        print(f"\nApplied {len(applied)} change(s).")

    if plan.unsupported:
        print("\nUnsupported or manual follow-ups:")
        for item in plan.unsupported:
            print(f"- {item.key}: {item.reason} (desired={json.dumps(item.desired, sort_keys=True)})")

    if plan.warnings:
        print("\nWarnings:")
        for warning in plan.warnings:
            print(f"- {warning}")


def write_json_report(path: Path, plan: PlanResult, applied: list[str] | None = None) -> None:
    payload = plan.to_json()
    if applied is not None:
        payload["applied_changes"] = applied
    path.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    owner, repo = split_repo(args.repo)
    config_path = Path(args.config)

    try:
        config = load_manifest(config_path)
        token = resolve_token()
        api = GitHubApi(token=token, hostname=args.hostname)
        plan = build_plan(api, owner, repo, config, args.mode, args.hostname)
        applied: list[str] | None = None
        if args.mode == "apply":
            applied = apply_plan(api, owner, repo, plan)
        print_report(plan, applied)
        if args.json_report:
            write_json_report(Path(args.json_report), plan, applied)
        return 0
    except (ConfigError, ApiError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
