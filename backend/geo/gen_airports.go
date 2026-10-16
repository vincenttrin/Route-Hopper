//go:build ignore

// gen_airports builds data/airports.csv, the offline airport table behind
// hostname location hints, from the OurAirports data set (public domain,
// https://ourairports.com/data/). Run it from this directory:
//
//	go run gen_airports.go [path-or-url-of-airports.csv]
//
// The table keeps airports with scheduled service, an IATA code and a size of
// medium or large: small airfields and heliports have codes nobody puts in a
// router name, and every extra code is another chance to misread a label.
package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
)

const defaultSource = "https://davidmegginson.github.io/ourairports-data/airports.csv"

// cityOverrides names the metro area for airports whose OurAirports
// municipality is a suburb or district that nobody would recognise on a map.
var cityOverrides = map[string]string{
	"IAD": "Washington, D.C.",
	"NRT": "Tokyo",
	"HND": "Tokyo",
	"ORY": "Paris",
	"CDG": "Paris",
}

var parens = regexp.MustCompile(`\s*\([^)]*\)`)

func main() {
	src := defaultSource
	if len(os.Args) > 1 {
		src = os.Args[1]
	}
	in, err := open(src)
	if err != nil {
		log.Fatal(err)
	}
	defer in.Close()

	r := csv.NewReader(in)
	head, err := r.Read()
	if err != nil {
		log.Fatal(err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[h] = i
	}
	for _, need := range []string{"type", "latitude_deg", "longitude_deg", "iso_country", "municipality", "scheduled_service", "iata_code"} {
		if _, ok := col[need]; !ok {
			log.Fatalf("source has no %q column", need)
		}
	}

	type row struct{ iata, city, country, lat, lng string }
	byCode := map[string]row{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		get := func(k string) string { return strings.TrimSpace(rec[col[k]]) }
		if get("scheduled_service") != "yes" || get("type") != "large_airport" && get("type") != "medium_airport" {
			continue
		}
		code := strings.ToUpper(get("iata_code"))
		if len(code) != 3 {
			continue
		}
		city := strings.TrimSpace(parens.ReplaceAllString(get("municipality"), ""))
		if o, ok := cityOverrides[code]; ok {
			city = o
		}
		if city == "" || get("iso_country") == "" {
			continue
		}
		if _, dup := byCode[code]; dup {
			continue
		}
		byCode[code] = row{code, city, get("iso_country"), get("latitude_deg"), get("longitude_deg")}
	}

	codes := make([]string, 0, len(byCode))
	for c := range byCode {
		codes = append(codes, c)
	}
	sort.Strings(codes)

	out, err := os.Create("data/airports.csv")
	if err != nil {
		log.Fatal(err)
	}
	w := csv.NewWriter(out)
	w.Write([]string{"iata", "city", "country", "lat", "lng"})
	for _, c := range codes {
		x := byCode[c]
		w.Write([]string{x.iata, x.city, x.country, x.lat, x.lng})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		log.Fatal(err)
	}
	if err := out.Close(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %d airports\n", len(codes))
}

func open(src string) (io.ReadCloser, error) {
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		resp, err := http.Get(src)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("GET %s: %s", src, resp.Status)
		}
		return resp.Body, nil
	}
	return os.Open(src)
}
