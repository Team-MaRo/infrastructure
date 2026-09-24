# infrastructure

A small Kubernetes cluster, `prod`, on Hetzner Cloud - running the k3s distribution - plus
the DNS and registrar configuration around it.
Everything is declarative; there is no application code here.

| Directory | What it does |
|---|---|
| [`tofu/`](tofu/README.md) | Creates and owns the cloud resources - servers, network, firewall, buckets, DNS, registrar delegation |
| [`cloud-init/`](cloud-init/README.md) | Prepares a node once, at first boot |
| [`ansible/`](ansible/README.md) | Keeps running nodes configured, installs k3s, bootstraps Argo CD |
| [`kubernetes/`](kubernetes/README.md) | What runs on the cluster, deployed by Argo CD from `master` |

They run in that order and do not reach back: OpenTofu creates a node, cloud-init prepares
it once as it comes up, Ansible configures it from then on, and Argo CD deploys onto the
cluster. Editing cloud-init changes only nodes created or rebuilt afterwards; editing
Ansible changes nodes on the next run; pushing to `kubernetes/` changes the cluster within
minutes.
