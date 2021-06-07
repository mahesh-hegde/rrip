package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"html"
)

const (
	UserAgent    = "rip for Reddit / Command Line Tool / Linux"
	DefaultLimit = 100
)

type Stats struct {
	Processed, Saved, Failed, Repeated int
	CopiedBytes                        int64
}

// (mostly) command line options
type Config struct {
	After, Sort, UserAgent, Folder string
	Limit, MaxFiles, MinKarma      int
	Debug, DryRun                  bool
	MaxStorage, MaxSize            int64
}

type PostData struct {
	Url, Name, Title string
	Ups              int
}

type Post struct {
	Data PostData
}

type ApiData struct {
	Children []Post
}

type ApiResponse struct {
	Data ApiData
}

// Entire Program is single threaded
// Passing it around is tedious
var stats Stats

// similarly for the interrupt channel

var interrupt chan os.Signal

func either(a, b string) string {
	if a == "" {
		return b
	}
	return a
}

func fatal(val ...interface{}) {
	fmt.Println(val...)
	os.Exit(1)
}

// unreadable code but saved my keyboard
func check(e error, extra ...string) {
	if e != nil {
		fmt.Println(extra)
		fatal(e.Error())
	}
}

func Log(debug bool, vals ...interface{}) {
	if debug {
		fmt.Fprintln(os.Stdout, vals...)
	}
}

func size(bytes int64) string {
	sizes := []int64{1000 * 1000 * 1000, 1000 * 1000, 1000}
	names := []string{"GB", "MB", "KB"}

	// content length in http response can be -1
	if (bytes == -1) {
		return "Unknown length"
	}

	for i, sz := range sizes {
		if bytes > sz {
			units := bytes / sz
			if (bytes % sz) >= (sz / 2) {
				units += 1
			}
			return strconv.FormatInt(units, 10) + names[i]
		}
	}
	return strconv.FormatInt(bytes, 10) + "B"
}

func Finish(stats *Stats) {
	fmt.Println(strings.Repeat("-", 20))
	fmt.Println("Processed Posts: ", stats.Processed)
	fmt.Println("Already Downloaded: ", stats.Repeated)
	fmt.Println("Failed: ", stats.Failed)
	fmt.Println("Saved: ", stats.Saved)
	fmt.Println("Other: ",
		stats.Processed-stats.Failed-stats.Repeated-stats.Saved)
	fmt.Println(strings.Repeat("-", 20))
	fmt.Println("Approx. Storage Used:", size(stats.CopiedBytes))
	fmt.Println(strings.Repeat("-", 20))
	os.Exit(0)
}

// body is the response body which contains json
// handler is run for every post entry unless handler exits early
// returns last posts's id ('name' attribute in json)
// which is useful to fetch next page
func HandlePosts(body io.ReadCloser, handler func(int, PostData)) (last string) {
	dec := json.NewDecoder(body)
	apiResponse := ApiResponse{}
	check(dec.Decode(&apiResponse))
	for i, post := range apiResponse.Data.Children {
		stats.Processed += 1
		handler(i, post.Data)
		last = post.Data.Name
	}
	return last
}

// Returns whether the image ends with ".jp[e]g", ".png", ".gif" or ".mp4"
// also the extension string that matched
func CheckImage(linkString string) (isImage bool, extension string) {
	var exts = []string{".jpeg", ".gif", ".gifv", ".mp4", ".jpg", ".png"}
	link, err := url.Parse(linkString)
	check(err)
	path := link.Path
	for _, ext := range exts {
		if strings.HasSuffix(path, ext) {
			return true, ext
		}
	}
	return false, ""
}

// Returns Resp : *http.Response
func FetchUrl(url string, userAgent string, client *http.Client) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	check(err)
	req.Header.Add("User-Agent", userAgent)
	// client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, err
}

