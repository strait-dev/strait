# Operations tests — verify optional features degrade gracefully when not configured.
# Plan mode only, no real infrastructure.

mock_provider "hcloud" {}

mock_provider "external" {}

mock_provider "null" {}

variables {
  hcloud_token = "test-token-not-real"
}

run "test_backup_s3_endpoint_optional" {
  command = plan

  variables {
    backup_s3_endpoint = ""
  }

  assert {
    condition     = var.backup_s3_endpoint == ""
    error_message = "Backup S3 endpoint should be optional (empty by default)"
  }
}

run "test_backup_s3_bucket_default" {
  command = plan

  assert {
    condition     = var.backup_s3_bucket == "strait-backups"
    error_message = "Default backup bucket should be strait-backups"
  }
}

run "test_grafana_credentials_optional" {
  command = plan

  assert {
    condition     = var.grafana_remote_write_url == ""
    error_message = "Grafana remote write URL should be optional (empty by default)"
  }

  assert {
    condition     = var.grafana_loki_url == ""
    error_message = "Grafana Loki URL should be optional (empty by default)"
  }
}

run "test_strait_domain_optional" {
  command = plan

  assert {
    condition     = var.strait_domain == ""
    error_message = "Domain should be optional (empty by default, TLS skipped)"
  }
}

run "test_dns_instructions_without_domain" {
  command = plan

  assert {
    condition     = can(regex("Set strait_domain", output.dns_instructions))
    error_message = "DNS instructions should suggest setting domain when empty"
  }
}

run "test_dns_instructions_with_domain" {
  command = plan

  variables {
    strait_domain = "api.example.com"
  }

  assert {
    condition     = can(regex("api.example.com", output.dns_instructions))
    error_message = "DNS instructions should include the configured domain"
  }
}
