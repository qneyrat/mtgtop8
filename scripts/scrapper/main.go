package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
)

type DeckElement struct {
	Quantity    int    `json:"quantity"`
	Name        string `json:"name"`
	IsCommander bool   `json:"is_commander"`
}

type Deck struct {
	Commanders []string      `json:"commanders"`
	List       []DeckElement `json:"list"`
}

type Result struct {
	ID     string `json:"id"`
	Link   string `json:"-"`
	Title  string `json:"title"`
	Player string `json:"player"`
	Rank   string `json:"rank"`
	Deck   *Deck  `json:"deck"`
}

type Event struct {
	ID       string    `json:"id"`
	Link     string    `json:"-"`
	Format   string    `json:"format"`
	IsOnline bool      `json:"is_online"`
	Title    string    `json:"title"`
	Location string    `json:"location"`
	Level    int       `json:"level"`
	Date     string    `json:"date"`
	Results  []*Result `json:"results"`
}

type EventList struct {
	ID     string   `json:"id"`
	Events []string `json:"events"`
}

func main() {
	c := colly.NewCollector(
		colly.AllowedDomains("www.mtgtop8.com", "mtgtop8.com"),
	)

	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		RandomDelay: 1 * time.Second,
	})

	eventList := map[string]*Event{}
	c.OnHTML("tr", func(e *colly.HTMLElement) {
		if e.Attr("class") != "hover_tr" {
			return
		}

		event := &Event{}
		e.ForEach("td", func(i int, element *colly.HTMLElement) {
			if element.Attr("class") == "S12" {
				if event.ID != "" {
					event.Date = element.Text
					return
				}
			}

			element.ForEach("a[href]", func(i int, element *colly.HTMLElement) {
				link := element.Attr("href")
				if strings.HasPrefix(link, "event") && event.ID == "" {
					event.Link = link
					u, err := url.Parse(link)
					if err != nil {
						fmt.Println(err)
						return
					}

					eventID := u.Query().Get("e")
					if eventID != "" {
						event.ID = eventID
					}

					format := u.Query().Get("f")
					if format != "" {
						event.Format = format
					}

					event.Title = element.Text
					event.Results = []*Result{}
				}

				if event.ID != "" {
					event.Location = element.Text
				}
			})

			element.ForEach("img[src]", func(i int, element *colly.HTMLElement) {
				if element.Attr("src") == "/graph/star.png" {
					event.Level++
				}
			})
		})

		if event.ID != "" {
			eventList[event.ID] = event
		}
	})

	f := func(e *colly.HTMLElement) {
		if !strings.HasPrefix(e.Request.URL.Path, "/event") {
			return
		}

		eventID := e.Request.URL.Query().Get("e")
		event := eventList[eventID]

		result := &Result{}

		e.ForEach("div.S14", func(i int, element *colly.HTMLElement) {
			if result.Rank == "" {
				result.Rank = element.Text
				return
			}

			if result.Title == "" {
				result.Title = element.Text
			}

			element.ForEach("a[href]", func(i int, element *colly.HTMLElement) {
				u, err := url.Parse(element.Attr("href"))
				if err != nil {
					fmt.Println(err)
					return
				}

				result.ID = u.Query().Get("d")
			})
		})

		e.ForEach("div.G11", func(i int, element *colly.HTMLElement) {
			result.Player = element.Text
		})

		if result.ID != "" {
			event.Results = append(event.Results, result)
		}
	}

	c.OnHTML("div.chosen_tr", f)
	c.OnHTML("div.hover_tr", f)

	err := c.Visit("https://www.mtgtop8.com/format?f=EDH&meta=209&a=")
	if err != nil {
		fmt.Println(err)
		return
	}

	ee := &EventList{
		ID:     "209",
		Events: []string{},
	}

	for _, event := range eventList {
		err := c.Visit("https://www.mtgtop8.com/" + event.Link)
		if err != nil {
			fmt.Println(err)
			return
		}

		err = os.MkdirAll(
			"data/edh/2024/"+
				strings.ReplaceAll(event.Date, "/", "-")+
				"-"+event.ID,
			0755,
		)

		for _, result := range event.Results {
			res, err := http.Get("https://www.mtgtop8.com/mtgo?d=" + result.ID)
			if err != nil {
				fmt.Println(err)
				return
			}

			bb, err := io.ReadAll(res.Body)
			if err != nil {
				fmt.Println(err)
				return
			}

			_ = res.Body.Close()

			d := &Deck{
				Commanders: []string{},
				List:       []DeckElement{},
			}

			sideboardPass := false
			splittedContent := bytes.Split(bb, []byte{13, 10})
			for _, line := range splittedContent {
				strLine := string(line)
				if strLine == "Sideboard" {
					sideboardPass = true
					continue
				}

				splittedLine1, splittedLine2, ok := bytes.Cut(line, []byte{32})
				if ok {
					if sideboardPass {
						d.Commanders = append(d.Commanders, string(splittedLine2))
					}

					q, _ := strconv.Atoi(string(splittedLine1))
					d.List = append(d.List, DeckElement{
						Quantity:    q,
						Name:        string(splittedLine2),
						IsCommander: sideboardPass,
					})
				}
			}

			sideboardPass = false

			result.Deck = d
		}

		bb, err := json.Marshal(event)
		if err != nil {
			fmt.Println(err)
			return
		}

		_ = os.WriteFile("data/edh/2024/"+
			strings.ReplaceAll(event.Date, "/", "-")+
			"-"+event.ID+"/index.json", bb, 0755)

		ee.Events = append(ee.Events, "data/edh/2024/"+
			strings.ReplaceAll(event.Date, "/", "-")+
			"-"+event.ID+"/index.json")

	}

	bb, err := json.Marshal(ee)
	if err != nil {
		fmt.Println(err)
		return
	}

	_ = os.WriteFile("data/edh/2024/"+"/index.json", bb, 0755)
}
