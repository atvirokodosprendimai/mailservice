{ lib, config, pkgs, ... }:

let
  cfg = config.services.mailserviceGitOps;
in
{
  options.services.mailserviceGitOps = {
    enable = lib.mkEnableOption "mailservice GitOps runtime";

    environmentFile = lib.mkOption {
      type = lib.types.str;
      default = "/var/lib/secrets/mailservice.env";
      description = "Runtime env file for secrets and non-Git-managed values.";
    };

    apiImage = lib.mkOption {
      type = lib.types.str;
      description = "Pinned OCI image reference for the API service.";
    };

    mailreceiveImage = lib.mkOption {
      type = lib.types.str;
      description = "Pinned OCI image reference for the inbound mail runtime.";
    };

    cloudflaredImage = lib.mkOption {
      type = lib.types.str;
      default = "cloudflare/cloudflared:2026.2.0";
      description = "Cloudflare tunnel image reference.";
    };
  };

  config = lib.mkIf cfg.enable {
    virtualisation.oci-containers.backend = "docker";
    virtualisation.docker.enable = true;

    networking.firewall.allowedTCPPorts = [ 25 143 ];

    systemd.tmpfiles.rules = [
      "d /var/lib/mailservice 0755 root root -"
      "d /var/lib/mailservice/data 0755 root root -"
      "d /var/lib/mailservice/vhosts 0755 root root -"
      "d /var/lib/secrets 0700 root root -"
    ];

    virtualisation.oci-containers.containers = {
      mailservice-api = {
        image = cfg.apiImage;
        environmentFiles = [ cfg.environmentFile ];
        environment = {
          HTTP_ADDR = ":8080";
          DATABASE_DSN = "/data/mailservice.db";
        };
        volumes = [
          "/var/lib/mailservice/data:/data"
        ];
        ports = [
          "127.0.0.1:8080:8080"
        ];
        extraOptions = [ "--pull=always" ];
      };

      mailservice-mailreceive = {
        image = cfg.mailreceiveImage;
        environmentFiles = [ cfg.environmentFile ];
        environment = {
          MAIL_DB_PATH = "/data/mailservice.db";
          MAIL_DEBUG = "1";
        };
        volumes = [
          "/var/lib/mailservice/data:/data"
          "/var/lib/mailservice/vhosts:/var/mail/vhosts"
        ];
        ports = [
          "25:25"
          "143:143"
        ];
        extraOptions = [ "--pull=always" ];
      };

      mailservice-cloudflared = {
        image = cfg.cloudflaredImage;
        environmentFiles = [ cfg.environmentFile ];
        cmd = [
          "tunnel"
          "--no-autoupdate"
          "run"
        ];
        dependsOn = [ "mailservice-api" ];
        extraOptions = [
          "--network=host"
          "--pull=always"
        ];
      };
    };
  };
}
