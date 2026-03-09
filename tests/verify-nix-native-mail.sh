#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

host_expr='
let
  cfg = (builtins.getFlake (toString ./.)).nixosConfigurations.truevipaccess.config;
in
{
  hasMailreceiveContainer = builtins.hasAttr "mailservice-mailreceive" cfg.virtualisation.oci-containers.containers;
  dockerEnabled = cfg.virtualisation.docker.enable or false;
  apiMailDomain = cfg.systemd.services.mailservice-api.environment.MAIL_DOMAIN or "";
  postfixEnabled = cfg.services.postfix.enable or false;
  dovecotEnabled = cfg.services.dovecot2.enable or false;
}
'

result="$(nix eval --impure --json --expr "$host_expr")"

has_mailreceive_container="$(printf '%s' "$result" | jq -r '.hasMailreceiveContainer')"
docker_enabled="$(printf '%s' "$result" | jq -r '.dockerEnabled')"
api_mail_domain="$(printf '%s' "$result" | jq -r '.apiMailDomain')"
postfix_enabled="$(printf '%s' "$result" | jq -r '.postfixEnabled')"
dovecot_enabled="$(printf '%s' "$result" | jq -r '.dovecotEnabled')"

if [[ "$has_mailreceive_container" != "false" ]]; then
  echo "expected native mail stack; mailservice-mailreceive container is still configured" >&2
  exit 1
fi

if [[ "$docker_enabled" != "false" ]]; then
  echo "expected docker to be disabled for the native NixOS mail stack" >&2
  exit 1
fi

if [[ "$api_mail_domain" != "truevipaccess.com" ]]; then
  echo "expected API MAIL_DOMAIN to be pinned from NixOS mailDomain" >&2
  exit 1
fi

if [[ "$postfix_enabled" != "true" ]]; then
  echo "expected services.postfix.enable = true" >&2
  exit 1
fi

if [[ "$dovecot_enabled" != "true" ]]; then
  echo "expected services.dovecot2.enable = true" >&2
  exit 1
fi
