package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/cheggaaa/pb/v3"
	"github.com/corpix/uarand"
	"golang.org/x/xerrors"
)

const (
	usernameAvailableAPI = "https://api.twitter.com/i/users/username_available.json"
	proxiesFileName      = "proxies.txt"
	maxGoroutineCount    = 100
)

type apiResponse struct {
	Valid  bool   `json:"valid"`
	Reason string `json:"reason"`
	Desc   string `json:"desc"`
}

func main() {
	fmt.Println("Twitter ID Finder")
	fmt.Println("Creator: @_m_vt")
	fmt.Println()

	fmt.Print("Digits: ")
	var digits int
	fmt.Scanf("%d", &digits)

	targets := make([]string, 0)
	for i := 0; float64(i) < math.Pow10(digits); i++ {
		targets = append(targets, fmt.Sprintf(zeroPaddingFormat(digits), i))
	}

	fmt.Printf("Target IDs: %d\n", len(targets))

	if !yn("Really?") {
		return
	}

	bar := pb.Full.Start(len(targets))
	ctx := context.Background()
	availableIDs := make([]string, 0)
	queue := make(chan string)
	var wg sync.WaitGroup
	var wgQueue sync.WaitGroup
	limiter := make(chan struct{}, maxGoroutineCount)

	proxies, err := loadProxies(proxiesFileName)
	if err != nil {
		log.Fatalf("failed to load proxies: %s\n", err.Error())
	}

	for _, target := range targets {
		wg.Add(1)
		limiter <- struct{}{}

		go func(target string) {
			defer wg.Done()
			defer func() { <-limiter }()
			defer bar.Increment()

			ok, err := check(ctx, target, proxies)
			if err != nil {
				log.Printf("An error occured: %s\n", err.Error())
				return
			}

			if ok {
				log.Printf("Found! @%s\n", target)
				wgQueue.Add(1)
				queue <- target
			}
		}(target)
	}

	go func() {
		for v := range queue {
			availableIDs = append(availableIDs, v)
			wgQueue.Done()
		}
	}()

	wg.Wait()
	wgQueue.Wait()

	bar.Finish()

	fmt.Printf("Available IDs: %d / %d\n", len(availableIDs), len(targets))

	for _, availableID := range availableIDs {
		fmt.Printf("@%s\n", availableID)
	}
}

func exists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func loadProxies(filename string) ([]*url.URL, error) {
	proxies := make([]*url.URL, 0)

	if !exists(filename) {
		return proxies, nil
	}

	fmt.Printf("%s found. load proxies...\n", proxiesFileName)

	file, err := os.Open(proxiesFileName)
	if err != nil {
		log.Fatalf("failed to open %s\n", proxiesFileName)
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		u, err := url.Parse(scanner.Text())
		if err != nil {
			continue
		}

		proxies = append(proxies, u)
	}

	if err = scanner.Err(); err != nil {
		log.Fatalf("failed to read %s: %s\n", proxiesFileName, err.Error())
	}

	fmt.Printf("%d proxies loaded\n", len(proxies))
	return proxies, nil
}

func zeroPaddingFormat(n int) string {
	return "%0" + strconv.Itoa(n) + "d"
}

func yn(question string) bool {
	for {
		fmt.Printf("%s [Y/n]: ", question)
		var answer string
		fmt.Scanf("%s", &answer)
		answerLower := strings.ToLower(answer)
		if len(answerLower) == 0 || answerLower == "y" {
			return true
		}
		if answerLower == "n" {
			return false
		}
	}
}

func check(ctx context.Context, id string, proxies []*url.URL) (bool, error) {
	query := url.Values{
		"username": []string{id},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s?%s", usernameAvailableAPI, query.Encode()), nil)
	if err != nil {
		return false, xerrors.Errorf("failed to initialize request: %w", err)
	}

	req.Header.Add("User-Agent", uarand.GetRandom())

	client := http.DefaultClient

	if len(proxies) != 0 {
		proxy := selectProxy(proxies)

		client = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxy),
			},
		}
	}

	response, err := client.Do(req)
	if err != nil {
		return false, xerrors.Errorf("failed to send HTTP request: %w", err)
	}

	defer response.Body.Close()

	data := new(apiResponse)

	if err = json.NewDecoder(response.Body).Decode(&data); err != nil {
		return false, xerrors.Errorf("failed to decode response body: %w", err)
	}

	return data.Valid, nil
}

func selectProxy(proxies []*url.URL) *url.URL {
	idx := rand.Intn(len(proxies))
	proxy := proxies[idx]

	return proxy
}
