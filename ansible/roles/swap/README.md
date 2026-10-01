# swap

Provisions a 2 GB swap file and the kernel tuning that makes swap safe on a Kubernetes
node.

## Written but unreferenced

**Written and verified, but not applied.** No playbook lists it, so it does nothing. That is
deliberate: nothing runs on these nodes yet, so there is no memory pressure to measure and
any tuning value would be guesswork.

To enable it, add it to the `roles:` list of the first two plays in `ansible/prod.yml`:

```yaml
  roles:
    - swap
    - role: k3s
```

That line *is* the decision, which is why there is no `swap_enabled` flag sitting at false
and then wonder about later. k3s was installed with `--kubelet-arg=fail-swap-on=false`, so
kubelet will not object (see "Two flags the install has to set up front" in the
[k3s role](../k3s/README.md)).

## The values, and what they do not claim

Values follow the *Recommended starting point* in the Kubernetes
[swap deep-dive](https://kubernetes.io/blog/2025/08/19/tuning-linux-swap-for-kubernetes-a-deep-dive/),
not the values that post used while experimenting:

| Setting | Value | Why |
|---|---|---|
| `vm.swappiness` | 60 | The kernel default. Set explicitly to record intent - this is not a change |
| `vm.min_free_kbytes` | 3% of RAM | Upper end of the blog's "2-3% of total node memory", scaled rather than hardcoded so one role serves a 4 GB cx23, an 8 GB cx33 and the home servers |
| `vm.watermark_scale_factor` | 2000 | The value that actually changes behaviour - widens the reclaim window so kswapd can page out before the node hits a critical state |

Two are easy to misread:

- **`vm.swappiness = 60` is the kernel default.** It is set explicitly to record intent,
  which means the role currently changes nothing about swappiness. Do not call it tuned.
  swappiness is a relative IO *cost ratio* (0-200, 100 = swap and file IO equally
  expensive), not an eagerness dial. A low value claims swap IO is far costlier than file
  IO - true of spinning disks, false of the local NVMe here - and does not avoid disk IO,
  it shifts thrashing onto the page cache. If a low value is ever wanted, 1 is the floor,
  not 0: since a 2012 vmscan change, 0 will not scan anonymous pages until severe
  contention (more under "On swappiness" below).
- **`vm.min_free_kbytes` is 3% of *reported* RAM**, the upper end of the blog's
  "2-3% of total node memory" - not a round number, because a 4 GB node reports about
  3900 MB. Verify against `/proc/meminfo`, never a fixed figure. The blog contradicts
  itself here: its own test used 512 MiB on a 5 GiB node, roughly 10%.

`vm.watermark_scale_factor = 2000` is the one value that genuinely changes behaviour, by
widening the reclaim window so kswapd can page out before the node hits a critical state.

`min_free_kbytes` is derived from what the kernel reports, which is well under the
nominal size, so it is not a round number:

| Node | Reported RAM | `min_free_kbytes` |
|---|---|---|
| `prod-03` (cx23) | 3826 MB | 117534 (~115 MiB) |
| `prod-01`, `prod-02` (cx33) | 7757 MB | 238295 (~233 MiB) |

Check it against the node rather than a fixed figure:

```shell
echo "want $(awk '/MemTotal/{printf "%d", $2*0.03}' /proc/meminfo), got $(sysctl -n vm.min_free_kbytes)"
```

If OOM kills still happen under real load, `min_free_kbytes` is the number to raise. The
blog's own test used far more than its recommendation, and its recommendation contradicts
its own test node size - so benchmark rather than trusting either.

### On swappiness

Worth knowing before changing `swap_swappiness` from the default 60, because it is one of
the most mythologised knobs in Linux.

It is **not** an eagerness dial. The kernel defines it as the relative IO cost of swapping
versus filesystem paging, on a 0-200 scale where 100 means the two are equally expensive
(`anon_prio = swappiness`, `file_prio = 200 - swappiness`). A low value asserts that swap
IO is far costlier than file IO - true of spinning disks, false here, where swap and the
filesystem share the same local NVMe.

Low values also do not avoid disk IO. They shift the thrashing from anonymous pages to the
page cache: instead of writing out cold anonymous memory once, the kernel repeatedly
evicts and re-reads binaries, libraries and cached data. That can be slower, and can help
cause the contention it was meant to avoid.

If a low value is ever wanted, **1 is the floor, not 0**: since a 2012 vmscan change, 0
refuses to scan anonymous pages at all until severe contention.

The hardware here would justify something nearer 100. It stays at 60 because an unmeasured
100 is no better founded than an unmeasured 1 - measure first.

## Swap is not encrypted

**Swap is deliberately not encrypted.** It would only protect paged-out memory against
someone reading the disk offline, and on the same unencrypted root filesystem sit the etcd
datastore with every Kubernetes Secret, the cluster CA private keys, the join token and a
cluster-admin kubeconfig. Encrypting swap alone shutters a window beside an open door; the
answer to that threat is full-disk encryption, which on Hetzner costs unattended reboots - a
real trade for a 3-node etcd cluster.
