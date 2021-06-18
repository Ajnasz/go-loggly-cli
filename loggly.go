package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	search "github.com/Ajnasz/go-loggly-cli/search"
)

// Version is the version string
var version string

//
// Usage information.
//

const usage = `
  Usage: loggly [options] [query...]

  Options:

    -account <name>   account name
    -token <word>     user token
    -size <count>     response event count [100]
    -from <time>      starting time [-24h]
    -to <time>        ending time [now]
    -count            print total event count
    -all              print the entire loggly event instead of just the message
    -maxPages <count> maximum number of pages to query [3]
    -version          print version information

  Operators:

    "foo bar" AND baz
    foo AND bar NOT baz
    +foo +bar -baz
    foo OR bar
    json.responseTime[50 TO 100]
    json.duration[1000 TO *]

  Fields:

    json.level:error
    json.type:"upload failed"
    json.hostname:"api-*"

  Grouping:

    foo AND (bar OR baz)

  Regexps:

    /Black(Berry)?/

`

// Command options.
var flags = flag.NewFlagSet("loggly", flag.ExitOnError)
var count = flags.Bool("count", false, "")
var versionQuery = flags.Bool("version", false, "")
var account = flags.String("account", "", "")
var maxPages = flags.Int("maxPages", 3, "")
var token = flags.String("token", "", "")
var size = flags.Int("size", 100, "")
var from = flags.String("from", "-24h", "")
var to = flags.String("to", "now", "")
var allMsg = flags.Bool("all", false, "")

// Print usage and exit.
func printUsage() {
	fmt.Println(usage)
	os.Exit(0)
}

// Assert with msg.
func assert(ok bool, msg string) {
	if !ok {
		fmt.Fprintf(os.Stderr, "\n  Error: %s\n\n", msg)
		os.Exit(1)
	}
}

func check(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n  Error: %s\n\n", err)
		os.Exit(1)
	}
}

func printJSON(events []interface{}) error {
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}

		fmt.Println(string(data))
	}

	return nil
}

func printLogMSG(events []interface{}) error {
	var ret []interface{}

	for i, event := range events {
		msg := event.(map[string]interface{})["logmsg"].(string)
		m := make(map[string]interface{})
		if err := json.Unmarshal([]byte(msg), &m); err != nil {
			return fmt.Errorf("Error at event %d: %w", i+1, err)
		}

		ret = append(ret, m)
	}

	return printJSON(ret)
}

func execCount(query string, from string, to string) {
	c := search.New(*account, *token)
	res, err := c.Query(query).Size(1).From(from).To(to).Fetch()
	for {
		select {
		case r := <-res:
			fmt.Println(r.Total)
			return
		case e := <-err:
			check(e)
			return
		}
	}
}

func printRes(res search.Response) {
	if *allMsg {
		check(printJSON(res.Events))
	} else {
		if err := printLogMSG(res.Events); err != nil {
			fmt.Fprintf(os.Stderr, "Invalid JSON in the 'logmsg' field. Consider to filter the messages, or use the -all flag and parse the message yourself.\n\n%s", err.Error())
		}
	}
}

func sendQuery(query string, size int, from string, to string, maxPages int) {
	doneChan := make(chan error)

	c := search.New(*account, *token)
	res, err := c.Query(query).Size(size).From(from).To(to).MaxPage(maxPages).Fetch()

	go func() {
		if e := <-err; e != nil {
			doneChan <- e
		}
	}()

	go func() {
		for i := range res {
			printRes(i)
		}
		doneChan <- nil
	}()

	check(<-doneChan)
}

func main() {
	flags.Usage = printUsage
	flags.Parse(os.Args[1:])

	if *versionQuery {
		fmt.Println(version)
		os.Exit(0)
	}

	assert(*account != "", "-account required")
	assert(*token != "", "-token required")

	args := flags.Args()
	query := strings.Join(args, " ")

	if *count {
		execCount(query, *from, *to)
		return
	}

	sendQuery(query, *size, *from, *to, *maxPages)
}
