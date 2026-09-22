# infrastructure

A small k3s cluster on Hetzner Cloud, plus the DNS and registrar configuration around it.
Everything is declarative; there is no application code here.

| Directory | What it does |
|---|---|
| [`tofu/`](tofu/README.md) | Creates and owns the cloud resources - servers, network, firewall, DNS, registrar delegation |
| [`cloud-init/`](cloud-init/README.md) | Prepares a node once, at first boot |
| [`ansible/`](ansible/README.md) | Keeps running nodes configured |

The three run in that order and do not reach back: OpenTofu creates a node, cloud-init
prepares it once as it comes up, and Ansible configures it from then on. Editing
cloud-init changes only nodes created or rebuilt afterwards; editing Ansible changes nodes
on the next run.
