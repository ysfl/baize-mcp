#!/usr/bin/env python3
"""
校验 Git 提交信息是否符合项目规范。
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

try:
    from validate_public_release_text import validate_text as validate_public_text
except ImportError:  # pragma: no cover - 仅在单文件复制部署时触发
    validate_public_text = None


ALLOWED_TYPES = {
    "feat",
    "fix",
    "docs",
    "style",
    "refactor",
    "perf",
    "test",
    "build",
    "ci",
}
HEADER_PATTERN = re.compile(
    r"^(?P<type>[a-z]+)(?:\((?P<scope>[a-z0-9][a-z0-9._/-]*)\))?(?P<breaking>!)?: (?P<subject>.+)$"
)
REQUIRED_SECTIONS = ("背景：", "变更：", "验证：")
SUBJECT_MAX_LENGTH = 50


class CommitMessageError(Exception):
    pass


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="校验提交信息格式。")
    parser.add_argument("--message-file", help="commit-msg hook 传入的消息文件路径。")
    parser.add_argument(
        "--commit",
        action="append",
        default=[],
        help="待校验的 commit SHA，可重复传入。",
    )
    parser.add_argument(
        "--stdin",
        action="store_true",
        help="从标准输入读取提交信息（供 pre-receive hook 使用）。",
    )
    parser.add_argument(
        "--audience",
        choices=("internal", "public"),
        default="internal",
        help="提交受众；public 额外执行产品化语言和内部信息过滤。",
    )
    return parser.parse_args()


def read_message_file(path: Path) -> str:
    if not path.exists():
        raise CommitMessageError(f"缺少提交信息文件：{path}")
    return path.read_text(encoding="utf-8")


def read_commit_message(commit: str) -> str:
    result = subprocess.run(
        ["git", "log", "-1", "--format=%B", commit],
        check=True,
        capture_output=True,
        text=True,
    )
    return result.stdout


def normalize_message(raw: str) -> list[str]:
    lines = []
    for line in raw.splitlines():
        if line.startswith("#"):
            continue
        lines.append(line.rstrip())
    while lines and not lines[-1].strip():
        lines.pop()
    return lines


def validate_header(header: str) -> list[str]:
    issues: list[str] = []
    if header.startswith("Merge ") or header.startswith("Revert "):
        return issues
    match = HEADER_PATTERN.match(header)
    if not match:
        issues.append("标题未使用 Conventional Commits 格式。")
        return issues
    commit_type = match.group("type")
    subject = match.group("subject").strip()
    if commit_type not in ALLOWED_TYPES:
        issues.append(f"不支持的 type：`{commit_type}`。")
    if not subject:
        issues.append("标题 subject 不能为空。")
        return issues
    # subject 按 UTF-8 字节数计（中文字符 3 字节），上限 50 字节；字节口径
    # 与显示宽度不同，超宽中文标题容易被误判为合规。
    subject_bytes = len(subject.encode("utf-8"))
    if subject_bytes > SUBJECT_MAX_LENGTH:
        issues.append(
            f"标题 subject 过长，当前 {subject_bytes} 字节，限制 {SUBJECT_MAX_LENGTH} 字节。"
        )
    if subject.endswith((".", "。")):
        issues.append("标题 subject 不应以句号结尾。")
    return issues


def validate_body(lines: list[str]) -> list[str]:
    issues: list[str] = []
    if len(lines) < 3:
        return ["提交信息缺少正文，必须填写 description/body。"]
    if lines[1].strip():
        issues.append("标题与正文之间必须保留一行空行。")
    body_lines = [line for line in lines[2:] if line.strip()]
    if not body_lines:
        issues.append("提交信息缺少正文，必须填写 description/body。")
        return issues
    for section in REQUIRED_SECTIONS:
        section_index = next(
            (index for index, line in enumerate(body_lines) if line.startswith(section)),
            -1,
        )
        if section_index < 0:
            issues.append(f"提交正文缺少必填章节：`{section}`")
            continue

        # 规范「标准格式」要求三段标记独占一行、说明用 `- ` 条目书写；
        # 行内续写会让章节边界无法可靠切分，也偏离提交模板。
        content_after = body_lines[section_index][len(section):].strip()
        if content_after:
            issues.append(f"必填章节 `{section}` 的标记必须独占一行，说明另起一行书写。")

        next_section_index = next(
            (
                index
                for index in range(section_index + 1, len(body_lines))
                if any(body_lines[index].startswith(item) for item in REQUIRED_SECTIONS)
            ),
            len(body_lines),
        )
        section_content = [
            line.strip()
            for line in body_lines[section_index + 1:next_section_index]
            if line.strip()
        ]
        if not section_content:
            issues.append(f"必填章节 `{section}` 后缺少实际内容。")
            continue
        if not any(line.startswith("-") for line in section_content):
            issues.append(f"必填章节 `{section}` 的内容必须使用 `- ` 条目书写。")
    return issues


def validate_message(raw: str, source: str, audience: str = "internal") -> list[str]:
    lines = normalize_message(raw)
    if not lines:
        return [f"{source}: 提交信息为空。"]
    header = lines[0].strip()
    issues = [f"{source}: {item}" for item in validate_header(header)]
    issues.extend(f"{source}: {item}" for item in validate_body(lines))
    if audience == "public":
        if validate_public_text is None:
            issues.append(f"{source}: 缺少公开内容过滤器，无法验证对外提交。")
        else:
            issues.extend(validate_public_text(source, raw, False, "commit"))
    return issues


def print_fix_guide(audience: str = "internal") -> None:
    print("修复建议：", file=sys.stderr)
    print("- 使用如下提交模板：", file=sys.stderr)
    print("  <type>(<scope>): <subject>", file=sys.stderr)
    print("", file=sys.stderr)
    print("  背景：", file=sys.stderr)
    print("  - 说明为什么要改", file=sys.stderr)
    print("", file=sys.stderr)
    print("  变更：", file=sys.stderr)
    print("  - 说明本次提交做了什么", file=sys.stderr)
    print("", file=sys.stderr)
    print("  验证：", file=sys.stderr)
    print("  - 说明验证命令与结果", file=sys.stderr)
    print("- 若提交已创建可执行：`git commit --amend`", file=sys.stderr)
    print("- 安装钩子：将 core.hooksPath 指向本仓 .githooks 目录。", file=sys.stderr)
    if audience == "public":
        print("- 对外提交只描述用户可感知的变化，不写仓库身份、内部流程或具体校验命令。", file=sys.stderr)


def main() -> int:
    args = parse_args()
    if not args.message_file and not args.commit and not args.stdin:
        raise CommitMessageError("必须传入 `--message-file`、`--commit` 或 `--stdin`。")

    all_issues: list[str] = []

    if args.stdin:
        message = sys.stdin.read()
        all_issues.extend(validate_message(message, "标准输入", args.audience))

    if args.message_file:
        message = read_message_file(Path(args.message_file).expanduser().resolve())
        all_issues.extend(validate_message(message, "提交消息文件", args.audience))

    for commit in args.commit:
        message = read_commit_message(commit)
        all_issues.extend(validate_message(message, f"提交 `{commit}`", args.audience))

    if all_issues:
        print("提交信息校验失败：", file=sys.stderr)
        for issue in all_issues:
            print(f"- {issue}", file=sys.stderr)
        print_fix_guide(args.audience)
        return 1

    print("提交信息校验通过。")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (CommitMessageError, subprocess.CalledProcessError) as error:
        print(f"校验失败：{error}", file=sys.stderr)
        sys.exit(1)
