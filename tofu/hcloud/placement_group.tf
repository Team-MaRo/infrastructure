resource "hcloud_placement_group" "prod" {
  name = "prod"
  type = "spread"
}
