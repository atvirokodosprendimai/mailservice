---
tldr: NixOS Postfix uses /var/lib/postfix/conf/ as config directory, not /etc/postfix/
category: infra
---

# Learning: NixOS Postfix config directory is /var/lib/postfix/conf/

On NixOS, the Postfix module manages `main.cf` in the Nix store and sets `config_directory` to `/var/lib/postfix/conf/`.

When referencing additional config files (like SQLite map files), use absolute paths to `/var/lib/postfix/conf/`, not `/etc/postfix/`.

The `postfix-setup.script` with `lib.mkAfter` can create symlinks there:
```nix
systemd.services.postfix-setup.script = lib.mkAfter ''
  ln -sf ${postfixSqliteDomainsFile} /var/lib/postfix/conf/sqlite-domains.cf
'';
```
