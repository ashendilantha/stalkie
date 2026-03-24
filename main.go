package main

import (
	"fmt"

	"github.com/ashendilantha/stalkie/loader"
)

func main() {
	sites, err := loader.Load("sites.json")
	if err != nil {
		panic(err)
	}
	for _, s := range sites {
		fmt.Println(loader.BuildURL(s.URL, "ashendilantha"))
	}
}
