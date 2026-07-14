package main

import (
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func metricsServerOptions(addr string) metricsserver.Options {
	return metricsserver.Options{BindAddress: addr}
}
