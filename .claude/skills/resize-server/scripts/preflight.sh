#!/usr/bin/env bash
# Read-only checks before resizing a prod node: changes nothing.
# Usage: preflight.sh <node> <server-type>     e.g. preflight.sh prod-02 cx43
set -uo pipefail

node=${1:?node, e.g. prod-02}
type=${2:?server type, e.g. cx43}
ctx=d3strukt0r-prod-admin
k() { kubectl --context "$ctx" "$@"; }
h() { hcloud --context d3strukt0r-infrastructure "$@"; }
# An empty or failed read must never look like "all clear".
die() { echo "PREFLIGHT FAILED: $* - stop, nothing here can be trusted"; exit 1; }

type_json=$(h server-type describe "$type" -o json) || die "hcloud could not read $type"
current_json=$(h server describe "$node" -o json) || die "hcloud could not read $node"
nodes_json=$(k get nodes -o json) || die "kubectl could not read the nodes"
pods_json=$(k get pods -A -o json) || die "kubectl could not read the pods"
# kured's own view per node; the switch CronJob in mariadb reads the same metric.
reboot_json=$(k get --raw '/api/v1/namespaces/monitoring/services/http:kube-prometheus-stack-prometheus:9090/proxy/api/v1/query?query=kured_reboot_required' 2>/dev/null) ||
  reboot_json='{"data":{"result":[]}}'

echo "== Server type $type in nbg1"
echo "$type_json" | jq -r '
  [.locations[]? | select(.name=="nbg1") | .available] as $a
  | "\(.name): \(.cores) vCPU, \(.memory) GB RAM, \(.disk) GB disk; orderable in nbg1: \(if ($a | length) == 0 then "not offered" elif $a[0] then "yes" else "NO" end); deprecated: \(.deprecation != null)"'
echo "Current: $(echo "$current_json" | jq -r '.server_type.name')"

