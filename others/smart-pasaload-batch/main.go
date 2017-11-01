package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/publicsuffix"
	"mvdan.cc/xurls"
)

type user struct {
	Username string
	Password string
}

func worker(id int, jobs <-chan *user, results chan<- int, targetMobileNo, pasaAmount string) {
	for u := range jobs {
		defer func() { results <- id }()

		jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
		client := &http.Client{}
		client.Jar = jar
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		client.Transport = NewTransport(client.Transport, jar)

		// login
		r := PostForm(client, "https://my.smart.com.ph/loginAuth/Login", url.Values{
			"mySmartID":        {u.Username},
			"Password":         {u.Password},
			"RememberMe":       {"true"},
			"X-Requested-With": {"XMLHttpRequest"},
		})

		location := xurls.Relaxed().FindString(r.Find("script").First().Text())

		if !strings.Contains(location, "my.smart.com.ph/Dashboard") {
			log.Printf("[%s] login failed\n", u.Username)
			return
		}

		r = GetRequestDoc(client, location)

		if html, _ := r.Html(); !strings.Contains(html, `<h2>Object moved to <a href="/Dashboard/Home/Overview">here</a>.</h2>`) {
			log.Printf("[%s] dashboard failed\n", u.Username)
			return
		}

		// get all mobile devices
		r = GetRequestDoc(client, "https://my.smart.com.ph/Dashboard/Home/GetSsoSubscriptions")
		switches := r.Find("li.subSwitch")
		switches.Each(func(i int, s *goquery.Selection) {
			subURL, ok := s.Attr("onclick")
			mobNum := s.Find("span.mobNum").Text()

			if ok {
				r = GetRequestDoc(client, xurls.Relaxed().FindString(subURL))

				location = xurls.Relaxed().FindString(r.Find("script").First().Text())

				if !strings.Contains(location, "my.smart.com.ph/Dashboard") {
					log.Printf("[%s] change subscription failed\n", u.Username)
					return
				}

				r = GetRequestDoc(client, location)

				if html, _ := r.Html(); !strings.Contains(html, `<h2>Object moved to <a href="/Dashboard/Home/Overview">here</a>.</h2>`) {
					log.Printf("[%s] subscription dashboard failed\n", u.Username)
					return
				}

				GetRequestDoc(client, "https://my.smart.com.ph/Dashboard/Management/AddOnsPromos")

				// pasaload
				r = PostForm(client, "https://my.smart.com.ph/Dashboard/Management/RequestPasaload", url.Values{
					"targetMobileNo": {targetMobileNo},
					"unit":           {pasaAmount},
				})

				log.Printf("[%s] %s %s\n", u.Username, mobNum, r.Text())
			}
		})

	}
}

func main() {
	numWorkers := flag.Int("w", 10, "number of workers to spawn")
	pasaAmount := flag.Int("a", 200, "amount to pasaload")
	userListPath := flag.String("u", "users.txt", "path to list of username:password")
	targetMobileNo := flag.String("n", "09194445558", "mobile number to send pasaload to")
	flag.Parse()

	userList := loadUserList(*userListPath)

	log.Printf("%d account(s) loaded\n", len(userList))

	jobs := make(chan *user, 100)
	results := make(chan int, 100)

	for w := 1; w <= *numWorkers; w++ {
		go worker(w, jobs, results, *targetMobileNo, fmt.Sprintf("%d.00", *pasaAmount))
	}

	for _, u := range userList {
		jobs <- u
	}
	close(jobs)

	for a := 1; a <= len(userList); a++ {
		<-results
	}
}

func loadUserList(filePath string) []*user {
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		log.Fatal(err)
	}

	var users []*user

	if file, err := os.Open(absFilePath); err == nil {
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			u := strings.SplitN(scanner.Text(), ":", 2)

			users = append(users, &user{
				Username: u[0],
				Password: u[1],
			})
		}

		if err = scanner.Err(); err != nil {
			log.Fatal(err)
		}
	} else {
		log.Fatal(err)
	}

	return users
}

// PostForm executes a post request
func PostForm(c *http.Client, url string, data url.Values) *goquery.Document {
	resp, err := c.PostForm(url, data)
	if err != nil {
		log.Printf("[error] %s\n", err)
		return nil
	}
	doc, err := goquery.NewDocumentFromResponse(resp)
	if err != nil {
		log.Printf("[error] %s\n", err)
		return nil
	}
	resp.Body.Close()
	return doc
}

// GetRequest executes a get request
func GetRequest(c *http.Client, url string) *http.Response {
	resp, err := c.Get(url)
	if err != nil {
		log.Printf("[error] %s\n", err)
		return nil
	}
	resp.Body.Close()
	return resp
}

// GetRequestDoc executes a get request and returns a document
func GetRequestDoc(c *http.Client, url string) *goquery.Document {
	resp, err := c.Get(url)
	if err != nil {
		log.Printf("[error] %s\n", err)
		return nil
	}
	doc, err := goquery.NewDocumentFromResponse(resp)
	if err != nil {
		log.Printf("[error] %s\n", err)
		return nil
	}
	resp.Body.Close()
	return doc
}

// Transport implements a RoundTripper adding space cookies support to an existing RoundTripper.
type Transport struct {
	wrap http.RoundTripper
	jar  http.CookieJar
}

// NewTransport wraps around a Transport and a Cookie Jar to support cookies with space in the name.
// Just like an http.Client if transport is nil, http.DefaultTransport is used.
func NewTransport(transport http.RoundTripper, jar http.CookieJar) *Transport {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &Transport{
		wrap: transport,
		jar:  jar,
	}
}

// RoundTrip executes a single HTTP transaction to add support for space cookies.
// It is safe for concurrent use by multiple goroutines.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.wrap.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	for _, s := range resp.Header[http.CanonicalHeaderKey("Set-Cookie")] {
		cookie := parse(s)
		if cookie != nil && strings.Contains(cookie.Name, "@") && len(cookie.Value) > 0 {
			// Only add the space cookies, normal ones are handled by net/http.
			t.jar.SetCookies(resp.Request.URL, []*http.Cookie{cookie})
		}
	}
	return resp, nil
}

// parse parses a Set-Cookie header into a cookie.
// It supports space in name unlike net/http.
// Returns nil if invalid.
func parse(s string) *http.Cookie {
	var c http.Cookie
	for i, field := range strings.Split(s, ";") {
		if len(field) == 0 {
			continue
		}
		nv := strings.SplitN(field, "=", 2)
		name := strings.TrimSpace(nv[0])
		value := ""
		if len(nv) > 1 {
			value = strings.TrimSpace(nv[1])
		}
		if i == 0 {
			if len(nv) != 2 {
				continue
			}
			c.Name = name
			c.Value = value
			continue
		}
		switch strings.ToLower(name) {
		case "secure":
			c.Secure = true
		case "httponly":
			c.HttpOnly = true
		case "domain":
			c.Domain = value
		case "max-age":
			secs, err := strconv.Atoi(value)
			if err != nil || secs != 0 && value[0] == '0' {
				continue
			}
			if secs <= 0 {
				c.MaxAge = -1
			} else {
				c.MaxAge = secs
			}
		case "expires":
			exptime, err := time.Parse(time.RFC1123, value)
			if err != nil {
				exptime, err = time.Parse("Mon, 02-Jan-2006 15:04:05 MST", value)
				if err != nil {
					c.Expires = time.Time{}
					continue
				}
			}
			c.Expires = exptime.UTC()
		case "path":
			c.Path = value
		}
	}
	if c.Name == "" {
		return nil
	}
	return &c
}
