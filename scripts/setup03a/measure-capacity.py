#!/usr/bin/env python3
"""Measure SETUP 03A GRAPH transport capacity with an owned, bounded stack."""

import argparse
import hashlib
import json
import pathlib
import signal
import socket
import subprocess
import threading
import time
import urllib.error
import urllib.request
import uuid

NATS_IMAGE = (
    "nats:2.14.4-alpine@sha256:"
    "f2123f533c2b0cada0a5c5ec434fb2b8cfe1cf220215ef9d7517e1372917ad66"
)
TERMINAL_EXCLUSIONS = {"seeding", "initializing", "pending", "ingesting"}


def arguments():
    """Parse explicit artifacts; never infer a binary revision from its label."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=pathlib.Path)
    parser.add_argument("--label", required=True)
    parser.add_argument("--out", required=True, type=pathlib.Path)
    parser.add_argument("--corpus", required=True, type=pathlib.Path)
    parser.add_argument("--identity", required=True, choices=("beta161", "governed"))
    return parser.parse_args()


def command(*args, timeout=45):
    return subprocess.check_output(args, text=True, timeout=timeout).strip()


def free_port():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


def read_json(url):
    with urllib.request.urlopen(url, timeout=3) as response:
        return json.load(response)


def save(out, name, value):
    (out / name).write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")


def corpus_manifest(corpus):
    """Fingerprint source bytes without editing or filtering the supplied corpus."""
    entries = []
    for path in sorted(corpus.rglob("*")):
        relative = path.relative_to(corpus)
        if ".git" in relative.parts:
            continue
        if path.is_symlink():
            entries.append({"path": str(relative), "symlink": str(path.readlink())})
        elif path.is_file():
            content = path.read_bytes()
            entries.append({"path": str(relative), "bytes": len(content),
                            "sha256": hashlib.sha256(content).hexdigest()})
    encoded = json.dumps(entries, sort_keys=True).encode()
    return {"sha256": hashlib.sha256(encoded).hexdigest(), "entries": entries,
            "note": "Only .git metadata omitted from fingerprint; corpus passed verbatim to SemSource."}


def accounted(status):
    sources = status.get("sources", [])
    return (
        status.get("phase") in ("ready", "degraded")
        and bool(sources)
        and all(
            source.get("phase") not in TERMINAL_EXCLUSIONS
            and source.get("offered_total", 0)
            == source.get("delivered_total", 0) + source.get("lost_total", 0)
            for source in sources
        )
    )


def transport_succeeded(reason, failure, application_exit, cleanup_exit, sources):
    return (reason == "all_offered_work_accounted" and not failure
            and application_exit == 0 and cleanup_exit == 0 and bool(sources)
            and all(source.get("lost_total", 0) == 0 and source.get("seed_lost", 0) == 0
                    and source.get("error_count", 0) == 0 for source in sources))


def main():
    args = arguments()
    binary, corpus, out = args.binary.resolve(), args.corpus.resolve(), args.out.resolve()
    if not binary.is_file() or not corpus.is_dir():
        raise SystemExit("--binary must be a file and --corpus an existing directory")
    out.mkdir(parents=True, exist_ok=True)
    if (out / "result.json").exists():
        raise SystemExit("evidence directory already has result.json; use a new --out")
    save(out, "corpus-manifest.json", corpus_manifest(corpus))
    name = "semsource-setup03a-capacity-" + uuid.uuid4().hex[:12]
    process = None
    app_log = None
    observations = []
    waiter = threading.Event()
    started = time.monotonic()
    reason = "timeout"
    failure = None
    # The unique owned name is known before docker runs, so partial startup can
    # still be cleaned after a command timeout with an uncertain create outcome.
    container = name
    try:
        container = command(
            "docker", "run", "-d", "--name", name, "--label",
            "c360.owner=semsource-setup03a", "--cpus=1", "--memory=1g",
            "-p", "127.0.0.1::4222", "-p", "127.0.0.1::8222",
            NATS_IMAGE, "-js", "-m", "8222",
        )
        nats_binding = command("docker", "port", container, "4222/tcp")
        monitor_binding = command("docker", "port", container, "8222/tcp")
        monitor = "http://" + monitor_binding
        ready_by = time.monotonic() + 15
        while True:
            try:
                read_json(monitor + "/healthz")
                break
            except (OSError, ValueError):
                if time.monotonic() >= ready_by:
                    raise RuntimeError("owned NATS did not become healthy within 15 seconds")
                waiter.wait(0.2)
        config = {
            "namespace": "setup03a", "http_port": free_port(),
            "sources": [
                {"type": "ast", "path": str(corpus), "languages": [
                    "go", "typescript", "javascript", "java", "python", "svelte"
                ], "watch": False},
                {"type": "docs", "paths": [str(corpus)], "watch": False},
                {"type": "config", "paths": [str(corpus)], "watch": False},
            ],
            "source_roots": [str(corpus)],
            "graph": {"embedder_type": "bm25", "gateway_bind": "127.0.0.1:" + str(free_port()),
                      "index_workers": 4, "coalesce_ms": 200},
            "metrics": {"port": free_port()},
            "websocket_bind": "127.0.0.1:" + str(free_port()),
        }
        if args.identity == "governed":
            config["platform_id"] = "semsource"
        save(out, "config.json", config)
        save(out, "resources.json", {
            "container": container, "name": name, "nats_binding": nats_binding,
            "monitor_binding": monitor_binding, "http_port": config["http_port"],
            "binary": str(binary), "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
            "label": args.label, "identity": args.identity, "nats_image": NATS_IMAGE,
            "inspection": json.loads(command("docker", "inspect", container)),
        })
        app_log = (out / "application.log").open("w", encoding="utf-8")
        process = subprocess.Popen(
            [str(binary), "run", "--config", str(out / "config.json"),
             "--nats-url", "nats://" + nats_binding],
            stdout=app_log, stderr=subprocess.STDOUT,
        )
        previous, changed, last_print = None, time.monotonic(), 0
        while time.monotonic() - started < 600:
            observation = {"elapsed_s": round(time.monotonic() - started, 2),
                           "process_exit": process.poll()}
            for key, url in (
                ("source_status", "http://127.0.0.1:" + str(config["http_port"])
                 + "/source-manifest/status"),
                ("jetstream", monitor + "/jsz?streams=true&consumers=true&accounts=true"),
            ):
                try:
                    observation[key] = read_json(url)
                except (OSError, ValueError) as error:
                    observation[key + "_error"] = str(error)
            observations.append(observation)
            save(out, "observations.json", observations)
            status = observation.get("source_status", {})
            summary = (status.get("total_entities"), [
                (source.get("phase"), source.get("delivered_total"),
                 source.get("lost_total"), source.get("error_count"))
                for source in status.get("sources", [])
            ])
            if summary != previous:
                changed, previous = time.monotonic(), summary
            if time.monotonic() - last_print >= 30:
                print(json.dumps({"elapsed_s": observation["elapsed_s"], "status": status,
                                  "broker_memory": observation.get("jetstream", {}).get("memory")}),
                      flush=True)
                last_print = time.monotonic()
            if process.poll() is not None:
                reason = "process_exit"
                break
            if accounted(status):
                reason = "all_offered_work_accounted"
                break
            if time.monotonic() - changed > 120 and observation["elapsed_s"] > 150:
                reason = "no_source_progress_120s"
                break
            waiter.wait(2)
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError, KeyboardInterrupt) as error:
        reason, failure = "operation_failed", str(error)
    finally:
        if process and process.poll() is None:
            process.send_signal(signal.SIGTERM)
            try:
                process.wait(timeout=40)
            except subprocess.TimeoutExpired:
                process.kill()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    failure = (failure or "") + ";application_join_timeout"
                reason += ";forced_application_stop"
        if app_log:
            app_log.close()
        with (out / "broker.log").open("w", encoding="utf-8") as broker_log:
            try:
                subprocess.run(["docker", "logs", container], stdout=broker_log,
                               stderr=subprocess.STDOUT, timeout=15, check=False)
            except (OSError, subprocess.TimeoutExpired) as error:
                failure = (failure or "") + ";broker_log_capture_failed:" + str(error)
        try:
            cleanup = subprocess.run(["docker", "rm", "-f", container],
                                     capture_output=True, text=True, timeout=20, check=False)
            cleanup_exit, cleanup_detail = cleanup.returncode, cleanup.stdout + cleanup.stderr
        except (OSError, subprocess.TimeoutExpired) as error:
            cleanup_exit, cleanup_detail = -1, "owned-container cleanup failed: " + str(error)
        final = observations[-1] if observations else None
        sources = (final or {}).get("source_status", {}).get("sources", [])
        application_exit = process.returncode if process else None
        transport_passed = transport_succeeded(reason, failure, application_exit, cleanup_exit, sources)
        result = {
            "label": args.label, "reason": reason, "failure": failure,
            "transport_passed": transport_passed,
            "application_exit": application_exit,
            "cleanup_exit": cleanup_exit, "cleanup_detail": cleanup_detail,
            "elapsed_s": round(time.monotonic() - started, 2), "final_observation": final,
            "limitation": "Transport-only probe; parser guard excludes minified assets before publish."
                          " Does not reproduce the historical unguarded payload volume or prove query correctness.",
        }
        save(out, "result.json", result)
        print(json.dumps({key: value for key, value in result.items() if key != "final_observation"}), flush=True)
    return 0 if transport_passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
