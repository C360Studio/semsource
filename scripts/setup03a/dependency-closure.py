#!/usr/bin/env python3
"""Measure SemStreams production and test closure without building/running tests."""
import argparse
import hashlib
import json
import pathlib
import subprocess

UPSTREAM = "github.com/c360studio/semstreams"
REGISTRIES = {UPSTREAM + "/componentregistry", UPSTREAM + "/payloadbuiltins"}
COMPOSED = {UPSTREAM + "/" + name for name in (
    "processor/graph-ingest", "processor/graph-index", "processor/graph-query",
    "processor/graph-embedding", "gateway/graph-gateway",
)}


def objects(raw):
    decoder = json.JSONDecoder()
    pos = 0
    while pos < len(raw):
        while pos < len(raw) and raw[pos].isspace():
            pos += 1
        if pos == len(raw):
            break
        item, pos = decoder.raw_decode(raw, pos)
        yield item


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", required=True, type=pathlib.Path)
    parser.add_argument("--out", required=True, type=pathlib.Path)
    parser.add_argument("--test-tags", default="integration,e2e,qualification")
    args = parser.parse_args()
    args.out.mkdir(parents=True, exist_ok=False)
    before = (args.repo / "go.mod").read_bytes()
    commands = []
    # Upstream tests are not normally part of a consumer's go.sum. Resolve only in
    # an evidence-local modfile, recording its exact resulting dependency graph.
    modfile = args.out.resolve() / "closure.mod"
    modfile.write_bytes(before)
    source_sum = args.repo / "go.sum"
    if source_sum.exists():
        modfile.with_suffix(".sum").write_bytes(source_sum.read_bytes())

    def run(label, flags, roots):
        command = ["go", "list", "-mod=mod", "-modfile=" + str(modfile), "-deps", "-json"] + flags + sorted(roots)
        commands.append(command)
        result = subprocess.run(command, cwd=args.repo, capture_output=True, text=True, timeout=300)
        (args.out / (label + ".json")).write_text(result.stdout)
        (args.out / (label + ".stderr")).write_text(result.stderr)
        if result.returncode:
            raise SystemExit(f"{label}: go list failed ({result.returncode}); see saved stderr")
        return list(objects(result.stdout))

    def summarize(packages, roots=None):
        # Synthetic test binaries/variants collapse onto their real package directory.
        retained = {}
        for package in packages:
            if package.get("Module", {}).get("Path") != UPSTREAM:
                continue
            path = package["ImportPath"].split(" [", 1)[0]
            if path.endswith(".test"):
                continue
            # External pkg_test variants share a production directory; count it once.
            relative = pathlib.Path(package["Dir"]).relative_to(package["Module"]["Dir"])
            canonical = UPSTREAM if str(relative) == "." else UPSTREAM + "/" + relative.as_posix()
            retained[canonical] = package["Dir"]
        lines = 0
        for directory in retained.values():
            for source in pathlib.Path(directory).glob("*.go"):
                if not source.name.endswith("_test.go"):
                    lines += len(source.read_bytes().splitlines())
        summary = {"semstreams_packages": len(retained), "non_test_lines": lines,
                   "packages": sorted(retained)}
        if roots is not None:
            summary["roots"] = sorted(roots)
        return summary

    production = run("consumer-production", [], ["./..."])
    direct = set()
    for package in production:
        if package.get("Module", {}).get("Main"):
            direct.update(p for p in package.get("Imports", []) if p == UPSTREAM or p.startswith(UPSTREAM + "/"))
    narrow = direct - REGISTRIES
    port = narrow | COMPOSED
    result = {
        "go_mod_sha256": hashlib.sha256(before).hexdigest(),
        "test_dependency_resolution": "Evidence-local closure.mod/closure.sum; consumer files remain unchanged",
        "package_policy": "Unique upstream source directories; synthetic external-test variants and test binaries are not extra directories",
        "line_policy": "Raw lines from every non-test .go file in each unique upstream package directory, including inactive build-tag files",
        "test_tags": args.test_tags,
        "consumer_production": summarize(production),
        "consumer_tests": summarize(run("consumer-tests", ["-test", "-tags=" + args.test_tags], ["./..."])),
        "direct_imports": summarize(run("direct-imports", [], direct), direct),
        "without_registries": summarize(run("without-registries", [], narrow), narrow),
        "port_production": summarize(run("port-production", [], port), port),
        "port_own_tests": summarize(run("port-own-tests", ["-test"], port), port),
        "port_tagged_tests": summarize(run("port-tagged-tests", ["-test", "-tags=" + args.test_tags], port), port),
    }
    result["commands"] = commands
    modules = subprocess.run(["go", "list", "-mod=readonly", "-modfile=" + str(modfile), "-m", "-json", "all"],
                             cwd=args.repo, capture_output=True, text=True, timeout=300, check=True)
    (args.out / "resolved-modules.json").write_text(modules.stdout)
    if (args.repo / "go.mod").read_bytes() != before:
        raise SystemExit("go.mod changed during measurement; discard comparison and rerun")
    (args.out / "summary.json").write_text(json.dumps(result, indent=2) + "\n")
    for scope, value in result.items():
        if isinstance(value, dict) and "semstreams_packages" in value:
            print(f"{scope}: {value['semstreams_packages']} packages / {value['non_test_lines']} lines", flush=True)


if __name__ == "__main__":
    main()
