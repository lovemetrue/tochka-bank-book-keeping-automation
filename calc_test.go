package main

import (
	"testing"
	"time"
)

func d(m, day int) time.Time { return time.Date(2026, time.Month(m), day, 0, 0, 0, 0, time.UTC) }

var tcfg = Config{Year: 2026, Rate: 0.06, ExtraPct: 0.01, ExtraThreshold: 300000}

func TestExtra(t *testing.T) {
	if ExtraContrib(2_400_000, tcfg) != 21000 || ExtraContrib(250_000, tcfg) != 0 {
		t.Fatal("extra")
	}
}

func TestAdvances(t *testing.T) {
	inc := []Income{{d(2, 1), 500000}, {d(5, 1), 500000}}
	r := Advances(inc, []Income{{d(3, 20), 57390}}, tcfg)
	if r[0].Due != 0 || r[0].Tax != 30000 || r[1].Due != 2610 {
		t.Fatalf("%+v", r)
	}
	r = Advances(inc, []Income{{d(12, 20), 57390}}, tcfg) // взнос в декабре не уменьшает Q1-Q3
	if r[0].Due != 30000 || r[1].Due != 30000 || r[3].Due != 0 {
		t.Fatalf("%+v", r)
	}
}
