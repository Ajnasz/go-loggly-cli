package search

import (
	"net/url"
	"strconv"
)

// Query builder struct
type Query struct {
	query    string
	from     string
	until    string
	order    string
	size     int
	maxPages int64
}

// Create a new query
func NewQuery(str string) *Query {
	return &Query{
		query: str,
		from:  "-24h",
		until: "now",
		order: "desc",
		size:  100,
	}
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
func (q *Query) MaxPage(maxPages int64) *Query {
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
