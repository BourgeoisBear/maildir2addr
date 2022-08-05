# maildir2addr

Scans maildir folders for e-mail addresses, outputs unique results in aerc-style `address-book-cmd` format:

```
[E-MAIL1]\t[NAME1]\n
[E-MAIL2]\t[NAME2]\n
...
```
   
The `[E-MAIL]` column is forced to lowercase in the output.

Default settings store data in the `$HOME/.local/share/maildir2addr` directory.

A file of exclude regexes can be specified with the `-e` option.
One regexp per line, each applied to the `[E-MAIL]` part only.
If this file is specified but does not exist, it will be created and populated with sane defaults.

## Installation

1. Install the latest Go compiler from https://golang.org/dl/
2. Install the program:

```sh
go install github.com/BourgeoisBear/maildir2addr@latest
```

## Usage

```sh

maildir2addr [OPTION...] [MAILDIR_PATH...]

OPTIONS

  -e string
        address exclusion regex file [one per line]
         (default "$HOME/.local/share/maildir2addr/excludes.regexp")
  -i string
        address database input file
         (default "$HOME/.local/share/maildir2addr/addrs.tsv")
  -o string
        address database output file
         (default "$HOME/.local/share/maildir2addr/addrs.tsv")
  -s    process spam messages (where X-Spam-Flag == YES)
  -v    verbose, log details to STDERR

```

To create an address book in the default location:

```sh
maildir2addr -v /path/to/my/maildir
```

After issuing this command, a new tab-separated-values address file will appear at `$HOME/.local/share/maildir2addr/addrs.tsv`.

This file is suitable for use with the address autocomplete feature of several TUI mail clients.

## `aerc` Integration

NOTE: Replace `$HOME` with the full path to your home directory.  `aerc.conf` does not expand environment vars for `address-book-cmd`.

```ini
address-book-cmd=grep -F -i -- "%s" "$HOME/.local/share/maildir2addr/addrs.tsv"
```
