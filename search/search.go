package search

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"

	"github.com/Ajnasz/go-loggly-cli/orderedbuffer"
	"github.com/Ajnasz/go-loggly-cli/semaphore"
	"github.com/bitly/go-simplejson"
	"golang.org/x/sync/errgroup"
)

// Client Loggly search client with user credentials, loggly
// does not seem to support tokens right now.
type Client struct {
	Token   string
	Account string

	// API
	endpoint string
	// Number of concurrent requests when fetching multiple pages.
	concurrency atomic.Int64
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
		endpoint: "loggly.com/apiv2",
	}

	return c
}

func (c *Client) SetConcurrency(n int) *Client {
	if n < 1 {
		n = 1
	}
	c.concurrency.Store(int64(n))
	return c
}

// URL Return the base api url.
func (c *Client) URL() string {
	return fmt.Sprintf("https://%s.%s", c.Account, c.endpoint)
}

// Get the given path.
func (c *Client) Get(ctx context.Context, path string) (*http.Response, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL()+path, nil)
	if err != nil {
		return nil, err
	}

	r.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.Token))
	r.Header.Set("User-Agent", "go-loggly-cli/1 author/Ajnasz")
	client := &http.Client{}
	return client.Do(r)
}

// GetJSON from the given path.
func (c *Client) GetJSON(ctx context.Context, path string) (j *simplejson.Json, err error) {
	res, err := c.Get(ctx, path)

	if err != nil {
		return
	}

	defer res.Body.Close()

	if res.StatusCode >= 400 {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			body = []byte(err.Error())
		}
		return nil, fmt.Errorf("go-loggly-search: %q, %s", res.Status, body)
	}

	body, err := io.ReadAll(res.Body)

	if err != nil {
		return nil, err
	}

	return simplejson.NewJson(body)
}

// CreateSearch Create a new search instance, loggly requires that a search
// is made before you may fetch events from it with a second call.
func (c *Client) CreateSearch(ctx context.Context, params string) (*simplejson.Json, error) {
	return c.GetJSON(ctx, "/search?"+params)
}

// GetEvents must be called after CreateSearch() with the
// correct rsid to reference the search.
func (c *Client) GetEvents(ctx context.Context, params string) (*simplejson.Json, error) {
	return c.GetJSON(ctx, "/events?"+params)
}

// Search response with total events, page number
// and the events array.
func (c *Client) Search(ctx context.Context, j *simplejson.Json, page int) (*Response, error) {
	id := j.GetPath("rsid", "id").MustString()

	qs := url.Values{}
	qs.Set("rsid", id)
	qs.Set("page", strconv.Itoa(page))

	j, err := c.GetEvents(ctx, qs.Encode())

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

func (c *Client) fetchAndStorePage(
	ctx context.Context,
	j *simplejson.Json,
	responsesStore *orderedbuffer.OrderedBuffer[Response],
	page int,
) (*Response, error) {
	res, err := c.Search(ctx, j, page)
	if err != nil {
		return nil, err
	}

	if res != nil {
		responsesStore.Store(page, *res)
	}

	return res, nil
}

func shouldStopFetching(err error, res *Response, pageSize int) bool {
	if err != nil {
		return true
	}

	if res == nil || len(res.Events) < pageSize {
		return true
	}

	return false
}

func (c *Client) fetchAllPages(ctx context.Context, q Query, resChan chan Response) error {
	defer close(resChan)
	j, err := c.CreateSearch(ctx, q.String())

	if err != nil {
		return err
	}

	concurrent := min(q.maxPages, c.concurrency.Load())
	sem := semaphore.New(concurrent)

	var page atomic.Int64
	page.Store(-1)

	var hasMore atomic.Bool
	hasMore.Store(true)
	responsesStore := orderedbuffer.NewOrderedBuffer(resChan)

	errg, ctx := errgroup.WithContext(ctx)

	for {
		if err := sem.Acquire(ctx); err != nil {
			return err
		}
		p := int(page.Add(1))
		errg.Go(func() error {
			defer sem.Release()

			res, err := c.fetchAndStorePage(ctx, j, responsesStore, p)

			if shouldStopFetching(err, res, q.size) {
				hasMore.Store(false)
			}
			return err
		})

		shouldBreak := page.Load() >= q.maxPages || !hasMore.Load()

		if shouldBreak {
			break
		}
	}

	return errg.Wait()
}

// Fetch Search response with total events, page number
// and the events array.
// Fetch will fetch all pages up to maxPages in the Query
// and return the results in order on the response channel.
// Errors are sent on the error channel.
// Both channels are closed when all fetching is done or an error occurs.
func (c *Client) Fetch(ctx context.Context, q Query) (chan Response, chan error) {
	resChan := make(chan Response)
	errChan := make(chan error)

	go func() {
		defer close(errChan)
		err := c.fetchAllPages(ctx, q, resChan)
		if err != nil {
			errChan <- err
		}
	}()

	return resChan, errChan
}
