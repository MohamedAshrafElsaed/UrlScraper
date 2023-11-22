package main

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"golang.org/x/net/html"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type CacheItem struct {
	Response *MetaTagsResponse
	Count    int
}

var (
	client = &http.Client{
		Timeout: 5 * time.Second,
	}
	cache = make(map[string]*CacheItem)
	mu    sync.RWMutex
)

func main() {
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/api/meta", handleRequest).Methods("POST")
	log.Println("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", router))
}

type MetaTagsResponse struct {
	URL  string `json:"url"`
	Data struct {
		Description string            `json:"description"`
		OgTags      map[string]string `json:"ogTags"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
}

type request struct {
	URLs []string `json:"urls"`
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	var req request
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	responses := make([]*MetaTagsResponse, len(req.URLs))
	var wg sync.WaitGroup
	for i, url := range req.URLs {
		wg.Add(1)
		go func(i int, url string) {
			defer wg.Done()
			tags, err := getMetaTags(url)
			if err != nil {
				log.Printf("Error fetching meta tags for URL %s: %v", url, err)
				responses[i] = &MetaTagsResponse{URL: url, Error: err.Error()}
				return
			}
			responses[i] = tags
		}(i, url)
	}
	wg.Wait()

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(responses)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}

func getMetaTags(url string) (*MetaTagsResponse, error) {
	mu.Lock()
	item, found := cache[url]
	if found {
		item.Count++
		if item.Count >= 2 {
			// Invalidate the cache for this URL
			delete(cache, url)
		} else {
			cache[url] = item // Update the cache with the incremented count
		}
		mu.Unlock()
		if item.Count < 2 {
			return item.Response, nil
		}
		// If the count is 2, we proceed to fetch the data again
	} else {
		mu.Unlock()
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			return
		}
	}(resp.Body)

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	tagsResponse := &MetaTagsResponse{
		URL: url,
		Data: struct {
			Description string            `json:"description"`
			OgTags      map[string]string `json:"ogTags"`
		}{
			OgTags: make(map[string]string),
		},
	}
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "meta" {
			var key, value string
			for _, a := range n.Attr {
				if a.Key == "name" || a.Key == "property" {
					key = a.Val
				} else if a.Key == "content" {
					value = a.Val
				}
			}
			if strings.HasPrefix(key, "og:") {
				camelCaseKey := toCamelCase(key)
				tagsResponse.Data.OgTags[camelCaseKey] = value
			} else if key == "description" {
				tagsResponse.Data.Description = value
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	mu.Lock()
	cache[url] = &CacheItem{Response: tagsResponse, Count: 1}
	mu.Unlock()
	return tagsResponse, nil
}

func toCamelCase(str string) string {
	parts := strings.Split(str, ":")
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.ToTitle(parts[i])
	}
	return strings.Join(parts, "")
}
