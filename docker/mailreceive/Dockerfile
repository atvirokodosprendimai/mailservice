FROM alpine:3.20

RUN apk add --no-cache \
    postfix \
    postfix-sqlite \
    dovecot \
    dovecot-lmtpd \
    dovecot-sqlite \
    sqlite \
    ca-certificates

RUN if ! grep -q '^vmail:' /etc/group; then addgroup -g 5000 vmail; fi && \
    if ! grep -q '^vmail:' /etc/passwd; then adduser -D -H -u 5000 -G vmail vmail; fi

COPY docker/mailreceive/postfix/main.cf /etc/postfix/main.cf
COPY docker/mailreceive/postfix/sqlite-domains.cf /etc/postfix/sqlite-domains.cf
COPY docker/mailreceive/postfix/sqlite-mailboxes.cf /etc/postfix/sqlite-mailboxes.cf

COPY docker/mailreceive/dovecot/dovecot.conf /etc/dovecot/dovecot.conf
COPY docker/mailreceive/dovecot/dovecot-sql.conf.ext /etc/dovecot/dovecot-sql.conf.ext
COPY docker/mailreceive/dovecot/conf.d/10-auth.conf /etc/dovecot/conf.d/10-auth.conf
COPY docker/mailreceive/dovecot/conf.d/auth-sql.conf.ext /etc/dovecot/conf.d/auth-sql.conf.ext
COPY docker/mailreceive/dovecot/conf.d/10-logging.conf /etc/dovecot/conf.d/10-logging.conf
COPY docker/mailreceive/dovecot/conf.d/10-ssl.conf /etc/dovecot/conf.d/10-ssl.conf
COPY docker/mailreceive/dovecot/conf.d/10-mail.conf /etc/dovecot/conf.d/10-mail.conf
COPY docker/mailreceive/dovecot/conf.d/10-master.conf /etc/dovecot/conf.d/10-master.conf

COPY docker/mailreceive/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 25 143

ENTRYPOINT ["/entrypoint.sh"]
