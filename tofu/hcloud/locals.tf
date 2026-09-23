locals {
  # Private IPs follow the node number: prod-NN gets 10.0.0.1NN. Not 10.0.0.NN, because
  # Hetzner reserves the first address of a network for its gateway, so .1 can never be
  # assigned. The 1NN form keeps the last two digits equal to the node number up to 99.
  #
  # Changing one replaces that server's network attachment (the IP cannot be changed in
  # place): the server drops off the private network until it is reattached, and the
  # OS picks up the new address on its next DHCP renewal or reboot. Never do that while
  # k3s runs on it - etcd peers find each other by these addresses.
  servers = {
    "prod-01" = {
      id          = 166653369
      server_type = "cx33"
      private_ip  = "10.0.0.101"
    }
    "prod-02" = {
      id          = 166653370
      server_type = "cx33"
      private_ip  = "10.0.0.102"
    }
    "prod-03" = {
      id          = 166653576
      server_type = "cx23"
      private_ip  = "10.0.0.103"
    }
  }

  location     = "nbg1"
  network_zone = "eu-central"

  # Which cluster a server belongs to. The firewall attaches through this label, not
  # through per-server rules, and the Ansible inventory selects by it.
  #
  # role = "k3s" is the previous label, kept only while the firewall switches selectors
  # (see firewall.tf) - removing both in one apply could leave the servers briefly
  # unfirewalled. Drop it together with the firewall's old apply_to.
  cluster_label = { cluster = "prod", role = "k3s" }
}
