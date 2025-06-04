package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

const keyPrefix = "sk_live_"

type StripeError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func checkKey(key string) (string, string, int, string, string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", "https://api.stripe.com/v1/balance", nil)
	if err != nil {
		return "ERROR", "", 0, "", "", err
	}
	req.SetBasicAuth(key, "")

	resp, err := client.Do(req)
	if err != nil {
		return "ERROR", "", 0, "", "", err
	}
	defer resp.Body.Close()

	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	body := string(bodyBytes)

	var stripeErr StripeError
	_ = json.Unmarshal(bodyBytes, &stripeErr)

	statusCode := resp.StatusCode
	message := stripeErr.Error.Message
	code := stripeErr.Error.Code

	if strings.Contains(body, "rate_limit") || strings.Contains(body, "too many requests") {
		return "RATE_LIMIT", body, statusCode, message, code, nil
	}
	if strings.Contains(body, "Invalid API Key") || strings.Contains(body, "api_key_expired") {
		return "DEAD", body, statusCode, message, code, nil
	}
	if statusCode == 200 {
		return "LIVE", body, statusCode, "", "", nil
	}
	return "DEAD", body, statusCode, message, code, nil
}

func trimBody(body string, max int) string {
	body = strings.ReplaceAll(body, "\n", " ")
	if len(body) > max {
		return body[:max] + "..."
	}
	return body
}

func main() {
	file, err := os.Open("keys.txt")
	if err != nil {
		fmt.Println("Cannot open keys.txt:", err)
		return
	}
	defer file.Close()

	rand.Seed(time.Now().UnixNano())
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		suffix := strings.TrimSpace(scanner.Text())
		if suffix == "" {
			continue
		}
		key := keyPrefix + suffix
		fmt.Printf("\n🔍 Checking key: %s\n", key)

		result, body, statusCode, reason, errCode, err := checkKey(key)
		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
			continue
		}

		fmt.Printf("→ HTTP Status Code: %d\n", statusCode)
		if reason != "" {
			fmt.Printf("→ Reason: %s\n", reason)
		}
		if errCode != "" {
			fmt.Printf("→ Error Code: %s\n", errCode)
		}
		fmt.Printf("→ Response Snippet: %s\n", trimBody(body, 300))

		switch result {
		case "LIVE":
			fmt.Println("✅ Result: LIVE")
		case "DEAD":
			fmt.Println("❌ Result: DEAD")
		case "RATE_LIMIT":
			fmt.Println("⏳ Rate limited! Waiting 60s before retrying...")
			time.Sleep(60 * time.Second)
			result, body, statusCode, reason, errCode, err = checkKey(key)
			fmt.Printf("→ Retry HTTP Code: %d | Reason: %s | Error Code: %s\n", statusCode, reason, errCode)
			if err == nil && result == "LIVE" {
				fmt.Println("✅ Retry Result: LIVE")
			} else if err == nil && result == "DEAD" {
				fmt.Println("❌ Retry Result: DEAD")
			} else {
				fmt.Println("⚠️ Retry Result: Still RATE LIMITED or ERROR")
			}
		default:
			fmt.Println("❓ Unknown result")
		}

		sleepSec := rand.Intn(5) + 3
		fmt.Printf("🕒 Sleeping %d seconds before next key...\n", sleepSec)
		time.Sleep(time.Duration(sleepSec) * time.Second)
	}
}
