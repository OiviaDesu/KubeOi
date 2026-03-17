#!/usr/bin/env bash
# Example node labels for the Oiviak3s cluster
# Apply these labels to your nodes using:
# kubectl label node <node-name> <label-key>=<label-value>

# Node: pi400 (Raspberry Pi 4B in Hanoi)
# kubectl label node pi400 oiviak3s.io/region=hanoi
# kubectl label node pi400 oiviak3s.io/tier=tertiary
# kubectl label node pi400 oiviak3s.io/power-stability=low

# Node: r620-01 (Dell R620 server in Hanoi)
# kubectl label node r620-01 oiviak3s.io/region=hanoi
# kubectl label node r620-01 oiviak3s.io/tier=primary
# kubectl label node r620-01 oiviak3s.io/power-stability=high

# Node: r620-02 (Dell R620 server in Hanoi)
# kubectl label node r620-02 oiviak3s.io/region=hanoi
# kubectl label node r620-02 oiviak3s.io/tier=primary
# kubectl label node r620-02 oiviak3s.io/power-stability=high

# Example cloud nodes (if applicable)
# kubectl label node cloud-node-01 oiviak3s.io/region=cloud
# kubectl label node cloud-node-01 oiviak3s.io/tier=secondary
# kubectl label node cloud-node-01 oiviak3s.io/power-stability=high

# Required labels:
# - oiviak3s.io/region: Geographic region (hanoi, cloud, etc.)
# - oiviak3s.io/tier: Node tier (primary, secondary, tertiary)
# - oiviak3s.io/power-stability: Power reliability (high, medium, low)
