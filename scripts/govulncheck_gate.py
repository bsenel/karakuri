#!/usr/bin/env python3
"""Run govulncheck over each module and fail on any reachable advisory not excepted.

govulncheck has no way to accept a single advisory, so this reads its JSON
stream and applies .govulncheck-exceptions. An exception names one advisory and
the date it lapses on; a lapsed exception fails like no exception at all, so an
accepted risk cannot quietly become a permanent one.

Usage: scripts/govulncheck_gate.py [module-dir ...]   (default: .)
"""

import datetime
import json
import pathlib
import subprocess
import sys

EXCEPTIONS_FILE = pathlib.Path(__file__).resolve().parent.parent / ".govulncheck-exceptions"


def load_exceptions(path: pathlib.Path) -> dict[str, tuple[datetime.date, str]]:
    """Read `<advisory> <YYYY-MM-DD> <reason>` lines; blank lines and # comments are skipped."""
    exceptions: dict[str, tuple[datetime.date, str]] = {}
    if not path.exists():
        return exceptions
    for number, line in enumerate(path.read_text().splitlines(), start=1):
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split(maxsplit=2)
        if len(parts) < 3:
            sys.exit(f"{path.name}:{number}: want '<advisory> <YYYY-MM-DD> <reason>', got {line!r}")
        advisory, expires, reason = parts
        try:
            exceptions[advisory] = (datetime.date.fromisoformat(expires), reason)
        except ValueError:
            sys.exit(f"{path.name}:{number}: {expires!r} is not a YYYY-MM-DD date")
    return exceptions


def reachable_advisories(module_dir: str) -> dict[str, str]:
    """Return {advisory: summary} for every advisory whose vulnerable symbol the module calls.

    A finding whose first trace frame names a function is one govulncheck's text
    mode reports as "your code is affected"; module- and package-level findings
    are not reachable and do not fail the text mode either.
    """
    result = subprocess.run(
        ["govulncheck", "-format", "json", "./..."],
        cwd=module_dir,
        capture_output=True,
        text=True,
        check=False,
    )
    # In JSON mode govulncheck exits 0 whatever it finds, so a non-zero exit is
    # a scan that did not run, and a scan that did not run must not pass.
    if result.returncode != 0:
        sys.stderr.write(result.stderr)
        sys.exit(f"govulncheck failed in {module_dir} (exit {result.returncode})")

    summaries: dict[str, str] = {}
    reachable: set[str] = set()
    decoder = json.JSONDecoder()
    stream, position = result.stdout, 0
    while position < len(stream):
        if stream[position].isspace():
            position += 1
            continue
        message, position = decoder.raw_decode(stream, position)
        if "osv" in message:
            summaries[message["osv"]["id"]] = message["osv"].get("summary", "")
        finding = message.get("finding")
        if finding and finding.get("trace") and finding["trace"][0].get("function"):
            reachable.add(finding["osv"])
    return {advisory: summaries.get(advisory, "") for advisory in reachable}


def main(module_dirs: list[str]) -> int:
    today = datetime.datetime.now(datetime.timezone.utc).date()
    exceptions = load_exceptions(EXCEPTIONS_FILE)
    seen: set[str] = set()
    failed = False

    for module_dir in module_dirs or ["."]:
        for advisory, summary in sorted(reachable_advisories(module_dir).items()):
            seen.add(advisory)
            exception = exceptions.get(advisory)
            if exception is None:
                print(f"::error::{module_dir}: {advisory} is reachable: {summary}")
                failed = True
            elif exception[0] < today:
                print(f"::error::{module_dir}: {advisory} is reachable and its exception lapsed on {exception[0]}: {exception[1]}")
                failed = True
            else:
                print(f"::warning::{module_dir}: {advisory} is reachable, excepted until {exception[0]}: {exception[1]}")

    for advisory in sorted(set(exceptions) - seen):
        print(f"::notice::{advisory} is no longer reachable; remove it from {EXCEPTIONS_FILE.name}")

    if not failed:
        print("govulncheck gate: no reachable advisory without a current exception")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
