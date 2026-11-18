#!/usr/bin/env python3
"""Read-only report of resource usage per node and per container, from Prometheus. Changes nothing.

Usage: resources.py [namespace|node] [--window 14d]
       window: <n>d or <n>h; the container figures come from the memory-sizing recording rules
"""

import json
import math
import subprocess
import sys
import time
import urllib.parse

CONTEXT = "d3strukt0r-prod-admin"
PROM = "/api/v1/namespaces/monitoring/services/http:kube-prometheus-stack-prometheus:9090/proxy/api/v1/query"
MIB = 1024 * 1024
# Subqueries over the window sample every 30 min: every 5 min, the containers' CPU over 14 days
# took Prometheus from ~1.5 to ~2.5 GiB (2026-11-18, its limit 2.7 GiB). A p95 holds at 30 min;
# a max can miss a peak shorter than that.
STEP = "30m"


def args():
    scope, window = None, "14d"
    rest = sys.argv[1:]
    while rest:
        a = rest.pop(0)
        if a in ("-h", "--help"):
            print(__doc__.strip())
            sys.exit(0)
        if a == "--window":
            window = rest.pop(0)
        elif a.startswith("--window="):
            window = a.split("=", 1)[1]
        else:
            scope = a
    if not (window[:-1].isdigit() and window[-1] in "dh"):
        sys.exit("--window takes <n>d or <n>h")
    return scope, window


def query(expr):
    """An instant query: list of (labels, float value)."""
    url = f"{PROM}?query={urllib.parse.quote(expr)}"
    out = subprocess.run(
        ["kubectl", "--context", CONTEXT, "get", "--raw", url],
        check=True, capture_output=True, text=True,
    ).stdout
    return [(r["metric"], float(r["value"][1])) for r in json.loads(out)["data"]["result"]]


def by(expr, *keys):
    return {tuple(m.get(k, "") for k in keys): v for m, v in query(expr)}


def up(value, step):
    return math.ceil(value / step) * step


def mib(value):
    return "-" if value is None else f"{value / MIB:.0f}"


def table(header, rows):
    widths = [max(len(str(r[i])) for r in [header] + rows) for i in range(len(header))]
    for r in [header] + rows:
        print("  ".join(str(c).ljust(w) for c, w in zip(r, widths)).rstrip())


