#!/bin/sh
# Prints the PTY size and reports every SIGWINCH. Used by the resize smoke test.
trap 'printf "RESIZED %s\n" "$(stty size)"' WINCH
printf 'UNICODE ñ 漢 🚀\n'
printf 'READY %s\n' "$(stty size)"
while :; do
  sleep 0.2
done
