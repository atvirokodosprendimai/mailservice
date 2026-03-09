variable "hcloud_token" {
  description = "Hetzner Cloud API token."
  type        = string
  sensitive   = true
}

variable "name" {
  description = "Deployment name prefix."
  type        = string
  default     = "mailservice"
}

variable "location" {
  description = "Hetzner location."
  type        = string
  default     = "hel1"
}

variable "server_type" {
  description = "Hetzner server type."
  type        = string
  default     = "cpx22"
}

variable "image" {
  description = "Server image."
  type        = string
  default     = "ubuntu-24.04"
}

variable "bootstrap_mode" {
  description = "Host bootstrap mode. Use ubuntu-docker for the current Docker-based host, or none for a prebuilt NixOS/custom image path."
  type        = string
  default     = "ubuntu-docker"

  validation {
    condition     = contains(["ubuntu-docker", "none"], var.bootstrap_mode)
    error_message = "bootstrap_mode must be one of: ubuntu-docker, none."
  }
}

variable "ssh_public_key" {
  description = "SSH public key material for server access."
  type        = string
}

variable "public_base_url" {
  description = "Public base URL for the application."
  type        = string
}

variable "public_hostname" {
  description = "Public DNS hostname for the application."
  type        = string
  default     = "truevipaccess.com"
}

variable "cloudflare_api_token" {
  description = "Cloudflare API token with DNS edit permissions."
  type        = string
  sensitive   = true
}
