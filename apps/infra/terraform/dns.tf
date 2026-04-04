# Optional reverse DNS for the master node public IP.
# This sets the PTR record so the IP resolves back to your domain.
# Only created when strait_domain is set.
resource "hcloud_rdns" "master" {
  count      = var.strait_domain != "" ? 1 : 0
  server_id  = hcloud_server.master.id
  ip_address = hcloud_server.master.ipv4_address
  dns_ptr    = var.strait_domain
}

# DNS A record must be created in your DNS provider (Cloudflare, Route53, etc.).
# Terraform outputs the required record below.
#
# Cloudflare example:
#   resource "cloudflare_record" "strait" {
#     zone_id = var.cloudflare_zone_id
#     name    = "api"
#     content = hcloud_server.master.ipv4_address
#     type    = "A"
#     proxied = false  # Caddy handles TLS, don't proxy through Cloudflare
#   }
#
# Route53 example:
#   resource "aws_route53_record" "strait" {
#     zone_id = var.route53_zone_id
#     name    = "api.yourdomain.com"
#     type    = "A"
#     ttl     = 300
#     records = [hcloud_server.master.ipv4_address]
#   }
