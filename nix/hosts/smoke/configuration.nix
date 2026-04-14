{ lib, pkgs, ... }:

{
  imports = [
    ../../modules/mailservice-gitops.nix
    ../../modules/coroot-node-agent.nix
    ./hardware-configuration.nix
  ];

  system.stateVersion = "24.11";

  networking.hostName = "smoke";

  time.timeZone = "UTC";

  services.openssh.enable = true;
  services.openssh.openFirewall = true;

  users.users.root.openssh.authorizedKeys = {
    keys = [
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFQCVB2dV5WyOd0dK8MH0tIkgmLv71OTEdcMpJ2Whet0 mailservice-deploy"
    ];
  };

  environment.systemPackages = with pkgs; [
    curl
    git
  ];

  services.mailserviceGitOps = {
    enable = true;
    mailDomain = "smoke.truevipaccess.com";
    environmentFile = "/var/lib/secrets/mailservice.env";
    cloudflaredEnvironmentFile = "/var/lib/secrets/cloudflared.env";
  };

  services.corootNodeAgent = {
    enable = true;
    collectorEndpoint = "https://table.beerpub.dev";
    apiKeyFile = "/var/lib/secrets/coroot.env";
    scrapeInterval = "15s";
  };

  # Disable cloudflared tunnel — smoke server uses direct nginx instead
  systemd.services.mailservice-cloudflared.enable = lib.mkForce false;

  # Reverse proxy: smoke.truevipaccess.com → local API
  services.nginx.virtualHosts."smoke.truevipaccess.com" = {
    enableACME = true;
    forceSSL = true;
    locations."/" = {
      proxyPass = "http://127.0.0.1:8080";
      proxyWebsockets = true;
    };
  };
}
