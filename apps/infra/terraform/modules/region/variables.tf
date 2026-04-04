# Variables for a single-region k3s cluster.
# Each region gets its own master, workers, network, and firewall.

variable "location" {
  description = "Hetzner datacenter location (ash, hil, fsn1, nbg1, hel1)."
  type        = string

  validation {
    condition     = contains(["ash", "hil", "fsn1", "nbg1", "hel1"], var.location)
    error_message = "Location must be one of: ash, hil, fsn1, nbg1, hel1."
  }
}

variable "cluster_name" {
  description = "Name prefix for all resources in this region."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,29}$", var.cluster_name))
    error_message = "Cluster name must be lowercase alphanumeric with hyphens, 1-30 chars."
  }
}

variable "hcloud_token" {
  description = "Hetzner Cloud API token."
  type        = string
  sensitive   = true
}

variable "ssh_key_path" {
  description = "Path to SSH public key."
  type        = string
  default     = "~/.ssh/id_ed25519.pub"
}

variable "ssh_private_key_path" {
  description = "Path to SSH private key."
  type        = string
  default     = "~/.ssh/id_ed25519"
}

variable "master_type" {
  description = "Server type for the k3s master node."
  type        = string
  default     = "cax21"
}

variable "general_count" {
  description = "Number of general pool worker nodes."
  type        = number
  default     = 1

  validation {
    condition     = var.general_count >= 0 && var.general_count <= 10
    error_message = "General pool count must be between 0 and 10."
  }
}

variable "general_type" {
  description = "Server type for general pool workers."
  type        = string
  default     = "cax21"
}

variable "perf_count" {
  description = "Number of performance pool worker nodes."
  type        = number
  default     = 1

  validation {
    condition     = var.perf_count >= 0 && var.perf_count <= 5
    error_message = "Performance pool count must be between 0 and 5."
  }
}

variable "perf_type" {
  description = "Server type for performance pool workers."
  type        = string
  default     = "cax31"
}

variable "heavy_count" {
  description = "Number of heavy pool worker nodes."
  type        = number
  default     = 0

  validation {
    condition     = var.heavy_count >= 0 && var.heavy_count <= 5
    error_message = "Heavy pool count must be between 0 and 5."
  }
}

variable "heavy_type" {
  description = "Server type for heavy pool workers."
  type        = string
  default     = "cax41"
}

variable "ssh_allowed_ips" {
  description = "CIDR blocks allowed to SSH and access K8s API."
  type        = list(string)
  default     = ["0.0.0.0/0", "::/0"]
}
