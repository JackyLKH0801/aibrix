#!/bin/bash

# AIBrix Multi-Tenancy Test Script
# This script sends requests with different Tenant IDs to verify metrics and routing.

GATEWAY_URL="http://localhost:8888/v1/completions"
MODEL="qwen-code-lora-tenant"

echo "Sending requests to $GATEWAY_URL for model $MODEL..."

# Tenant A
echo "Sending request for Tenant A..."
curl -X POST $GATEWAY_URL \
  -H "Content-Type: application/json" \
  -H "X-Tenant-ID: tenant-a" \
  -d '{
    "model": "'"$MODEL"'",
    "prompt": "Write a hello world function in Python",
    "max_tokens": 50
  }'
echo -e "\n"

# Tenant B
echo "Sending request for Tenant B..."
curl -X POST $GATEWAY_URL \
  -H "Content-Type: application/json" \
  -H "X-Tenant-ID: tenant-b" \
  -d '{
    "model": "'"$MODEL"'",
    "prompt": "Write a hello world function in Go",
    "max_tokens": 50
  }'
echo -e "\n"

# Default Tenant (No Header)
echo "Sending request for Default Tenant..."
curl -X POST $GATEWAY_URL \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"$MODEL"'",
    "prompt": "Write a hello world function in Java",
    "max_tokens": 50
  }'
echo -e "\n"

echo "Requests completed. Check Grafana dashboard for metrics."
