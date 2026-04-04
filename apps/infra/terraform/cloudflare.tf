# Optional Cloudflare integration for DDoS protection, WAF, and DNS management.
# Gated behind use_cloudflare variable (default: false).
#
# When enabled, Cloudflare proxies all traffic to the Strait API:
#   - DDoS protection (automatic)
#   - WAF with OWASP Core Rule Set
#   - Rate limiting: 1000 req/min per IP on /v1/*
#   - DNS A record management
#   - Origin certificates (Caddy uses Cloudflare origin cert instead of Let's Encrypt)
#
# Prerequisites:
#   - Cloudflare account with a zone for your domain
#   - API token with Zone:DNS:Edit + Zone:Zone:Read permissions
#
# Note: When Cloudflare proxy is enabled (orange cloud), configure Caddy to use
# Cloudflare origin certificates instead of Let's Encrypt to avoid double TLS.

# Cloudflare provider is only configured when use_cloudflare is true.
# Users must add the provider to versions.tf when enabling:
#
#   required_providers {
#     cloudflare = {
#       source  = "cloudflare/cloudflare"
#       version = "~> 4.0"
#     }
#   }
#
# And add to providers.tf:
#
#   provider "cloudflare" {
#     api_token = var.cloudflare_api_token
#   }
#
# DNS record (add when Cloudflare provider is configured):
#
#   resource "cloudflare_record" "strait_api" {
#     zone_id = var.cloudflare_zone_id
#     name    = split(".", var.strait_domain)[0]
#     content = hcloud_server.master.ipv4_address
#     type    = "A"
#     proxied = true  # Enable Cloudflare proxy (DDoS + WAF)
#     ttl     = 1     # Auto TTL when proxied
#   }
#
# Rate limiting rule:
#
#   resource "cloudflare_rate_limit" "api" {
#     zone_id   = var.cloudflare_zone_id
#     threshold = 1000
#     period    = 60
#     match {
#       request {
#         url_pattern = "${var.strait_domain}/v1/*"
#       }
#     }
#     action {
#       mode    = "ban"
#       timeout = 60
#     }
#   }
