# Single-region k3s cluster module.
# Creates a master node, worker pools, private network, and firewall.
#
# Usage:
#   module "us_east" {
#     source       = "./modules/region"
#     location     = "ash"
#     cluster_name = "strait-us-east"
#     hcloud_token = var.hcloud_token
#   }

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

resource "hcloud_ssh_key" "default" {
  name       = "${var.cluster_name}-key"
  public_key = file(pathexpand(var.ssh_key_path))
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

resource "hcloud_firewall" "cluster" {
  name = "${var.cluster_name}-firewall"

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "22"
    source_ips = var.ssh_allowed_ips
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "6443"
    source_ips = var.ssh_allowed_ips
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "10250"
    source_ips = ["10.0.0.0/16"]
  }

  rule {
    direction  = "in"
    protocol   = "udp"
    port       = "8472"
    source_ips = ["10.0.0.0/16"]
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "30443"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "30080"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  rule {
    direction       = "out"
    protocol        = "tcp"
    port            = "any"
    destination_ips = ["0.0.0.0/0", "::/0"]
  }

  rule {
    direction       = "out"
    protocol        = "udp"
    port            = "any"
    destination_ips = ["0.0.0.0/0", "::/0"]
  }

  rule {
    direction       = "out"
    protocol        = "icmp"
    destination_ips = ["0.0.0.0/0", "::/0"]
  }
}

resource "hcloud_server" "master" {
  depends_on = [hcloud_network_subnet.nodes]

  name         = "${var.cluster_name}-master"
  image        = "ubuntu-24.04"
  server_type  = var.master_type
  location     = var.location
  ssh_keys     = [hcloud_ssh_key.default.id]
  firewall_ids = [hcloud_firewall.cluster.id]

  network {
    network_id = hcloud_network.cluster.id
    ip         = "10.0.1.1"
  }

  labels = {
    role    = "master"
    cluster = var.cluster_name
  }
}

resource "hcloud_server" "general" {
  count      = var.general_count
  depends_on = [hcloud_network_subnet.nodes]

  name         = "${var.cluster_name}-general-${count.index + 1}"
  image        = "ubuntu-24.04"
  server_type  = var.general_type
  location     = var.location
  ssh_keys     = [hcloud_ssh_key.default.id]
  firewall_ids = [hcloud_firewall.cluster.id]

  network {
    network_id = hcloud_network.cluster.id
  }

  labels = {
    role    = "worker"
    pool    = "general"
    cluster = var.cluster_name
  }

  lifecycle {
    create_before_destroy = true
  }
}

resource "hcloud_server" "performance" {
  count      = var.perf_count
  depends_on = [hcloud_network_subnet.nodes]

  name         = "${var.cluster_name}-perf-${count.index + 1}"
  image        = "ubuntu-24.04"
  server_type  = var.perf_type
  location     = var.location
  ssh_keys     = [hcloud_ssh_key.default.id]
  firewall_ids = [hcloud_firewall.cluster.id]

  network {
    network_id = hcloud_network.cluster.id
  }

  labels = {
    role    = "worker"
    pool    = "performance"
    cluster = var.cluster_name
  }

  lifecycle {
    create_before_destroy = true
  }
}

resource "hcloud_server" "heavy" {
  count      = var.heavy_count
  depends_on = [hcloud_network_subnet.nodes]

  name         = "${var.cluster_name}-heavy-${count.index + 1}"
  image        = "ubuntu-24.04"
  server_type  = var.heavy_type
  location     = var.location
  ssh_keys     = [hcloud_ssh_key.default.id]
  firewall_ids = [hcloud_firewall.cluster.id]

  network {
    network_id = hcloud_network.cluster.id
  }

  labels = {
    role    = "worker"
    pool    = "heavy"
    cluster = var.cluster_name
  }

  lifecycle {
    create_before_destroy = true
  }
}
