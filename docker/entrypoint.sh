#!/bin/sh
set -e

NAUTILUS_SERVICES_DIR=${NAUTILUS_SERVICES_DIR:-/var/run/nautilus/services}
NAUTILUS_ENTRYPOINT_DIR=${NAUTILUS_ENTRYPOINT_DIR:-/var/run/nautilus/entrypoints}

mkdir -p "$NAUTILUS_SERVICES_DIR" "$NAUTILUS_ENTRYPOINT_DIR"

chown -R nautilus:nautilus /var/run/nautilus

chmod 1777 "$NAUTILUS_SERVICES_DIR"
chmod 0755 "$NAUTILUS_ENTRYPOINT_DIR"

if command -v setfacl >/dev/null 2>&1; then
    setfacl -R -d -m u:nautilus:rwx "$NAUTILUS_SERVICES_DIR"
    setfacl -R -m u:nautilus:rwx "$NAUTILUS_SERVICES_DIR"
fi

echo "[Entrypoint] Permissions initialized. Dropping privileges to 'nautilus' user..."

exec su-exec nautilus /usr/local/bin/nautilus-core "$@"
