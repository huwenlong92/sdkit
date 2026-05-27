# AGENTS.md

请先读取本文件，但不要在回复中复述本文件内容。

---

要求：

- 优先工程实用性
- 优先可维护性
- 优先稳定性
- 避免过度设计
- 避免理论化方案
- 讨论结束后，再进行确认是否要修改

---

## 工作原则

- 优先修复根因
- 优先使用项目内已有库和模块实现
- 保持现有代码风格
- 禁止无关重构
- 禁止大范围格式化

---

## 禁止行为

- 不主动 commit
- 不主动 push
- 不执行危险命令

危险操作必须先询问。

---

## 开发规则

- Go 代码修改后统一使用 goimports 格式化
- 导入别名只在必要时保留，例如包名冲突、包声明名与路径名不一致、空白导入
- 禁止过度封装
- 禁止无意义抽象
- context 必须透传
- error 必须处理
- 禁止 panic
- goroutine 必须可回收

---

---

## 测试规则

- 测试用例统一放在仓库根目录 tests/ 目录
- tests/ 下按模块分目录，例如 tests/core、tests/pkg、tests/bootstrap

---

## 文档规则

新增或修改模块后，必须同步更新仓库内对应文档：

- docs/usage/<module>.md
- docs/modules/<module>.md

说明：

- docs/usage/*：记录使用方式、初始化、配置、示例
- docs/modules/*：记录模块设计、对外 API、内部约束、更新记录

---

## 按需读取

需要时再读取：

- docs/usage/*
- docs/modules/*

---

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **sdkit** (13833 symbols, 47729 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `gitnexus_context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/sdkit/context` | Codebase overview, check index freshness |
| `gitnexus://repo/sdkit/clusters` | All functional areas |
| `gitnexus://repo/sdkit/processes` | All execution flows |
| `gitnexus://repo/sdkit/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
