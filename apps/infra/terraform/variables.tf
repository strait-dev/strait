variable "hcloud_token" {
  description = "Hetzner Cloud API token. Get from https://console.hetzner.cloud > API Tokens."
  type        = string
  sensitive   = true
}

variable "ssh_key_path" {
  description = "Path to SSH public key for server access."
  type        = string
  default     = "~/.ssh/id_ed25519.pub"
}

variable "ssh_private_key_path" {
  description = "Path to SSH private key for provisioning."
  type        = string
  default     = "~/.ssh/id_ed25519"
}

variable "location" {
  description = "Hetzner datacenter location. ash = Ashburn VA (us-east-1 equivalent), fsn1 = Falkenstein, nbg1 = Nuremberg, hel1 = Helsinki."
  type        = string
  default     = "ash"
}

variable "cluster_name" {
  description = "Name prefix for all resources."
  type        = string
  default     = "strait"
}

# Node types (Hetzner ARM servers).
variable "master_type" {
  description = "Server type for the k3s master node."
  type        = string
  default     = "cax21" # 4 vCPU ARM, 8GB, 40GB — $7/mo
}

variable "general_type" {
  description = "Server type for general pool workers (micro, small jobs)."
  type        = string
  default     = "cax21" # 4 vCPU ARM, 8GB, 40GB — $7/mo
}

variable "general_count" {
  description = "Number of general pool worker nodes."
  type        = number
  default     = 1
}

variable "perf_type" {
  description = "Server type for performance pool workers (medium jobs)."
  type        = string
  default     = "cax31" # 8 vCPU ARM, 16GB, 80GB — $14/mo
}

variable "perf_count" {
  description = "Number of performance pool worker nodes."
  type        = number
  default     = 1
}

variable "heavy_type" {
  description = "Server type for heavy pool workers (large jobs)."
  type        = string
  default     = "cax41" # 16 vCPU ARM, 32GB, 160GB — $27/mo
}

variable "heavy_count" {
  description = "Number of heavy pool worker nodes. Set to 0 to skip."
  type        = number
  default     = 0
}

# HA configuration.
variable "server_count" {
  description = "Number of k3s server nodes. Set to 3 for HA (etcd quorum)."
  type        = number
  default     = 1
}

# Backup configuration (Hetzner Object Storage / S3-compatible).
variable "backup_s3_endpoint" {
  description = "S3 endpoint for etcd snapshot backups. Leave empty to disable."
  type        = string
  default     = ""
}

variable "backup_s3_bucket" {
  description = "S3 bucket name for etcd snapshots."
  type        = string
  default     = "strait-backups"
}

variable "backup_s3_access_key" {
  description = "S3 access key for backup storage."
  type        = string
  sensitive   = true
  default     = ""
}

variable "backup_s3_secret_key" {
  description = "S3 secret key for backup storage."
  type        = string
  sensitive   = true
  default     = ""
}

# Monitoring (Grafana Cloud).
variable "grafana_remote_write_url" {
  description = "Grafana Cloud Prometheus remote write URL."
  type        = string
  default     = ""
}

variable "grafana_remote_write_username" {
  description = "Grafana Cloud Prometheus username."
  type        = string
  default     = ""
}

variable "grafana_remote_write_password" {
  description = "Grafana Cloud Prometheus API key."
  type        = string
  sensitive   = true
  default     = ""
}

variable "grafana_loki_url" {
  description = "Grafana Cloud Loki push URL."
  type        = string
  default     = ""
}

variable "grafana_loki_username" {
  description = "Grafana Cloud Loki username."
  type        = string
  default     = ""
}

variable "grafana_loki_password" {
  description = "Grafana Cloud Loki API key."
  type        = string
  sensitive   = true
  default     = ""
}
