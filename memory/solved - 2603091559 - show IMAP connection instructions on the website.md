---
tldr: Add IMAP client setup info to the website so users know how to connect
category: feature
---

# Todo: show IMAP connection instructions on the website

The website needs a section explaining how to connect an IMAP client to read mailbox contents.

## What needs doing

- Display IMAP connection details on the website (host, port, encryption, auth)
- Recommend specific clients (Thunderbird, Apple Mail, CLI tools like `curl --url imaps://`)
- Clarify which port/encryption to use (143+STARTTLS vs 993+TLS) once IMAPS is enabled
- Include example code snippets for programmatic access (Python imaplib, Go, curl)

## Related files

- `docs/website-copy.md` — already has a "Connect with IMAP" section with examples
- `internal/adapters/httpapi/handler.go` — serves the landing page HTML
- [[todo - 2603091556 - enable IMAPS port 993 with TLS in Dovecot]] — prerequisite for recommending port 993
