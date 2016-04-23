package main

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/headzoo/surf"
	"gopkg.in/yaml.v2"
)

type configs struct {
	Sites []struct {
		URL  string
		Form string
		Find string
		User string
		Pass string
	}
}

func main() {
	config := readConfig()

	for _, site := range config.Sites {
		fmt.Println(fmt.Sprintf("check [%s] %s", site.Pass, site.User))

		bow := surf.NewBrowser()
		bow.Open(site.URL)

		fm, err := bow.Form(site.Form)
		if err != nil {
			panic(err)
		}

		fm.Input(site.User, "rdcruz_md@mac.com")
		fm.Input(site.Pass, "ravc6694")
		err = fm.Submit()
		if err != nil {
			panic(err)
		}

		if strings.Contains(bow.Body(), site.Find) {
			fmt.Println("found")
		} else {
			fmt.Println(bow.Body())
		}
	}
}

func readConfig() configs {
	filename, _ := filepath.Abs("config.yml")
	yamlFile, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}

	var config configs
	yaml.Unmarshal(yamlFile, &config)

	return config
}
