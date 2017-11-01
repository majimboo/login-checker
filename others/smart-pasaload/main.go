package main

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/publicsuffix"
	"mvdan.cc/xurls"
)

func main() {
	argsWithoutProg := os.Args[1:]

	if len(argsWithoutProg) != 2 {
		fmt.Println(fmt.Sprintf("[usage] %s <username> <password>", os.Args[0]))
		return
	}
	username := os.Args[1]
	password := os.Args[2]

	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	client := http.DefaultClient
	client.Jar = jar
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// Wrap around the client's transport to add support for space cookies.
	client.Transport = NewTransport(client.Transport, jar)

	// login
	r := PostForm(client, "https://my.smart.com.ph/loginAuth/Login", url.Values{
		"mySmartID":        {username},
		"Password":         {password},
		"RememberMe":       {"true"},
		"X-Requested-With": {"XMLHttpRequest"},
	})

	location := xurls.Relaxed().FindString(r.Find("script").First().Text())

	// check if login was success
	if !strings.Contains(location, "my.smart.com.ph/Dashboard") {
		fmt.Println("[error] failed to authenticate username and password")
		return
	}

	r = GetRequestDoc(client, location)

	if html, _ := r.Html(); !strings.Contains(html, `<h2>Object moved to <a href="/Dashboard/Home/Overview">here</a>.</h2>`) {
		fmt.Println("[error] failed to access dashboard")
		return
	}

	// get all mobile devices
	r = GetRequestDoc(client, "https://my.smart.com.ph/Dashboard/Home/GetSsoSubscriptions")
	switches := r.Find("li.subSwitch")
	switches.Each(func(i int, s *goquery.Selection) {
		// fmt.Println(s.Attr("onclick"))
		nickLabel := s.Find("span.nickLabel").Text()
		mobNum := s.Find("span.mobNum").Text()
		fmt.Println(i+1, ">", mobNum, nickLabel)
	})

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println(fmt.Sprintf("[smart] please select a subscription to use: [1-%d]", switches.Length()))
	scanner.Scan()
	subNum := scanner.Text()
	if scanner.Err() != nil {
		fmt.Println("[error] something went wrong. please try again.")
		return
	}

	subselect, err := strconv.Atoi(subNum)
	if err != nil {
		fmt.Println(fmt.Sprintf("[error] %s", err.Error()))
		return
	}

	subNode := switches.Get(subselect - 1)
	subURL := xurls.Relaxed().FindString(fmt.Sprintf("%s", subNode.Attr))

	r = GetRequestDoc(client, subURL)

	location = xurls.Relaxed().FindString(r.Find("script").First().Text())

	// check if login was success
	if !strings.Contains(location, "my.smart.com.ph/Dashboard") {
		fmt.Println("[error] failed to change subscription")
		return
	}

	r = GetRequestDoc(client, location)

	if html, _ := r.Html(); !strings.Contains(html, `<h2>Object moved to <a href="/Dashboard/Home/Overview">here</a>.</h2>`) {
		fmt.Println("[error] failed to access dashboard")
		return
	}

	fmt.Println("[smart] mobile number to send load to: [09191234567]")
	scanner.Scan()
	mobileNum := scanner.Text()
	if scanner.Err() != nil {
		fmt.Println("[error] something went wrong. please try again.")
		return
	}

	fmt.Println("[smart] amount: [2-200]")
	scanner.Scan()
	amount := scanner.Text()
	if scanner.Err() != nil {
		fmt.Println("[error] something went wrong. please try again.")
		return
	}

	GetRequestDoc(client, "https://my.smart.com.ph/Dashboard/Management/AddOnsPromos")

	// pasaload
	r = PostForm(client, "https://my.smart.com.ph/Dashboard/Management/RequestPasaload", url.Values{
		"targetMobileNo": {mobileNum},
		"unit":           {fmt.Sprintf("%s.00", amount)},
	})

	fmt.Println("[smart]", r.Text())
	fmt.Println("BYE BYE LOLLIPOP!!!")
}

// PostForm executes a post request
func PostForm(c *http.Client, url string, data url.Values) *goquery.Document {
	resp, err := c.PostForm(url, data)
	if err != nil {
		fmt.Println(fmt.Sprintf("[error] %s", err.Error()))
		return nil
	}
	doc, err := goquery.NewDocumentFromResponse(resp)
	if err != nil {
		fmt.Println(fmt.Sprintf("[error] %s", err.Error()))
		return nil
	}
	resp.Body.Close()
	return doc
}

// GetRequest executes a get request
func GetRequest(c *http.Client, url string) *http.Response {
	resp, err := c.Get(url)
	if err != nil {
		fmt.Println(fmt.Sprintf("[error] %s", err.Error()))
		return nil
	}
	resp.Body.Close()
	return resp
}

// GetRequestDoc executes a get request and returns a document
func GetRequestDoc(c *http.Client, url string) *goquery.Document {
	resp, err := c.Get(url)
	if err != nil {
		fmt.Println(fmt.Sprintf("[error] %s", err.Error()))
		return nil
	}
	doc, err := goquery.NewDocumentFromResponse(resp)
	if err != nil {
		fmt.Println(fmt.Sprintf("[error] %s", err.Error()))
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
