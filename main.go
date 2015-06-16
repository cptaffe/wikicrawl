package main

import (
	"fmt"
	"golang.org/x/net/html"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
)

// Wikipedia puts the main section of the article
// within a div tag with the id "mw-content-text"
var divId string = "mw-content-text"

type Page struct {
	Url   *url.URL
	Next  *Page
}

func (page *Page) IsMatch(opage *Page) bool {
	return page.Url.String() == opage.Url.String()
}

// Gets the first link of a wikipedia page.
func (page *Page) FollowLink(acceptFunc func(ur *url.URL) bool) (*Page, error) {
	resp, err := http.Get(page.Url.String())
	if err != nil {
		return page, err
	}

	body := resp.Body
	defer body.Close()

	z := html.NewTokenizer(body)
	inBody := false
	inP := 0
	depth := 0
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return page, z.Err()
		case html.StartTagToken, html.EndTagToken:
			tn, _ := z.TagName()
			if string(tn) == "div" {
				if tt == html.StartTagToken {
					if inBody {
						// Descend into an inner div
						depth++
					} else {
						// This is a div tag
						// Loop through attributes for an id
						more := true
						for more {
							key, val, m := z.TagAttr()
							more = m
							if string(key) == "id" && string(val) == divId {
								inBody = true
							}
						}
					}
				} else {
					if depth == 0 {
						inBody = false
					}
				}
			} else if inBody && string(tn) == "p" {
				if tt == html.StartTagToken {
					inP++
				} else {
					inP--
				}
			} else if inP > 0 && tt == html.StartTagToken && string(tn) == "a" {
				// This is an anchor tag
				// This is an anchor tag in a div
				// Check if it has an href attribute
				more := true
				for more {
					key, val, m := z.TagAttr()
					more = m
					if string(key) == "href" {
						// Parse URL
						ur, err := page.Url.Parse(string(val))
						if err != nil {
							// If this url is not parseable,
							// skip to the second url
							break
						}

						if acceptFunc(ur) {
							p := &Page{Url: ur}
							page.Next = p
							return p, nil
						}
					}
				}
			}
		}
	}
}

func main() {
	haveVisited := make(map[url.URL]Page)
	visits := 0

	prefix := "http://en.wikipedia.org/wiki/"

	var targetPage *Page
	var startPage *Page

	if len(os.Args) == 3 {
		ur, err := url.Parse(prefix+os.Args[1])
		if err != nil {
			log.Fatal(err)
		}

		targetPage = &Page{Url: ur}

		ur, err = url.Parse(prefix+os.Args[2])
		if err != nil {
			log.Fatal(err)
		}

		// Initial page to start crawler
		startPage = &Page{Url: ur}
	} else {
		fmt.Println("Needs url to start crawler")
		return
	}

	done := make(chan bool)
	go func () {
		page := startPage
		for {
			pg, err := page.FollowLink(func(ur *url.URL) bool {
				// Don't Revisit pages
				p := haveVisited[*ur]
				if p.Url != nil {
					return false
				}

				// Don't leave the world of Wikipedia
				if !strings.HasPrefix(ur.String(), prefix) {
					return false
				}

				// check after prefix url
				str := strings.TrimPrefix(ur.String(), prefix)

				// Cannot be a file, e.g. a resource page
				// Cannot be a non top-level Wikipedia page
				// Cannot be a sup page hash link
				if strings.Contains(str, ":") || strings.Contains(str, "/") || strings.Contains(str, "#") {
					return false
				}


				return true
			})
			if err != nil {
				if err.Error() == "EOF" {
					// Could not find a link on this file,
					// Go back up one page
					lp := startPage
					p := startPage
					for p.Next != nil {
						lp = p
						p = p.Next
					}
					if (lp == startPage) {
						log.Fatal("Cannot find links on provided page")
					}
					page = lp
					visits--
					continue
				}
				log.Fatal(err)
			}
			page = pg

			fmt.Printf("Have Followed %d links\r", visits)

			haveVisited[*page.Url] = *page
			visits++

			if page.IsMatch(targetPage) {
				fmt.Printf("Found match, took %d follows\n", visits)
				break
			}
		}
		done <- true
	}()

	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt)

	// Wait for successful path or sigint
	select {
	case <-done:
	case <-sig:
	}

	// Print path
	page := startPage
	i := 0
	for page != nil {
		fmt.Printf("%d: %s\n", i, page.Url)
		page = page.Next
		i++
	}
}
