package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
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
	}

	resp, err := http.Post(*baseURI+"/api/", "text/plain", bytes.NewReader([]byte(images[0])))

	if resp != nil && resp.Body != nil {
		d, _ := ioutil.ReadAll(resp.Body)
		ctx = context.WithValue(ctx, imageKey{}, string(d))
		resp.Body.Close()
	}

	if err != nil || resp.StatusCode != 200 {
		return ctx, fmt.Errorf("Post to API failed status: %d error: %s", resp.StatusCode, err)
	}

	return ctx, nil
}

// fetch from the cache
func getCache(ctx context.Context) (context.Context, error) {
	resp, err := http.Get(fmt.Sprintf(*baseURI+"/api/cache/%s", ctx.Value(imageKey{})))
	if resp != nil && resp.Body != nil {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}

	if err != nil || resp.StatusCode != 200 {
		return ctx, fmt.Errorf("Fetch cache failed status: %d error: %s", resp.StatusCode, err)
	}

	return ctx, nil
}