// downloads all images reachable from reddit.com/<path>.json
func TraversePages(path string, config *Config, handler func(int, PostData)) {
	target := "https://reddit.com/" + strings.TrimSuffix(path, "/")

	// so that we don't modify at a distance
	after := config.After
	// Handle sort options
	topBy := ""
	switch config.Sort {
	case "best", "hot", "new", "rising":
		target += "/" + config.Sort
	case "top-hour", "top-day", "top-month", "top-year", "top-all":
		target += "/top"
		topBy = strings.TrimPrefix(config.Sort, "top-")
	case "":
		_ = "best" // do nothing
	default:
		fatal("Invalid option passed to sort")
	}
	target += ".json?limit=" + strconv.Itoa(config.Limit)
	// for sort by top voted
	if topBy != "" {
		target += "&t=" + topBy
	}
	// create client
	// http.Client caches connections
	// that's why preferable to use same client
	redditClient := &http.Client{}
	for {
		var link = target // final link
		if after != "" {
			link += "&after=" + after
		}
		Log(config.Debug, "REQUEST: ", link)
		response, err := FetchUrl(link, config.UserAgent, redditClient)
		check(err, "Cannot get JSON response")

		processed := stats.Processed
		after = HandlePosts(response.Body, handler)
		if stats.Processed == processed {
			// no items were got in this page
			Finish(&stats)
		}
	}
}

func DownloadLink(_ int, post PostData, config *Config, client *http.Client) {
	// signal handling:
	// when ctrl+c is pressed, it should be buffered to a channel
	// check that after every download
	// if it's pressed then exit

	select {
	case sig := <-interrupt:
		if sig == os.Interrupt {
			fmt.Println("Interrupt Received, Exit")
			Finish(&stats)
		}
	default:
		// do nothing
	}
	// process title, truncate if too long
	title := strings.TrimSpace(strings.ReplaceAll(post.Title, "/", "|"))
	title = html.UnescapeString(title) // &amp; etc.. are escaped in json
	if len(title) > 194 {
		title = title[:192] + ".."
	}
	// check if Karma limit is met
	// will any post ever get 2B upvotes?
	// also: default limit is 1 karma point.
	if post.Ups < config.MinKarma {
		Log(config.Debug, "Skipped Due to Less Karma:", title,
			"| Ups:", post.Ups, "|", post.Url, "\n")
		fmt.Println()
		if strings.HasPrefix(config.Sort, "top-") {
			fmt.Println("Skipping posts with less points, since sort=" + config.Sort)
			Finish(&stats)
		}
		return
	}

	// check if url is image
	isImage, extension := CheckImage(post.Url)
	if !isImage {
		Log(config.Debug, "Skip non-imagelike entry: ", title, " | ", post.Url, "\n")
		return
	}
	filename := title + " [" + strings.TrimPrefix(post.Name, "t3_") + "]" + extension
	Log(config.Debug, "URL: ", post.Url, " | Ups:", post.Ups)
	fmt.Print(filename)
	// check if already downloaded file
	_, err := os.Stat(filename)
	if err == nil {
		fmt.Println("    [Already Downloaded]")
		fmt.Println()
		stats.Repeated += 1
		return
	}

	// If dry run, don't fetch media, or create a file
	// but you still have to increase number of files for config.MaxFiles to work
	if config.DryRun {
		fmt.Println("    [Dry Run]")
		fmt.Println()
		stats.Saved += 1
		if stats.Saved == config.MaxFiles {
			Finish(&stats)
		}
		return
	}

	// CHECK: any edge case?
	var output *os.File = nil // don't create until needed

	// Common error handling code
	netError := func(kind string) {
		stats.Failed += 1
		fmt.Println("    [" + kind + " Error: " + err.Error() + "]")
		fmt.Println()
		if output != nil {
			output.Close()
			check(os.Remove(filename))
		}
		return
	}
	// Fetch
	response, err := FetchUrl(post.Url, config.UserAgent, client)
	if err != nil {
		netError("Request ")
		return
	}
	length := response.ContentLength
	// If larger or unknown length, skip
	skipDueToSize := (config.MaxSize != -1) &&
		(config.MaxSize < length || length == -1)
	// if file length unknown and there is storage limit, skip
	skipDueToSize = skipDueToSize ||
		(config.MaxStorage != -1 && length == -1)
	if skipDueToSize {
		fmt.Printf("    [Too Large: %s]\n", size(length))
		fmt.Println()
		return
	}
	// if file length will go past the storage limit, finish 
	if config.MaxStorage != -1 && config.MaxStorage < length+stats.CopiedBytes {
		fmt.Printf("    [%s | Crosses storage limit]\n", size(length))
		fmt.Println()
		Finish(&stats)
	}

	// Create file
	output, err = os.Create(filename)
	if err != nil {
		fmt.Println("    [Cannot create file]")
		fmt.Println()
		stats.Failed += 1
		return
	}

	// Copy
	n, err := io.Copy(output, response.Body)

	// add n to how much diskspace is consumed even if there's an error
	// because it would give a more appropriate approximation of bandwidth consumption
	// But if you're using that option to limit data usage, give 80% of airtime you can use
	stats.CopiedBytes += n

	if err != nil {
		netError("Transfer ")
		response.Body.Close()
		return
	}

	// Transfer success I hope
	// write stats
	fmt.Printf("    [Done: %s]\n", size(n))
	fmt.Println()
	stats.Saved += 1
	if stats.Saved == config.MaxFiles {
		Finish(&stats)
	}

	// close files
	check(response.Body.Close())
	check(output.Close())
	return
}

