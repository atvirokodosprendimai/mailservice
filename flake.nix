{
  description = "Mailservice NixOS GitOps deployment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-24.11";
  };

  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
    in {
      nixosConfigurations.truevipaccess = nixpkgs.lib.nixosSystem {
        inherit system;
        modules = [
          ./nix/hosts/truevipaccess/configuration.nix
        ];
      };
    };
}
