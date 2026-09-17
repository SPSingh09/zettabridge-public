package paper

import (
	"github.com/SPSingh09/zettabridge/internal/paperengine"
)

// Engine is the paper trading adapter (re-exported from paperengine for phase 2).
type Engine = paperengine.Engine

// Store is the persistence dependency for paper fills.
type Store = paperengine.Store

// New constructs a paper trading engine.
var New = paperengine.New
