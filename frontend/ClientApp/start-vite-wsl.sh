#!/usr/bin/env bash
export PATH="/home/ginoezue/.local/node22/bin:$PATH"
cd "$(dirname "$0")"
export DCS_API_TARGET=http://localhost:8991
export DCS_HYDRA_TARGET=http://localhost:30444
exec npm run dev
