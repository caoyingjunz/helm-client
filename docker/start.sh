#!/bin/sh
set -o errexit
set -o xtrace

if [[ ! -d "/etc/rainbow" ]]; then
    mkdir -p /etc/rainbow
fi

/app
