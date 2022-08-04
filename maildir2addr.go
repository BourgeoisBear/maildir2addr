package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

func main() {

	var E error

	defer func() {

		if E == nil {
			return
		}

		Flog(
			os.Stderr,
			"1;91m",
			"ERROR",
			E.Error(),
		)
		os.Exit(1)

	}()

	// TODO: additional help on dir

	var sO Opts

	flag.BoolVar(&sO.IncludeSpamMsgs, "s", false, "process spam messages (where X-Spam-Flag == YES)")
	flag.BoolVar(&sO.Verbose, "v", false, "verbose")

	var dbInFile, dbOutFile, szExcludesFile string
	defaultDir := os.ExpandEnv("$HOME/.local/share/" + filepath.Base(os.Args[0]))
	flag.StringVar(&dbInFile, "i", defaultDir+"/addrs.tsv", "address database input file\n")
	flag.StringVar(&dbOutFile, "o", defaultDir+"/addrs.tsv", "address database output file\n")
	flag.StringVar(&szExcludesFile, "e", defaultDir+"/excludes.regexp", "address exclusion regex file [one per line]\n")

	flag.Parse()

	// LOAD ADDRS
	if len(dbInFile) > 0 {

		sO.LogVerbose(
			"1;93m",
			"LOADING ADDRS",
			dbInFile,
		)

		if E2 := sO.AddrsReadFromFile(dbInFile); E2 != nil {
			E = errors.Wrap(E2, "load addrs "+dbInFile)
			return
		}
	}

	// EXCLUDE RULES INIT
	if len(szExcludesFile) > 0 {

		// CREATE DEFAULT EXCLUDES IF FILE SPECIFIED, BUT NOT FOUND
		if !FileExists(szExcludesFile) {

			sO.LogVerbose(
				"1;93m",
				"INITIALIZING EXCLUDE RULES",
				szExcludesFile,
			)

			sRgx := []string{
				`^(customer|message|orders|webdesign|receipts|sales|service|support|submissions)`,
				`subscribe`,
				`daemon`,
				`[[:xdigit:]]{7,}`,
				`not?[-_.]?reply`,
				`.{50,}`,
			}

			exclDir := filepath.Dir(szExcludesFile)
			if E = os.MkdirAll(exclDir, 0770); E != nil {
				return
			}

			if E = os.WriteFile(szExcludesFile, []byte(strings.Join(sRgx, "\n")), 0660); E != nil {
				return
			}
		}

		// LOAD EXCLUDE RULES
		sO.LogVerbose(
			"1;93m",
			"LOADING EXCLUDE RULES",
			szExcludesFile,
		)

		if E = sO.ExcludesReadFromFile(szExcludesFile); E != nil {
			return
		}
	}

	// WALKDIR FUNC: SCANS ADDRS FROM SELECTED FILES INTO DB
	fnWalk := func(path string, de fs.DirEntry, err error) error {

		if err != nil {
			return err
		}

		// SKIP DIRS
		if de.IsDir() {
			return nil
		}

		// SKIP DOTFILES
		bname := filepath.Base(path)
		if strings.HasPrefix(bname, ".") {
			return nil
		}

		// ABSOLUTE PATH
		absPath, eMsg := filepath.Abs(path)
		if eMsg != nil {
			Flog(
				os.Stderr,
				"1;91m",
				"MSGERR",
				eMsg.Error(),
				path,
			)
		}

		// SCAN ADDRS
		sO.nScannedMsgs += 1
		if eMsg := sO.ScanMsgsForAddrs(absPath); eMsg != nil {

			Flog(
				os.Stderr,
				"1;91m",
				"MSGERR",
				eMsg.Error(),
				absPath,
			)
		}

		return nil
	}

	// PROCESS FILES IN SPECIFIED MAILDIR(S)
	sArgs := flag.Args()
	if len(sArgs) > 0 {

		for _, mailDir := range sArgs {

			if E = filepath.WalkDir(mailDir, fnWalk); E != nil {
				return
			}
		}

		sO.LogVerbose(
			"1;92m",
			"SCAN COMPLETE",
			fmt.Sprintf(
				"SCANNED %d ADDRS IN %d MSGS; %d NEW ADDRS FOUND (before exclusions)",
				sO.nScannedAddrs,
				sO.nScannedMsgs,
				sO.nNewAddrs,
			),
		)
	}

	sO.AddrsPurgeExcluded()

	// WRITE DB
	sO.LogVerbose(
		"1;93m",
		"SAVING ADDRS",
		dbOutFile,
	)

	if E2 := sO.AddrsWriteToFile(dbOutFile); E2 != nil {
		E = errors.Wrap(E2, "save addrs "+dbOutFile)
		return
	}

}
