resource "hcloud_server" "this" {
  for_each = local.servers

  # Also the OS hostname, the Kubernetes node name and the etcd member name. At creation
  # cloud-init takes it from Hetzner's metadata, but the metadata keeps the creation-time
  # name, so a rename here only reaches the host through Ansible's hostname role. Never
  # rename a server while k3s runs on it - that changes the node's identity under etcd.
  name        = each.key
  server_type = each.value.server_type
  image       = "debian-13"
  location    = local.location

  labels = local.cluster_label

  ssh_keys           = [hcloud_ssh_key.d3strukt0r.id]
  placement_group_id = hcloud_placement_group.prod.id

  delete_protection  = true
  rebuild_protection = true

  user_data = <<-EOT
    #include
    https://raw.githubusercontent.com/Team-MaRo/infrastructure/refs/heads/master/cloud-init/node.yaml
  EOT

  lifecycle {
    # cx23/cx33 are cost-optimized types with limited availability - a node
    # that gets destroyed may not be creatable again. This turns any plan that
    # would destroy or replace a node into an error instead of a diff, and
    # makes `tofu destroy` fail outright. To retire a node on purpose, delete
    # this line first, in its own reviewable commit.
    prevent_destroy = true

    # The Hetzner API returns neither user_data nor ssh_keys, so both are null
    # in state after an import while both force replacement. Without this the
    # plan would destroy and recreate every node. ignore_changes does not apply
    # on create, so a new node still gets both values.
    ignore_changes = [user_data, ssh_keys]
  }
}
