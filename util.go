package main

import (
	"bufio"
	"fmt"
	"io"
	"mime"
	"net/mail"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
)

type Opts struct {
	Recurse         bool
	IncludeSpamMsgs bool
	Verbose         bool

	nNewAddrs     int
	nScannedAddrs int
	nScannedMsgs  int

	byAddr map[string]*mail.Address
	rxExcl []*regexp.Regexp
}

var pRxYes *regexp.Regexp
var pRxQuote *regexp.Regexp
var bIsTty bool

func init() {

	pRxYes = regexp.MustCompile(`(?i)y(?:es)?`)
	pRxQuote = regexp.MustCompile(`^'(.*)'$`)
	bIsTty = isatty.IsTerminal(os.Stdout.Fd())
}

/* -------------------------- UTILS -------------------------- */

func norml(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func FileExists(path string) bool {

	_, eDir := os.Stat(path)
	return !os.IsNotExist(eDir)
}

func Flog(iWri io.Writer, esc, szTitle string, sParams ...string) (int, error) {

	parts := []string{"", szTitle, "", "\t"}

	if bIsTty && (len(esc) > 0) {
		parts[0] = "\x1b[" + esc
		parts[2] = "\x1b[0m"
	}

	return fmt.Fprint(
		iWri,
		strings.Join(parts, ""),
		strings.Join(sParams, "\t"),
		"\n",
	)
}

func (sO *Opts) LogVerbose(esc, szTitle string, sParams ...string) (int, error) {

	if !sO.Verbose {
		return 0, nil
	}

	return Flog(
		os.Stdout,
		esc,
		szTitle,
		sParams...,
	)
}

/* -------------------------- EXCLUDES DB -------------------------- */

func (sO *Opts) ExcludesReadFromFile(path string) error {

	bsExcl, E := os.ReadFile(path)
	if os.IsNotExist(E) {
		return nil
	}
	if E != nil {
		return E
	}

	sPat := strings.Split(string(bsExcl), "\n")
	for _, pat := range sPat {

		pat = strings.TrimSpace(pat)
		if len(pat) == 0 {
			continue
		}

		rx, e := regexp.Compile(strings.ToLower(pat))
		if e != nil {
			return errors.Wrap(e, fmt.Sprintf("exclusion pattern (%s)", pat))
		} else {
			sO.rxExcl = append(sO.rxExcl, rx)
		}
	}

	return nil
}

func (sO *Opts) AddrsPurgeExcluded() {

	if (sO.byAddr == nil) || (len(sO.rxExcl) == 0) {
		return
	}

	for addr := range sO.byAddr {

		for _, pRx := range sO.rxExcl {

			if pRx.MatchString(addr) {

				sO.LogVerbose(
					"1;95m",
					"EXCLUDED",
					"/"+pRx.String()+"/",
					addr,
				)

				delete(sO.byAddr, addr)
				break
			}
		}
	}

	return
}

/* -------------------------- ADDRESSES DB -------------------------- */

func (sO *Opts) AddrsReadFromFile(fname string) error {

	if len(fname) == 0 {
		return nil
	}

	if sO.byAddr == nil {
		sO.byAddr = make(map[string]*mail.Address)
	}

	pF, E := os.Open(fname)
	if E != nil {

		if os.IsNotExist(E) {
			return nil
		}
		return E
	}

	ER := sO.AddrsRead(pF)
	EC := pF.Close()
	if ER != nil {
		return ER
	}

	return EC
}

func (sO *Opts) AddrsRead(iRdr io.Reader) error {

	// READ MIME HEADERS
	pRdr := bufio.NewReaderSize(iRdr, 64*1024)

	for {

		line, E := pRdr.ReadString('\n')

		// ADD TO DB
		if len(line) > 0 {

			sP := strings.Split(line, "\t")
			if len(sP) > 0 {

				pA := new(mail.Address)
				pA.Address = norml(sP[0])

				if len(sP) > 1 {
					pA.Name = norml(sP[1])
				}

				sO.byAddr[pA.Address] = pA
			}
		}

		if E == io.EOF {
			break
		}

		if E != nil {
			return E
		}
	}

	return nil
}

// Writes address database in *Opts to an io.Writer
func (sO *Opts) AddrsWrite(iWri io.Writer) error {

	if len(sO.byAddr) == 0 {
		return nil
	}

	pWri := bufio.NewWriterSize(iWri, 64*1024)

	// WRITE ADDRESSES
	for _, pA := range sO.byAddr {

		pWri.WriteString(pA.Address)

		if len(pA.Name) > 0 {
			pWri.WriteString("\t")
			pWri.WriteString(pA.Name)
		}

		pWri.WriteString("\n")
	}

	return pWri.Flush()
}

// Moves address database to file located at `fname`
// Contents are fully replaced
func (sO *Opts) AddrsWriteToFile(fname string) error {

	if (len(fname) == 0) || (len(sO.byAddr) == 0) {
		return nil
	}

	tgtDir := filepath.Dir(fname)
	if E := os.MkdirAll(tgtDir, 0770); E != nil {
		return E
	}

	pF, E := os.OpenFile(fname, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0660)
	if E != nil {
		return E
	}

	EF := sO.AddrsWrite(pF)
	EC := pF.Close()

	if EF != nil {
		return EF
	}

	return EC
}

/* -------------------------- MAILDIR SCANNER -------------------------- */

func (sO *Opts) ScanMsgsForAddrs(fname string) (E error) {

	sO.LogVerbose(
		"1;93m",
		"MSG",
		fname,
	)

	pF, E := os.Open(fname)
	if E != nil {
		return
	}
	defer pF.Close()

	// READ MIME HEADERS
	pRdr := bufio.NewReaderSize(pF, 64*1024)
	pTp := textproto.NewReader(pRdr)
	MH, E := pTp.ReadMIMEHeader()
	if E != nil {
		return
	}

	// SKIP MESSAGES MARKED AS SPAM
	if !sO.IncludeSpamMsgs {

		if vsf, ok := MH["X-Spam-Flag"]; ok {

			for ix := range vsf {

				if !pRxYes.MatchString(vsf[ix]) {
					continue
				}

				sO.LogVerbose(
					"1;93m",
					"SKIPPING",
					"Reason: X-Spam-Flag=YES",
				)

				return
			}
		}
	}

	if sO.byAddr == nil {
		sO.byAddr = make(map[string]*mail.Address)
	}

	var mimeDec mime.WordDecoder
	sKeys := []string{"To", "From", "Cc", "Bcc", "Reply-To"}
	for _, key := range sKeys {

		sVals, bOK := MH[key]
		if !bOK || (len(sVals) == 0) {
			continue
		}

		for _, hdr7bit := range sVals {

			// SKIP EMPTY
			hdr7bit = strings.TrimSpace(hdr7bit)
			if len(hdr7bit) == 0 {
				continue
			}

			// CONVERT ASCII HEADERS TO UTF-8
			var szHdr string
			szHdr, E = mimeDec.DecodeHeader(hdr7bit)
			if E != nil {
				E = errors.Wrapf(E, "[%s] %s", key, hdr7bit)
				return
			}

			// PARSE EMAIL ADDRS
			var sAddrs []*mail.Address
			sAddrs, E = mail.ParseAddressList(szHdr)
			if E != nil {
				E = errors.Wrapf(E, "[%s] %s", key, szHdr)
				return
			}

			// ADD TO DB
			for _, addr := range sAddrs {

				sO.nScannedAddrs += 1

				addr.Address = norml(addr.Address)
				addr.Name = strings.TrimSpace(addr.Name)

				// UNQUOTE NAME
				if len(addr.Name) > 0 {

					sMtch := pRxQuote.FindStringSubmatch(addr.Name)
					if len(sMtch) > 1 {
						addr.Name = sMtch[1]
					}
				}

				sO.LogVerbose(
					"1;36m",
					"\t"+key,
					addr.Address,
					addr.Name,
				)

				pPrev := sO.byAddr[addr.Address]
				if pPrev == nil {
					sO.byAddr[addr.Address] = addr
					sO.nNewAddrs += 1
				} else if (len(pPrev.Name) == 0) && (len(addr.Name) > 0) {
					pPrev.Name = addr.Name
				}
			}
		}
	}

	return
}
