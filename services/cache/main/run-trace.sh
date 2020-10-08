#!/usr/bin/env bash
set -ueo pipefail

go run . -type lru -trace trace.$(hostname).lru.get.out
go run . -type str -trace trace.$(hostname).str.get.out
#go run . -type lru -writeIntensive -trace trace.$(hostname).lru.set.out
#go run . -type str -writeIntensive -trace trace.$(hostname).str.set.out


go tool trace trace.$(hostname).lru.get.out &
sleep 3
go tool trace trace.$(hostname).str.get.out &

#go tool trace trace.$(hostname).lru.set.out &
#sleep 5
#go tool trace trace.$(hostname).str.set.out &
