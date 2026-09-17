package livebrokers

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"fmt"
	"time"

)

func submittedResult(prefix string) *domain.OrderResult {
	return &domain.OrderResult{
		Status:  domain.StatusSubmitted,
		OrderID: fmt.Sprintf("%s-%d", prefix, time.Now().UnixMilli()),
	}
}
