#!/bin/bash

set -euo pipefail

echo "pv-safe Cluster Diagnostic Tool"
echo "================================"
echo ""

# Check Docker
echo "1. Docker Status"
echo "----------------"
if ! docker info > /dev/null 2>&1; then
    echo "ERROR: Docker is not running or not accessible"
    exit 1
fi
echo "Docker is running"
CGROUP_VERSION=$(docker info 2>/dev/null | grep "Cgroup Version" | awk '{print $3}')
CGROUP_DRIVER=$(docker info 2>/dev/null | grep "Cgroup Driver" | awk '{print $3}')
echo "Cgroup Version: $CGROUP_VERSION"
echo "Cgroup Driver: $CGROUP_DRIVER"
echo ""

# Check kind
echo "2. Kind Version"
echo "---------------"
if ! command -v kind &> /dev/null; then
    echo "ERROR: kind is not installed"
    exit 1
fi
kind version
echo ""

# Check existing kind clusters
echo "3. Existing Kind Clusters"
echo "-------------------------"
kind get clusters 2>/dev/null || echo "No existing clusters"
echo ""

# Check Docker resources
echo "4. Docker Resources"
echo "-------------------"
docker info 2>/dev/null | grep -E "CPUs|Total Memory"
echo ""

# Check for leftover containers
echo "5. Kind Containers"
echo "------------------"
CONTAINERS=$(docker ps -a --filter "name=pv-safe-test" --format "{{.Names}}")
if [ -z "$CONTAINERS" ]; then
    echo "No pv-safe-test containers found"
else
    echo "Found containers:"
    echo "$CONTAINERS"
    echo ""
    read -p "Do you want to clean up these containers? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        docker ps -a --filter "name=pv-safe-test" --format "{{.Names}}" | xargs -r docker rm -f
        echo "Containers removed"
    fi
fi
echo ""

# Check for leftover networks
echo "6. Kind Networks"
echo "----------------"
NETWORKS=$(docker network ls --filter "name=pv-safe-test" --format "{{.Name}}")
if [ -z "$NETWORKS" ]; then
    echo "No pv-safe-test networks found"
else
    echo "Found networks:"
    echo "$NETWORKS"
    echo ""
    read -p "Do you want to clean up these networks? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        docker network ls --filter "name=pv-safe-test" --format "{{.Name}}" | xargs -r docker network rm
        echo "Networks removed"
    fi
fi
echo ""

# System info
echo "7. System Information"
echo "---------------------"
echo "Kernel: $(uname -r)"
echo "OS: $(cat /etc/os-release | grep PRETTY_NAME | cut -d'=' -f2 | tr -d '"')"
echo ""

# Recommendations
echo "8. Recommendations"
echo "------------------"
if [ "$CGROUP_VERSION" = "2" ]; then
    echo "- You're using cgroup v2. This is generally supported but may require kind v0.20.0+"
    echo "- Current kind version: $(kind version | grep kind | awk '{print $2}')"
fi

echo ""
echo "9. Suggested Fixes"
echo "------------------"
echo "Try these in order:"
echo ""
echo "a) Clean up and retry:"
echo "   make cluster-delete"
echo "   docker system prune -f"
echo "   make cluster-create"
echo ""
echo "b) Use simplified config:"
echo "   kind create cluster --name pv-safe-test --wait 5m"
echo ""
echo "c) Check Docker daemon logs:"
echo "   sudo journalctl -u docker -n 50"
echo ""
echo "d) Restart Docker:"
echo "   sudo systemctl restart docker"
echo ""
