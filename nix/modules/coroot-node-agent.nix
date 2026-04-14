{ lib, config, pkgs, ... }:

let
  cfg = config.services.corootNodeAgent;
  version = "1.31.0";
  agentUrl = "https://github.com/coroot/coroot-node-agent/releases/download/v${version}/coroot-node-agent-amd64";
  agentPath = "/opt/coroot-node-agent/coroot-node-agent";

  downloadScript = pkgs.writeShellScript "coroot-node-agent-download" ''
    set -euo pipefail
    if [ -f "${agentPath}" ]; then
      exit 0
    fi
    ${pkgs.coreutils}/bin/install -d -m 755 /opt/coroot-node-agent
    ${pkgs.curl}/bin/curl -fsSL -o "${agentPath}.tmp" "${agentUrl}"
    ${pkgs.coreutils}/bin/chmod 755 "${agentPath}.tmp"
    ${pkgs.coreutils}/bin/mv "${agentPath}.tmp" "${agentPath}"
  '';
in
{
  options.services.corootNodeAgent = {
    enable = lib.mkEnableOption "Coroot node agent for observability";

    collectorEndpoint = lib.mkOption {
      type = lib.types.str;
      description = "Coroot collector endpoint URL.";
    };

    apiKeyFile = lib.mkOption {
      type = lib.types.str;
      description = "Path to file containing API_KEY=<key> for Coroot authentication.";
    };

    scrapeInterval = lib.mkOption {
      type = lib.types.str;
      default = "15s";
      description = "Metrics scrape interval.";
    };
  };

  config = lib.mkIf cfg.enable {
    systemd.services.coroot-node-agent = {
      description = "Coroot Node Agent";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      serviceConfig = {
        Type = "simple";
        Restart = "always";
        RestartSec = 10;
        EnvironmentFile = cfg.apiKeyFile;
        ExecStartPre = "${downloadScript}";
        ExecStart = "${agentPath} --collector-endpoint=${cfg.collectorEndpoint} --scrape-interval=${cfg.scrapeInterval}";
      };
    };
  };
}
