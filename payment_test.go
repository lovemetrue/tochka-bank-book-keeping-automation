package main

import (
	"testing"
	"time"
)

func TestBuildENP(t *testing.T) {
	c := Config{ENP: ENP{KBK: "18201061201010000510", KPP: "770701001", PayerStatus: "01"}}
	b, err := BuildENP(c, "40802810820001244449/044525104", "123456789012", 1234.5, time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	d := b["Data"].(map[string]any)
	if d["accountCode"] != "40802810820001244449" || d["bankCode"] != "044525104" || d["paymentAmount"] != 1234.5 ||
		d["payerKPP"] != "0" || d["taxInfoOKATO"] != "0" || d["taxInfoStatus"] != "01" || d["paymentDate"] != "2026-11-13" {
		t.Fatalf("%v", d)
	}
	if _, err := BuildENP(c, "x", "1", 1, time.Now()); err == nil {
		t.Fatal("плохой accountId должен давать ошибку")
	}
	if _, err := BuildENP(c, "40802810820001244449/044525104", "1", 0, time.Now()); err == nil {
		t.Fatal("нулевая сумма должна давать ошибку")
	}
}
