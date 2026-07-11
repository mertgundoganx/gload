package faker

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	mrand "math/rand"
	"strings"
	"time"
)

// Generate replaces {{gen.XXX}} placeholders with random values.
func Generate(s string) string {
	// Fast path: no placeholders
	if !strings.Contains(s, "{{gen.") {
		return s
	}

	replacements := map[string]func() string{
		"{{gen.uuid}}":       genUUID,
		"{{gen.timestamp}}":  func() string { return time.Now().UTC().Format(time.RFC3339) },
		"{{gen.unix}}":       func() string { return fmt.Sprintf("%d", time.Now().Unix()) },
		"{{gen.unix_ms}}":    func() string { return fmt.Sprintf("%d", time.Now().UnixMilli()) },
		"{{gen.date}}":       func() string { return time.Now().UTC().Format("2006-01-02") },
		"{{gen.email}}":      genEmail,
		"{{gen.name}}":       genName,
		"{{gen.first_name}}": genFirstName,
		"{{gen.last_name}}":  genLastName,
		"{{gen.phone}}":      genPhone,
		"{{gen.int}}":        func() string { return fmt.Sprintf("%d", mrand.Intn(10000)) },
		"{{gen.int100}}":     func() string { return fmt.Sprintf("%d", mrand.Intn(100)) },
		"{{gen.int1000}}":    func() string { return fmt.Sprintf("%d", mrand.Intn(1000)) },
		"{{gen.float}}":      func() string { return fmt.Sprintf("%.2f", mrand.Float64()*1000) },
		"{{gen.bool}}": func() string {
			if mrand.Intn(2) == 0 {
				return "true"
			}
			return "false"
		},
		"{{gen.hex16}}":     func() string { return randomHex(16) },
		"{{gen.hex32}}":     func() string { return randomHex(32) },
		"{{gen.alpha8}}":    func() string { return randomAlpha(8) },
		"{{gen.alpha16}}":   func() string { return randomAlpha(16) },
		"{{gen.alnum12}}":   func() string { return randomAlnum(12) },
		"{{gen.ip}}":        genIP,
		"{{gen.useragent}}": genUserAgent,
		"{{gen.paragraph}}": genParagraph,
		"{{gen.word}}":      genWord,
		"{{gen.color}}":     genColor,
		"{{gen.country}}":   genCountry,
		"{{gen.city}}":      genCity,
	}

	result := s
	for placeholder, genFn := range replacements {
		for strings.Contains(result, placeholder) {
			result = strings.Replace(result, placeholder, genFn(), 1)
		}
	}
	return result
}

func genUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func randomHex(n int) string {
	b := make([]byte, n/2)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

const alpha = "abcdefghijklmnopqrstuvwxyz"
const alnum = "abcdefghijklmnopqrstuvwxyz0123456789"

func randomAlpha(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alpha[mrand.Intn(len(alpha))]
	}
	return string(b)
}

func randomAlnum(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alnum[mrand.Intn(len(alnum))]
	}
	return string(b)
}

var firstNames = []string{"James", "Mary", "John", "Patricia", "Robert", "Jennifer", "Michael", "Linda", "David", "Elizabeth", "William", "Barbara", "Richard", "Susan", "Joseph", "Jessica", "Thomas", "Sarah", "Charles", "Karen"}
var lastNames = []string{"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller", "Davis", "Rodriguez", "Martinez", "Hernandez", "Lopez", "Gonzalez", "Wilson", "Anderson", "Thomas", "Taylor", "Moore", "Jackson", "Martin"}
var domains = []string{"example.com", "test.io", "demo.org", "mail.net", "inbox.dev"}
var cities = []string{"New York", "London", "Tokyo", "Paris", "Berlin", "Istanbul", "Dubai", "Singapore", "Sydney", "Toronto"}
var countries = []string{"US", "UK", "JP", "FR", "DE", "TR", "AE", "SG", "AU", "CA"}
var colors = []string{"red", "blue", "green", "yellow", "purple", "orange", "cyan", "magenta", "white", "black"}
var words = []string{"lorem", "ipsum", "dolor", "sit", "amet", "consectetur", "adipiscing", "elit", "sed", "do", "eiusmod", "tempor"}
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/120.0.0.0",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120.0.0.0",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15",
}

func genFirstName() string { return firstNames[mrand.Intn(len(firstNames))] }
func genLastName() string  { return lastNames[mrand.Intn(len(lastNames))] }
func genName() string      { return genFirstName() + " " + genLastName() }
func genEmail() string {
	return strings.ToLower(randomAlpha(8)) + "@" + domains[mrand.Intn(len(domains))]
}
func genPhone() string {
	return fmt.Sprintf("+1%d%07d", mrand.Intn(9)+1, mrand.Intn(10000000))
}
func genIP() string {
	return fmt.Sprintf("%d.%d.%d.%d", mrand.Intn(256), mrand.Intn(256), mrand.Intn(256), mrand.Intn(256))
}
func genUserAgent() string { return userAgents[mrand.Intn(len(userAgents))] }
func genWord() string      { return words[mrand.Intn(len(words))] }
func genCity() string      { return cities[mrand.Intn(len(cities))] }
func genCountry() string   { return countries[mrand.Intn(len(countries))] }
func genColor() string     { return colors[mrand.Intn(len(colors))] }
func genParagraph() string {
	n := mrand.Intn(5) + 3
	w := make([]string, n)
	for i := range w {
		w[i] = words[mrand.Intn(len(words))]
	}
	return strings.Join(w, " ") + "."
}

