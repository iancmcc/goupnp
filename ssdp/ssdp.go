package ssdp

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/iancmcc/goupnp/httpu"
)

const (
	ssdpDiscover   = `"ssdp:discover"`
	ntsAlive       = `ssdp:alive`
	ntsByebye      = `ssdp:byebye`
	ntsUpdate      = `ssdp:update`
	ssdpUDP4Addr   = "239.255.255.250:1900"
	ssdpSearchPort = 1900
	methodSearch   = "M-SEARCH"
	methodNotify   = "NOTIFY"
)

// SSDPRawSearch performs a fairly raw SSDP search request, and returns the
// unique response(s) that it receives. Each response has the requested
// searchTarget, a USN, and a valid location. maxWaitSeconds states how long to
// wait for responses in seconds, and must be a minimum of 1 (the
// implementation waits an additional 100ms for responses to arrive), 2 is a
// reasonable value for this. numSends is the number of requests to send - 3 is
// a reasonable value for this.
func SSDPRawSearch(httpu *httpu.HTTPUClient, searchTarget string, maxWaitSeconds int, numSends int) (<-chan *http.Response, error) {
	if maxWaitSeconds < 1 {
		return nil, errors.New("ssdp: maxWaitSeconds must be >= 1")
	}
	responses := make(chan *http.Response)
	seenUsns := make(map[string]bool)
	req := http.Request{
		Method: methodSearch,
		// TODO: Support both IPv4 and IPv6.
		Host: ssdpUDP4Addr,
		URL:  &url.URL{Opaque: "*"},
		Header: http.Header{
			// Putting headers in here avoids them being title-cased.
			// (The UPnP discovery protocol uses case-sensitive headers)
			"HOST": []string{ssdpUDP4Addr},
			"MX":   []string{strconv.FormatInt(int64(maxWaitSeconds), 10)},
			"MAN":  []string{ssdpDiscover},
			"ST":   []string{searchTarget},
		},
	}
	allResponses, err := httpu.Do(&req, time.Duration(maxWaitSeconds)*time.Second+100*time.Millisecond, numSends)
	if err != nil {
		return nil, err
	}
	go func() {
		defer func() {
			close(responses)
		}()
		for response := range allResponses {
			if response.StatusCode != 200 {
				log.Printf("ssdp: got response status code %q in search response", response.Status)
				continue
			}
			if st := response.Header.Get("ST"); st != searchTarget {
				log.Printf("ssdp: got unexpected search target result %q", st)
				continue
			}
			location, err := response.Location()
			if err != nil {
				log.Printf("ssdp: no usable location in search response (discarding): %v", err)
				continue
			}
			usn := response.Header.Get("USN")
			if usn == "" {
				log.Printf("ssdp: empty/missing USN in search response (using location instead): %v", err)
				usn = location.String()
			}
			if _, alreadySeen := seenUsns[usn]; !alreadySeen {
				seenUsns[usn] = true
				responses <- response
			}
		}
	}()

	return responses, nil
}
