package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/andersfylling/snowflake"
	"github.com/nickname32/discordhook"
)

var proxyIteration int
var proxyList []string
var proxy string
var delay int = 3600
var webhookID snowflake.Snowflake
var webhookField string

func main() {

	websiteList := formatWebsiteList("websites.txt")
	formatProxyList("proxies.txt") // makes the proxy list
	proxyArrayToProxyURL()         // converts user:pass@ip:port to http://user:pass@ip:port
	proxyRotation()                // initializes the proxy rotation & first proxy in list
	startGoRoutines(websiteList)
	makeWebhook()

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
	os.Exit(0) // infinite loop until ctrl + c in cmd
}

func monitor(websiteURL string, currentProxy string, taskNumber int) {
	type ImageVariants struct {
		Id          int             `json:"id,omitempty"`
		Created_at  string          `json:"created_at,omitempty"`
		Position    int             `json:"position,omitempty"`
		Updated_at  string          `json:"updated_at,omitempty"`
		Product_id  int             `json:"product_id,omitempty"`
		Variant_ids []ImageVariants `json:"variant_ids,omitempty"`
		Src         string          `json:"src,omitempty"`
		Width       int             `json:"width,omitempty"`
		Height      int             `json:"height,omitempty"`
	}

	type AllImages struct {
		Id          int             `json:"id"`
		Created_at  string          `json:"created_at,omitempty"`
		Position    int             `json:"position,omitempty"`
		Updated_at  string          `json:"updated_at,omitempty"`
		Product_id  int             `json:"product_id,omitempty"`
		Variant_ids []ImageVariants `json:"variant_ids,omitempty"`
		Src         string          `json:"src,omitempty"`
		Width       int             `json:"width,omitempty"`
		Height      int             `json:"height,omitempty"`
	}

	type AllVariants struct {
		Id                int     `json:"id"`
		Title             string  `json:"title"`
		Option1           string  `json:"option1,omitempty"`
		Option2           string  `json:"option2,omitempty"`
		Option3           string  `json:"option3,omitempty"`
		Sku               string  `json:"sku,omitempty"`
		Requires_shipping bool    `json:"requires_shipping,omitempty"`
		Taxable           bool    `json:"taxable,omitempty"`
		Featured_image    string  `json:"featured_image,omitempty"`
		Available         bool    `json:"available,omitempty"`
		Price             float64 `json:"price,omitempty"`
		Grams             int     `json:"grams,omitempty"`
		Compare_at_price  float64 `json:"compare_at_price,omitempty"`
		Position          int     `json:"position,omitempty"`
		Product_id        int     `json:"product_id,omitempty"`
		Created_at        string  `json:"created_at,omitempty"`
		Updated_at        string  `json:"updated_at,omitempty"`
	}

	type IndividualProducts struct {
		Id       int           `json:"id"`
		Title    string        `json:"title"`
		Handle   string        `json:"handle"`
		Variants []AllVariants `json:"variants"`
		Images   []AllImages   `json:"images"`
	}

	type Products struct {
		Products []IndividualProducts `json:"products"`
	}

	var latestProduct string
	var variantsAndSizes []string

monitorLoop:
	for {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			panic(err)
		}
		proxyFromURL := http.ProxyURL(proxyURL)
		transport := &http.Transport{Proxy: proxyFromURL}
		client := &http.Client{Transport: transport}
		if len(proxyList) == 0 {
			client = &http.Client{}
		}
		httpMethod := "GET"

		request, err := http.NewRequest(httpMethod, websiteURL+"/products.json?limit=1", nil)

		if err != nil {
			fmt.Println(err)
			time.Sleep(time.Duration(delay) * time.Millisecond)
			continue monitorLoop
		}

		request.Header.Add("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/89.0.4389.128 Safari/537.36")
		request.Header.Add("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,/;q=0.8,application/signed-exchange;v=b3;q=0.9")
		request.Header.Add("pragma", "no-cache")
		request.Header.Add("cache-control", "no-cache")
		request.Header.Add("upgrade-insecure-requests", "1")
		request.Header.Add("sec-fetch-site", "none")
		request.Header.Add("sec-fetch-mode", "navigate")
		request.Header.Add("sec-fetch-user", "?1")
		request.Header.Add("sec-fetch-dest", "document")
		request.Header.Add("accept-language", "en-US,en;q=0.9")

		response, err := client.Do(request)

		if err != nil {
			fmt.Println(err)
			time.Sleep(time.Duration(delay) * time.Millisecond)
			continue monitorLoop
		}

		responseString, err := ioutil.ReadAll(response.Body)

		var products Products
		json.Unmarshal(responseString, &products)

		if err != nil {
			fmt.Println(err)
			time.Sleep(time.Duration(delay) * time.Millisecond)
			continue monitorLoop
		}

		if products.Products[0].Title != latestProduct {
			fmt.Println(websiteURL, " NEW PRODUCT FOUND: ", products.Products[0].Title)
			latestProduct = products.Products[0].Title
			for i := 0; i < len(products.Products[0].Variants); i++ {
				fmt.Printf("%v:", products.Products[0].Variants[i].Option1)
				fmt.Printf(" %v\n", strconv.Itoa(products.Products[0].Variants[i].Id))
				variantsAndSizes = append(variantsAndSizes, products.Products[0].Variants[i].Option1+" - "+strconv.Itoa(products.Products[0].Variants[i].Id))
			}
			discordWebhook(websiteURL, products.Products[0].Images[0].Src, products.Products[0].Title, products.Products[0].Handle, variantsAndSizes)
			time.Sleep(time.Duration(delay) * time.Millisecond)
			continue monitorLoop
		}

		if products.Products[0].Title == latestProduct {
			fmt.Printf("Task %v: Monitoring.... Delay %vms\n", taskNumber, delay)
			latestProduct = products.Products[0].Title
		}

		response.Body.Close()
		time.Sleep(time.Duration(delay) * time.Millisecond)
		proxyIteration++
		proxyRotation()
		continue monitorLoop
	}
}

