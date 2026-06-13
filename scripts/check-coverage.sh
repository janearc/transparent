#!/bin/bash
set -e

echo "Running tests and gathering coverage..."
go test ./... -coverprofile=coverage.out -covermode=atomic > /dev/null

COVERAGE=$(go tool cover -func=coverage.out | grep total: | awk '{print $3}' | sed 's/%//')

if (( $(echo "$COVERAGE < 80.0" | bc -l) )); then
    echo "❌ FAILURE: Test coverage is $COVERAGE% (Below 80% hyperscaler standard)"
    exit 1
fi

echo "✅ SUCCESS: Test coverage is $COVERAGE% (Meets 80% hyperscaler standard)"
