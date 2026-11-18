"""Replays a namespace's requests through Traefik's token bucket, sorts its 429s into genuine and
scanner, and prints one tab-separated row.

Arguments: requests file ("start time (ns) path" per line, sorted), 429 file ("epoch host visitor
path" per line), probe-paths file, namespace, average, burst, when the limit was last set (epoch
seconds), hosts (comma-separated).
"""

import re
import sys
import time

requests, refusals, probes, ns, average, burst, changed, hosts = sys.argv[1:9]
average, burst, changed = float(average), float(burst), int(changed)
hosts = set(hosts.split(","))

# A visitor who asks for even one of these paths is probing for secrets or leftovers, not using a
# site: all its 429s count as a scanner's. Genuine use (a gallery, a dashboard) never touches them.
PROBE = re.compile(open(probes).read().strip())

scanners, rows = set(), []
for line in open(refusals):
    parts = line.rstrip("\n").split(" ", 3)
    if len(parts) < 4 or parts[1] not in hosts:
        continue
    at, _, visitor, path = parts
    rows.append((int(at), visitor))
    if PROBE.search(path):
        scanners.add(visitor)
scanner = sum(1 for _, v in rows if v in scanners)
# Genuine refusals count only since the limit was last set - earlier ones are what led to it.
genuine = sum(1 for at, v in rows if v not in scanners and at >= changed)

# Requests to probed paths are left out: behind a login gate a probe gets a redirect, not a 4xx.
times = []
for line in open(requests):
    start, _, path = line.rstrip("\n").partition(" ")
    if start and not PROBE.search(path):
        times.append(int(start) / 1e9)
refused, peak = 0, 0
lowest, lowest_at = burst, None
if times:
    tokens, last = burst, times[0]
    second, in_second = None, 0
    for t in times:
        tokens = min(burst, tokens + (t - last) * average)
        last = t
        if tokens >= 1:
            tokens -= 1
        else:
            refused += 1
        if tokens < lowest:
            lowest, lowest_at = tokens, t
        if int(t) == second:
            in_second += 1
        else:
            second, in_second = int(t), 1
        peak = max(peak, in_second)

used = round(100 * (burst - lowest) / burst)
# Only a refusal that happened makes it "refusing": the replay puts every visitor and every Traefik
# node into one bucket, so a refusal there alone is a worst case worth a look, not a fact.
if genuine:
    status = "refusing"
elif refused or used >= 80:
    status = "tight"
elif not times:
    status = "quiet"
else:
    status = "ok"
print(
    "\t".join(
        [
            ns,
            f"{average:g}/{burst:g}",
            time.strftime("%m-%d %H:%M", time.localtime(changed)),
            str(len(times)),
            str(peak),
            f"{used}%",
            time.strftime("%m-%d %H:%M:%S", time.localtime(lowest_at)) if lowest_at else "-",
            str(refused),
            str(genuine),
            str(scanner),
            status,
            ",".join(sorted(hosts)),
        ]
    )
)
