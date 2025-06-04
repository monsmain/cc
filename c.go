package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
	"golang.org/x/net/proxy"
)

const githubDataURL = "https://raw.githubusercontent.com/monsmain/cc/main/deta.JSON"

var countryMap map[string]string

func clearTerminal() {
	if strings.Contains(strings.ToLower(runtime.GOOS), "windows") {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		cmd.Run()
	} else {
		cmd := exec.Command("clear")
		cmd.Stdout = os.Stdout
		cmd.Run()
	}
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

func isValidCardNumber(number string) bool {
	return regexp.MustCompile(`^\d{13,19}$`).MatchString(number)
}
func isValidMonth(month string) bool {
	return regexp.MustCompile(`^(0[1-9]|1[0-2])$`).MatchString(month)
}
func isValidYear(year string) bool {
	return regexp.MustCompile(`^\d{4}$`).MatchString(year)
}
func isValidCVC(cvc string) bool {
	return regexp.MustCompile(`^\d{3,4}$`).MatchString(cvc)
}

func downloadCountryDataIfNotExists(localFilename string) error {
	if _, err := os.Stat(localFilename); err == nil {

		return nil
	}

	resp, err := http.Get(githubDataURL)
	if err != nil {
		return fmt.Errorf("failed to download data from GitHub: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to download data from GitHub: status %v", resp.Status)
	}
	file, err := os.Create(localFilename)
	if err != nil {
		return fmt.Errorf("failed to create local file: %v", err)
	}
	defer file.Close()
	_, err = io.Copy(file, resp.Body)
	return err
}

func loadCountryMap(filename string) map[string]string {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatalf("Could not open country data file: %v", err)
	}
	defer file.Close()

	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatalf("Could not read country data file: %v", err)
	}

	var countries map[string]string
	if err := json.Unmarshal(bytes, &countries); err != nil {
		log.Fatalf("Could not parse country data: %v", err)
	}
	return countries
}

func countryName(code string) string {
	if name, ok := countryMap[code]; ok {
		return name
	}
	return code
}

func main() {
	err := downloadCountryDataIfNotExists("data.JSON")
	if err != nil {
		log.Fatalf("Could not prepare country data file: %v", err)
	}
	countryMap = loadCountryMap("data.JSON")

	sk := "sk_test_BQokikJOvBiI2HlWgH4olfQ2"

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\033[H\033[2J")
	fmt.Print("Enter card number (e.g. 4912461004526326): ")
	cardNumber, _ := reader.ReadString('\n')
	cardNumber = strings.TrimSpace(cardNumber)
	if !isValidCardNumber(cardNumber) {
		fmt.Println("Invalid card number format.")
		return
	}

	fmt.Print("Enter card expiry month (e.g. 04): ")
	expMonth, _ := reader.ReadString('\n')
	expMonth = strings.TrimSpace(expMonth)
	if !isValidMonth(expMonth) {
		fmt.Println("Invalid expiry month format.")
		return
	}

	fmt.Print("Enter card expiry year (e.g. 2026): ")
	expYear, _ := reader.ReadString('\n')
	expYear = strings.TrimSpace(expYear)
	if !isValidYear(expYear) {
		fmt.Println("Invalid expiry year format.")
		return
	}

	fmt.Print("Enter card CVC (e.g. 011): ")
	cvc, _ := reader.ReadString('\n')
	cvc = strings.TrimSpace(cvc)
	if !isValidCVC(cvc) {
		fmt.Println("Invalid CVC format.")
		return
	}

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
		fmt.Printf("\n#RATE-LIMIT\nRESPONSE:  RATE LIMIT⚠️\nBALANCE: %s\nCURRENCY: %s\n", balance, currency)
	case strings.Contains(resp1Str, "tok_"):
		fmt.Printf("\nStatus: Active✅\nBALANCE: %s\nCURRENCY: %s\n", balance, currency)
	case strings.Contains(resp1Str, "api_key_expired"):
		fmt.Printf("\nDEAD\nRESPONSE: API KEY REVOKED ❌\n")
	case strings.Contains(resp1Str, "Invalid API Key provided"):
		fmt.Printf("\nDEAD\nRESPONSE: INVALID API KEY ❌\n")
	case strings.Contains(resp1Str, "testmode_charges_only"):
		fmt.Printf("\nDEAD\nRESPONSE: TESTMODE CHARGES ONLY ❌\n")
	case strings.Contains(resp1Str, "Your card was declined"):
		fmt.Printf("\nStatus: Active✅\nBALANCE: %s\nCURRENCY: %s\n", balance, currency)
	default:
		fmt.Printf("\nDEAD\nStatus: %s Declined❌\n", tokenData.Message)
	}

fmt.Println(string(body1))

	if tokenData.Card != nil {
		fmt.Printf("Type Card: %s\n", tokenData.Card.Brand)
		fmt.Printf("Country: %s\n", countryName(tokenData.Card.Country))
		if tokenData.Card.Name != "" {
			fmt.Printf("Name: %s\n", tokenData.Card.Name)
		} else {
			fmt.Println("Name: monsmain")
		}
	} else {
		fmt.Println("Card info not available.")
	}
}
