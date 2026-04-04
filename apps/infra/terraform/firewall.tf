resource "hcloud_firewall" "cluster" {
  name = "${var.cluster_name}-firewall"

  # SSH access (restrict via ssh_allowed_ips in production).
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "22"
    source_ips = var.ssh_allowed_ips
  }

  # K8s API server (restrict via ssh_allowed_ips — same admin access pattern).
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "6443"
    source_ips = var.ssh_allowed_ips
  }

  # Kubelet API (node-to-node, restrict to private network).
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "10250"
    source_ips = ["10.0.0.0/16"]
  }

  # Flannel VXLAN (k3s default CNI, node-to-node).
  rule {
    direction  = "in"
    protocol   = "udp"
    port       = "8472"
    source_ips = ["10.0.0.0/16"]
  }

  # Caddy HTTPS (Strait API with TLS termination).
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "30443"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  # Caddy HTTP (Let's Encrypt ACME challenge + HTTPS redirect).
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "30080"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  # Allow all outbound traffic.
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
