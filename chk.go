package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"
)

func checkKey(key string) (bool, error) {
	client := &http.Client{
		Timeout: 15 * time.Second,
	}
	req, err := http.NewRequest("GET", "https://api.stripe.com/v1/balance", nil)
	if err != nil {
		return false, err
	}
	req.SetBasicAuth(key, "")
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	strBody := string(body)
	// تعیین وضعیت کلید
	if strings.Contains(strBody, "Invalid API Key provided") || strings.Contains(strBody, "api_key_expired") {
		return false, nil
	}
	if resp.StatusCode == 200 {
		return true, nil
	}
	return false, nil
}

func main() {
	file, err := os.Open("keys.txt")
	if err != nil {
		fmt.Println("Cannot open keys.txt:", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key := strings.TrimSpace(scanner.Text())
		if key == "" {
			continue
		}
		fmt.Printf("Checking key: %s ... ", key)
		ok, err := checkKey(key)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		if ok {
			fmt.Println("LIVE ✅")
		} else {
			fmt.Println("DEAD ❌")
		}
	}
}
