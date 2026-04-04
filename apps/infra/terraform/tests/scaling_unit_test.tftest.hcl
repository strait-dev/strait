# Scaling tests — verify that worker pool counts scale correctly.
# Plan mode only, no real infrastructure.

mock_provider "hcloud" {}

mock_provider "external" {}

mock_provider "null" {}

variables {
  hcloud_token = "test-token-not-real"
}

run "test_scale_general_to_three" {
  command = plan

  variables {
    general_count = 3
  }

  assert {
    condition     = length(hcloud_server.general) == 3
    error_message = "Should create exactly 3 general workers"
  }
}

run "test_scale_general_to_zero" {
  command = plan

  variables {
    general_count = 0
  }

  assert {
    condition     = length(hcloud_server.general) == 0
    error_message = "Should create no general workers when count is 0"
  }
}

run "test_enable_heavy_pool" {
  command = plan

  variables {
    heavy_count = 2
  }

  assert {
    condition     = length(hcloud_server.heavy) == 2
    error_message = "Should create 2 heavy workers"
  }

  assert {
    condition     = hcloud_server.heavy[0].server_type == "cax41"
    error_message = "Heavy workers should use cax41 server type"
  }

  assert {
    condition     = hcloud_server.heavy[0].labels["pool"] == "heavy"
    error_message = "Heavy workers should have pool=heavy label"
  }
}

run "test_scale_performance_to_three" {
  command = plan

  variables {
    perf_count = 3
  }

  assert {
    condition     = length(hcloud_server.performance) == 3
    error_message = "Should create exactly 3 performance workers"
  }

  assert {
    condition     = hcloud_server.performance[0].server_type == "cax31"
    error_message = "Performance workers should use cax31 server type"
  }
}

run "test_all_pools_at_max" {
  command = plan

  variables {
    general_count = 10
    perf_count    = 5
    heavy_count   = 5
  }

  assert {
    condition     = length(hcloud_server.general) == 10
    error_message = "Should create 10 general workers at max"
  }

  assert {
    condition     = length(hcloud_server.performance) == 5
    error_message = "Should create 5 performance workers at max"
  }

  assert {
    condition     = length(hcloud_server.heavy) == 5
    error_message = "Should create 5 heavy workers at max"
  }
}

run "test_custom_server_types" {
  command = plan

  variables {
    general_type = "cax31"
    perf_type    = "cax41"
  }

  assert {
    condition     = hcloud_server.general[0].server_type == "cax31"
    error_message = "General workers should use overridden server type"
  }

  assert {
    condition     = hcloud_server.performance[0].server_type == "cax41"
    error_message = "Performance workers should use overridden server type"
  }
}

run "test_eu_location_with_correct_zone" {
  command = plan

  variables {
    location = "fsn1"
  }

  assert {
    condition     = hcloud_server.master.location == "fsn1"
    error_message = "Master should use fsn1 location"
  }

  assert {
    condition     = hcloud_server.general[0].location == "fsn1"
    error_message = "Workers should use same location as master"
  }
}
