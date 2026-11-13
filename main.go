// УСН 6% для ИП через API Точки: sync | calc | remind.
// Токен: TOCHKA_JWT из окружения или .env рядом. TLS проверяется всегда; свой корень УЦ: TOCHKA_CA=/путь/ca.pem.
// Поля транзакций сверены с документацией, не с живым ответом (счёт пока без поступлений): см. txns().
package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const base = "https://enter.tochka.com/uapi"
const ob = "/open-banking/v1.0"

type Config struct {
	Year           int               `json:"year"`
	Rate           float64           `json:"rate"`
	FixedContrib   float64           `json:"fixed_contrib"`
	ExtraPct       float64           `json:"extra_pct"`
	ExtraThreshold float64           `json:"extra_threshold"`
	Deadlines      map[string]string `json:"deadlines"`
	NotIncomeKW    []string          `json:"not_income_keywords"`
	ENP            ENP               `json:"enp"`
	Paid           []struct {
		Date   string  `json:"date"`
		Amount float64 `json:"amount"`
		Kind   string  `json:"kind"`
	} `json:"paid"`
}

var (
	dir    string
	cfg    Config
	client *http.Client
)

func die(a ...any) { fmt.Fprintln(os.Stderr, a...); os.Exit(1) }

func token() string {
	if t := os.Getenv("TOCHKA_JWT"); t != "" {
		return t
	}
	if f, err := os.Open(filepath.Join(dir, ".env")); err == nil {
		defer f.Close()
		for s := bufio.NewScanner(f); s.Scan(); {
			if v, ok := strings.CutPrefix(s.Text(), "TOCHKA_JWT="); ok {
				return strings.Trim(strings.TrimSpace(v), `"'`)
			}
		}
	}
	die("TOCHKA_JWT не найден")
	return ""
}

func api(method, path string, body any) map[string]any {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, base+path, rd)
	req.Header.Set("Authorization", "Bearer "+token())
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		die("запрос:", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		die(fmt.Sprintf("%s %s: %d %.200s", method, path, resp.StatusCode, raw)) // тело ответа, не токен
	}
	var out map[string]any
	json.Unmarshal(raw, &out)
	return out
}

// get достаёт вложенное значение по ключам; индекс "0" берёт первый элемент массива.
func get(v any, keys ...string) any {
	for _, k := range keys {
		switch t := v.(type) {
		case map[string]any:
			v = t[k]
		case []any:
			if k != "0" || len(t) == 0 {
				return nil
			}
			v = t[0]
		default:
			return nil
		}
	}
	return v
}

func str(v any) string { s, _ := v.(string); return s }

type Tx struct {
	ID, Date, Purpose string
	Amount            float64
	Credit            bool
}

func txns(st map[string]any) (out []Tx) {
	// ponytail: ключи по документации Get Statement; поправить по первому живому ответу.
	list, _ := get(st, "Data", "Statement", "0", "Transaction").([]any)
	for _, x := range list {
		m, _ := x.(map[string]any)
		id := str(m["paymentId"])
		if id == "" {
			id = str(m["transactionId"])
		}
		d := str(m["documentProcessDate"])
		if d == "" {
			d = str(m["bookingDateTime"])
		}
		amt, _ := get(m, "Amount", "amount").(float64)
		if a, ok := get(m, "Amount", "amount").(string); ok {
			fmt.Sscan(a, &amt)
		}
		if len(d) >= 10 {
			d = d[:10]
		}
		out = append(out, Tx{id, d, str(m["description"]), amt, str(m["creditDebitIndicator"]) == "Credit"})
	}
	return
}

func readLines(name string) (res []Tx) {
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return
	}
	defer f.Close()
	for s := bufio.NewScanner(f); s.Scan(); {
		var t Tx
		if json.Unmarshal(s.Bytes(), &t) == nil {
			res = append(res, t)
		}
	}
	return
}

func appendLine(name string, t Tx) {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		die(err)
	}
	defer f.Close()
	b, _ := json.Marshal(t)
	f.Write(append(b, '\n'))
}

