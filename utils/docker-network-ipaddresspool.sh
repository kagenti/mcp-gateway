#!/bin/bash

# Generates a MetalLB IpAddressPool for the given docker network.
# https://metallb.org/configuration/#defining-the-ips-to-assign-to-the-load-balancer-services
#
# Example:
# ./utils/docker-network-ipaddresspool.sh kind | kubectl apply -n metallb-system -f -

set -euo pipefail

networkName=$1
YQ="${2:-yq}"
offset=${3:-0}
cidr=${4:-28}
numIPs=${5:-16}

## Parse kind network subnet
## Take only IPv4 subnets, exclude IPv6
SUBNET=""
# Try podman version of cmd first. docker alias may be used for podman, so network
# command will be different
set +e
if command -v podman &>/dev/null; then
  SUBNET=$(podman network inspect -f '{{range .Subnets}}{{if eq (len .Subnet.IP) 4}}{{.Subnet}}{{end}}{{end}}' $networkName)
  if [[ -z "$SUBNET" ]]; then
    echo "Failed to obtain subnet using podman. Trying docker instead..." >&2
  fi
else
  echo "podman not found. Trying docker..." >&2
fi
set -e

# Fallback to docker version of cmd
if [[ -z "$SUBNET" ]]; then
  SUBNET=$(docker network inspect $networkName --format '{{json .IPAM.Config}}' | ${YQ} '.[] | select( .Subnet | test("^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}/\d+$")) | .Subnet')
fi
# Neither worked, error out
if [[ -z "$SUBNET" ]]; then
  echo "Error: parsing IPv4 network address for '$networkName' docker network"
  exit 1
fi

network=$(echo $SUBNET | cut -d/ -f1)
# shellcheck disable=SC2206
octets=(${network//./ })

# Default values of numIPs:16 and cidr:28 allows up to 16 clusters (offset 0-15) each with 16 possible IPs
# Note: Assumes the network will always give us a range allowing the use of all 256 ips, 0.0.0.[0-255]
address="${octets[0]}.${octets[1]}.${octets[2]}.$((numIPs * offset))/${cidr}"

echo "IPAddressPool address calculated to be '$address' for docker network subnet: '$SUBNET', numIps: '$numIPs' and offset: '$offset'" >&2

cat <<EOF | ADDRESS=$address ${YQ} '(select(.kind == "IPAddressPool") | .spec.addresses[0]) = env(ADDRESS)'
---
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: mcp-gateway
spec:
  addresses: [] # set by script based on docker network
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: empty
EOF
