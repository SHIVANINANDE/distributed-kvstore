#!/bin/bash

set -e

echo "Building distributed-kvstore Docker image..."

# Build args
BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
VERSION=${VERSION:-"1.0.0"}

# Build the image
docker build \
    --build-arg BUILD_DATE="${BUILD_DATE}" \
    --build-arg GIT_COMMIT="${GIT_COMMIT}" \
    --build-arg VERSION="${VERSION}" \
    -t distributed-kvstore:latest \
    -t distributed-kvstore:${VERSION} \
    .

echo "Docker image built successfully!"
echo "Tags: distributed-kvstore:latest, distributed-kvstore:${VERSION}"

# Optional: Run basic tests on the image
if [ "${RUN_TESTS:-false}" = "true" ]; then
    echo "Running basic image tests..."
    
    # Test that the image starts
    CONTAINER_ID=$(docker run -d --rm distributed-kvstore:latest)
    sleep 5
    
    # Check if container is still running
    if docker ps | grep -q $CONTAINER_ID; then
        echo "✅ Container starts successfully"
        docker stop $CONTAINER_ID
    else
        echo "❌ Container failed to start"
        docker logs $CONTAINER_ID
        exit 1
    fi
fi