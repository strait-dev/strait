resource "hcloud_ssh_key" "default" {
  name       = "${var.cluster_name}-key"
  public_key = file(pathexpand(var.ssh_key_path))
}
