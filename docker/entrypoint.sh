#!/bin/sh
set -e

NAUTILUS_SERVICES_DIR=${NAUTILUS_SERVICES_DIR:-/var/run/nautilus/services}
NAUTILUS_ENTRYPOINT_DIR=${NAUTILUS_ENTRYPOINT_DIR:-/var/run/nautilus/entrypoints}

mkdir -p "$NAUTILUS_SERVICES_DIR" "$NAUTILUS_ENTRYPOINT_DIR"

chown -R nautilus:nautilus /var/run/nautilus

chown nautilus:nautilus "$NAUTILUS_SERVICES_DIR"
chmod 1777 "$NAUTILUS_SERVICES_DIR"
chmod 0755 "$NAUTILUS_ENTRYPOINT_DIR"

if command -v setfacl >/dev/null 2>&1; then
    setfacl -b "$NAUTILUS_SERVICES_DIR"

    setfacl -m u:nautilus:rwx "$NAUTILUS_SERVICES_DIR"
    setfacl -d -m u:nautilus:rwx "$NAUTILUS_SERVICES_DIR"

    setfacl -m o::rwx "$NAUTILUS_SERVICES_DIR"
    setfacl -d -m o::rwx "$NAUTILUS_SERVICES_DIR"

    setfacl -R -m m::rwx "$NAUTILUS_SERVICES_DIR"
    setfacl -R -d -m m::rwx "$NAUTILUS_SERVICES_DIR"
fi

exec su-exec nautilus /usr/local/bin/nautilus-core "$@"
