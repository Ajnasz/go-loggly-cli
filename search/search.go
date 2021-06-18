package search

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"

	"github.com/bitly/go-simplejson"
)

// Client Loggly search client with user credentials, loggly
// does not seem to support tokens right now.
type Client struct {
	Token    string
	Account  string
	Endpoint string
}

// Response Search response with total events, page number
// and the events array.
type Response struct {
	Total  int64
	Page   int64
	Events []interface{}
}

// Query builder struct
type Query struct {
	client   *Client
	query    string
	from     string
	until    string
	order    string
	size     int
	maxPages int
}

// Create a new query
func newQuery(c *Client, str string) *Query {
	return &Query{
		client: c,
		query:  str,
		from:   "-24h",
		until:  "now",
		order:  "desc",
		size:   100,
	}
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
		return nil, fmt.Errorf("go-loggly-search: %q", res.Status)
	}

	body, err := ioutil.ReadAll(res.Body)

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

// Query Create a new search query using the fluent api.
func (c *Client) Query(str string) *Query {
	return newQuery(c, str)
}

// Return the encoded query-string.
func (q *Query) String() string {
	qs := url.Values{}
	qs.Set("q", q.query)
	qs.Set("size", strconv.Itoa(q.size))
	qs.Set("from", q.from)
	qs.Set("until", q.until)
	qs.Set("order", q.order)
	return qs.Encode()
}

// Size Set response size.
func (q *Query) Size(n int) *Query {
	q.size = n
	return q
}

// From Set from time.
func (q *Query) From(str string) *Query {
	q.from = str
	return q
}

// MaxPage sets the max page
func (q *Query) MaxPage(maxPages int) *Query {
	q.maxPages = maxPages
	return q
}

// Until Set until time.
func (q *Query) Until(str string) *Query {
	q.until = str
	return q
}

// To Set until time.
func (q *Query) To(str string) *Query {
	q.until = str
	return q
}

// Fetch Search response with total events, page number
// and the events array.
func (q *Query) Fetch() (chan Response, chan error) {
	resChan := make(chan Response)
	errChan := make(chan error)

	page := 0
	go func() {
		defer close(resChan)
		defer close(errChan)
		j, err := q.client.CreateSearch(q.String())

		if err != nil {
			errChan <- err
			return
		}

		for {
			res, err := q.client.Search(j, page)

			if err != nil {
				errChan <- err
				return
			}

			resChan <- *res

			if page+1 == q.maxPages {
				return
			}

			if len(res.Events) < q.size {
				return
			}

			page++
		}

	}()

	return resChan, errChan
}
