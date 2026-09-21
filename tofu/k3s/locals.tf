locals {
  # Private IPs are not in name order - that is how they were handed out when
  # the nodes were created by hand, and changing one would detach/reattach the
  # server from the network.
  servers = {
    "k3s-01" = {
      id          = 166653369
      server_type = "cx33"
      private_ip  = "10.0.0.3"
    }
    "k3s-02" = {
      id          = 166653370
      server_type = "cx33"
      private_ip  = "10.0.0.2"
    }
    "k3s-03" = {
      id          = 166653576
      server_type = "cx23"
      private_ip  = "10.0.0.4"
    }
  }

  location     = "nbg1"
  network_zone = "eu-central"

  # The firewall is attached through this label, not through per-server rules.
  role_label = { role = "k3s" }
}
