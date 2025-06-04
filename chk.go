package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

func checkKey(key string) (string, error) {
	client := &http.Client{
		Timeout: 15 * time.Second,
	}
	req, err := http.NewRequest("GET", "https://api.stripe.com/v1/balance", nil)
	if err != nil {
		return "ERROR", err
	}
	req.SetBasicAuth(key, "")
	resp, err := client.Do(req)
	if err != nil {
		return "ERROR", err
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	strBody := string(body)
	if strings.Contains(strBody, "rate_limit") || strings.Contains(strBody, "too many requests") {
		return "RATE_LIMIT", nil
	}
	if strings.Contains(strBody, "Invalid API Key provided") || strings.Contains(strBody, "api_key_expired") {
		return "DEAD", nil
	}
	if resp.StatusCode == 200 {
		return "LIVE", nil
	}
	return "DEAD", nil
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
		key := strings.TrimSpace(scanner.Text())
		if key == "" {
			continue
		}
		fmt.Printf("Checking key: %s ... ", key)
		result, err := checkKey(key)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		switch result {
		case "LIVE":
			fmt.Println("LIVE ✅")
		case "DEAD":
			fmt.Println("DEAD ❌")
		case "RATE_LIMIT":
			fmt.Println("RATE LIMITED ⏳ (waiting 60s)")
			time.Sleep(60 * time.Second)
			result, err = checkKey(key)
			if err == nil && result == "LIVE" {
				fmt.Println("LIVE ✅")
			} else if err == nil && result == "DEAD" {
				fmt.Println("DEAD ❌")
			} else {
				fmt.Println("STILL RATE LIMITED OR ERROR")
			}
		default:
			fmt.Println("UNKNOWN RESPONSE")
		}
		sleepSec := rand.Intn(5) + 3
		time.Sleep(time.Duration(sleepSec) * time.Second)
	}
}
