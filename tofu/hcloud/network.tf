resource "hcloud_network" "prod" {
  name     = "prod"
  ip_range = "10.0.0.0/16"

  lifecycle {
    # Destroying the network detaches every node from private networking.
    prevent_destroy = true
  }
}

# Auto-created alongside the network in the console, hence the /24 inside the
# /16 network range.
resource "hcloud_network_subnet" "prod" {
  network_id   = hcloud_network.prod.id
  type         = "cloud"
  network_zone = local.network_zone
  ip_range     = "10.0.0.0/24"

  lifecycle {
    # Every attribute here is ForceNew and the servers hold IPs out of this
    # range, so a replace would mean detaching all three nodes.
    prevent_destroy = true
  }
}

# Attachments are standalone resources so the server resource keeps no "network"
# block. Addressed by network_id + ip: subnet_id is RequiresReplace and is never
# read back from the API, so using it would plan a replace on import.
resource "hcloud_server_network" "this" {
  for_each = local.servers

  server_id  = hcloud_server.this[each.key].id
  network_id = hcloud_network.prod.id
  ip         = each.value.private_ip

  depends_on = [hcloud_network_subnet.prod]
}
