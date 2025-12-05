#!/bin/bash

# Set environment variables here for the cluster to be tested.
export TOOLS_TESTING_AUTH_USERNAME="rcownie"
export TOOLS_TESTING_AUTH_PASSWORD="penguin"
export TOOLS_TESTING_MONGOD="mongodb+srv://rcownie:penguin@rcownie1.aswvt.mongodb-dev.net/?retryWrites=true&w=majority&appName=rcownie1"
export TOOLS_TESTING_AUTH="true"
export TOOLS_TESTING_REPLSET="true"

echo "Running integration tests for cluster connection ${TOOLS_TESTING_MONGOD}"
go run build.go test:integration

