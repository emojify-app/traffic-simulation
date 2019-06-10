package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/nicholasjackson/bench"
	"github.com/nicholasjackson/bench/output"
	"github.com/nicholasjackson/bench/util"
	"github.com/nicholasjackson/env"
)

var version = "dev"

type imageKey struct{}
type postResponse struct{}

var baseURI = env.String("BASE_URI", true, "", "base URI for requests")
var timeout = env.Duration("TIMEOUT", false, 60*time.Second, "timeout for each scenario")
var duration = env.Duration("DURATION", false, 30*time.Minute, "test duration")
var users = env.Int("USERS", false, 5, "concurrent users")
var showProgress = env.Bool("SHOW_PROGRESS", false, true, "show graphical progress")

var help = flag.Bool("help", false, "--help to show help")

func main() {

	flag.Parse()

	// if the help flag is passed show configuration options
	if *help == true {
		fmt.Println("Emojify Traffic version:", version)
		fmt.Println("Configuration values are set using environment variables, for info please see the following list:")
		fmt.Println("")
		fmt.Println(env.Help())
		os.Exit(0)
	}

	// Parse the config env vars
	err := env.Parse()
	if err != nil {
		fmt.Println(env.Help())
		os.Exit(1)
	}

	fmt.Println("Benchmarking application")

	b := bench.New(*showProgress, *users, *duration, 0*time.Second, *timeout)
	b.AddOutput(0*time.Second, os.Stdout, output.WriteTabularData)
	b.AddOutput(1*time.Second, util.NewFile("./output.txt"), output.WriteTabularData)
	b.AddOutput(1*time.Second, util.NewFile("./output.png"), output.PlotData)
	b.AddOutput(0*time.Second, util.NewFile("./error.txt"), output.WriteErrorLogs)

	b.RunBenchmarks(EmojifyFlow)
}

// EmojifyFlow tests the emojify application
func EmojifyFlow() error {
	ctx := context.Background()

	ctx, err := homePage(ctx)
	if err != nil {
		return err
	}

	time.Sleep(1 * time.Second)

	ctx, err = postAPI(ctx)
	if err != nil {
		return err
	}

	// wait until the api is finished
	ctx, err = queryAPI(ctx)
	if err != nil {
		return err
	}

	ctx, err = getCache(ctx)
	if err != nil {
		return err
	}

	return nil
}

// load the homepage
func homePage(ctx context.Context) (context.Context, error) {
	urls := []string{
		"/",
		"/config/env.js",
		"/images/emojify_small.png",
		"/images/consul.png",
		"/images/emojify.png",
	}

	var retErrors *multierror.Error

	wg := sync.WaitGroup{}
	wg.Add(len(urls))

	// process these files asynchronously like the browser would
	for _, u := range urls {
		go func(u string) {
			resp, err := http.Get(*baseURI + u)
			defer func(response *http.Response) {
				if response != nil && response.Body != nil {
					io.Copy(ioutil.Discard, response.Body)
					response.Body.Close()
				}
			}(resp)

			if err != nil || resp.StatusCode != 200 {
				retErrors = multierror.Append(retErrors, err)
			}

			wg.Done()
		}(u)
	}

	wg.Wait()

	return ctx, retErrors.ErrorOrNil()
}

// post to the api
func postAPI(ctx context.Context) (context.Context, error) {
	images := []string{
		*baseURI + "/pictures/1.jpg",
		*baseURI + "/pictures/2.jpg",
		*baseURI + "/pictures/3.jpg",
		*baseURI + "/pictures/4.jpg",
		*baseURI + "/pictures/5.jpg",
	}

	// select a random image
	rndImage := images[rand.Intn(len(images))]

	resp, err := http.Post(*baseURI+"/v2/api/emojify/", "text/plain", bytes.NewReader([]byte(rndImage)))

	if resp != nil && resp.Body != nil {
		d, _ := ioutil.ReadAll(resp.Body)
		ctx = context.WithValue(ctx, postResponse{}, d)
		resp.Body.Close()
	}

	if err != nil || resp.StatusCode != 200 {
		return ctx, fmt.Errorf("Post to API failed status: %d error: %s", resp.StatusCode, err)
	}

	return ctx, nil
}

// query the api and block until job finished
func queryAPI(ctx context.Context) (context.Context, error) {
	// get the id from the response
	r := ctx.Value(postResponse{})
	keys := make(map[string]string)
	json.Unmarshal(r.([]byte), &keys)

	// loop until the queue has processed the job
	for n := 0; n < 100; n++ {
		resp, err := http.Get(fmt.Sprintf("%s/v2/api/emojify/%s", *baseURI, keys["id"]))

		if resp != nil && resp.Body != nil {
			d, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			// check the status
			json.Unmarshal(d, &keys)
			if keys["status"] == "FINISHED" {
				ctx = context.WithValue(ctx, imageKey{}, keys["id"])
				return ctx, nil
			}
		}

		if err != nil || resp.StatusCode != 200 {
			return ctx, fmt.Errorf("Query API failed status: %d error: %s", resp.StatusCode, err)
		}

		time.Sleep(1 * time.Second)
	}

	return ctx, nil
}

// fetch from the cache
func getCache(ctx context.Context) (context.Context, error) {
	resp, err := http.Get(fmt.Sprintf(*baseURI+"/v2/api/cache/%s", ctx.Value(imageKey{})))
	if resp != nil && resp.Body != nil {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}

	if err != nil || resp.StatusCode != 200 {
		return ctx, fmt.Errorf("Fetch cache failed status: %d error: %s", resp.StatusCode, err)
	}

	return ctx, nil
}
