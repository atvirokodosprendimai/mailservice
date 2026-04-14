{ lib, config, pkgs, ... }:

let
  cfg = config.services.corootNodeAgent;

  version = "1.31.0";

  agent = pkgs.stdenv.mkDerivation {
    pname = "coroot-node-agent";
    inherit version;
    src = pkgs.fetchurl {
      url = "https://github.com/coroot/coroot-node-agent/releases/download/v${version}/coroot-node-agent-amd64";
      executable = true;
      # Build will fail with expected-vs-actual hash on first run.
      # Replace this placeholder with the real hash from the build error.
      hash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
    };
    dontUnpack = true;
    installPhase = ''
      install -Dm755 $src $out/bin/coroot-node-agent
    '';
  };
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
        RestartSec = 5;
        EnvironmentFile = cfg.apiKeyFile;
        ExecStart = "${agent}/bin/coroot-node-agent --collector-endpoint=${cfg.collectorEndpoint} --scrape-interval=${cfg.scrapeInterval}";
      };
    };
  };
}