func proxyArrayToProxyURL() {
	for i := 0; i < len(proxyList); i++ {
		proxyList[i] = "http://" + proxyList[i]
	}
	fmt.Println(proxyList)
}

func proxyRotation() string {

	if proxyIteration >= len(proxyList) {
		proxyIteration = 0
	}

	proxy = proxyList[proxyIteration]
	return proxy
}

func formatProxyList(fileName string) []string {
	f, err := os.Open(fileName)
	if err != nil {
		panic(err)
	}
	scanner := bufio.NewScanner(f)
	result := []string{}
	for scanner.Scan() {
		line := scanner.Text()
		result = append(result, line)
	}
	proxyList = result
	fmt.Println(proxyList)
	return result
}

func formatWebsiteList(fileName string) []string {
	f, err := os.Open(fileName)
	if err != nil {
		panic(err)
	}
	scanner := bufio.NewScanner(f)
	result := []string{}
	for scanner.Scan() {
		line := scanner.Text()
		result = append(result, line)
	}
	fmt.Println(result)
	return result
}

func startGoRoutines(websiteList []string) {
	for v := range websiteList {
		go monitor(websiteList[v], proxy, v)
	}
}

func discordWebhook(baseWebsiteURL string, productImage string, productTitle string, handle string, variantsAndSizes []string) {
	wa, err := discordhook.NewWebhookAPI(webhookID, webhookField, true, nil)
	if err != nil {
		panic(err)
	}

	thumbnailEmbed := discordhook.EmbedThumbnail{
		URL: productImage,
	}

	msg, err := wa.Execute(context.TODO(), &discordhook.WebhookExecuteParams{
		//Content: "Example text",
		Embeds: []*discordhook.Embed{
			{
				Title:       baseWebsiteURL,
				Description: baseWebsiteURL + "/products/" + handle + "\n\n**NEW PRODUCT FOUND: **" + "\n" + "```" + varsAndSizesToNewLines(variantsAndSizes) + "```",
				Thumbnail:   &thumbnailEmbed,
			},
		},
	}, nil, "")
	if err != nil {
		panic(err)
	}

	fmt.Println(msg.ID)
}

func varsAndSizesToNewLines(variantsAndSizes []string) string {
	var bigAssString string
	for i := 0; i < len(variantsAndSizes); i++ {
		bigAssString = bigAssString + variantsAndSizes[i] + "\n"
	}
	fmt.Println(bigAssString)
	return bigAssString
}

func makeWebhook() {

	type Webhook struct {
		Webhookid    string `json:"webhookid"`
		Webhookfield string `json:"webhookfield"`
	}

	var webhook Webhook
	r, err := os.ReadFile("webhook.json")
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(r, &webhook)
	if err != nil {
		panic(err)
	}

	intid, err := strconv.Atoi(webhook.Webhookid)
	if err != nil {
		panic(err)
	}
	webhookID = snowflake.NewSnowflake(uint64(intid))
	webhookField = webhook.Webhookfield

}
