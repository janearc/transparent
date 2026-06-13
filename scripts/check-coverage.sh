#!/bin/bash
set -e

echo "Running tests and gathering coverage..."
go test ./... -coverprofile=coverage.out.tmp -covermode=atomic > /dev/null
grep -v "/cmd/" coverage.out.tmp > coverage.out

COVERAGE=$(go tool cover -func=coverage.out | grep total: | awk '{print $3}' | sed 's/%//')

if (( $(echo "$COVERAGE < 90.0" | bc -l) )); then
    echo "❌ FAILURE: Test coverage is $COVERAGE% (Below 90% hyperscaler standard)"
    exit 1
fi

echo "✅ SUCCESS: Test coverage is $COVERAGE% (Meets 90% hyperscaler standard)"
