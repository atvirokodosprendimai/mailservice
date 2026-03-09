{
  description = "Mailservice NixOS GitOps deployment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-24.11";
    nixpkgs-unstable.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs, nixpkgs-unstable }:
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

      mkMailserviceApi = pkgs:
        let
          unstablePkgs = import nixpkgs-unstable { system = pkgs.system; };
        in
        unstablePkgs.buildGoModule {
          pname = "mailservice-api";
          version = self.shortRev or self.dirtyShortRev or "dev";
          src = self;
          vendorHash = "sha256-kyC1XJhRpEL42PfOnjswEAbA5P80LQ0RPgmY0DX6K+8=";
          subPackages = [ "cmd/app" ];
          env.CGO_ENABLED = "0";
          ldflags = [
            "-s"
            "-w"
          ];
        };
    in {
      nixopsConfigurations.default = import ./nixops/default.nix { inherit nixpkgs; };

      nixosConfigurations.truevipaccess = nixpkgs.lib.nixosSystem {
        system = nixosSystem;
        specialArgs = { inherit self; };
        modules = [
          ./nix/hosts/truevipaccess/configuration.nix
        ];
      };

      packages = forAllSystems (pkgs: {
        mailservice-api = mkMailserviceApi pkgs;
      });

      apps = forAllSystems (pkgs: {
        nixops-create = mkNixOpsApp "create" pkgs;
        nixops-deploy = mkNixOpsApp "deploy" pkgs;
        nixops-rollback = mkNixOpsApp "rollback" pkgs;
      });
    };
}
