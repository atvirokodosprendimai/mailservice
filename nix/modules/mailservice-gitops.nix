{ lib, config, pkgs, self, ... }:

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

    cloudflaredEnvironmentFile = lib.mkOption {
      type = lib.types.str;
      default = "/var/lib/secrets/cloudflared.env";
      description = "Environment file containing TUNNEL_TOKEN for cloudflared.";
    };

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.system}.mailservice-api;
      description = "Mailservice API package to run directly under systemd.";
    };

    mailreceiveImage = lib.mkOption {
      type = lib.types.str;
      description = "Pinned OCI image reference for the inbound mail runtime.";
    };

  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = !(lib.hasInfix "PLACEHOLDER" cfg.mailreceiveImage);
        message = "services.mailserviceGitOps.mailreceiveImage must be pinned to a real image tag before deployment.";
      }
    ];

    virtualisation.oci-containers.backend = "docker";
    virtualisation.docker.enable = true;

    networking.firewall.allowedTCPPorts = [ 25 143 ];

    users.groups.mailservice = { };
    users.users.mailservice = {
      isSystemUser = true;
      group = "mailservice";
      home = "/var/lib/mailservice";
      createHome = false;
    };

    systemd.tmpfiles.rules = [
      "d /var/lib/mailservice 0755 root root -"
      "d /var/lib/mailservice/data 0770 mailservice mailservice -"
      "d /var/lib/mailservice/vhosts 0755 root root -"
      "d /var/lib/secrets 0700 root root -"
    ];

    systemd.services.mailservice-api = {
      description = "Mailservice API";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      serviceConfig = {
        User = "mailservice";
        Group = "mailservice";
        Restart = "always";
        RestartSec = 5;
        WorkingDirectory = "/var/lib/mailservice";
        EnvironmentFile = cfg.environmentFile;
        ExecStart = "${cfg.package}/bin/app";
      };
      environment = {
        HTTP_ADDR = "127.0.0.1:8080";
        DATABASE_DSN = "/var/lib/mailservice/data/mailservice.db";
      };
    };

    virtualisation.oci-containers.containers = {
      mailservice-mailreceive = {
        image = cfg.mailreceiveImage;
        environmentFiles = [ cfg.environmentFile ];
        environment = {
          MAIL_DB_PATH = "/data/mailservice.db";
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

    };

    systemd.services.mailservice-cloudflared = {
      description = "Cloudflare Tunnel for mailservice";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" "mailservice-api.service" ];
      wants = [ "network-online.target" "mailservice-api.service" ];
      serviceConfig = {
        Restart = "always";
        RestartSec = 5;
        EnvironmentFile = cfg.cloudflaredEnvironmentFile;
        ExecStart = "${pkgs.cloudflared}/bin/cloudflared tunnel --no-autoupdate run";
      };
    };
  };
}
