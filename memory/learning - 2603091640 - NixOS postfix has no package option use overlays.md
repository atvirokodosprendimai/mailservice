---
tldr: services.postfix.package doesn't exist in NixOS — override the package via nixpkgs.overlays instead
category: infra
---

# Learning: NixOS services.postfix has no `package` option

Unlike many NixOS service modules (e.g. `services.dovecot2`), the Postfix module does not expose a `package` option.

Setting `services.postfix.package = ...` produces:
```
error: The option `services.postfix.package' does not exist.
```

To customize the Postfix derivation, use `nixpkgs.overlays`:
```nix
nixpkgs.overlays = [
  (final: prev: {
    postfix = prev.postfix.override { withSQLite = true; };
  })
];
```

This replaces the package globally, which is what the Postfix module picks up via `pkgs.postfix`.
