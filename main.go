package main

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"golang.org/x/net/html"
	"log"
	"net/http"
)

func main() {

	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/api/meta", handleRequest).Methods("POST")
	log.Println("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", router))
}

type request struct {
	URL string `json:"url"`
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	var req request
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tags, err := getMetaTags(req.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tags)
}

type MetaTagsResponse struct {
	Data struct {
		Description   string `json:"description"`
		OgDescription string `json:"ogDescription"`
		OgTitle       string `json:"ogTitle"`
	} `json:"data"`
}

func getMetaTags(url string) (*MetaTagsResponse, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	tagsResponse := &MetaTagsResponse{}
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
			switch key {
			case "description":
				tagsResponse.Data.Description = value
			case "og:description":
				tagsResponse.Data.OgDescription = value
			case "og:title":
				tagsResponse.Data.OgTitle = value
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}

	f(doc)
	return tagsResponse, nil
}
