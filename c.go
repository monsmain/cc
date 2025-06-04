package main
import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"golang.org/x/net/proxy"
)

type Config struct {
	StripeKey string
	ProxyAddr string
}

type CardInput struct {
	Number   string
	ExpMonth string
	ExpYear  string
	CVC      string
}

type CardInfo struct {
	Brand   string `json:"brand"`
	Country string `json:"country"`
	Name    string `json:"name"`
}

type TokenResponse struct {
	ID      string    `json:"id"`
	Object  string    `json:"object"`
	Card    *CardInfo `json:"card"`
	Message string    `json:"message"`
}

type BalanceInfo struct {
	Amount   string
	Currency string
}

func LoadConfig() (*Config, error) {
	key := os.Getenv("STRIPE_KEY")
	if key == "" {
		return nil, errors.New("STRIPE_KEY env variable is required")
	}
	proxyAddr := os.Getenv("TOR_PROXY")
	if proxyAddr == "" {
		proxyAddr = "127.0.0.1:9050"
	}
	return &Config{
		StripeKey: key,
		ProxyAddr: proxyAddr,
	}, nil
}

func NewTorClient(proxyAddr string) (*http.Client, error) {
	dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("error creating SOCKS5 dialer: %w", err)
	}
	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.Dial(network, addr)
	}
	transport := &http.Transport{
		DialContext:           dialContext,
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}, nil
}

func ReadCardInput() (*CardInput, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter card number (e.g. 4912461004526326): ")
	number, _ := reader.ReadString('\n')
	number = strings.TrimSpace(number)
	if !regexp.MustCompile(`^\d{13,19}$`).MatchString(number) {
		return nil, errors.New("invalid card number format")
	}

	fmt.Print("Enter card expiry month (MM, e.g. 04): ")
	month, _ := reader.ReadString('\n')
	month = strings.TrimSpace(month)
	if !regexp.MustCompile(`^(0[1-9]|1[0-2])$`).MatchString(month) {
		return nil, errors.New("invalid expiry month format")
	}

	fmt.Print("Enter card expiry year (YYYY, e.g. 2027): ")
	year, _ := reader.ReadString('\n')
	year = strings.TrimSpace(year)
	if !regexp.MustCompile(`^\d{4}$`).MatchString(year) {
		return nil, errors.New("invalid expiry year format")
	}

	fmt.Print("Enter card CVC (e.g. 011): ")
	cvc, _ := reader.ReadString('\n')
	cvc = strings.TrimSpace(cvc)
	if !regexp.MustCompile(`^\d{3,4}$`).MatchString(cvc) {
		return nil, errors.New("invalid CVC format")
	}

	return &CardInput{
		Number:   number,
		ExpMonth: month,
		ExpYear:  year,
		CVC:      cvc,
	}, nil
}

func CreateStripeToken(client *http.Client, key string, card *CardInput) (*TokenResponse, string, error) {
	data := url.Values{}
	data.Set("card[number]", card.Number)
	data.Set("card[exp_month]", card.ExpMonth)
	data.Set("card[exp_year]", card.ExpYear)
	data.Set("card[cvc]", card.CVC)

	req, err := http.NewRequest("POST", "https://api.stripe.com/v1/tokens", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, "", err
	}
	req.Header.Add("Authorization", "Bearer "+key)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; StripeChecker/2.0)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("error in Stripe token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	var tokenData TokenResponse
	_ = json.Unmarshal(body, &tokenData) 
	return &tokenData, string(body), nil
}

func GetStripeBalance(client *http.Client, key string) (*BalanceInfo, error) {
	req, err := http.NewRequest("GET", "https://api.stripe.com/v1/balance", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(key, "")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error in Stripe balance request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var balanceJSON map[string]interface{}
	if err := json.Unmarshal(body, &balanceJSON); err != nil {
		return &BalanceInfo{Amount: "N/A", Currency: "N/A"}, nil
	}

	amount := "N/A"
	currency := "N/A"
	if available, ok := balanceJSON["available"].([]interface{}); ok && len(available) > 0 {
		if first, ok := available[0].(map[string]interface{}); ok {
			if amt, ok := first["amount"]; ok {
				amount = fmt.Sprintf("%v", amt)
			}
			if curr, ok := first["currency"]; ok {
				currency = fmt.Sprintf("%v", curr)
			}
		}
	}

	return &BalanceInfo{
		Amount:   amount,
		Currency: currency,
	}, nil
}
func PrintResult(tokenData *TokenResponse, rawResponse string, balance *BalanceInfo, stripeKey string) {
	switch {
	case strings.Contains(rawResponse, "rate_limit"):
		fmt.Printf("\n#RATE-LIMIT : %s\nRESPONSE:  RATE LIMIT⚠️\nBALANCE: %s\nCURRENCY: %s\n", stripeKey, balance.Amount, balance.Currency)
	case strings.Contains(rawResponse, "tok_"):
		fmt.Printf("\n#LIVE : %s\nStatus: Active✅\nBALANCE: %s\nCURRENCY: %s\n", stripeKey, balance.Amount, balance.Currency)
	case strings.Contains(rawResponse, "api_key_expired"):
		fmt.Printf("\nDEAD : %s\nRESPONSE: API KEY REVOKED ❌\n", stripeKey)
	case strings.Contains(rawResponse, "Invalid API Key provided"):
		fmt.Printf("\nDEAD : %s\nRESPONSE: INVALID API KEY ❌\n", stripeKey)
	case strings.Contains(rawResponse, "testmode_charges_only"):
		fmt.Printf("\nDEAD : %s\nRESPONSE: TESTMODE CHARGES ONLY ❌\n", stripeKey)
	case strings.Contains(rawResponse, "Your card was declined"):
		fmt.Printf("\n#LIVE : %s\nStatus: Active✅\nBALANCE: %s\nCURRENCY: %s\n", stripeKey, balance.Amount, balance.Currency)
	default:
		msg := "Unknown decline"
		if tokenData != nil && tokenData.Message != "" {
			msg = tokenData.Message
		}
		fmt.Printf("\nDEAD: %s\nStatus: %s Declined❌\n", stripeKey, msg)
	}

	fmt.Println("\n--- Card Info (from Stripe) ---")
	if tokenData != nil && tokenData.Card != nil {
		fmt.Printf("Type (brand): %s\n", tokenData.Card.Brand)
		fmt.Printf("Country: %s\n", tokenData.Card.Country)
		if tokenData.Card.Name != "" {
			fmt.Printf("Name: %s\n", tokenData.Card.Name)
		} else {
			fmt.Println("Name: (not provided)")
		}
	} else {
		fmt.Println("Card info not available (token creation failed or invalid card).")
	}
}

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}

	card, err := ReadCardInput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Card input error: %v\n", err)
		os.Exit(1)
	}

	client, err := NewTorClient(cfg.ProxyAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Tor client error: %v\n", err)
		os.Exit(1)
	}

	tokenData, rawResp, err := CreateStripeToken(client, cfg.StripeKey, card)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Stripe token error: %v\n", err)
		os.Exit(1)
	}

	balance, err := GetStripeBalance(client, cfg.StripeKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Stripe balance error: %v\n", err)
		balance = &BalanceInfo{Amount: "N/A", Currency: "N/A"}
	}

	PrintResult(tokenData, rawResp, balance, cfg.StripeKey)
}
