#!/bin/sh
# Runs the binary named by $BINARY (blog_platform by default).
set -e
exec "./${BINARY:-blog_platform}" "$@"