func main() {
	config := Config{}
	help := false
	// whether help option is provided
	flag.BoolVar(&help, "help", false, "Show this help message")
	// option parsing
	flag.BoolVar(&config.Debug, "v", false, "Enable verbose output")
	flag.BoolVar(&config.DryRun, "d", false, "DryRun i.e just print urls and names")
	flag.StringVar(&config.After, "after", "", "Get posts after the given ID")
	flag.StringVar(&config.UserAgent, "useragent", UserAgent, "UserAgent string")
	flag.Int64Var(&config.MaxStorage, "max-storage", -1, "Data usage limit in MB, -1 for no limit")
	flag.Int64Var(&config.MaxSize, "max-size", -1, "Max size of media file in KB, -1 for no limit")
	flag.StringVar(&config.Folder, "folder", "", "Target folder name")
	flag.StringVar(&config.Sort, "sort", "", "Sort: best|hot|new|rising|top-<all|year|month|week|day>")
	flag.IntVar(&config.MaxFiles, "max", -1, "Max number of files to download (+ve), -1 for no limit")
	flag.IntVar(&config.MinKarma, "min-karma", 0, "Minimum Karma of the post")
	flag.IntVar(&config.Limit, "l", 100, "Number of entries to fetch in one API request (devel)")
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 || help {
		fmt.Fprintf(os.Stdout, "Usage: %s <options> <r/subreddit>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	path := strings.TrimSuffix(args[0], "/")

	// validate some arguments
	// handling 0 correctly probably requires some more code, so don't
	toCheck := map[string]int64{
		"-max":         int64(config.MaxFiles),
		"-max-storage": config.MaxStorage,
		"-max-size":    config.MaxSize,
	}
	for option, value := range toCheck {
		if value < 1 && value != -1 {
			fatal("Invalid value for option " + option)
		}
	}

	// enable debug output in case of dry run
	config.Debug = config.Debug || config.DryRun

	// when we provide an -after= parameter manually, we expect a t3_ prefix
	if config.After != "" && !strings.HasSuffix(config.After, "t3_") {
		config.After = "t3_" + config.After
	}

	// compute actual MaxStorage in bytes
	// if you're overflowing this, you have bigger problems
	if config.MaxStorage != -1 {
		config.MaxStorage *= 1000 * 1000 // MB
	}

	if config.MaxSize != -1 {
		config.MaxSize *= 1000 // KB
	}
	// Create folder
	config.Folder = either(config.Folder,
		strings.TrimPrefix(strings.ReplaceAll(path, "/", "."), "r."))
	_, err := os.Stat(config.Folder)

	// Note: not creating folder anew if dry run
	if os.IsNotExist(err) && !config.DryRun {
		check(os.MkdirAll(config.Folder, 0755))
	}

	// if dry run, change to folder only if folder already existed
	if err == nil || !config.DryRun {
		check(os.Chdir(config.Folder))
	}

	// to properly handle Ctrl+C, notify os.Interrupt
	interrupt = make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// create a client for all image downloader connections
	client := &http.Client{}
	// start downloading
	TraversePages(path, &config, func(i int, post PostData) {
		DownloadLink(i, post, &config, client)
	})
}
