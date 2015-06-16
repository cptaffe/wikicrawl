
//
// ffcrawl [target regexp] [start article]
//
// Takes a regexp expression matching a target article name
// and a start article name, e.g. "ffcrawl Car Vehicle"
// will accept any url with "Car" in the name as a target,
// and begins at http://en.wikipedia.org/wiki/Vehicle
//
// Starting at the start article, the program follows the first
// link in the article's text that links directly to another
// article until the current article matches the target regexp.
//
// If the traversal is taking too long, sending SIGINT
// (pressing ^C usually) will print the trip so far. Each
// url next to its offset from the original page.
//
// This tool was created in part because during school there
// was once a saying that if one followed the first link on
// a Wikipedia page and repeated this process long enough,
// one would eventually get to a certian prominent historical
// figure's Wikipedia page. Now, you can test how many links
// it takes to do it, and get a readout of the trip.
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
	"regexp"
)

// Wikipedia puts the main section of the article
// within a div tag with the id "mw-content-text"
var divId string = "mw-content-text"

// Page serves as a linked list of URLs.
type Page struct {

	// URL of this page.
	Url   *url.URL

	// Page created from an accepted link on this page.
	Next  *Page
}

// FollowLink returns the first accepted link from a Page.
// The body of the response from a GET request on the Page's Url
// is parsed as html for a <p> tag within a <div> tag with an id
// attribute matching divId.
// An accepted html tag sequence may look like the following
// psuedo regex expression:
// <div id={divId}><div>+<p>+<a href={accepted url}>...
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

	var targetRegex *regexp.Regexp
	var startPage *Page

	if len(os.Args) == 3 {
		var err error
		targetRegex, err = regexp.Compile(os.Args[1])
		if err != nil {
			log.Fatal(err.Error())
		}

		var ur *url.URL
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
			fmt.Printf("Have Followed %d links\r", visits)

			haveVisited[*page.Url] = *page
			visits++

			// Match against user provided regex
			if targetRegex.MatchString(strings.TrimPrefix(page.Url.String(), prefix)) {
				fmt.Printf("Found match, took %d follows\n", visits)
				break
			}

			// Get next link
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
