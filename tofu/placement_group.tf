resource "hcloud_placement_group" "k3s" {
  name = "k3s"
  type = "spread"
}
