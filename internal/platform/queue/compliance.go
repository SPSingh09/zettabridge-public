package queue

import (
	"errors"

	"github.com/SPSingh09/zettabridge/internal/compliance"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/plan"
)

func tradeCodeForLiveAccess(err error) brokererr.Code {
	if errors.Is(err, plan.ErrLiveNotAllowed) {
		return brokererr.CodeLiveNotAllowed
	}
	return brokererr.CodeInternal
}

func tradeCodeForCompliance(err error) brokererr.Code {
	if errors.Is(err, compliance.ErrAccountSuspended) {
		return brokererr.CodeAccountSuspended
	}
	if errors.Is(err, compliance.ErrOrgSuspended) {
		return brokererr.CodeOrgSuspended
	}
	return brokererr.CodeInternal
}
