# Changelog

## 0.1.0 (2026-08-26)


### ⚠ BREAKING CHANGES

* **execution:** Capability names whose top-level segment is a standard Starlark universe name, sum, json, or math now fail Build with ErrInvalidRegistration.

### Features

* add bounded relevance-ranked search ([#44](https://github.com/meigma/codemode/issues/44)) ([f50f113](https://github.com/meigma/codemode/commit/f50f113181a0333eabf094a3ee22932e49dcd01a))
* add capability contracts and catalog ([#7](https://github.com/meigma/codemode/issues/7)) ([830d70d](https://github.com/meigma/codemode/commit/830d70dff955e286428c7de7a99b455417a14e1d))
* add MCP server adapter ([#9](https://github.com/meigma/codemode/issues/9)) ([5eb0b1f](https://github.com/meigma/codemode/commit/5eb0b1f46596fc2cc3457eb2530eccfa49f62b11))
* add secure execution server ([#8](https://github.com/meigma/codemode/issues/8)) ([0f3044e](https://github.com/meigma/codemode/commit/0f3044e4c118ec09342bdb44c3411ceadce12c1e))
* **api:** reduce first-touch server ceremony ([#34](https://github.com/meigma/codemode/issues/34)) ([715a716](https://github.com/meigma/codemode/commit/715a716bd6bd5d436368db6367310452960fa8d6))
* **authz:** add Rego authorizer adapter ([#14](https://github.com/meigma/codemode/issues/14)) ([aac86bc](https://github.com/meigma/codemode/commit/aac86bc8cf4bcebb91e9257767fe6c31981847bd))
* **binding:** support composite capability outputs ([#37](https://github.com/meigma/codemode/issues/37)) ([0c66e5e](https://github.com/meigma/codemode/commit/0c66e5e1f86822bc6f889fb186ecede0c36089b5))
* **binding:** widen scalar capability inputs ([#35](https://github.com/meigma/codemode/issues/35)) ([004538c](https://github.com/meigma/codemode/commit/004538cb8694001ae7231f7284e677b57be07473))
* **diagnostics:** echo safe model-derived error detail ([#43](https://github.com/meigma/codemode/issues/43)) ([4bbb367](https://github.com/meigma/codemode/commit/4bbb36734256501a8e7d204b556addd8c65e9350))
* **execution:** add fixed compute stdlib ([#46](https://github.com/meigma/codemode/issues/46)) ([344b79b](https://github.com/meigma/codemode/commit/344b79b48adb040914b8f7dfa8b22b7b35b1272b))
* **mcp:** publish model authoring guidance ([#19](https://github.com/meigma/codemode/issues/19)) ([f1d3736](https://github.com/meigma/codemode/commit/f1d373648024d97f3d3fc46e9b9c5bcb4f0f5bf8))
* **worker:** add bounded protocol framing ([#29](https://github.com/meigma/codemode/issues/29)) ([ca0a490](https://github.com/meigma/codemode/commit/ca0a490aa633e480e501e0dc48cab4ca19dbe798))
* **worker:** add worker process supervision ([#30](https://github.com/meigma/codemode/issues/30)) ([0dee26a](https://github.com/meigma/codemode/commit/0dee26adf7fcd938db3488d06263e987c86f8e8d))
* **worker:** bound intermediate native results ([#36](https://github.com/meigma/codemode/issues/36)) ([493d8fa](https://github.com/meigma/codemode/commit/493d8fa71b3b09634cb3dd23f41afdb9869dd64b))
* **worker:** require fresh-process execution ([#31](https://github.com/meigma/codemode/issues/31)) ([3b541fc](https://github.com/meigma/codemode/commit/3b541fca34a0fdf3bcf5775fbebe7de1bd5a51df))


### Bug Fixes

* **api:** make capability signatures invocation-only ([#20](https://github.com/meigma/codemode/issues/20)) ([8b5302b](https://github.com/meigma/codemode/commit/8b5302b9a4bcab1dbbf78283862c54ef9bce8efa))
* **authz:** distinguish invalid Rego decisions ([#16](https://github.com/meigma/codemode/issues/16)) ([057faa8](https://github.com/meigma/codemode/commit/057faa8cf5ba112d73f88f25ebbcc56d3ccaf9ed))
* **binding:** reject oversized final-value allocations ([#26](https://github.com/meigma/codemode/issues/26)) ([e2b1e56](https://github.com/meigma/codemode/commit/e2b1e5641bd0800e2f88667e294debe69d84f5f7))
* **mcp:** advertise exact output schemas ([#21](https://github.com/meigma/codemode/issues/21)) ([7c77a80](https://github.com/meigma/codemode/commit/7c77a8027b29f1ea3aa10d83e704b524b499fce7))

## Changelog
