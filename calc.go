package main

import (
	"math"
	"time"
)

type Income struct {
	Date   time.Time
	Amount float64
}

type Row struct {
	To                        time.Time
	Income, Tax, Contrib, Due float64
}

func r2(x float64) float64 { return math.Round(x*100) / 100 }

// Advances: аванс = доход_нарастающим*ставка - взносы_уплаченные - авансы_предыдущих.
// ponytail: ИП без сотрудников, вычет 100%; для работодателя нужен кап 50%.
func Advances(inc, contribs []Income, cfg Config) []Row {
	y := cfg.Year
	ends := [][2]int{{3, 31}, {6, 30}, {9, 30}, {12, 31}}
	start := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
	sum := func(xs []Income, end time.Time) (s float64) {
		for _, x := range xs {
			if !x.Date.Before(start) && !x.Date.After(end) {
				s += x.Amount
			}
		}
		return
	}
	var out []Row
	prev := 0.0
	for _, e := range ends {
		end := time.Date(y, time.Month(e[0]), e[1], 0, 0, 0, 0, time.UTC)
		in, con := sum(inc, end), sum(contribs, end)
		tax := r2(in * cfg.Rate)
		due := math.Max(0, r2(tax-con-prev))
		out = append(out, Row{end, in, tax, con, due})
		prev += due
	}
	return out
}

// ExtraContrib: 1% с дохода свыше порога (до 1 июля следующего года).
func ExtraContrib(yearIncome float64, cfg Config) float64 {
	return r2(math.Max(0, yearIncome-cfg.ExtraThreshold) * cfg.ExtraPct)
}
