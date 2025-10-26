# MCP Gateway Deployment on OpenShift

The MCP Gateway can be deployed to an OpenShift environment. The assets included within this directory include resources in both Kustomize and Helm Chart formats including:

* OpenShift Service Mesh
* Red Hat Connectivity Link
* MCP Gateway
* MCP Gateway Ingress

## Deploying to OpenShift

A script named [deploy_openshift.sh](deploy_openshift.sh) is available to facilitate the deployment to OpenShift.

Execute the following command to deploy each of the aforementioned components to an OpenShift cluster:

```shell
./deploy_openshift.sh
```

The MCP Gateway will be available at the output of the following command:

```shell
echo https://$(oc get routes -n gateway-system -o jsonpath='{ .items[0].spec.host }')/mcp
```

## Deploying to OpenShift using OpenShift GitOps (Argo CD)

OpenShift GitOps (Argo CD) can be used to deploy the MCP Gateway to an OpenShift environment.

### Prerequisites

1. Cluster scoped OpenShift GitOps previously deployed

### Deployment

Execute the following command to deploy the MCP Gateway to OpenShift using OpenShift GitOps


```shell
./deploy_openshift_argocd.sh
```

The MCP Gateway will be available at the output of the following command:

```shell
echo https://$(oc get routes -n gateway-system -o jsonpath='{ .items[0].spec.host }')/mcp
```

## Verify Access to the MCP Gateway

To confirm that MCP Gateway has been deployed successfully, execute the following command:

```shell
curl -k -LX POST https://$(oc get routes -n gateway-system -o jsonpath='{ .items[0].spec.host }')/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize"}'
```

A response similar to the following indicates the MCP Gateway was successfully deployed


```shell
{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26","capabilities":{"tools":{"listChanged":true}},"serverInfo":{"name":"Kagenti MCP Broker","version":"0.0.1"}}}
```
