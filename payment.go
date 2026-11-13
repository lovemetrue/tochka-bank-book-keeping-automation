package main

import (
	"fmt"
	"strings"
	"time"
)

type ENP struct {
	Name, INN, KPP, Account, BIC string
	CorrAccount                  string `json:"corr_account"`
	KBK                          string `json:"kbk"`
	PayerStatus                  string `json:"payer_status"`
	Purpose                      string `json:"purpose"`
}

// BuildENP собирает тело Create Payment For Sign для единого налогового платежа ИП.
// Поля 105-109 для ЕНП = "0", КПП плательщика-ИП = "0" (ФНС, приказ Минфина № 58н).
func BuildENP(cfg Config, accountID, payerINN string, amount float64, day time.Time) (map[string]any, error) {
	acc, bank, ok := strings.Cut(accountID, "/")
	if !ok || len(acc) != 20 || len(bank) != 9 {
		return nil, fmt.Errorf("accountId ожидается как <20 цифр>/<БИК>")
	}
	if amount <= 0 {
		return nil, fmt.Errorf("сумма платежа %.2f: нечего платить", amount)
	}
	if payerINN == "" {
		return nil, fmt.Errorf("API не вернул ИНН (taxCode)")
	}
	e := cfg.ENP
	return map[string]any{"Data": map[string]any{
		"accountCode": acc, "bankCode": bank,
		"payerINN": payerINN, "payerKPP": "0",
		"counterpartyBankBic": e.BIC, "counterpartyBankCorrAccount": e.CorrAccount,
		"counterpartyAccountNumber": e.Account, "counterpartyINN": e.INN, "counterpartyKPP": e.KPP,
		"counterpartyName": e.Name,
		"paymentAmount":    r2(amount), "paymentDate": day.Format("2006-01-02"),
		"paymentPurpose": e.Purpose, "paymentPriority": "5",
		"taxInfoKBK": e.KBK, "taxInfoOKATO": "0", "taxInfoStatus": e.PayerStatus,
		"taxInfoReasonCode": "0", "taxInfoPeriod": "0", "taxInfoDocumentNumber": "0", "taxInfoDocumentDate": "0",
	}}, nil
}

// draft: `draft advance|fixed|extra [--amount N] [--send]`. Без --send только показывает черновик.
// Платёж уходит в банк «на подпись»; подтверждает человек в интернет-банке, мы ничего не списываем.
func draft(args []string) {
	if len(args) == 0 {
		die("draft advance|fixed|extra [--amount N] [--send]")
	}
	kind, amount, send := args[0], 0.0, false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--send":
			send = true
		case "--amount":
			i++
			if i >= len(args) {
				die("--amount N")
			}
			fmt.Sscan(strings.ReplaceAll(args[i], ",", "."), &amount)
		}
	}
	rows := reportRows()
	switch {
	case amount > 0:
	case kind == "fixed":
		amount = cfg.FixedContrib
	case kind == "advance":
		keys := []string{"advance_q1", "advance_h1", "advance_9m", "tax_year"}
		for i, k := range keys {
			if d, _ := time.Parse("2006-01-02", cfg.Deadlines[k]); !d.Before(time.Now().Truncate(24 * time.Hour)) {
				amount = rows[i].Due
				fmt.Printf("аванс по сроку %s: %.2f ₽ (расчёт не вычитает уже уплаченные авансы: сверьте или задайте --amount)\n", k, amount)
				break
			}
		}
	case kind == "extra":
		die("для extra задайте --amount (1% с дохода прошлого года, считается по доходу предыдущего периода)")
	default:
		die("неизвестный вид платежа:", kind)
	}

	accs, _ := get(api("GET", ob+"/accounts", nil), "Data", "Account").([]any)
	if len(accs) == 0 {
		die("нет счетов")
	}
	aid := str(get(accs[0], "accountId"))
	cust, _ := get(api("GET", ob+"/customers", nil), "Data", "Customer", "0").(map[string]any)
	body, err := BuildENP(cfg, aid, str(cust["taxCode"]), amount, time.Now().In(time.FixedZone("MSK", 3*3600)))
	if err != nil {
		die(err)
	}
	d := body["Data"].(map[string]any)
	inn := d["payerINN"].(string)
	fmt.Printf("ЕНП %s: %.2f ₽ → %s, КБК %s, дата %s, ИНН плательщика ***%s\n",
		kind, amount, cfg.ENP.Name, cfg.ENP.KBK, d["paymentDate"], inn[max(0, len(inn)-4):])
	if !send {
		fmt.Println("черновик НЕ отправлен (добавьте --send)")
		return
	}
	resp := api("POST", "/payment/v1.0/for-sign", body)
	data, _ := resp["Data"].(map[string]any)
	fmt.Println("создан черновик «на подпись». Подтвердите в интернет-банке.")
	for _, k := range []string{"requestId", "redirectURL", "redirectUrl"} {
		if v := str(data[k]); v != "" {
			fmt.Printf("%s: %s\n", k, v)
		}
	}
	fmt.Println("Напоминание: уведомление об исчисленных суммах по ЕНП подаётся отдельно (до 25-го, ЛК ФНС).")
}
