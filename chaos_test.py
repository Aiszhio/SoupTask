import json
import random
import subprocess
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime, timezone
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen

run = "chaos-" + datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S%f")
folder = Path("chaos-runs") / run
folder.mkdir(parents=True)
Path("chaos-runs/current").write_text(run + "\n")

requests_log = (folder / "requests.log").open("w", buffering=1)
kills_log = (folder / "kills.log").open("w", buffering=1)
lock = threading.Lock()
stop = threading.Event()
kills = [0]

def write(log, text):
    with lock:
        log.write(text + "\n")

def http(method, path, body=b""):
    for attempt in range(1, 4):
        try:
            request = Request(
                "http://127.0.0.1:8080" + path,
                data=body if method == "POST" else None,
                headers={"Content-Type": "application/json"},
                method=method,
            )
            with urlopen(request, timeout=15) as response:
                text = response.read().decode()
                write(requests_log, f"{method} {path} HTTP={response.status} {text}")
                return json.loads(text)
        except HTTPError as error:
            text = error.read().decode(errors="replace")
            write(requests_log, f"{method} {path} HTTP={error.code} attempt={attempt} {text}")
        except Exception as error:
            write(requests_log, f"{method} {path} attempt={attempt} ERROR={error!r}")
        time.sleep(attempt * 0.2)

    raise RuntimeError(f"{method} {path} failed after 3 attempts")

def create(number):
    body = json.dumps({
        "idempotencyKey": f"{run}-{number:05d}",
        "payload": {"kind": "chaos", "number": number},
    }).encode()
    result = http("POST", "/v1/jobs", body)
    return result["job"]["id"]

def cancel(created):
    job_id = created.result()
    http("POST", f"/v1/jobs/{job_id}:cancel")

def docker(*args):
    return subprocess.run(
        ["docker", *args], text=True, capture_output=True
    )

def kill_workers():
    while not stop.is_set():
        workers = docker("compose", "ps", "-q", "worker").stdout.splitlines()

        if workers:
            worker = random.choice(workers)
            pid = docker("inspect", "-f", "{{.State.Pid}}", worker).stdout.strip()

            if pid.isdigit() and pid != "0":
                result = docker(
                    "run", "--rm", "--pid=host", "--network=none",
                    "--read-only", "--cap-drop=ALL", "--user=65532:65532",
                    "-e", f"TARGET_PID={pid}",
                    "--entrypoint=/bin/sh", "postgres:16", "-c",
                    'test "$(cat /proc/$TARGET_PID/comm)" = SoupWorker '
                    '&& kill -9 "$TARGET_PID"',
                )
                if result.returncode == 0:
                    kills[0] += 1
                write(
                    kills_log,
                    f"worker={worker[:12]} pid={pid} "
                    f"exit={result.returncode} {result.stderr.strip()}",
                )
        else:
            write(kills_log, "NO_RUNNING_WORKER")

        stop.wait(2)

def pending_jobs():
    query = (
        "SELECT count(*) FROM jobs "
        f"WHERE idempotency_key LIKE '{run}-%' "
        "AND status IN ('queued', 'running')"
    )
    result = docker(
        "compose", "exec", "-T", "db",
        "psql", "-U", "souptask", "-d", "souptask", "-Atc", query,
    )
    if result.returncode != 0:
        raise RuntimeError(result.stderr)
    return int(result.stdout.strip())

killer = threading.Thread(target=kill_workers)
killer.start()
failures = 0
mismatches = 0
pending = -1

try:
    tasks = []
    duplicates = []

    with ThreadPoolExecutor(max_workers=32) as pool:
        for number in range(10000):
            original = pool.submit(create, number)
            tasks.append(original)

            if number % 10 == 0:
                repeated = pool.submit(create, number)
                tasks.append(repeated)
                duplicates.append((original, repeated))

            if number % 20 == 0:
                tasks.append(pool.submit(cancel, original))

        for task in as_completed(tasks):
            if task.exception() is not None:
                failures += 1

        for original, repeated in duplicates:
            if (
                original.exception() is None
                and repeated.exception() is None
                and original.result() != repeated.result()
            ):
                mismatches += 1

    deadline = time.monotonic() + 1800
    while time.monotonic() < deadline:
        pending = pending_jobs()
        print(f"RUN={run} pending={pending} kills={kills[0]}", flush=True)
        if pending == 0:
            break
        time.sleep(5)
finally:
    stop.set()
    killer.join()
    requests_log.close()
    kills_log.close()

summary = (
    f"RUN={run} request_errors={failures} "
    f"duplicate_id_mismatches={mismatches} "
    f"kills={kills[0]} pending={pending}"
)
print(summary)
(folder / "harness_summary.txt").write_text(summary + "\n")

if failures or mismatches or pending != 0 or kills[0] == 0:
    raise SystemExit(1)
