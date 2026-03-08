{ nixpkgs }:

let
  envOr = name: fallback:
    let
      value = builtins.getEnv name;
    in if value != "" then value else fallback;
in
{
  nixpkgs = nixpkgs;
  network.description = "mailservice truevipaccess";
  network.enableRollback = true;
  network.storage.legacy = {
    databasefile = envOr "NIXOPS_STATE" ".nixops/deployments/mailservice-truevipaccess.nixops";
  };

  truevipaccess =
    { ... }:
    {
      imports = [
        ../nix/hosts/truevipaccess/configuration.nix
      ];

      nixpkgs.system = "x86_64-linux";
      deployment.targetHost = envOr "NIXOPS_TARGET_HOST" "46.62.133.191";
      deployment.targetUser = envOr "NIXOPS_TARGET_USER" "root";
    };
}
