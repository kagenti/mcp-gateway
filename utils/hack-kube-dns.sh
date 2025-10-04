#!/bin/bash

# Hacks the cluster's DNS to hijack *.sslip.io requests for the mcp-gateway – used for demos, do not use in production

set -euo pipefail

gateway_ip=$(kubectl get gateway/mcp-gateway -n gateway-system -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)

if [[ -z "$gateway_ip" ]]; then
  echo "Error: could not determine mcp-gateway IP address. Is the gateway installed and running?" >&2
  exit 1
fi

patched=$(kubectl get configmap/coredns -n kube-system -o jsonpath='{.data.Corefile}' | grep $gateway_ip || true)

if [[ -n "$patched" ]]; then
  echo "CoreDNS is already patched to resolve *.sslip.io to $gateway_ip"
  exit 0
fi

echo "Patching CoreDNS to resolve *.sslip.io to the gateway IP..."

temp_dir=$(mktemp -d)
trap "rm -rf $temp_dir" EXIT

cat - <<EOF > $temp_dir/Corefile
sslip.io:53 {
    template IN A sslip.io {
        answer "{{ .Name }} 60 IN A $gateway_ip"
        fallthrough
    }
    template IN AAAA sslip.io {
        rcode NXDOMAIN
    }
    forward . /etc/resolv.conf
    cache 30
}
$(kubectl get configmap/coredns -n kube-system -o jsonpath='{.data.Corefile}')
EOF

kubectl create configmap coredns -n kube-system --from-file $temp_dir/Corefile --dry-run=client -o yaml | kubectl apply -f -
