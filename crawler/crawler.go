//go:generate mockgen -package crawler -source=crawler.go -destination crawler_mock.go

package crawler

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"

	"github.com/pkg/errors"
	"golang.org/x/net/html"
)

var ErrHttpStatusCode = errors.New("received HTTP error status code")

type httpClient interface {
	Get(string) (*http.Response, error)
}

type Page struct {
	URL   *url.URL
	Links []*url.URL
}

func (p *Page) Marshal() []byte {
	out := []byte("URL:\n\t" + p.URL.String() + "\nLinks: \n")
	for _, link := range p.Links {
		out = append(out, []byte("\t"+link.String()+"\n")...)
	}
	return out
}

type Crawler interface {
	Crawl(string, io.Writer) error
}

type crawler struct {
	workerCount int
	httpClient  httpClient
}

func New(workerCount int, httpClient httpClient) Crawler {
	return &crawler{
		workerCount: workerCount,
		httpClient:  httpClient,
	}
}

func (c *crawler) Crawl(rawURL string, out io.Writer) error {
	/*
		parse the rawURL argument into a *url.URL. Return an error if unable to parse.

		create
			- a cache of type map[string]struct{}{} to store urls that have already been seen
			- a channel of type *url.URL to be used for new urls
			- a *sync.WaitGroup to contrl the flow of the application

		increment the waitgroup by 1

		concurrently add the url argument to the newURLs channel

		concurrently
			- wait for the waitgroup to finish using the Wait function
			- close the newurls channel to indicate that there are no more new urls

		start n workers where n equals c.workerCount passing in the newURLs channel

		fan in the channels returned by the workers (channels of type *Page and error ) and listen to them in a select

		on receiving any error from a worker exit the application with a non-zero exit code

		on receiving a *Page from a worker
			- serialize the page and print it to the output stream
			- filter out links not of the same domain
			- filter out links that have already been seen
			- for each of the links left increment the waitgroup by 1 and put them on to the newurls channel concurrently (important!)
			- decrement the wait group by 1
	*/
	seedURL, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	cache := map[string]struct{}{}
	newURLs := make(chan *url.URL)

	wg.Add(1)
	go func() {
		newURLs <- seedURL
	}()

	go func() {
		defer close(newURLs)
		wg.Wait()
	}()

	pageChans := []<-chan *Page{}
	errChans := []<-chan error{}
	for i := 0; i < c.workerCount; i++ {
		pageChan, errChan := getPages(c.httpClient, newURLs)
		pageChans = append(pageChans, pageChan)
		errChans = append(errChans, errChan)
	}
	pageChan := mergePages(pageChans...)
	errChan := mergeErrors(errChans...)

	for {
		select {
		case page, ok := <-pageChan:
			if !ok {
				return nil
			}

			if _, err := out.Write(page.Marshal()); err != nil {
				return err
			}

			for _, link := range page.Links {
				if link.Hostname() == seedURL.Hostname() {
					if _, ok := cache[link.String()]; !ok {
						cache[link.String()] = struct{}{}

						wg.Add(1)
						go func(newURL *url.URL) {
							newURLs <- newURL
						}(link)
					}
				}
			}

		case err, ok := <-errChan:
			if !ok {
				return nil
			}

			if errors.Cause(err) == ErrHttpStatusCode {
				fmt.Println(err)
				break
			}
			if err, ok := err.(net.Error); ok && err.Timeout() {
				fmt.Println(err)
				break
			}
			return err
		}

		wg.Done()
	}
}

func getPages(httpClient httpClient, urls <-chan *url.URL) (<-chan *Page, <-chan error) {
	pages := make(chan *Page)
	errs := make(chan error)

	go func(pages chan<- *Page, errs chan<- error) {
		defer close(pages)
		defer close(errs)

		for url := range urls {
			resp, err := httpClient.Get(url.String())
			if err != nil {
				errs <- err
				return
			}

			if resp.StatusCode >= 400 {
				errs <- errors.Wrapf(ErrHttpStatusCode, "%s returned status code: %d", url, resp.StatusCode)
				continue
			}

			var buf bytes.Buffer
			if _, err := io.Copy(&buf, resp.Body); err != nil {
				errs <- err
				return
			}

			if err := resp.Body.Close(); err != nil {
				errs <- err
				return
			}

			pages <- &Page{URL: url, Links: collectLinks(url, &buf)}
		}
	}(pages, errs)

	return pages, errs
}

// collectLinks collects and formats each anchor tag link found on a web page
func collectLinks(pageURL *url.URL, r io.Reader) []*url.URL {
	links := []*url.URL{}

	t := html.NewTokenizer(r)
	for {
		if tkn := t.Next(); tkn == html.ErrorToken {
			return links
		}

		tag := t.Token()
		if tag.Data == "a" {
			for _, attr := range tag.Attr {
				if attr.Key == "href" {
					if link := formatURL(pageURL, attr.Val); link != nil {
						links = append(links, link)
						continue
					}
				}
			}
		}
	}
}

// formatURL formats a url relative to the page which it links from and strips the query fragment if found.
func formatURL(pageURL *url.URL, rawURL string) *url.URL {
	rel, err := pageURL.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	if rel.Scheme == "http" || rel.Scheme == "https" {
		rel.Fragment = "" // strip anchors to avoid crawling the same page twice...
		return rel
	}

	return nil
}

// merge fans in zero or more page channels in to a single page channel
func mergePages(pageChans ...<-chan *Page) <-chan *Page {
	var wg sync.WaitGroup
	out := make(chan *Page)

	wg.Add(len(pageChans))
	for _, pageChan := range pageChans {
		go func(pageChan <-chan *Page) {
			defer wg.Done()

			for page := range pageChan {
				out <- page
			}
		}(pageChan)
	}

	go func() {
		defer close(out)
		wg.Wait()
	}()

	return out
}

// merge fans in zero or more error channels in to a single error channel
func mergeErrors(errChans ...<-chan error) <-chan error {
	var wg sync.WaitGroup
	out := make(chan error)

	wg.Add(len(errChans))
	for _, errChan := range errChans {
		go func(errChan <-chan error, out chan<- error) {
			defer wg.Done()

			for err := range errChan {
				out <- err
			}
		}(errChan, out)
	}

	go func() {
		defer close(out)
		wg.Wait()
	}()

	return out
}
