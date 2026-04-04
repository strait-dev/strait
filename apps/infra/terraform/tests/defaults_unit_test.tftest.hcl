# Unit tests for default variable values and resource configuration.
# Plan mode only — no real infrastructure created.

mock_provider "hcloud" {}

mock_provider "external" {}

mock_provider "null" {}

variables {
  hcloud_token = "test-token-not-real"
}

run "test_default_location_is_ashburn" {
  command = plan

  assert {
    condition     = var.location == "ash"
    error_message = "Default location should be ash (Ashburn VA) for us-east-1 co-location"
  }
}

run "test_default_cluster_name" {
  command = plan

  assert {
    condition     = var.cluster_name == "strait"
    error_message = "Default cluster name should be strait"
  }
}

run "test_default_node_counts" {
  command = plan

  assert {
    condition     = var.general_count == 1
    error_message = "Default general pool should have 1 worker"
  }

  assert {
    condition     = var.perf_count == 1
    error_message = "Default performance pool should have 1 worker"
  }

  assert {
    condition     = var.heavy_count == 0
    error_message = "Default heavy pool should be disabled (0 workers)"
  }
}

run "test_default_server_types_are_arm" {
  command = plan

  assert {
    condition     = var.master_type == "cax21"
    error_message = "Master should default to ARM cax21"
  }

  assert {
    condition     = var.general_type == "cax21"
    error_message = "General workers should default to ARM cax21"
  }

  assert {
    condition     = var.perf_type == "cax31"
    error_message = "Performance workers should default to ARM cax31"
  }

  assert {
    condition     = var.heavy_type == "cax41"
    error_message = "Heavy workers should default to ARM cax41"
  }
}

run "test_ha_default_is_single_server" {
  command = plan

  assert {
    condition     = var.server_count == 1
    error_message = "Default should be single server (non-HA)"
  }
}

run "test_master_server_configuration" {
  command = plan

  assert {
    condition     = hcloud_server.master.image == "ubuntu-24.04"
    error_message = "Master should use Ubuntu 24.04"
  }

  assert {
    condition     = hcloud_server.master.server_type == "cax21"
    error_message = "Master server type should match master_type variable"
  }

  assert {
    condition     = hcloud_server.master.location == "ash"
    error_message = "Master should be in Ashburn"
  }
}

run "test_master_labels" {
  command = plan

  assert {
    condition     = hcloud_server.master.labels["role"] == "master"
    error_message = "Master should have role=master label"
  }

  assert {
    condition     = hcloud_server.master.labels["cluster"] == "strait"
    error_message = "Master should have cluster=strait label"
  }
}

run "test_network_configuration" {
  command = plan

  assert {
    condition     = hcloud_network.cluster.ip_range == "10.0.0.0/16"
    error_message = "Network CIDR should be 10.0.0.0/16"
  }

  assert {
    condition     = hcloud_network_subnet.nodes.ip_range == "10.0.1.0/24"
    error_message = "Subnet should be 10.0.1.0/24"
  }

  assert {
    condition     = hcloud_network_subnet.nodes.network_zone == "us-east"
    error_message = "Network zone should be us-east for Ashburn"
  }
}

run "test_general_worker_count" {
  command = plan

  assert {
    condition     = length(hcloud_server.general) == 1
    error_message = "Should create exactly 1 general worker by default"
  }
}

run "test_general_worker_labels" {
  command = plan

  assert {
    condition     = hcloud_server.general[0].labels["role"] == "worker"
    error_message = "General worker should have role=worker label"
  }

  assert {
    condition     = hcloud_server.general[0].labels["pool"] == "general"
    error_message = "General worker should have pool=general label"
  }
}

run "test_performance_worker_count" {
  command = plan

  assert {
    condition     = length(hcloud_server.performance) == 1
    error_message = "Should create exactly 1 performance worker by default"
  }
}

run "test_heavy_pool_disabled_by_default" {
  command = plan

  assert {
    condition     = length(hcloud_server.heavy) == 0
    error_message = "Heavy pool should be disabled (0 workers) by default"
  }
}

run "test_firewall_name" {
  command = plan

  assert {
    condition     = hcloud_firewall.cluster.name == "strait-firewall"
    error_message = "Firewall name should follow cluster_name-firewall pattern"
  }
}

run "test_ssh_key_name" {
  command = plan

  assert {
    condition     = hcloud_ssh_key.default.name == "strait-key"
    error_message = "SSH key name should follow cluster_name-key pattern"
  }
}
