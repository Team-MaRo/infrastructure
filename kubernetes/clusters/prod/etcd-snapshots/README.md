# etcd snapshots

Back to [`kubernetes/README.md`](../../../README.md).

k3s takes its scheduled snapshots (00:00 and 12:00 UTC) onto each server's disk and uploads
each to `d3strukt0r-prod-etcd` with the cluster's own S3 key. The ExternalSecret in this
directory (`external-secret.yaml`, Application `etcd-snapshots`, in the cluster directory
rather than `components/` because the bucket is prod's) builds Secret
`kube-system/k3s-etcd-snapshot-s3-config` from it: endpoint, region `nbg1` and bucket in git,
the key from OpenBao `secret/etcd-snapshot-s3` (`access-key`, `secret-key`). `k3s_config` in
Ansible points k3s at that Secret. Retention and the k3s settings for the upload (`etcd-s3`,
`etcd-s3-config-secret` and the Secret it names) are in
[`ansible/roles/k3s/README.md`](../../../../ansible/roles/k3s/README.md).

## The bucket

- **k3s's pruning only adds delete markers** on this versioned bucket; each version stays
  locked 7 days and the lifecycle rule removes it afterwards. The cluster key cannot bypass
  the lock or touch any other bucket, and no other cluster key can touch this one (the
  bucket policies, see [`tofu/objectstorage/README.md`](../../../../tofu/objectstorage/README.md)).
- **A restore cannot use the Secret** - the apiserver is not running then. It takes the S3
  settings as CLI flags (`--etcd-s3-endpoint`, `--etcd-s3-region`, `--etcd-s3-bucket`,
  keys from the `d3strukt0r-hetzner` profile) **and the original server token**, which
  decrypts the bootstrap data inside the snapshot. The token is in 1Password,
  [`k3s | Prod | Server token`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=2eic3puykugnvm6tsdh2jf4nvy&h=my.1password.com), copied from `/var/lib/rancher/k3s/server/token`.

## Setting up or replacing the key

1. Create an S3 key in the Hetzner console, labelled `prod etcd snapshots`, and store it in
   1Password as [`Hetzner | S3 | prod etcd snapshots`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=w2po7jqmxren7y7eruj73p5w4a&h=my.1password.com) (access key as username, secret key as
   credential).
2. With the port-forward to OpenBao open and `BAO_TOKEN` set (see `tofu/openbao/README.md`),
   copy it over - the values never appear in shell history:

   ```shell
   bao kv put secret/etcd-snapshot-s3 \
     access-key="$(op item get 'Hetzner | S3 | prod etcd snapshots' --account my.1password.com --vault Private --fields username)" \
     secret-key="$(op item get 'Hetzner | S3 | prod etcd snapshots' --account my.1password.com --vault Private --fields credential --reveal)"
   ```
3. External Secrets refreshes the Secret within its interval; k3s reads it at the next
   snapshot.

## Taking and listing snapshots

```shell
ssh prod-01 sudo k3s etcd-snapshot save
ssh prod-01 sudo k3s etcd-snapshot ls
kubectl --context d3strukt0r-prod-admin get etcdsnapshotfiles
```

The bucket lists only the newest 5 snapshots of all nodes together - about a day. Older ones,
up to 7 days, are noncurrent versions; fetch one with the admin key and restore from the file
instead of from S3 (`--cluster-reset-restore-path=<local file>`, without the `--etcd-s3*`
flags):

```shell
aws --profile d3strukt0r-hetzner s3api list-object-versions --bucket d3strukt0r-prod-etcd --prefix etcd-snapshot-prod-01
aws --profile d3strukt0r-hetzner s3api get-object --bucket d3strukt0r-prod-etcd --key <key> --version-id <id> <local file>
```

## Restoring

**A restore cannot read the Secret**, since the apiserver is down. It needs the S3 settings
as flags and the server token from 1Password ([`k3s | Prod | Server token`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=2eic3puykugnvm6tsdh2jf4nvy&h=my.1password.com)); see the
[k3s docs](https://docs.k3s.io/cli/etcd-snapshot) for the full procedure:

```shell
k3s server --cluster-reset --cluster-reset-restore-path=<snapshot name> \
  --token=<server token> --etcd-s3 --etcd-s3-endpoint=nbg1.your-objectstorage.com \
  --etcd-s3-region=nbg1 --etcd-s3-bucket=d3strukt0r-prod-etcd \
  --etcd-s3-access-key=<admin key> --etcd-s3-secret-key=<admin secret>
```
