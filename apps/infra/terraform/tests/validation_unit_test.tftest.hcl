# Validation tests — verify that variable validation rules reject bad inputs.
# Plan mode only, no real infrastructure.

mock_provider "hcloud" {}

mock_provider "external" {}

mock_provider "null" {}

variables {
  hcloud_token = "test-token-not-real"
}

run "test_invalid_location_rejected" {
  command = plan

  variables {
    location = "invalid-dc"
  }

  expect_failures = [
    var.location
  ]
}

run "test_valid_locations_accepted" {
  command = plan

  variables {
    location = "fsn1"
  }

  assert {
    condition     = var.location == "fsn1"
    error_message = "fsn1 should be accepted as a valid location"
  }
}

run "test_invalid_cluster_name_uppercase" {
  command = plan

  variables {
    cluster_name = "MyCluster"
  }

  expect_failures = [
    var.cluster_name
  ]
}

run "test_invalid_cluster_name_too_long" {
  command = plan

  variables {
    cluster_name = "this-name-is-way-too-long-for-the-validation-rule"
  }

  expect_failures = [
    var.cluster_name
  ]
}

run "test_invalid_server_count" {
  command = plan

  variables {
    server_count = 2
  }

  expect_failures = [
    var.server_count
  ]
}

run "test_valid_ha_server_count" {
  command = plan

  variables {
    server_count = 3
  }

  assert {
    condition     = var.server_count == 3
    error_message = "Server count of 3 (HA) should be accepted"
  }
}

run "test_negative_general_count_rejected" {
  command = plan

  variables {
    general_count = -1
  }

  expect_failures = [
    var.general_count
  ]
}

run "test_excessive_general_count_rejected" {
  command = plan

  variables {
    general_count = 11
  }

  expect_failures = [
    var.general_count
  ]
}

run "test_negative_perf_count_rejected" {
  command = plan

  variables {
    perf_count = -1
  }

  expect_failures = [
    var.perf_count
  ]
}

run "test_negative_heavy_count_rejected" {
  command = plan

  variables {
    heavy_count = -1
  }

  expect_failures = [
    var.heavy_count
  ]
}

run "test_excessive_heavy_count_rejected" {
  command = plan

  variables {
    heavy_count = 6
  }

  expect_failures = [
    var.heavy_count
  ]
}
