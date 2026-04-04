# ──────────────────────────────────────────────
# Master node (k3s server)
# ──────────────────────────────────────────────

resource "hcloud_server" "master" {
  name         = "${var.cluster_name}-master"
  image        = "ubuntu-24.04"
  server_type  = var.master_type
  location     = var.location
  ssh_keys     = [hcloud_ssh_key.default.id]
  firewall_ids = [hcloud_firewall.cluster.id]

  user_data = templatefile("${path.module}/cloud-init-master.yaml", {
    master_public_ip = "" # Will be set after creation via null_resource
  })

  network {
    network_id = hcloud_network.cluster.id
    ip         = "10.0.1.1"
  }

  depends_on = [hcloud_network_subnet.nodes]

  labels = {
    role    = "master"
    cluster = var.cluster_name
  }
}

# Re-run cloud-init after we know the public IP (for TLS SAN).
resource "null_resource" "master_k3s" {
  depends_on = [hcloud_server.master]

  connection {
    type        = "ssh"
    host        = hcloud_server.master.ipv4_address
    user        = "root"
    private_key = file(pathexpand(var.ssh_private_key_path))
  }

  provisioner "remote-exec" {
    inline = [
      "curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC='server --disable traefik --disable servicelb --flannel-iface eth1 --node-label strait.dev/pool=general --tls-san ${hcloud_server.master.ipv4_address}' sh -",
      "while ! /usr/local/bin/kubectl get nodes 2>/dev/null; do sleep 2; done",
      "echo 'k3s master ready'",
    ]
  }
}

# Fetch the node token for workers.
data "external" "k3s_token" {
  depends_on = [null_resource.master_k3s]

  program = ["bash", "-c", <<-EOF
    TOKEN=$(ssh -o StrictHostKeyChecking=no -i ${pathexpand(var.ssh_private_key_path)} root@${hcloud_server.master.ipv4_address} "cat /var/lib/rancher/k3s/server/node-token" 2>/dev/null)
    echo "{\"token\": \"$TOKEN\"}"
  EOF
  ]
}

# ──────────────────────────────────────────────
# General pool workers (micro, small jobs)
# ──────────────────────────────────────────────

resource "hcloud_server" "general" {
  count        = var.general_count
  name         = "${var.cluster_name}-general-${count.index + 1}"
  image        = "ubuntu-24.04"
  server_type  = var.general_type
  location     = var.location
  ssh_keys     = [hcloud_ssh_key.default.id]
  firewall_ids = [hcloud_firewall.cluster.id]

  user_data = templatefile("${path.module}/cloud-init-worker.yaml", {
    master_private_ip = "10.0.1.1"
    node_pool         = "general"
    k3s_token         = data.external.k3s_token.result.token
  })

  network {
    network_id = hcloud_network.cluster.id
  }

  depends_on = [hcloud_network_subnet.nodes, null_resource.master_k3s]

  labels = {
    role    = "worker"
    pool    = "general"
    cluster = var.cluster_name
  }
}

# ──────────────────────────────────────────────
# Performance pool workers (medium jobs)
# ──────────────────────────────────────────────

resource "hcloud_server" "performance" {
  count        = var.perf_count
  name         = "${var.cluster_name}-perf-${count.index + 1}"
  image        = "ubuntu-24.04"
  server_type  = var.perf_type
  location     = var.location
  ssh_keys     = [hcloud_ssh_key.default.id]
  firewall_ids = [hcloud_firewall.cluster.id]

  user_data = templatefile("${path.module}/cloud-init-worker.yaml", {
    master_private_ip = "10.0.1.1"
    node_pool         = "performance"
    k3s_token         = data.external.k3s_token.result.token
  })

  network {
    network_id = hcloud_network.cluster.id
  }

  depends_on = [hcloud_network_subnet.nodes, null_resource.master_k3s]

  labels = {
    role    = "worker"
    pool    = "performance"
    cluster = var.cluster_name
  }
}

# ──────────────────────────────────────────────
# Heavy pool workers (large jobs) — disabled by default
# ──────────────────────────────────────────────

resource "hcloud_server" "heavy" {
  count        = var.heavy_count
  name         = "${var.cluster_name}-heavy-${count.index + 1}"
  image        = "ubuntu-24.04"
  server_type  = var.heavy_type
  location     = var.location
  ssh_keys     = [hcloud_ssh_key.default.id]
  firewall_ids = [hcloud_firewall.cluster.id]

  user_data = templatefile("${path.module}/cloud-init-worker.yaml", {
    master_private_ip = "10.0.1.1"
    node_pool         = "heavy"
    k3s_token         = data.external.k3s_token.result.token
  })

  network {
    network_id = hcloud_network.cluster.id
  }

  depends_on = [hcloud_network_subnet.nodes, null_resource.master_k3s]

  labels = {
    role    = "worker"
    pool    = "heavy"
    cluster = var.cluster_name
  }
}