def main():
    scope, w = args()
    nodes_now = by("node_uname_info", "instance", "nodename")
    node_of = {inst: name for (inst, name) in nodes_now}
    node_names = sorted(set(node_of.values()))
    node_scope = scope if scope in node_names else None
    ns_scope = scope if scope and not node_scope else None

    def per_node(expr):
        return {node_of.get(k[0], k[0]): v for k, v in by(expr, "instance").items()}

    # --- Nodes -----------------------------------------------------------------------------------
    total = per_node("node_memory_MemTotal_bytes")
    used = per_node("node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes")
    used_max = per_node(f"max_over_time((node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes)[{w}:{STEP}])")
    pods_ws = {k[0]: v for k, v in by('sum by (node) (container_memory_working_set_bytes{container!="",pod!=""})', "node").items()}
    alloc = {k[0]: v for k, v in by('kube_node_status_allocatable{resource="memory"}', "node").items()}
    req = {k[0]: v for k, v in by("sum by (node) (cluster:namespace:pod_memory:active:kube_pod_container_resource_requests)", "node").items()}
    cores = {k[0]: v for k, v in by('kube_node_status_capacity{resource="cpu"}', "node").items()}
    cpu_now = per_node('sum by (instance) (rate(node_cpu_seconds_total{mode!="idle"}[5m]))')
    cpu_p95 = per_node(f'quantile_over_time(0.95, sum by (instance) (rate(node_cpu_seconds_total{{mode!="idle"}}[5m]))[{w}:{STEP}])')
    disk = per_node('max by (instance) (1 - node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"})')

    free = {n: alloc.get(n, 0) - req.get(n, 0) for n in node_names}
    rows = []
    for n in node_names:
        if node_scope and n != node_scope:
            continue
        others_free = sum(free[o] for o in node_names if o != n)
        fits = "yes" if req.get(n, 0) <= others_free else "NO"
        rows.append([
            n,
            mib(total.get(n)),
            f"{mib(used.get(n))} ({used.get(n, 0) / total.get(n, 1):.0%})",
            mib(used_max.get(n)),
            mib(pods_ws.get(n)),
            mib(used.get(n, 0) - pods_ws.get(n, 0)),
            f"{mib(req.get(n))}/{mib(alloc.get(n))} ({req.get(n, 0) / alloc.get(n, 1):.0%})",
            f"{cpu_now.get(n, 0):.2f}/{cpu_p95.get(n, 0):.2f} of {cores.get(n, 0):g}",
            f"{disk.get(n, 0):.0%}",
            f"{fits} ({mib(req.get(n))} -> {mib(others_free)} free)",
        ])

    # Days of recorded container data: the memory-sizing rules started 2026-11-13.
    first = query(f"min(min_over_time(timestamp(workload_container:memory_real:max_bytes)[{w}:1h]))")
    days = (time.time() - first[0][1]) / 86400 if first else 0
    wanted = int(w[:-1]) * (1 if w[-1] == "d" else 1 / 24)
    print(f"Window {w}; recorded container data covers {days:.1f} days"
          + ("" if days >= wanted - 0.5 else f" - shorter than the window, so p95 and max cover only those days"))
    print("\nNODES (MiB; CPU in cores)")
    table(["NODE", "MEMORY", "USED NOW", f"USED MAX {w}", "PODS WS", "OUTSIDE PODS",
           "REQUESTS/ALLOCATABLE", f"CPU NOW/P95 {w}", "DISK /", "DRAIN FITS"], rows)

    # --- Containers ------------------------------------------------------------------------------
    sel = f'{{namespace="{ns_scope}"}}' if ns_scope else ""
    key = ("namespace", "workload", "container")
    real_p95 = by(f"quantile_over_time(0.95, workload_container:memory_real:max_bytes{sel}[{w}])", *key)
    ws_p95 = by(f"quantile_over_time(0.95, workload_container:memory_working_set:max_bytes{sel}[{w}])", *key)
    ws_max = by(f"max_over_time(workload_container:memory_working_set:max_bytes{sel}[{w}])", *key)
    c_req = by(f"max_over_time(workload_container:memory_request:max_bytes{sel}[1h])", *key)
    c_lim = by(f"max_over_time(workload_container:memory_limit:max_bytes{sel}[1h])", *key)
    vpa = by(f'kube_customresource_vpa_memory_recommendation{{bound="target"{"," + sel[1:-1] if sel else ""}}}', "namespace", "target_name", "container")
    owner = 'max by (namespace, pod, workload) (namespace_workload_pod:kube_pod_owner:relabel)'
    cpu = f"max by (namespace, workload, container) (node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate5m{sel} * on (namespace, pod) group_left (workload) {owner})"
    cpu_p95 = by(f"quantile_over_time(0.95, ({cpu})[{w}:{STEP}])", *key)
    cpu_max = by(f"max_over_time(({cpu})[{w}:{STEP}])", *key)
    restarts = by(f"sum by (namespace, workload, container) (increase(kube_pod_container_status_restarts_total{sel}[{w}]) * on (namespace, pod) group_left (workload) {owner})", *key)
    oom = by(f'max by (namespace, workload, container) (kube_pod_container_status_last_terminated_reason{{reason="OOMKilled"{"," + sel[1:-1] if sel else ""}}} * on (namespace, pod) group_left (workload) {owner})', *key)
    if node_scope:
        on_node = {(m["namespace"], m["workload"]) for m, _ in query(
            f'namespace_workload_pod:kube_pod_owner:relabel * on (namespace, pod) group_left (node) kube_pod_info{{node="{node_scope}"}}')}

    severity = {"oom-killed": 0, "over-limit-risk": 1, "under-requested": 2, "over-requested": 3, "ok": 4}
    crows = []
    for k in sorted(set(real_p95) | set(ws_max)):
        ns, wl, ctr = k
        if node_scope and (ns, wl) not in on_node:
            continue
        rp, wp, wm = real_p95.get(k), ws_p95.get(k), ws_max.get(k)
        if rp is None or wp is None or wm is None:
            continue
        rule_req = up(rp, 16 * MIB)
        rule_lim = max(up(2 * wp, 32 * MIB), up(1.1 * wm, 32 * MIB))
        r, lim = c_req.get(k), c_lim.get(k)
        if oom.get(k):
            flag = "oom-killed"
        elif lim and wm > 0.8 * lim:
            flag = "over-limit-risk"
        elif r and rule_req > r:
            flag = "under-requested"
        elif r and r > 2 * rule_req:
            flag = "over-requested"
        else:
            flag = "ok"
        crows.append((severity[flag], -wm, [
            f"{ns}/{wl}", ctr, mib(r), mib(lim), mib(rp), mib(wp), mib(wm),
            f"{mib(rule_req)}/{mib(rule_lim)}", mib(vpa.get(k)),
            f"{cpu_p95.get(k, 0):.3f}/{cpu_max.get(k, 0):.2f}",
            f"{restarts.get(k, 0):.0f}", flag,
        ]))
    crows.sort(key=lambda x: (x[0], x[1]))
    print(f"\nCONTAINERS (MiB; CPU in cores; {w})")
    table(["WORKLOAD", "CONTAINER", "REQUEST", "LIMIT", "REAL P95", "WS P95", "WS MAX",
           "RULE REQ/LIM", "VPA", "CPU P95/MAX", "RESTARTS", "FLAG"], [r for _, _, r in crows])


if __name__ == "__main__":
    main()
