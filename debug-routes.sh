#!/bin/bash

cd $(dirname "$0")

reset; go build -gcflags "all=-N -l" -o bin/routes ./cmd/svc/routes && bin/routes -v=2
