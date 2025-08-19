# Cluster setup without creating Kind
.PHONY: cluster-setup
cluster-setup:
	$(MAKE) gateway-api-install
	$(MAKE) istio-install
	$(MAKE) metallb-install
