#!/bin/sh
set -eu
code=$(curl --silent --max-time 3 --output /dev/null --write-out '%{http_code}' http://127.0.0.1:14904/api/health)
# Access JWTs are deliberately not stored in a healthcheck. 401 proves the
# protected listener is serving; DB readiness is separately monitored by db.
[ "$code" = 200 ] || [ "$code" = 401 ]
