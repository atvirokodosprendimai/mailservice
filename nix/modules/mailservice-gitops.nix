{ lib, config, pkgs, self, ... }:

let
  cfg = config.services.mailserviceGitOps;
  mailDbPath = "/var/lib/mailservice/data/mailservice.db";
  mailRoot = "/var/mail/vhosts";
  postfixSqliteDomainsFile = pkgs.writeText "postfix-sqlite-domains.cf" ''
    dbpath = ${mailDbPath}
    query = SELECT domain FROM mail_domains WHERE domain='%s'
  '';
  postfixSqliteMailboxesFile = pkgs.writeText "postfix-sqlite-mailboxes.cf" ''
    dbpath = ${mailDbPath}
    query = SELECT maildir FROM mail_users WHERE enabled=1 AND (email='%s' OR login='%u')
  '';
  dovecotSqlConfigFile = pkgs.writeText "dovecot-sql.conf.ext" ''
    driver = sqlite
    connect = ${mailDbPath}
    default_pass_scheme = PLAIN
    password_query = SELECT email AS user, password FROM mail_users WHERE enabled=1 AND (email='%u' OR login='%n')
    user_query = SELECT '${mailRoot}/' || maildir || '/Maildir' AS home, 5000 AS uid, 5000 AS gid FROM mail_users WHERE enabled=1 AND (email='%u' OR login='%n')
  '';
in
{
  options.services.mailserviceGitOps = {
    enable = lib.mkEnableOption "mailservice GitOps runtime";

    mailDomain = lib.mkOption {
      type = lib.types.str;
      default = "";
      description = "Domain used for mailbox addressing and native mail service configuration.";
    };

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
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = cfg.mailDomain != "";
        message = "services.mailserviceGitOps.mailDomain must be set for the native mail stack.";
      }
    ];

    networking.firewall.allowedTCPPorts = [ 25 143 ];

    users.groups.mailservice = { };
    users.groups.vmail.gid = 5000;
    users.users.mailservice = {
      isSystemUser = true;
      group = "mailservice";
      home = "/var/lib/mailservice";
      createHome = false;
    };
    users.users.vmail = {
      isSystemUser = true;
      uid = 5000;
      group = "vmail";
      extraGroups = [ "mailservice" ];
      home = mailRoot;
      createHome = false;
    };
    users.users.cloudflared = {
      isSystemUser = true;
      group = "mailservice";
      home = "/var/lib/mailservice";
      createHome = false;
    };
    users.users.postfix.extraGroups = [ "mailservice" ];

    systemd.tmpfiles.rules = [
      "d /var/lib/mailservice 0755 root root -"
      "d /var/lib/mailservice/data 2770 mailservice mailservice -"
      "Z /var/lib/mailservice/data 2770 mailservice mailservice - -"
      "d ${mailRoot} 2770 vmail vmail -"
      "Z ${mailRoot} 2770 vmail vmail - -"
      "d /var/lib/secrets 0750 root mailservice -"
      "f ${cfg.environmentFile} 0640 root mailservice - -"
      "f ${cfg.cloudflaredEnvironmentFile} 0640 root mailservice - -"
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
        DATABASE_DSN = mailDbPath;
        MAIL_DOMAIN = cfg.mailDomain;
      };
    };

    services.postfix = {
      enable = true;
      enableSmtp = true;
      hostname = cfg.mailDomain;
      domain = cfg.mailDomain;
      origin = cfg.mailDomain;
      destination = [ ];
      networks = [ "127.0.0.0/8" "[::1]/128" ];
      recipientDelimiter = "+";
      config = {
        inet_interfaces = "all";
        inet_protocols = "ipv4";
        smtpd_tls_security_level = "none";
        smtpd_relay_restrictions = "reject_unauth_destination";
        default_transport = "error:outbound_mail_disabled";
        relay_transport = "error:outbound_mail_disabled";
        virtual_mailbox_domains = "sqlite:/etc/postfix/sqlite-domains.cf";
        virtual_mailbox_maps = "sqlite:/etc/postfix/sqlite-mailboxes.cf";
        virtual_transport = "lmtp:unix:private/dovecot-lmtp";
        virtual_mailbox_base = mailRoot;
        virtual_minimum_uid = 5000;
        virtual_uid_maps = "static:5000";
        virtual_gid_maps = "static:5000";
        message_size_limit = 52428800;
        mailbox_size_limit = 0;
        debugger_command = "/bin/true";
      };
    };
    systemd.services.postfix.after = [ "mailservice-api.service" "dovecot2.service" ];
    systemd.services.postfix.wants = [ "mailservice-api.service" "dovecot2.service" ];
    systemd.services.postfix-setup.script = lib.mkAfter ''
      ln -sf ${postfixSqliteDomainsFile} /var/lib/postfix/conf/sqlite-domains.cf
      ln -sf ${postfixSqliteMailboxesFile} /var/lib/postfix/conf/sqlite-mailboxes.cf
    '';

    environment.etc."dovecot/dovecot-sql.conf.ext".source = dovecotSqlConfigFile;
    services.dovecot2 = {
      enable = true;
      enableImap = true;
      enableLmtp = true;
      mailLocation = "maildir:${mailRoot}/%d/%n/Maildir";
      mailUser = "vmail";
      mailGroup = "vmail";
      createMailUser = false;
      extraConfig = ''
        passdb {
          driver = sql
          args = /etc/dovecot/dovecot-sql.conf.ext
        }

        userdb {
          driver = sql
          args = /etc/dovecot/dovecot-sql.conf.ext
        }

        protocol lmtp {
          postmaster_address = postmaster@${cfg.mailDomain}
        }

        service lmtp {
          unix_listener /var/lib/postfix/queue/private/dovecot-lmtp {
            mode = 0600
            user = postfix
            group = postfix
          }
        }
      '';
    };
    systemd.services.dovecot2.after = [ "mailservice-api.service" "postfix-setup.service" ];
    systemd.services.dovecot2.wants = [ "mailservice-api.service" "postfix-setup.service" ];

    systemd.services.mailservice-cloudflared = {
      description = "Cloudflare Tunnel for mailservice";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" "mailservice-api.service" ];
      wants = [ "network-online.target" "mailservice-api.service" ];
      serviceConfig = {
        User = "cloudflared";
        Group = "mailservice";
        Restart = "always";
        RestartSec = 5;
        EnvironmentFile = cfg.cloudflaredEnvironmentFile;
        ExecStart = "${pkgs.cloudflared}/bin/cloudflared tunnel --no-autoupdate run";
      };
    };
  };
}
