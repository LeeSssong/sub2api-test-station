#!/bin/sh
set -eu

site_address="${SITE_ADDRESS:?SITE_ADDRESS is required}"
cert_root="${NGINX_TLS_CERT_ROOT:-/data/caddy/certificates}"
configured_cert_source="${NGINX_TLS_CERT_SOURCE:-}"
configured_key_source="${NGINX_TLS_KEY_SOURCE:-}"
cert_target="${NGINX_TLS_CERT_FILE:?NGINX_TLS_CERT_FILE is required}"
key_target="${NGINX_TLS_KEY_FILE:?NGINX_TLS_KEY_FILE is required}"

cert_source=''
key_source=''

discover_certificate_pair() {
    if [ -n "$configured_cert_source" ] || [ -n "$configured_key_source" ]; then
        [ -n "$configured_cert_source" ] && [ -n "$configured_key_source" ] || return 1
        [ -r "$configured_cert_source" ] && [ -r "$configured_key_source" ] || return 1
        cert_source="$configured_cert_source"
        key_source="$configured_key_source"
        return 0
    fi

    newest_cert=''
    for candidate in "$cert_root"/*/"$site_address"/"$site_address".crt; do
        candidate_key="${candidate%.crt}.key"
        [ -r "$candidate" ] && [ -r "$candidate_key" ] || continue
        if [ -z "$newest_cert" ] || [ "$candidate" -nt "$newest_cert" ]; then
            newest_cert="$candidate"
        fi
    done

    [ -n "$newest_cert" ] || return 1
    cert_source="$newest_cert"
    key_source="${newest_cert%.crt}.key"
}

copy_certificate_pair() {
    cp "$cert_source" "$cert_target" && \
        cp "$key_source" "$key_target" && \
        chmod 600 "$key_target"
}

mkdir -p /run/nginx-tls

for attempt in $(seq 1 120); do
    if discover_certificate_pair; then
        break
    fi
    if [ "$attempt" -eq 120 ]; then
        echo "Caddy-managed TLS certificate did not become readable" >&2
        exit 1
    fi
    sleep 1
done

copy_certificate_pair

/docker-entrypoint.sh nginx -t
/docker-entrypoint.sh nginx -g 'daemon off;' &
nginx_pid=$!

watcher_pid=''
stop() {
    if [ -n "$watcher_pid" ]; then
        kill -TERM "$watcher_pid" 2>/dev/null || true
    fi
    kill -TERM "$nginx_pid" 2>/dev/null || true
    wait "$nginx_pid" 2>/dev/null || true
    exit 0
}
trap stop TERM INT QUIT

watch_certificates() {
    last_fingerprint=''
    while :; do
        if discover_certificate_pair && current_fingerprint=$(sha256sum "$cert_source" "$key_source"); then
            if [ -z "$last_fingerprint" ]; then
                last_fingerprint="$current_fingerprint"
            elif [ "$current_fingerprint" != "$last_fingerprint" ]; then
                if copy_certificate_pair && nginx -t && nginx -s reload; then
                    last_fingerprint="$current_fingerprint"
                else
                    echo "Refusing to reload nginx after invalid certificate update" >&2
                fi
            fi
        fi

        sleep 300 &
        wait "$!" || return 0
    done
}

watch_certificates &
watcher_pid=$!

if wait "$nginx_pid"; then
    nginx_status=0
else
    nginx_status=$?
fi
kill -TERM "$watcher_pid" 2>/dev/null || true
wait "$watcher_pid" 2>/dev/null || true
exit "$nginx_status"
