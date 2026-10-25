#!/usr/bin/env bash
set -euo pipefail

: "${PROJECT_DB_PASSWORD:?Missing project password}"
: "${DEPLOYMENT_DB_PASSWORD:?Missing deployment password}"

psql -v ON_ERROR_STOP=1 \
  --username "$POSTGRES_USER" \
  --dbname postgres \
  -v project_password="$PROJECT_DB_PASSWORD" \
  -v deployment_password="$DEPLOYMENT_DB_PASSWORD" <<'SQL'

SELECT format(
  'CREATE ROLE project_user LOGIN PASSWORD %L',
  :'project_password'
) \gexec

SELECT format(
  'CREATE ROLE deployment_user LOGIN PASSWORD %L',
  :'deployment_password'
) \gexec

CREATE DATABASE projects_db OWNER project_user;
CREATE DATABASE deployments_db OWNER deployment_user;

SQL