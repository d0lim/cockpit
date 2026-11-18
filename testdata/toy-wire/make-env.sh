#!/bin/sh
set -eu
token() {
  head -c 4096 /dev/urandom | tr -dc 'A-Za-z0-9' | head -c "$1"
}
db_password="$(token 32)"
app_token="toy_$(token 24)"
partner_key="$(token 28)"
cat > .env <<ENV
POSTGRES_USER=toy
POSTGRES_DB=toy
POSTGRES_PASSWORD=${db_password}
APP_TOKEN=${app_token}
ENV
cat > e2e.private.env.json <<JSON
{
  "local": {
    "appToken": "${app_token}",
    "partnerKey": "${partner_key}"
  }
}
JSON