func sync(dry bool) {
	accs, _ := get(api("GET", ob+"/accounts", nil), "Data", "Account").([]any)
	seen := map[string]bool{}
	for _, t := range append(readLines("income.jsonl"), readLines("review.jsonl")...) {
		seen[t.ID] = true
	}
	n := 0
	for _, a := range accs {
		aid := str(get(a, "accountId"))
		fmt.Println("счёт:", aid)
		end := time.Now()
		init := api("POST", ob+"/statements", map[string]any{"Data": map[string]any{"Statement": map[string]any{
			"accountId":     aid,
			"startDateTime": end.AddDate(0, 0, -40).Format("2006-01-02"),
			"endDateTime":   end.Format("2006-01-02")}}})
		sid := str(get(init, "Data", "Statement", "statementId"))
		var st map[string]any
		for i := 0; ; i++ { // Created -> Processing -> Ready
			st = api("GET", ob+"/accounts/"+aid+"/statements/"+sid, nil)
			if str(get(st, "Data", "Statement", "0", "status")) == "Ready" {
				break
			}
			if i >= 30 {
				die("выписка не стала Ready")
			}
			time.Sleep(2 * time.Second)
		}
		for _, t := range txns(st) {
			if !t.Credit || seen[t.ID] {
				continue
			}
			doubt := false
			for _, k := range cfg.NotIncomeKW {
				doubt = doubt || strings.Contains(strings.ToLower(t.Purpose), strings.ToLower(k))
			}
			mark, file := "+", "income.jsonl"
			if doubt {
				mark, file = "?", "review.jsonl"
			}
			fmt.Println(mark, t.Date, t.Amount, t.Purpose)
			if !dry {
				appendLine(file, t)
			}
			seen[t.ID] = true
			n++
		}
	}
	fmt.Println("новых:", n)
}

func reportRows() []Row {
	var inc, con []Income
	for _, t := range readLines("income.jsonl") {
		d, _ := time.Parse("2006-01-02", t.Date)
		inc = append(inc, Income{d, t.Amount})
	}
	for _, p := range cfg.Paid {
		if p.Kind == "fixed" || p.Kind == "1pct" {
			d, _ := time.Parse("2006-01-02", p.Date)
			con = append(con, Income{d, p.Amount})
		}
	}
	return Advances(inc, con, cfg)
}

func report() string {
	rows := reportRows()
	total := rows[3].Income
	var b strings.Builder
	fmt.Fprintf(&b, "УСН %d: доход %.0f ₽\n", cfg.Year, total)
	names := []string{"I кв", "полугодие", "9 мес", "год"}
	keys := []string{"advance_q1", "advance_h1", "advance_9m", "tax_year"}
	for i, r := range rows {
		fmt.Fprintf(&b, "%s: аванс %.0f ₽ до %s\n", names[i], r.Due, cfg.Deadlines[keys[i]])
	}
	fmt.Fprintf(&b, "Взнос фикс. %.0f ₽ до %s; 1%% свыше 300 000: %.0f ₽ до %s\n",
		cfg.FixedContrib, cfg.Deadlines["fixed_contrib"], ExtraContrib(total, cfg), cfg.Deadlines["extra_contrib"])
	if n := len(readLines("review.jsonl")); n > 0 {
		fmt.Fprintf(&b, "Проверить вручную (не засчитано в доход): %d поступл.\n", n)
	}
	return b.String()
}

func remind() {
	var soon []string
	for k, v := range cfg.Deadlines {
		if d, err := time.Parse("2006-01-02", v); err == nil {
			if days := time.Until(d).Hours() / 24; days >= -1 && days <= 7 {
				soon = append(soon, k)
			}
		}
	}
	if len(soon) == 0 {
		return
	}
	// ponytail: Telegram через существующий notify.py (обход DPI там, не дублируем); нужен python3 на ВМ.
	py := `import sys;sys.path.insert(0,sys.argv[1]);sys.path.insert(0,sys.argv[2]);from notify import send;send(sys.stdin.read())`
	scripts := os.Getenv("NOTIFY_DIR")
	if scripts == "" {
		scripts = "/opt/fable-opus-trader/scripts"
	}
	c := exec.Command("python3", "-c", py, scripts, dir) // dir: заглушка dotenv.py
	c.Stdin = strings.NewReader("Скоро срок: " + strings.Join(soon, ", ") + "\n" + report())
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		die("notify:", err)
	}
}

func main() {
	exe, _ := os.Executable()
	dir = filepath.Dir(exe)
	if d := os.Getenv("TOCHKA_DIR"); d != "" { // для `go run`
		dir = d
	}
	b, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil || json.Unmarshal(b, &cfg) != nil {
		die("config.json:", err)
	}
	tlsCfg := &tls.Config{}
	if ca := os.Getenv("TOCHKA_CA"); ca != "" {
		pem, err := os.ReadFile(ca)
		if err != nil {
			die(err)
		}
		pool, _ := x509.SystemCertPool()
		if pool == nil {
			pool = x509.NewCertPool()
		}
		pool.AppendCertsFromPEM(pem)
		tlsCfg.RootCAs = pool
	}
	client = &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: tlsCfg}}

	cmd := "calc"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "sync":
		sync(len(os.Args) > 2 && os.Args[2] == "--dry")
	case "calc":
		fmt.Print(report())
	case "remind":
		remind()
	case "draft":
		draft(os.Args[2:])
	default:
		die("команды: sync [--dry] | calc | remind | draft")
	}
}
