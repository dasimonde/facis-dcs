#!/usr/bin/env bash
export PATH="/home/ginoezue/.local/node22/bin:$PATH"
cd "$(dirname "$0")"
npm install @rollup/rollup-linux-x64-gnu --no-save
