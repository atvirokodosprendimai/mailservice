---
tldr: NixOS default Postfix lacks SQLite — enable via nixpkgs.overlays with postfix.override { withSQLite = true; }
category: infra
---

# Learning: NixOS Postfix needs explicit SQLite support via overlay

The default NixOS `postfix` package is not compiled with SQLite support.
If you use `sqlite:` map types in Postfix config (e.g. `virtual_mailbox_domains`), all lookups silently fail with "451 Temporary lookup failure".

## Fix

```nix
nixpkgs.overlays = [
  (final: prev: {
    postfix = prev.postfix.override { withSQLite = true; };
  })
];
```

`services.postfix.package` does not exist in the NixOS module — you must use `nixpkgs.overlays`.
