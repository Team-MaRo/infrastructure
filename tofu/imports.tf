# Adoption of the hand-created infrastructure. These blocks are kept in version
# control instead of using `tofu import`, so the adoption itself is reviewable.

import {
  to = hcloud_ssh_key.d3strukt0r
  id = "130265588"
}

import {
  to = hcloud_placement_group.k3s
  id = "1885872"
}

import {
  to = hcloud_network.k3s
  id = "12670314"
}

import {
  to = hcloud_network_subnet.k3s
  id = "12670314-10.0.0.0/24"
}

import {
  to = hcloud_firewall.k3s_public
  id = "11653280"
}

import {
  for_each = local.servers

  to = hcloud_server.this[each.key]
  id = tostring(each.value.id)
}

import {
  for_each = local.servers

  to = hcloud_server_network.this[each.key]
  id = "${each.value.id}-12670314"
}
