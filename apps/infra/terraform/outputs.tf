output "general_ips" {
  description = "Public IPs of general pool workers."
  value       = hcloud_server.general[*].ipv4_address
}

output "heavy_ips" {
  description = "Public IPs of heavy pool workers."
  value       = hcloud_server.heavy[*].ipv4_address
}

output "kubeconfig_command" {
  description = "Command to fetch the kubeconfig from the master node."
  value       = "scp -i ${var.ssh_private_key_path} root@${hcloud_server.master.ipv4_address}:/etc/rancher/k3s/k3s.yaml ./kubeconfig && sed -i '' 's/127.0.0.1/${hcloud_server.master.ipv4_address}/g' ./kubeconfig"
}

output "master_ip" {
  description = "Public IP of the k3s master node."
  value       = hcloud_server.master.ipv4_address
}

output "master_private_ip" {
  description = "Private IP of the k3s master node."
  value       = "10.0.1.1"
}

output "performance_ips" {
  description = "Public IPs of performance pool workers."
  value       = hcloud_server.performance[*].ipv4_address
}

output "ssh_command" {
  description = "SSH into the master node."
  value       = "ssh -i ${var.ssh_private_key_path} root@${hcloud_server.master.ipv4_address}"
}

output "dns_instructions" {
  description = "DNS A record to create for the Strait API."
  value       = var.strait_domain != "" ? "Create DNS A record: ${var.strait_domain} -> ${hcloud_server.master.ipv4_address}" : "Set strait_domain variable to enable TLS. Then create an A record pointing to ${hcloud_server.master.ipv4_address}."
}
