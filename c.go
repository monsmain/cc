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

func getStr(source, start, end string) string {
	i := strings.Index(source, start)
	if i == -1 {
		return ""
	}
	i += len(start)
	j := strings.Index(source[i:], end)
	if j == -1 {
		return ""
	}
	return strings.TrimSpace(source[i : i+j])
}

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
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter your Stripe Secret Key (sk_live_... or sk_test_...): ")
	sk, _ := reader.ReadString('\n')
	sk = strings.TrimSpace(sk)

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

	fmt.Print("Enter card type (e.g. Visa, MasterCard, Amex): ")
	cardType, _ := reader.ReadString('\n')
	cardType = strings.TrimSpace(cardType)

	fmt.Print("Enter country code (e.g. IR, US, SE): ")
	country, _ := reader.ReadString('\n')
	country = strings.TrimSpace(country)

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
	resp1Str := string(body1)

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

	msg := getStr(resp1Str, `"message": "`, `"`)
	switch {
	case strings.Contains(resp1Str, "rate_limit"):
		fmt.Printf("\n#RATE-LIMIT : %s\nRESPONSE:  RATE LIMIT ⚠️\nBALANCE: %s\nCURRENCY: %s\n", sk, balance, currency)
	case strings.Contains(resp1Str, "tok_"):
		fmt.Printf("\n#LIVE : %s\nRESPONSE: VALID LIVE SK KEY✅\nBALANCE: %s\nCURRENCY: %s\n", sk, balance, currency)
	case strings.Contains(resp1Str, "api_key_expired"):
		fmt.Printf("\nDEAD : %s\nRESPONSE: API KEY REVOKED ❌\n", sk)
	case strings.Contains(resp1Str, "Invalid API Key provided"):
		fmt.Printf("\nDEAD : %s\nRESPONSE: INVALID API KEY ❌\n", sk)
	case strings.Contains(resp1Str, "testmode_charges_only"):
		fmt.Printf("\nDEAD : %s\nRESPONSE: TESTMODE CHARGES ONLY ❌\n", sk)
	case strings.Contains(resp1Str, "Your card was declined"):
		fmt.Printf("\n#LIVE : %s\nRESPONSE: VALID LIVE SK KEY✅\nBALANCE: %s\nCURRENCY: %s\n", sk, balance, currency)
	default:
		fmt.Printf("\nDEAD: %s\nRESPONSE: %s ❌\n", sk, msg)
	}

	fmt.Println("\n--- Card Details ---")
	fmt.Printf("Card Number: %s\n", cardNumber)
	fmt.Printf("Expiry: %s/%s\n", expMonth, expYear)
	fmt.Printf("CVC: %s\n", cvc)
	fmt.Printf("Card Type: %s\n", cardType)
	fmt.Printf("Country: %s\n", country)
}
