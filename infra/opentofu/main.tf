locals {
  server_user_data = var.bootstrap_mode == "ubuntu-docker" ? templatefile("${path.module}/cloud-init.tftpl", {
    public_base_url = var.public_base_url
  }) : null
}

resource "hcloud_ssh_key" "deploy" {
  name       = "${var.name}-deploy"
  public_key = var.ssh_public_key
}

resource "hcloud_firewall" "mailservice" {
  name = "${var.name}-fw"

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "22"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "80"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "443"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "25"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "143"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "993"
    source_ips = ["0.0.0.0/0", "::/0"]
  }
}

resource "hcloud_server" "app" {
  name         = var.name
  image        = var.image
  server_type  = var.server_type
  location     = var.location
  ssh_keys     = [hcloud_ssh_key.deploy.id]
  firewall_ids = [hcloud_firewall.mailservice.id]

  labels = {
    app            = var.name
    bootstrap_mode = var.bootstrap_mode
  }

  user_data = local.server_user_data
}

locals {
  cloudflare_zone_name = var.cloudflare_zone_name != "" ? var.cloudflare_zone_name : var.public_hostname
}

data "cloudflare_zones" "domain" {
  filter {
    name = local.cloudflare_zone_name
  }
}

locals {
  zone_id = data.cloudflare_zones.domain.zones[0].id
}

resource "cloudflare_record" "domain_a" {
  count   = var.create_domain_a_record ? 1 : 0
  zone_id = local.zone_id
  name    = var.public_hostname
  content = hcloud_server.app.ipv4_address
  type    = "A"
  ttl     = 300
  proxied = false
}

resource "cloudflare_record" "mail_a" {
  zone_id = local.zone_id
  name    = "mail.${var.public_hostname}"
  content = hcloud_server.app.ipv4_address
  type    = "A"
  ttl     = 300
  proxied = false
}

resource "cloudflare_record" "mx_primary" {
  zone_id  = local.zone_id
  name     = var.public_hostname
  content  = "mail.${var.public_hostname}"
  type     = "MX"
  priority = 10
  ttl      = 300
}

output "server_ipv4" {
  value = hcloud_server.app.ipv4_address
}

output "server_name" {
  value = hcloud_server.app.name
}

output "public_hostname" {
  value = var.public_hostname
}

output "mail_hostname" {
  value = cloudflare_record.mail_a.hostname
}
