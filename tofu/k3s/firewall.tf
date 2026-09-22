resource "hcloud_firewall" "k3s_public" {
  name = "k3s-public"

  rule {
    direction   = "in"
    protocol    = "tcp"
    port        = "22"
    source_ips  = ["0.0.0.0/0", "::/0"]
    description = "SSH"
  }

  rule {
    direction   = "in"
    protocol    = "tcp"
    port        = "80"
    source_ips  = ["0.0.0.0/0", "::/0"]
    description = "HTTP"
  }

  rule {
    direction   = "in"
    protocol    = "tcp"
    port        = "443"
    source_ips  = ["0.0.0.0/0", "::/0"]
    description = "HTTPS"
  }

  rule {
    direction   = "in"
    protocol    = "udp"
    port        = "443"
    source_ips  = ["0.0.0.0/0", "::/0"]
    description = "HTTP/3"
  }

  rule {
    direction   = "in"
    protocol    = "tcp"
    port        = "25565"
    source_ips  = ["0.0.0.0/0", "::/0"]
    description = "Minecraft"
  }

  rule {
    direction   = "in"
    protocol    = "icmp"
    source_ips  = ["0.0.0.0/0", "::/0"]
    description = "Ping/Traceroute"
  }

  # The Kubernetes API, unlike every other rule here, is not open to the world. Cluster
  # traffic between the nodes - etcd, flannel VXLAN, kubelet - needs no rule at all:
  # Hetzner firewalls do not filter private network traffic, and everything binds to
  # 10.0.0.0/24.
  rule {
    direction   = "in"
    protocol    = "tcp"
    port        = "6443"
    source_ips  = var.admin_ips
    description = "Kubernetes API (admin only)"
  }

  # Label selector is the only attachment mechanism. Do not add
  # hcloud_firewall_attachment alongside this - the two fight over the same
  # API field.
  apply_to {
    label_selector = "role=k3s"
  }
}
