{
  description = "Mailservice NixOS GitOps deployment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-24.11";
  };

  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };

      mkNixOpsApp = name:
        let
          scriptName = "${name}.sh";
        in {
          type = "app";
          program = toString (pkgs.writeShellScript "mailservice-${name}" ''
            exec ${pkgs.bash}/bin/bash ${self}/ops/nixops/${scriptName} "$@"
          '');
        };
    in {
      nixosConfigurations.truevipaccess = nixpkgs.lib.nixosSystem {
        inherit system;
        modules = [
          ./nix/hosts/truevipaccess/configuration.nix
        ];
      };

      apps.${system} = {
        nixops-create = mkNixOpsApp "create";
        nixops-deploy = mkNixOpsApp "deploy";
        nixops-rollback = mkNixOpsApp "rollback";
      };
    };
}
