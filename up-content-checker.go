package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/Financial-Times/up-content-checker/imagechecker"
	"github.com/Financial-Times/up-content-checker/util"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type (
	Notification struct {
		Type         string `json:type`
		Id           string `json:id`
		ApiUrl       string `json:apiUrl`
		LastModified string `json:lastModified`
	}

	Link struct {
		Href string `json:href`
		Rel  string `json:rel`
	}

	NotificationsPage struct {
		RequestUrl    string         `json:requestUrl`
		Notifications []Notification `json:notifications`
		Links         []Link         `json:links`
	}
)

const (
	DELETE_NOTIFICATION = "http://www.ft.com/thing/ThingChangeType/DELETE"
)

var (
	uuidMatcher      = regexp.MustCompile(".*/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$")
	sinceMatcher     = regexp.MustCompile("https?://.*[?&]since=([^&]+).*")
	wg               sync.WaitGroup
	checkers         []util.Checker
	earliest         string
	notificationsUrl string
	uuids            string
	client *http.Client
	out          *csv.Writer
	articleCount int
	errorCount   int
)

func init() {
	flag.StringVar(&uuids, "uuids", "", "check one or more (comma-separated or whitespace-separated in quotes) UUIDs")
	flag.StringVar(&earliest, "since", "2016-03-31T00:00:00Z", "check content from the given RFC3339 date/time")
	flag.StringVar(&notificationsUrl, "notifications", "http://api.ft.com/content/notifications", "Notifications endpoint URL")
}

func main() {
	flag.Parse()

	if uuids == "" {
		log.Printf("Earliest: %s", earliest)
	} else {
		log.Printf("UUIDs: %s", uuids)
	}

	client = &http.Client{}

	checkers = append(checkers, imagechecker.NewChecker(client))

	f, _ := os.Create("up-content-check.csv")
	out = csv.NewWriter(f)
	defer f.Close()

	if uuids != "" {
		for _, uuid := range parseUuidList(uuids) {
			check(uuid)
		}
	} else {
		since := getStartDate()

		checkerPoolSize := 5
		uuidStream := make(chan string, checkerPoolSize - 1)
		for i := 0; i < checkerPoolSize; i++ {
			go checkItem(uuidStream)
			wg.Add(1)
		}

		pageStream := make(chan []string)
		go checker(pageStream, uuidStream)
		wg.Add(1)

		for {
			notifications, nextUrl, err := fetchPage(since)
			if err != nil {
				log.Fatal(err)
			}

			var uuids []string
			for _, notification := range *notifications {
				uuid, found := util.ExtractUuid(notification.ApiUrl)
				if !found {
					log.Printf("Skipping unexpected URL: %s", notification.ApiUrl)
					continue
				}

				if notification.Type == DELETE_NOTIFICATION {
					log.Printf("Skipping delete notification: %s", notification.ApiUrl)
					continue
				}

				uuids = append(uuids, uuid)
			}
			pageStream <- uuids

			nextSince := getNextSince(nextUrl)
			if (since == nextSince) || (nextSince == "") {
				log.Printf("Latest notification: %s", nextSince)
				log.Println("No more notifications to fetch")
				break
			}

			since = nextSince
		}
		close(pageStream)
		close(uuidStream)
	}

	wg.Wait()
	log.Printf("Processed %d UUIDs, %d had errors", articleCount, errorCount)
}

func check(uuid string) {
	for _, c := range checkers {
		rows, err := c.Check(uuid)
		if err != nil {
			log.Printf("UUID: %s result: %s error: %s", uuid, rows, err)
			out.Write([]string{uuid, err.Error()})
		}

		if len(rows) > 0 {
			errorCount++
			out.WriteAll(rows)
		}
	}
}

func parseUuidList(list string) []string {
	var uuids []string

	for _, s := range strings.Split(list, " ") {
		for _, uuid := range strings.Split(s, ",") {
			if util.IsUuid(uuid) {
				uuids = append(uuids, uuid)
			} else {
				log.Printf("Discarding invalid UUID: %s", uuid)
			}
		}
	}

	return uuids
}

func getStartDate() string {
	_, err := time.Parse(time.RFC3339, earliest)
	if err != nil {
		log.Fatal(err)
	}

	return earliest
}

func getNextSince(nextUrl string) string {
	match := sinceMatcher.FindStringSubmatch(nextUrl)
	if match == nil {
		return ""
	}

	return match[1]
}

func checker(uuids chan []string, checker chan string) {
	defer wg.Done()

	for {
		page, ok := <-uuids
		if !ok {
			break
		}

		for _, uuid := range page {
			checker <- uuid
		}
	}
}

func fetchPage(since string) (*[]Notification, string, error) {
	log.Printf("Fetching notifications since: %s", since)

	url := fmt.Sprintf("%s?since=%s", notificationsUrl, since)

	req, _ := http.NewRequest("GET", url, nil)
	util.AddBasicAuthentication(req)

	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return nil, "", err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Unexpected HTTP response %d", resp.StatusCode)
		return nil, "", err
	}

	var page NotificationsPage

	err = json.NewDecoder(resp.Body).Decode(&page)
	io.Copy(ioutil.Discard, resp.Body) // consume everything
	if err != nil {
		log.Printf("Unable to deserialize JSON: %s", err)
		return nil, "", err
	}

	var nextUrl string
	for _, link := range page.Links {
		if link.Rel == "next" {
			nextUrl = link.Href
			break
		}
	}

	return &page.Notifications, nextUrl, nil
}

func checkItem(uuids chan string) {
	defer wg.Done()

	for {
		uuid, ok := <-uuids
		if !ok {
			break
		}
		articleCount++
		check(uuid)
	}
}
