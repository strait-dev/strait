# Map Hetzner datacenter locations to network zones.
locals {
  network_zone_map = {
    ash  = "us-east"
    hil  = "us-west"
    fsn1 = "eu-central"
    nbg1 = "eu-central"
    hel1 = "eu-central"
  }
  network_zone = local.network_zone_map[var.location]
}

resource "hcloud_network" "cluster" {
  name     = "${var.cluster_name}-network"
  ip_range = "10.0.0.0/16"
}

resource "hcloud_network_subnet" "nodes" {
  network_id   = hcloud_network.cluster.id
  type         = "cloud"
  network_zone = local.network_zone
  ip_range     = "10.0.1.0/24"
}
