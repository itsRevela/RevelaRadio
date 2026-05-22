#!/bin/sh
set -eu

: "${ICECAST_SOURCE_PASSWORD:?source password required}"
: "${ICECAST_RELAY_PASSWORD:?relay password required}"
: "${ICECAST_ADMIN_PASSWORD:?admin password required}"
: "${ICECAST_ADMIN_USER:=admin}"
: "${ICECAST_ADMIN_EMAIL:=admin@localhost}"
: "${ICECAST_HOSTNAME:=localhost}"
: "${ICECAST_MAX_LISTENERS:=50}"

export ICECAST_SOURCE_PASSWORD ICECAST_RELAY_PASSWORD ICECAST_ADMIN_PASSWORD \
       ICECAST_ADMIN_USER ICECAST_ADMIN_EMAIL ICECAST_HOSTNAME ICECAST_MAX_LISTENERS

envsubst < /etc/icecast2/icecast.xml.tmpl > /tmp/icecast.xml

exec /usr/bin/icecast2 -c /tmp/icecast.xml
