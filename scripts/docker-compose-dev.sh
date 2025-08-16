#!/bin/bash

set -e

ACTION=${1:-up}

echo "Running Docker Compose for development..."

case $ACTION in
    "up")
        echo "Starting development environment..."
        docker-compose -f docker-compose.yml -f docker-compose.dev.yml up -d
        echo "Services starting. Check status with: docker-compose ps"
        echo ""
        echo "Available services:"
        echo "  - KV Store API: http://localhost:8080"
        echo "  - KV Store gRPC: localhost:9090"
        echo "  - Prometheus: http://localhost:9090"
        echo "  - Grafana: http://localhost:3000 (admin/dev)"
        echo "  - Jaeger UI: http://localhost:16686"
        echo "  - Load Balancer: http://localhost:80"
        echo "  - Adminer: http://localhost:8081"
        ;;
    "down")
        echo "Stopping development environment..."
        docker-compose -f docker-compose.yml -f docker-compose.dev.yml down
        ;;
    "logs")
        echo "Showing logs..."
        docker-compose -f docker-compose.yml -f docker-compose.dev.yml logs -f
        ;;
    "ps")
        echo "Service status:"
        docker-compose -f docker-compose.yml -f docker-compose.dev.yml ps
        ;;
    "restart")
        echo "Restarting services..."
        docker-compose -f docker-compose.yml -f docker-compose.dev.yml restart
        ;;
    *)
        echo "Usage: $0 {up|down|logs|ps|restart}"
        echo ""
        echo "Commands:"
        echo "  up      - Start development environment"
        echo "  down    - Stop development environment"
        echo "  logs    - Show service logs"
        echo "  ps      - Show service status"
        echo "  restart - Restart services"
        exit 1
        ;;
esac