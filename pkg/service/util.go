package service

import (
	"github.com/docker/docker/api/types/container"
)

func sumNetworkBytes(networks map[string]container.NetworkStats) (recv int64, send int64) {
	for _, ns := range networks {
		recv += int64(ns.RxBytes)
		send += int64(ns.TxBytes)
	}
	return
}