echo
echo "== Nodes (every other server must be Ready); due = kured_reboot_required; a kured mark shows only where a node has one"
nodes_out=$(echo "$nodes_json" | jq -r --argjson m "$reboot_json" '
  ($m.data.result // [] | group_by(.metric.node)
    | map({key: .[0].metric.node, value: (map(.value[1]) | if index("1") then "yes" else "no" end)}) | from_entries) as $due
  | .items[]
  | .metadata.name as $n
  | (.metadata.annotations["weave.works/kured-reboot-in-progress"] // null) as $mark
  | "\($n): Ready=\([.status.conditions[]? | select(.type=="Ready") | .status] | first // "missing"), etcd=\(.metadata.labels["node-role.kubernetes.io/etcd"] // "false"), cordoned=\(.spec.unschedulable // false), due for a reboot=\($due[$n] // "unknown (no metric)")\(
      if $mark == null then ""
      elif $due[$n] == "no" then ", stale kured mark from \($mark) (remove it)"
      else ", kured mark since \($mark)" end)"') || die "could not read the nodes' state"
[ -n "$nodes_out" ] || die "no nodes"
echo "$nodes_out"

echo
echo "== Database primaries (must leave $node before the drain)"
pg=$(k -n postgres get cluster postgres -o jsonpath='{.status.currentPrimary}') || die "could not read the Postgres cluster"
[ -n "$pg" ] || die "the Postgres cluster names no primary"
pgline=$(echo "$pods_json" | jq -r --arg node "$node" --arg pg "$pg" '
  def ready: any(.status.conditions[]?; .type == "Ready" and .status == "True");
  [.items[] | select(.metadata.namespace == "postgres" and .metadata.labels["cnpg.io/cluster"] == "postgres" and .metadata.labels["cnpg.io/podRole"] == "instance")] as $instances
  | ($instances[] | select(.metadata.name == $pg) | .spec.nodeName) as $pgnode
  | ([$instances[] | select(.metadata.name != $pg and .spec.nodeName != $pgnode and ready) | .metadata.name] | first) as $other
  | "Postgres primary: \($pg) on \($pgnode)"
    + if $pgnode != $node then ""
      elif $other == null then "   <-- ON THIS NODE, and no Ready instance on another node to promote: stop"
      else "   <-- ON THIS NODE: promote \($other) first" end')
[ -n "$pgline" ] || die "no pod for the Postgres primary $pg"
echo "$pgline"
mdb=$(echo "$pods_json" | jq -r '[.items[] | select(.metadata.namespace == "mariadb" and .metadata.labels["k8s.mariadb.com/role"] == "primary")] | if length == 1 then "\(.[0].metadata.name) \(.[0].spec.nodeName)" else "" end')
[ -n "$mdb" ] || die "no single MariaDB primary pod (a switchover under way?)"
mdbpod=${mdb% *} mdbnode=${mdb#* }
echo "MariaDB primary: $mdbpod on $mdbnode$([ "$mdbnode" = "$node" ] && echo "   <-- ON THIS NODE: set podIndex $((1 - ${mdbpod##*-})) first")"

echo
echo "== Pods on $node (DaemonSets and finished Jobs excluded); * = has a volume, moves with a short outage"
echo "$pods_json" | jq -r --arg node "$node" '
  .items[]
  | select(.spec.nodeName == $node and .status.phase != "Succeeded" and .status.phase != "Failed")
  | select(([.metadata.ownerReferences[]?.kind] | index("DaemonSet")) | not)
  | "\(if ([.spec.volumes[]? | select(.persistentVolumeClaim)] | length) > 0 then "*" else " " end) \(.metadata.namespace)/\(.metadata.name)"'

echo
echo "== Disruption budgets that allow no eviction right now"
k get pdb -A -o json | jq -r '.items[] | select(.status.disruptionsAllowed == 0) | "\(.metadata.namespace)/\(.metadata.name)"' ||
  die "could not read the disruption budgets"

echo
echo "== Memory: requests of running and starting pods vs allocatable, MiB (the others must take $node's pods)"
# Memory the kernel reports per type (MiB), read on our nodes; another type is scaled from this one.
jq -rn --arg node "$node" --arg type "$type" \
  --argjson newram "$(echo "$type_json" | jq '.memory')" --argjson curram "$(echo "$current_json" | jq '.server_type.memory')" \
  --argjson known '{"cx23": 3826, "cx33": 7758, "cx43": 15621}' \
  --slurpfile pods <(echo "$pods_json") --slurpfile nodes <(echo "$nodes_json") '
  def mib: if . == null then 0
    elif test("^[0-9.]+[A-Za-z]*$") | not then error("unreadable memory quantity: \(.)") else
    capture("^(?<n>[0-9.]+)(?<u>[A-Za-z]*)$") as $q
    | ({"": 1, m: 1e-3, k: 1e3, Ki: 1024, M: 1e6, Mi: 1048576, G: 1e9, Gi: 1073741824,
        T: 1e12, Ti: 1099511627776, P: 1e15, Pi: 1125899906842624}[$q.u]
       // error("unknown memory unit: \(.)")) * ($q.n | tonumber) / 1048576 end;
  # containers plus native sidecars (init containers that keep running), as the scheduler counts
  def req: [(.spec.containers[], (.spec.initContainers[]? | select(.restartPolicy == "Always")))
    | .resources.requests.memory | mib] | add;
  def ds: [.metadata.ownerReferences[]?.kind] | index("DaemonSet");
  ($pods[0].items | map(select(.spec.nodeName != null and (.status.phase == "Running" or .status.phase == "Pending")))) as $running
  | ($running | group_by(.spec.nodeName)
    | map({key: .[0].spec.nodeName, value: (map(req) | add)}) | from_entries) as $req
  | ($running | map(select(.spec.nodeName == $node and (ds | not)) | req) | add // 0 | floor) as $moving
  | $nodes[0].items[]
  | .metadata.name as $n | (.status.capacity.memory | mib) as $cap | (.status.allocatable.memory | mib) as $alloc
  | ($req[$n] // 0 | floor) as $r
  | "\($n): \($r) requested of \($alloc | floor) allocatable, \($alloc - $r | floor) free"
    + if $n == $node then
        ($known[$type] // ($cap * $newram / $curram)) as $newcap
        | "\n  as \($type): about \($newcap - ($cap - $alloc) | floor) allocatable (\(if $known[$type] then "measured" else "RAM-scaled" end) memory minus the \($cap - $alloc | floor) reserved now) - a downgrade must hold the \($r); a drain moves \($moving) of it (no DaemonSets) to the others"
      else "" end' || die "could not compute the memory figures"

echo
echo "== kured lock (must be empty) and time"
lock=$(k -n kube-system get ds kured -o jsonpath='{.metadata.annotations.weave\.works/kured-node-lock}') || die "could not read kured's lock"
echo "lock: ${lock:-none}"
echo "now: $(date '+%H:%M') - avoid 01:00-06:00 (etcd snapshot at 01:00 in winter, 02:00 in summer; k3s upgrades, OS updates, kured reboots)"
