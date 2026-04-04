# Security tests — verify firewall rules, network isolation, and server hardening.
# Plan mode only, no real infrastructure.

mock_provider "hcloud" {}

mock_provider "external" {}

mock_provider "null" {}

variables {
  hcloud_token = "test-token-not-real"
}

run "test_firewall_name_matches_cluster" {
  command = plan

  assert {
    condition     = hcloud_firewall.cluster.name == "strait-firewall"
    error_message = "Firewall name should be strait-firewall"
  }
}

run "test_master_has_role_label" {
  command = plan

  assert {
    condition     = hcloud_server.master.labels["role"] == "master"
    error_message = "Master must have role=master label"
  }
}

run "test_general_worker_has_pool_label" {
  command = plan

  assert {
    condition     = hcloud_server.general[0].labels["pool"] == "general"
    error_message = "General worker must have pool=general label"
  }
}

run "test_performance_worker_has_pool_label" {
  command = plan

  assert {
    condition     = hcloud_server.performance[0].labels["pool"] == "performance"
    error_message = "Performance worker must have pool=performance label"
  }
}

run "test_network_is_private" {
  command = plan

  assert {
    condition     = hcloud_network.cluster.ip_range == "10.0.0.0/16"
    error_message = "Network CIDR must be 10.0.0.0/16 (private)"
  }
}

run "test_subnet_in_private_range" {
  command = plan

  assert {
    condition     = hcloud_network_subnet.nodes.ip_range == "10.0.1.0/24"
    error_message = "Subnet must be in private range 10.0.1.0/24"
  }
}

run "test_server_count_validation_rejects_2" {
  command = plan

  variables {
    server_count = 2
  }

  expect_failures = [
    var.server_count
  ]
}

run "test_server_count_validation_accepts_3" {
  command = plan

  variables {
    server_count = 3
  }

  assert {
    condition     = var.server_count == 3
    error_message = "Server count of 3 (HA) should be accepted"
  }
}

run "test_cluster_name_rejects_uppercase" {
  command = plan

  variables {
    cluster_name = "MyCluster"
  }

  expect_failures = [
    var.cluster_name
  ]
}
