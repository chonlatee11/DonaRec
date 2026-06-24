#!/bin/bash
# create-multiple-dbs.sh
# Creates additional databases in PostgreSQL on first container startup.
# Referenced by docker-compose.yml as an init script.
#
# Usage: Set POSTGRES_MULTIPLE_DATABASES env var to a comma-separated list
# of database names to create (the POSTGRES_DB database is already created by postgres image).
#
# Example: POSTGRES_MULTIPLE_DATABASES=donnarec_app,donnarec_keycloak

set -e
set -u

function create_database() {
    local database=$1
    echo "Creating database: $database"
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
        CREATE DATABASE "$database";
        GRANT ALL PRIVILEGES ON DATABASE "$database" TO "$POSTGRES_USER";
EOSQL
}

if [ -n "${POSTGRES_MULTIPLE_DATABASES:-}" ]; then
    echo "Multiple databases requested: $POSTGRES_MULTIPLE_DATABASES"
    for db in $(echo "$POSTGRES_MULTIPLE_DATABASES" | tr ',' ' '); do
        # Skip the primary database (already created by postgres image)
        if [ "$db" != "$POSTGRES_DB" ]; then
            create_database "$db"
        fi
    done
    echo "Multiple database setup complete."
fi
