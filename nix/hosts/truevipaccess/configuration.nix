{ lib, pkgs, ... }:

{
  imports = [
    ../../modules/mailservice-gitops.nix
  ];

  system.stateVersion = "24.11";

  networking.hostName = "truevipaccess";

  time.timeZone = "UTC";

  services.openssh.enable = true;

  users.users.root.openssh.authorizedKeys.keys = [
    # Replace with the real deploy key before switching the host.
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFQCVB2dV5WyOd0dK8MH0tIkgmLv71OTEdcMpJ2Whet0 mailservice-deploy"
  ];

  environment.systemPackages = with pkgs; [
    curl
    git
  ];

  services.mailserviceGitOps = {
    enable = true;
    environmentFile = "/var/lib/secrets/mailservice.env";

    # Git is the source of truth for the deployed artifact.
    # Update these pinned image refs in Git for each rollout.
    apiImage = "ghcr.io/atvirokodosprendimai/mailservice-api:sha-PLACEHOLDER";
    mailreceiveImage = "ghcr.io/atvirokodosprendimai/mailservice-mailreceive:sha-PLACEHOLDER";
  };
}
