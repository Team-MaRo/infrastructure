locals {
  # Private IPs follow the node number: prod-NN gets 10.0.0.1NN. Not 10.0.0.NN, because
  # Hetzner reserves the first address of a network for its gateway, so .1 can never be
  # assigned. The 1NN form keeps the last two digits equal to the node number up to 99.
  #
  # Changing one replaces that server's network attachment (the IP cannot be changed in
  # place): the server drops off the private network until it is reattached, and the
  # OS picks up the new address on its next DHCP renewal or reboot. Never do that while
  # k3s runs on it - etcd peers find each other by these addresses.
  #
  # role is the k3s role, written to the server's labels: "server" runs the control plane and
  # etcd, "agent" is a worker the Ansible inventory groups into prod_agents. Never change a
  # running node's role - a server cannot become an agent or the other way round in place.
  #
  # id is the live server's ID, for the import blocks that adopted the hand-created servers;
  # null for a server OpenTofu creates itself.
  servers = {
    "prod-01" = {
      id          = 166653369
      server_type = "cx33"
      private_ip  = "10.0.0.101"
      role        = "server"
    }
    "prod-02" = {
      id          = 166653370
      server_type = "cx33"
      private_ip  = "10.0.0.102"
      role        = "server"
    }
    "prod-03" = {
      id          = 166653576
      server_type = "cx33"
      private_ip  = "10.0.0.103"
      role        = "server"
    }
    # The first worker (2026-10-07), so a node failure no longer leaves pods without room:
    # without etcd and an API server it costs far less memory than a server.
    "prod-04" = {
      id          = null
      server_type = "cx33"
      private_ip  = "10.0.0.104"
      role        = "agent"
    }
  }

  location     = "nbg1"
  network_zone = "eu-central"

  # Which cluster a server belongs to. The firewall attaches through this label, not
  # through per-server rules, and the Ansible inventory selects by it. Changing it takes
  # two applies - see firewall.tf.
  cluster_label = { cluster = "prod" }
}
