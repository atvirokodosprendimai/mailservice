{ lib, config, pkgs, ... }:

let
  cfg = config.services.corootNodeAgent;
  version = "1.31.0";
  expectedSize = "191384712"; # from GitHub release API — prevents truncated downloads
  agentUrl = "https://github.com/coroot/coroot-node-agent/releases/download/v${version}/coroot-node-agent-amd64";
  agentDir = "/opt/coroot-node-agent";
  agentPath = "${agentDir}/coroot-node-agent";
  hashPath = "${agentDir}/coroot-node-agent.sha256";

  downloadScript = pkgs.writeShellScript "coroot-node-agent-download" ''
    set -euo pipefail

    # Skip if binary exists and passes integrity check.
    if [ -f "${agentPath}" ] && [ -f "${hashPath}" ]; then
      stored_hash=$(${pkgs.coreutils}/bin/cat "${hashPath}")
      actual_hash=$(${pkgs.coreutils}/bin/sha256sum "${agentPath}" | ${pkgs.coreutils}/bin/cut -d' ' -f1)
      if [ "$stored_hash" = "$actual_hash" ]; then
        exit 0
      fi
      echo "Hash mismatch — re-downloading." >&2
    fi

    ${pkgs.coreutils}/bin/install -d -m 755 "${agentDir}"
    ${pkgs.curl}/bin/curl -fsSL -o "${agentPath}.tmp" "${agentUrl}"

    # Verify file size matches the pinned release size.
    actual_size=$(${pkgs.coreutils}/bin/stat -c%s "${agentPath}.tmp")
    if [ "$actual_size" != "${expectedSize}" ]; then
      echo "FATAL: expected ${expectedSize} bytes, got $actual_size — aborting." >&2
      ${pkgs.coreutils}/bin/rm -f "${agentPath}.tmp"
      exit 1
    fi

    # Pin the sha256 for future integrity checks.
    ${pkgs.coreutils}/bin/sha256sum "${agentPath}.tmp" | ${pkgs.coreutils}/bin/cut -d' ' -f1 > "${hashPath}.tmp"
    ${pkgs.coreutils}/bin/chmod 755 "${agentPath}.tmp"
    ${pkgs.coreutils}/bin/mv "${agentPath}.tmp" "${agentPath}"
    ${pkgs.coreutils}/bin/mv "${hashPath}.tmp" "${hashPath}"
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
