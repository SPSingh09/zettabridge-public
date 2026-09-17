package pnl

// Row is a minimal trade record for win-rate computation (FIFO per symbol).
type Row struct {
	Signal    string
	Symbol    string
	LotSize   float64
	FillPrice float64
	Status    string
}

// WinRate returns wins / (wins + losses) for closed lots with fill prices, or nil when none.
func WinRate(trades []Row) *float64 {
	type position struct {
		lots     float64
		avgPrice float64
	}
	pos := make(map[string]position)
	var wins, losses int

	for _, t := range trades {
		if t.FillPrice <= 0 || t.Status != "filled" {
			continue
		}
		lot := t.LotSize
		if lot <= 0 {
			lot = 1
		}
		switch t.Signal {
		case "BUY":
			p := pos[t.Symbol]
			totalCost := p.avgPrice*p.lots + t.FillPrice*lot
			p.lots += lot
			if p.lots > 0 {
				p.avgPrice = totalCost / p.lots
			}
			pos[t.Symbol] = p
		case "SELL", "CLOSE":
			p, ok := pos[t.Symbol]
			if !ok || p.lots <= 0 {
				continue
			}
			closeLot := lot
			if closeLot > p.lots {
				closeLot = p.lots
			}
			if t.FillPrice > p.avgPrice {
				wins++
			} else if t.FillPrice < p.avgPrice {
				losses++
			}
			p.lots -= closeLot
			if p.lots <= 0 {
				delete(pos, t.Symbol)
			} else {
				pos[t.Symbol] = p
			}
		}
	}

	total := wins + losses
	if total == 0 {
		return nil
	}
	rate := float64(wins) / float64(total)
	return &rate
}
