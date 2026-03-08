{
  description = "Mailservice NixOS GitOps deployment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-24.11";
  };

  outputs = { self, nixpkgs }:
    let
      nixosSystem = "x86_64-linux";
      appSystems = [
        "x86_64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];

      forAllSystems = f:
        nixpkgs.lib.genAttrs appSystems (system:
          f (import nixpkgs { inherit system; })
        );

      mkNixOpsApp = name:
        pkgs:
        let
          scriptName = "${name}.sh";
        in {
          type = "app";
          program = toString (pkgs.writeShellScript "mailservice-${name}" ''
            exec ${pkgs.bash}/bin/bash ${self}/ops/nixops/${scriptName} "$@"
          '');
        };
    in {
      nixopsConfigurations.default = import ./nixops/default.nix { inherit nixpkgs; };

      nixosConfigurations.truevipaccess = nixpkgs.lib.nixosSystem {
        system = nixosSystem;
        modules = [
          ./nix/hosts/truevipaccess/configuration.nix
        ];
      };

      apps = forAllSystems (pkgs: {
        nixops-create = mkNixOpsApp "create" pkgs;
        nixops-deploy = mkNixOpsApp "deploy" pkgs;
        nixops-rollback = mkNixOpsApp "rollback" pkgs;
      });
    };
}
