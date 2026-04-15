{ lib, config, pkgs, ... }:

let
  cfg = config.services.corootNodeAgent;
  version = "1.31.0";
  # sha256 of the upstream release binary (before patchelf).
  expectedSha256 = "d4d1acde006cadffff8b3ce78c88254d90b4d20f89972b07003216932a0cf179";
  expectedSize = "191384712";
  agentUrl = "https://github.com/coroot/coroot-node-agent/releases/download/v${version}/coroot-node-agent-amd64";
  agentDir = "/opt/coroot-node-agent";
  agentPath = "${agentDir}/coroot-node-agent";
  versionStamp = "${agentDir}/.version-${version}";

  downloadScript = pkgs.writeShellScript "coroot-node-agent-download" ''
    set -euo pipefail

    # Skip if this exact version was already downloaded and patched.
    if [ -f "${agentPath}" ] && [ -f "${versionStamp}" ]; then
      exit 0
    fi

    ${pkgs.coreutils}/bin/install -d -m 755 "${agentDir}"
    ${pkgs.curl}/bin/curl -fsSL -o "${agentPath}.tmp" "${agentUrl}"

    # Verify file size.
    actual_size=$(${pkgs.coreutils}/bin/stat -c%s "${agentPath}.tmp")
    if [ "$actual_size" != "${expectedSize}" ]; then
      echo "FATAL: expected ${expectedSize} bytes, got $actual_size — aborting." >&2
      ${pkgs.coreutils}/bin/rm -f "${agentPath}.tmp"
      exit 1
    fi

    # Verify sha256 against committed hash (pre-patchelf).
    actual_hash=$(${pkgs.coreutils}/bin/sha256sum "${agentPath}.tmp" | ${pkgs.coreutils}/bin/cut -d' ' -f1)
    if [ "$actual_hash" != "${expectedSha256}" ]; then
      echo "FATAL: sha256 mismatch (expected ${expectedSha256}, got $actual_hash) — aborting." >&2
      ${pkgs.coreutils}/bin/rm -f "${agentPath}.tmp"
      exit 1
    fi

    # Patch the ELF interpreter for NixOS (no /lib64/ld-linux-x86-64.so.2).
    ${pkgs.patchelf}/bin/patchelf --set-interpreter "$(${pkgs.coreutils}/bin/cat ${pkgs.stdenv.cc.bintools.dynamicLinker})" "${agentPath}.tmp"

    ${pkgs.coreutils}/bin/chmod 755 "${agentPath}.tmp"
    ${pkgs.coreutils}/bin/mv "${agentPath}.tmp" "${agentPath}"
    ${pkgs.coreutils}/bin/rm -f ${agentDir}/.version-*
    ${pkgs.coreutils}/bin/touch "${versionStamp}"
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
        ExecStart = agentPath;
      };
      environment = {
        COLLECTOR_ENDPOINT = cfg.collectorEndpoint;
        SCRAPE_INTERVAL = cfg.scrapeInterval;
      };
    };
  };
}
