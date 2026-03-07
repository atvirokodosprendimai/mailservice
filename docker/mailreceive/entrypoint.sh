#!/bin/sh

set -eu

MAIL_DB_PATH="${MAIL_DB_PATH:-/data/mailservice.db}"
POSTFIX_SQLITE_DB="/var/lib/mail/shared-mailservice.db"
MAIL_DOMAIN="${MAIL_DOMAIN:-mail.local}"
MAILBOX_USER="${MAILBOX_USER:-}"
MAILBOX_PASSWORD="${MAILBOX_PASSWORD:-}"
MAIL_DEBUG="${MAIL_DEBUG:-0}"

mkdir -p /var/lib/mail /var/mail/vhosts /var/spool/postfix /run/dovecot
chown -R vmail:vmail /var/mail/vhosts
chmod 2770 /var/mail/vhosts || true

ln -sf "${MAIL_DB_PATH}" "${POSTFIX_SQLITE_DB}"

# Apply runtime Postfix identity from env (avoid hardcoded mail.local).
postconf -e "myhostname=${MAIL_DOMAIN}"
postconf -e "mydomain=${MAIL_DOMAIN}"
postconf -e 'myorigin=$myhostname'

sqlite3 "${MAIL_DB_PATH}" <<SQL
CREATE TABLE IF NOT EXISTS mail_domains (
  domain TEXT PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS mail_users (
  login TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  password TEXT NOT NULL,
  maildir TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
SQL

sqlite3 "${MAIL_DB_PATH}" "INSERT OR IGNORE INTO mail_domains(domain) VALUES('${MAIL_DOMAIN}');"

if [ -n "${MAILBOX_USER}" ] && [ -n "${MAILBOX_PASSWORD}" ]; then
  MAILBOX_EMAIL="${MAILBOX_USER}@${MAIL_DOMAIN}"
  MAILDIR_PATH="${MAIL_DOMAIN}/${MAILBOX_USER}"

  sqlite3 "${MAIL_DB_PATH}" \
    "INSERT INTO mail_users(login,email,password,maildir,enabled,updated_at) VALUES('${MAILBOX_USER}','${MAILBOX_EMAIL}','${MAILBOX_PASSWORD}','${MAILDIR_PATH}',1,CURRENT_TIMESTAMP) ON CONFLICT(login) DO UPDATE SET email=excluded.email,password=excluded.password,maildir=excluded.maildir,enabled=1,updated_at=CURRENT_TIMESTAMP;"

  mkdir -p "/var/mail/vhosts/${MAIL_DOMAIN}/${MAILBOX_USER}/Maildir"
  chown -R vmail:vmail "/var/mail/vhosts/${MAIL_DOMAIN}/${MAILBOX_USER}"
fi

# Start syslog in foreground mode and emit logs to stdout.
syslogd -n -O /dev/stdout &

if ! postfix start; then
  echo "postfix failed to start; dumping effective postfix config" >&2
  postfix check >&2 || true
  postconf -n >&2 || true
  exit 1
fi

echo "mailreceive started: domain=${MAIL_DOMAIN} db=${MAIL_DB_PATH} debug=${MAIL_DEBUG}"
postconf myhostname mydomain myorigin maillog_file debugger_command || true

if [ "${MAIL_DEBUG}" = "1" ]; then
  doveconf -n || true
fi

exec dovecot -F
