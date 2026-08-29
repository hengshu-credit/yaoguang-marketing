#!/bin/sh
set -eu

until mc alias set notifuse "$S3_ENDPOINT" "$S3_ACCESS_KEY" "$S3_SECRET_KEY" >/dev/null 2>&1; do
  sleep 2
done

mc mb --ignore-existing "notifuse/$S3_BUCKET"
mc anonymous set none "notifuse/$S3_BUCKET"
