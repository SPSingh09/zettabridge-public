package zerodha

import (
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers"
)

// Broker is the Zerodha Kite Connect live adapter (phase 2 alias; full move in later phases).
type Broker = livebrokers.ZerodhaBroker
