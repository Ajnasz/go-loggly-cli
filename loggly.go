package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Ajnasz/go-loggly-cli/search"
)

var version string

const usage = `
  Usage: loggly [options] [query...]

  Commands:
    interactive   Launch interactive query builder (alias: i)

  Options:

    -account <name>   account name
    -token <word>     user token
    -size <count>     response event count [100]
    -from <time>      starting time [-24h]
    -to <time>        ending time [now]
    -count            print total event count
    -all              print the entire loggly event instead of just the message
    -maxPages <count> maximum number of pages to query [3]
    -concurrency <count> number of concurrent page fetchers [3]. If loggly returns with http error consider reducing this value.
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
var concurrency = flags.Int("concurrency", 3, "")
var versionQuery = flags.Bool("version", false, "")
var tui = flags.Bool("tui", false, "")
var account = flags.String("account", "", "")
var maxPages = flags.Int64("maxPages", 3, "")
var token = flags.String("token", "", "")
var size = flags.Int("size", 100, "")
var from = flags.String("from", "-24h", "")
var to = flags.String("to", "now", "")
var allMsg = flags.Bool("all", false, "")

// Print usage and exit.
func printUsage() {
	fmt.Print(usage)
	os.Exit(0)
}

// Assert with msg.
func assert(ok bool, msg string) {
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: %s", msg)
		os.Exit(1)
	}
}

func check(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err)
		os.Exit(1)
	}
}

func printJSON(events []any) error {
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}

		fmt.Println(string(data))
	}

	return nil
}

func printLogMSG(events []any) error {
	var ret []any

	for i, event := range events {
		msg := event.(map[string]any)["logmsg"].(string)
		m := make(map[string]any)
		if err := json.Unmarshal([]byte(msg), &m); err != nil {
			return fmt.Errorf("Error at event %d: %w", i+1, err)
		}

		ret = append(ret, m)
	}

	return printJSON(ret)
}

func execCount(ctx context.Context, query string, from string, to string) {
	c := search.New(*account, *token)
	q := search.NewQuery(query).Size(1).From(from).To(to)
	res, err := c.Fetch(ctx, *q)
	for {
		select {
		case <-ctx.Done():
			check(ctx.Err())
			return
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
		return
	}

	if err := printLogMSG(res.Events); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid JSON in the 'logmsg' field. Consider to filter the messages, or use the -all flag and parse the message yourself.\n\n%s", err.Error())
	}
}

func sendQuery(
	ctx context.Context,
	query string,
	size int,
	from string,
	to string,
	maxPages int64,
	concurrency int,
) {
	c := search.New(*account, *token).SetConcurrency(concurrency)
	q := search.NewQuery(query).Size(size).From(from).To(to).MaxPage(maxPages)
	res, err := c.Fetch(ctx, *q)

	for {
		select {
		case <-ctx.Done():
			check(ctx.Err())
			return
		case r := <-res:
			printRes(r)
		case e := <-err:
			check(e)
			return
		}
	}

}

func warnInvalidFlagPlacement(args []string) {
	currentFlags := make(map[string]bool)
	flags.VisitAll(func(f *flag.Flag) {
		currentFlags["-"+f.Name] = true
	})

	var invalidFlags []string
	for _, arg := range args {
		if currentFlags[arg] {
			invalidFlags = append(invalidFlags, arg)
		}
	}

	if len(invalidFlags) > 0 {
		fmt.Fprintf(os.Stderr, "Warning: Possible invalid flag placement. Flags must be specified before the query. Ignoring flags: %s\n", strings.Join(invalidFlags, ", "))
	}
}

func warnHighConcurrency(concurrency int) {
	if concurrency > 3 {
		fmt.Fprintf(os.Stderr, "Warning: High concurrency (%d) may lead to rate limiting or temporary blocking by Loggly. If loggly returns with error, consider reducing the concurrency level.\n", concurrency)
	}
}

func contextWithInterrupt(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-sigChan:
			cancel()
		}
	}()
	return ctx, cancel
}

func main() {
	flags.Usage = printUsage
	flags.Parse(os.Args[1:])

	if *versionQuery {
		fmt.Println(version)
		return
	}

	args := flags.Args()
	warnInvalidFlagPlacement(args)
	warnHighConcurrency(*concurrency)
	query := strings.Join(args, " ")
	ctx, cancel := contextWithInterrupt(context.Background())
	defer cancel()

	assert(*account != "", "-account required")
	assert(*token != "", "-token required")

	if *tui {
		runInteractive(ctx, *account, query, *token, *from, *to)
		return
	}

	if *count {
		execCount(ctx, query, *from, *to)
		return
	}

	sendQuery(ctx, query, *size, *from, *to, *maxPages, *concurrency)
}
