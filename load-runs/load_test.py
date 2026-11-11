import csv
import json
import subprocess
import threading
import time
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen

RATE = 100
SECONDS = 60
TOTAL = RATE * SECONDS
run = "load-" + datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S%f")
folder = Path("load-runs") / run
folder.mkdir(parents=True)
Path("load-runs/current").write_text(run + "\n")
stop = threading.Event()
samples = []


def snapshot():
    query = (
        "SELECT count(*), "
        "count(*) FILTER (WHERE status='queued'), "
        "count(*) FILTER (WHERE status='running'), "
        "count(*) FILTER (WHERE status='succeeded'), "
        "count(*) FILTER (WHERE status='failed') "
        "FROM jobs WHERE idempotency_key LIKE '" + run + "-%'"
    )
    result = subprocess.run(
        ["docker", "compose", "exec", "-T", "db", "psql", "-U", "souptask",
         "-d", "souptask", "-Atc", query],
        text=True, capture_output=True, timeout=15,
    )
    second = round(time.monotonic() - start, 2)
    if result.returncode:
        return [second, "ERROR", result.stderr.strip(), "", "", ""]
    return [second, *result.stdout.strip().split("|")]


def monitor():
    while not stop.is_set():
        samples.append(snapshot())
        stop.wait(5)


def send(number):
    body = json.dumps({
        "idempotencyKey": f"{run}-{number:05d}",
        "payload": {"kind": "load", "number": number},
    }).encode()
    sent = time.monotonic()
    request = Request(
        "http://127.0.0.1:8080/v1/jobs", data=body,
        headers={"Content-Type": "application/json"}, method="POST",
    )
    try:
        with urlopen(request, timeout=15) as response:
            status = response.status
            text = response.read().decode()
    except HTTPError as error:
        status = error.code
        text = error.read().decode(errors="replace")
    except Exception as error:
        status = 0
        text = repr(error)
    done = time.monotonic()
    return [number, round(sent - start, 4), round(done - start, 4),
            round((done - sent) * 1000, 2), status, text]


start = time.monotonic()
watcher = threading.Thread(target=monitor)
watcher.start()

with ThreadPoolExecutor(max_workers=64) as pool:
    futures = []
    for number in range(TOTAL):
        delay = start + number / RATE - time.monotonic()
        if delay > 0:
            time.sleep(delay)
        futures.append(pool.submit(send, number))
    rows = [future.result() for future in futures]

end = time.monotonic()
stop.set()
watcher.join()
samples.append(snapshot())

with (folder / "requests.csv").open("w", newline="") as file:
    writer = csv.writer(file)
    writer.writerow(["number", "sent_s", "done_s", "latency_ms", "http", "response"])
    writer.writerows(rows)

with (folder / "queue.csv").open("w", newline="") as file:
    writer = csv.writer(file)
    writer.writerow(["elapsed_s", "total", "queued", "running", "succeeded", "failed"])
    writer.writerows(samples)

ok = [row for row in rows if row[4] == 200]
latencies = sorted(row[3] for row in ok)
percentile = lambda p: latencies[round((len(latencies) - 1) * p)] if latencies else -1
spread = max(row[1] for row in rows) - min(row[1] for row in rows)
actual_rps = (len(rows) - 1) / spread if spread > 0 else 0
summary = (
    f"run={run} target_rps={RATE} requests={TOTAL} "
    f"actual_send_rps={actual_rps:.1f} elapsed_s={end-start:.1f} "
    f"http_200={len(ok)} errors={TOTAL-len(ok)} "
    f"p50_ms={percentile(0.50)} p95_ms={percentile(0.95)} "
    f"p99_ms={percentile(0.99)} final_queue={samples[-1]}"
)
print(summary)
(folder / "summary.txt").write_text(summary + "\n")
