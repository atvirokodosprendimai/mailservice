---
tldr: "451 Temporary lookup failure" in Postfix often means the map type (sqlite, ldap, etc.) isn't compiled in
category: debugging
---

# Learning: Postfix "Temporary lookup failure" means unsupported map type

When Postfix returns `451 4.3.0 Temporary lookup failure`, the instinct is to check file permissions or database contents.

But the actual cause can be that Postfix doesn't support the map type at all — e.g. `sqlite:` maps when Postfix wasn't compiled with `--enable-sqlite`.

## Diagnostic

Test directly with SMTP:
```python
import smtplib
s = smtplib.SMTP('mail.example.com', 25)
s.sendmail('test@example.com', 'user@example.com', 'test')
```

If you get `451 Temporary lookup failure`, check `postconf -m` on the server to see which map types are available.
