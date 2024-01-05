package core

import (
	"github.com/cometbft/cometbft/metrics"
	rpctypes "github.com/cometbft/cometbft/rpc/jsonrpc/types"
)

func Metrics(ctx *rpctypes.Context) (map[string]map[string]string, error) {
	return metrics.Instance().GetValues(), nil
}
