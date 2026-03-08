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
  default     = "fsn1"
}

variable "server_type" {
  description = "Hetzner server type."
  type        = string
  default     = "cpx21"
}

variable "image" {
  description = "Server image."
  type        = string
  default     = "ubuntu-24.04"
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
