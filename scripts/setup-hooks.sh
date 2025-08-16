#!/bin/bash

# Setup script for pre-commit hooks

set -e

echo "Setting up pre-commit hooks for distributed-kvstore..."

# Check if pre-commit is installed
if ! command -v pre-commit &> /dev/null; then
    echo "pre-commit could not be found. Installing via pip..."
    
    # Check if pip is available
    if command -v pip3 &> /dev/null; then
        pip3 install pre-commit
    elif command -v pip &> /dev/null; then
        pip install pre-commit
    else
        echo "Error: pip not found. Please install pre-commit manually:"
        echo "  pip install pre-commit"
        echo "  or visit: https://pre-commit.com/#installation"
        exit 1
    fi
fi

# Install the pre-commit hooks
echo "Installing pre-commit hooks..."
pre-commit install

# Install commit-msg hook for conventional commits (optional)
pre-commit install --hook-type commit-msg

echo "Pre-commit hooks installed successfully!"
echo ""
echo "The following hooks will run before each commit:"
echo "  - Code formatting (gofmt, goimports)"
echo "  - Linting (golangci-lint)"
echo "  - Go vet"
echo "  - Basic tests"
echo "  - Security checks"
echo "  - Coverage checks"
echo "  - File checks (trailing whitespace, etc.)"
echo ""
echo "To run hooks manually: pre-commit run --all-files"
echo "To skip hooks temporarily: git commit --no-verify"