# Resource addresses renamed from k3s to prod. These move entries in the state only; no
# API call is made for them. The name and label changes that go with them are ordinary
# in-place updates in the same plan.
#
# Kept rather than deleted after the apply, so any state copy still holding the old
# addresses migrates the same way.

moved {
  from = hcloud_network.k3s
  to   = hcloud_network.prod
}

moved {
  from = hcloud_network_subnet.k3s
  to   = hcloud_network_subnet.prod
}

moved {
  from = hcloud_placement_group.k3s
  to   = hcloud_placement_group.prod
}

moved {
  from = hcloud_firewall.k3s_public
  to   = hcloud_firewall.prod_public
}

moved {
  from = hcloud_server.this["k3s-01"]
  to   = hcloud_server.this["prod-01"]
}

moved {
  from = hcloud_server.this["k3s-02"]
  to   = hcloud_server.this["prod-02"]
}

moved {
  from = hcloud_server.this["k3s-03"]
  to   = hcloud_server.this["prod-03"]
}

moved {
  from = hcloud_server_network.this["k3s-01"]
  to   = hcloud_server_network.this["prod-01"]
}

moved {
  from = hcloud_server_network.this["k3s-02"]
  to   = hcloud_server_network.this["prod-02"]
}

moved {
  from = hcloud_server_network.this["k3s-03"]
  to   = hcloud_server_network.this["prod-03"]
}
