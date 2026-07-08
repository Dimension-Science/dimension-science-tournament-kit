#!/bin/bash
set -e

BACKUP_DIR="/opt/speedrun/deploy-backups"

if [ -z "$1" ]; then
    echo "Usage: $0 <backup-filename.tgz>"
    echo "Available backups:"
    ls -lh "$BACKUP_DIR"/*.tgz 2>/dev/null || echo "No backups found"
    exit 1
fi

BACKUP_FILE="$BACKUP_DIR/$1"

if [ ! -f "$BACKUP_FILE" ]; then
    echo "Error: Backup file $BACKUP_FILE does not exist!"
    exit 1
fi

echo "Restoring from $BACKUP_FILE..."

# Stop the running application
cd /opt/speedrun
docker compose down

# Extract the backup files
echo "Extracting backup..."
tar -xzf "$BACKUP_FILE" -C /

# Re-build and start the containers
echo "Starting containers..."
docker compose up -d --build app

echo "Restore complete!"
