# Loggly CLI

Loggly search command-line tool.

## Installation

Download from the [releases](https://github.com/Ajnasz/go-loggly-cli/releases)

Quick install via go-get:

```
$ go get github.com/segmentio/go-loggly-cli
$ go-loggly-cli -version
```

## Usage

```

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
    -concurrency <count> number of concurrent page fetchers [3]
    -version          print version information
```

## Setup

Loggly's search API requires basic auth credentials, so you _must_ pass the
`-acount`, `-token` flags. To make this less annoying
I suggest creating an alias:

```sh
alias logs='loggly -account loggly-account -token "foobarbaz"'
```

This is a great place to stick personal defaults as well. Since flags are
clobbered if defined multiple times you can define whatever defaults you'd like
here, while still changing them via `log`:

```sh
alias logs='loggly -account loggly-account -token "foobarbaz" --size 5'
```

## Usage

logs "one.field: something AND other.field: somethingelse"


## License

 MIT
