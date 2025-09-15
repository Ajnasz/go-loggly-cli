package search

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/Ajnasz/go-loggly-cli/orderedbuffer"
	"github.com/bitly/go-simplejson"
)

// Client Loggly search client with user credentials, loggly
// does not seem to support tokens right now.
type Client struct {
	Token       string
	Account     string
	Endpoint    string
	concurrency int
}

// Response Search response with total events, page number
// and the events array.
type Response struct {
	Total  int64
	Page   int64
	Events []any
}

// New Create a new loggly search client with credentials.
func New(account string, token string) *Client {
	c := &Client{
		Account:  account,
		Token:    token,
		Endpoint: "loggly.com/apiv2",
	}

	return c
}

func (c *Client) SetConcurrency(n int) *Client {
	if n < 1 {
		n = 1
	}
	c.concurrency = n
	return c
}

// URL Return the base api url.
func (c *Client) URL() string {
	return fmt.Sprintf("https://%s.%s", c.Account, c.Endpoint)
}

// Get the given path.
func (c *Client) Get(path string) (*http.Response, error) {
	r, err := http.NewRequest(http.MethodGet, c.URL()+path, nil)
	if err != nil {
		return nil, err
	}

	r.Header.Add("Authorization", fmt.Sprintf("bearer %s", c.Token))
	r.Header.Set("User-Agent", "go-loggly-cli/1 author/Ajnasz")
	client := &http.Client{}
	return client.Do(r)
}

// GetJSON from the given path.
func (c *Client) GetJSON(path string) (j *simplejson.Json, err error) {
	res, err := c.Get(path)

	if err != nil {
		return
	}

	defer res.Body.Close()

	if res.StatusCode >= 400 {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("go-loggly-search: %q, %s", res.Status, body)
	}

	body, err := io.ReadAll(res.Body)

	if err != nil {
		return
	}

	j, err = simplejson.NewJson(body)
	return
}

// CreateSearch Create a new search instance, loggly requires that a search
// is made before you may fetch events from it with a second call.
func (c *Client) CreateSearch(params string) (*simplejson.Json, error) {
	return c.GetJSON("/search?" + params)
}

// GetEvents must be called after CreateSearch() with the
// correct rsid to reference the search.
func (c *Client) GetEvents(params string) (*simplejson.Json, error) {
	return c.GetJSON("/events?" + params)
}

// Search response with total events, page number
// and the events array.
func (c *Client) Search(j *simplejson.Json, page int) (*Response, error) {
	id := j.GetPath("rsid", "id").MustString()

	j, err := c.GetEvents("rsid=" + id + "&page=" + strconv.Itoa(page))

	if err != nil {
		return nil, err
	}

	// Search response with total events, page number
	// and the events array.
	return &Response{
		Total:  j.Get("total_events").MustInt64(),
		Page:   j.Get("page").MustInt64(),
		Events: j.Get("events").MustArray(),
	}, nil

}

// Fetch Search response with total events, page number
// and the events array.
func (c *Client) Fetch(q Query) (chan Response, chan error) {
	resChan := make(chan Response)
	errChan := make(chan error)

	concurrent := min(q.maxPages, c.concurrency)

	pool := make(chan struct{}, concurrent)

	var page atomic.Int64
	page.Store(-1)
	go func() {
		defer close(resChan)
		defer close(errChan)
		j, err := c.CreateSearch(q.String())

		if err != nil {
			errChan <- err
			return
		}

		var wg sync.WaitGroup
		var hasMore atomic.Bool
		hasMore.Store(true)
		var lastPrintedPage atomic.Int32
		lastPrintedPage.Store(-1)
		responsesStore := orderedbuffer.NewOrderedBuffer(resChan)
		for {
			pool <- struct{}{}
			wg.Add(1)
			go func(page int) {
				defer wg.Done()
				defer func() { <-pool }()

				res, err := c.Search(j, page)
				if err != nil {
					errChan <- err
					hasMore.Store(false)
					return
				}

				if res != nil {
					responsesStore.Store(page, *res)

					if len(res.Events) < q.size {
						hasMore.Store(false)
						return
					}
				} else {
					hasMore.Store(false)
					return
				}
			}(int(page.Add(1)))
			if int(page.Load()) >= q.maxPages || !hasMore.Load() {
				break
			}
		}

		wg.Wait()
	}()

	return resChan, errChan
}
