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
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cheggaaa/pb/v3"
	"golang.org/x/xerrors"
)

const (
	version = "0.0.3"

	userAvailableAPI     = "https://api.twitter.com/i/users/username_available.json"
	userTimelineAPI      = "https://api.twitter.com/1.1/statuses/user_timeline.json"
	proxiesFileName      = "proxies.txt"
	resultFileNameFormat = "result_20060102150405.txt"
	maxGoroutineCount    = 5
	twitterToken         = "AAAAAAAAAAAAAAAAAAAAANRILgAAAAAAnNwIzUejRCOuH5E6I8xnZz4puTs%3D1Zv7ttfk8LF81IUq16cHjhLTvJu4FA33AGWWjCpTnA"
)

type userAvailableAPIResponse struct {
	Valid  bool   `json:"valid"`
	Reason string `json:"reason"`
	Desc   string `json:"desc"`
}

type userTimelineAPIResponse struct {
	Errors []Error `json:"errors"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	fmt.Println("Twitter ID Finder")
	fmt.Printf("Version %s\n", version)
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
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt)
	finishChannel := make(chan struct{}, 1)

	proxies, err := loadProxies(proxiesFileName)
	if err != nil {
		log.Fatalf("failed to load proxies: %s\n", err.Error())
	}

	wg.Add(len(targets))

	go func() {
		for i, target := range targets {
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

			if (i+1)%50 == 0 {
				time.Sleep(5 * time.Second)
			}
		}
	}()

	go func() {
		for v := range queue {
			availableIDs = append(availableIDs, v)
			wgQueue.Done()
		}
	}()

	go func() {
		for {
			select {
			case <-signalChannel:
				filename := time.Now().Format(resultFileNameFormat)
				if err := save(availableIDs, filename); err != nil {
					log.Fatalf("failed to save available IDs: %s\n", err.Error())
				}

				fmt.Printf("Saved to %s\n", filename)
				os.Exit(0)

			case <-finishChannel:
				return
			}
		}
	}()

	wg.Wait()
	wgQueue.Wait()

	bar.Finish()
	finishChannel <- struct{}{}

	fmt.Printf("Available IDs: %d / %d\n", len(availableIDs), len(targets))

	filename := time.Now().Format(resultFileNameFormat)
	if err := save(availableIDs, filename); err != nil {
		log.Fatalf("failed to save available IDs: %s\n", err.Error())
	}

	fmt.Printf("Saved to %s\n", filename)

	fmt.Print("Press Enter to close")
	fmt.Scan()
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
		"screen_name": []string{id},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s?%s", userTimelineAPI, query.Encode()), nil)
	if err != nil {
		return false, xerrors.Errorf("failed to initialize request: %w", err)
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", twitterToken))

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

	if response.StatusCode != http.StatusNotFound {
		return false, nil
	}

	data := new(userTimelineAPIResponse)

	if err = json.NewDecoder(response.Body).Decode(&data); err != nil {
		return false, xerrors.Errorf("failed to decode response body: %w", err)
	}

	if data.Errors[0].Code != 34 {
		return false, nil
	}

	query = url.Values{
		"username": []string{id},
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s?%s", userAvailableAPI, query.Encode()), nil)
	if err != nil {
		return false, xerrors.Errorf("failed to initialize request: %w", err)
	}

	response, err = client.Do(req)
	if err != nil {
		return false, xerrors.Errorf("failed to send HTTP request: %w", err)
	}

	defer response.Body.Close()

	d := new(userAvailableAPIResponse)

	if err = json.NewDecoder(response.Body).Decode(&d); err != nil {
		return false, xerrors.Errorf("failed to decode response body: %w", err)
	}

	return d.Valid, nil
}

func selectProxy(proxies []*url.URL) *url.URL {
	idx := rand.Intn(len(proxies))
	proxy := proxies[idx]

	return proxy
}

func save(availableIDs []string, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return xerrors.Errorf("failed to create file: %w", err)
	}

	defer file.Close()

	text := ""
	for _, availableID := range availableIDs {
		text += fmt.Sprintf("@%s\n", availableID)
	}

	_, err = file.WriteString(text)
	if err != nil {
		return xerrors.Errorf("failed to write file: %w", err)
	}

	return nil
}
