"""Whether nothing a workflow's push paths cover changed between two commits: a push that changes none of them starts
no run of that workflow, so its run on the first commit stands for the second. Reads the workflow's `paths` list
(GitHub's globs: ** any characters, * any but /; the last pattern a file matches decides). Prints what changed and
exits 1 when something did.
Usage: python3 packaging/release/untouched.py .github/workflows/ci.yml <base> <head>"""
import re
import subprocess
import sys


def patterns(workflow):
    out, inside = [], False
    for line in open(workflow, encoding="utf-8"):
        if line.strip() == "paths:":
            inside = True
            continue
        if inside:
            m = re.match(r"\s+- '([^']+)'\s*$", line)
            if not m:
                break
            out.append(m.group(1))
    if not out:
        sys.exit(f"{workflow}: no paths list")
    return out


def glob(pattern):
    tokens = re.findall(r"\*\*|\*|[^*]+", pattern)
    return re.compile("".join(".*" if t == "**" else "[^/]*" if t == "*" else re.escape(t) for t in tokens) + "$")


def covered(rules, path):
    hit = False
    for negative, rx in rules:
        if rx.match(path):
            hit = not negative
    return hit


def main():
    workflow, base, head = sys.argv[1:]
    rules = [(p.startswith("!"), glob(p.removeprefix("!"))) for p in patterns(workflow)]
    # Both sides of a rename: a file moved out of the paths changed what they cover too.
    diff = subprocess.run(["git", "diff", "--no-renames", "--name-only", base, head],
                          check=True, capture_output=True, text=True).stdout
    changed = [f for f in diff.splitlines() if covered(rules, f)]
    for f in changed:
        print(f"{f} changed since {base[:7]}")
    sys.exit(1 if changed else 0)


if __name__ == "__main__":
    main()
