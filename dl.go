package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"crypto/tls" // For disabling http/2!
	"os"
	"os/signal"
	"strconv"
	"strings"
)

const (
	UserAgent    = "Rip for Reddit / CLI Tool"
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
	Debug, DryRun, AllowSpecialChars  bool
	MaxStorage, MaxSize            int64
	OgType                         string
	PostLinksFile, MediaLinksFile  io.Writer
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

var stats Stats
var config Config


var interrupt chan os.Signal
var completion = make(chan bool)

var downloadingFilename string

// BugFix: with transparent HTTP/2, sometimes reddit servers send HTML instead of JSON
// So create a custom client
var client = http.Client{
	Transport: &http.Transport{
		TLSNextProto: map[string]func(authority string, c *tls.Conn) http.RoundTripper{},
	},
}

func either(a, b string) string {
	if a == "" {
		return b
	}
	return a
}

func fatal(val ...interface{}) {
	fmt.Fprintln(os.Stderr, val...)
	os.Exit(1)
}

// always print user visible info to standard error
func eprintln(vals ...interface{}) {
	fmt.Fprintln(os.Stderr, vals...)
}

func eprintf(format string, vals ...interface{}) {
	fmt.Fprintf(os.Stderr, format, vals...)
}

func eprint(vals ...interface{}) {
	fmt.Fprint(os.Stderr, vals...)
}

func check(e error, extra ...interface{}) {
	if e != nil {
		fmt.Fprintln(os.Stderr, extra...)
		fatal(e.Error())
	}
}

func log(debug bool, vals ...interface{}) {
	if debug {
		fmt.Fprintln(os.Stderr, vals...)
	}
}

// return size as human readable string
func size(bytes int64) string {
	sizes := []int64{1000 * 1000 * 1000, 1000 * 1000, 1000}
	names := []string{"GB", "MB", "KB"}

	// content length in http response can be -1
	if bytes == -1 {
		return "Unknown length"
	}

	for i, sz := range sizes {
		if bytes > sz {
			units := float64(bytes) / float64(sz)
			return strconv.FormatFloat(units, 'f', 1, 64) + names[i]
		}
	}
	return strconv.FormatInt(bytes, 10) + "B"
}

func PrintStat() {
	eprintln(strings.Repeat("-", 20))
	eprintln("Processed Posts: ", stats.Processed)
	eprintln("Already Downloaded: ", stats.Repeated)
	eprintln("Failed: ", stats.Failed)
	eprintln("Saved: ", stats.Saved)
	eprintln("Other: ",
		stats.Processed-stats.Failed-stats.Repeated-stats.Saved)
	eprintln(strings.Repeat("-", 20))
	eprintln("Approx. Storage Used:", size(stats.CopiedBytes))
	eprintln(strings.Repeat("-", 20))
}

func Finish() {
	PrintStat()
	completion <- true
}

// body is the response body which contains json
// handler is run for every post entry unless handler exits early
// returns last posts's id ('name' attribute in json)
// which is useful to fetch next page
func HandlePosts(body io.ReadCloser, handler func(PostData)) (last string) {
	dec := json.NewDecoder(body)
	apiResponse := ApiResponse{}
	err := dec.Decode(&apiResponse)
	check(err)
	for _, post := range apiResponse.Data.Children {
		stats.Processed += 1
		handler(post.Data)
		last = post.Data.Name
	}
	return last
}

// Returns whether the image link can be downloaded
// if downloadable, return final URL, else return empty string
// also the extension string that matched

func removeSpecialChars(filename string) string {
	var b strings.Builder
	var banned map[rune]string
	if config.AllowSpecialChars {
		banned = minimalSubst
	} else {
		banned = windowsSubst
	}
	for _, r := range filename {
		repl, spec := banned[r]
		if spec {
			b.WriteString(repl)
		} else if !strconv.IsPrint(r) {
			b.WriteRune('-')
		} else {
			b.WriteRune(r)
		}
	}
	if config.AllowSpecialChars {
		return b.String();
	}
	return sanitizeWindowsFilename(b.String())
}

func CheckImage(linkString string) (finalLink string, extension string) {
	var exts = []string{".jpeg", ".gif", ".mp4", ".jpg", ".png"}
	link, err := url.Parse(linkString)
	check(err)
	path := link.Path

	// imgur gifv links are generally MP4
	if (link.Host == "i.imgur.com" || link.Host == "imgur.com") &&
			strings.HasSuffix(path, ".gifv") {
		trimmed := strings.TrimSuffix(path, ".gifv")
		link.Path = trimmed + ".mp4"
		link.Host = "i.imgur.com"
		return link.String(), ".mp4"
	}

	for _, ext := range exts {
		if strings.HasSuffix(path, ext) {
			return linkString, ext
		}
	}

	// if ogType is given, read the link and get it's og:video or og:image
	if config.OgType != "" {
		log(config.Debug, "REQUEST PAGE: "+linkString)
		response, err := FetchUrl(linkString)
		log(config.Debug, "Response Headers=", response.Header)
		if err != nil {
			log(config.Debug, err.Error())
			return "", ""
		}
		defer response.Body.Close()
		contentType := response.Header.Get("Content-Type")
		if strings.ToLower(contentType) != "text/html; charset=utf-8" {
			log(config.Debug, "Unsupported ContentType when looking for og: url")
			return "", ""
		}
		ogUrl, err := GetOgUrl(response.Body)
		if ogUrl != "" {
			return CheckImage(ogUrl)
		}
	}
	return "", ""
}

// Returns Resp : *http.Response
func FetchUrlWithMethod(url, method string, acceptMimeType string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	check(err)
	req.Header.Add("User-Agent", config.UserAgent)

	if acceptMimeType != "" {
		req.Header.Add("Accept", acceptMimeType)
	}

	log(config.Debug, "Headers: ", req.Header)
	log(config.Debug, "Protocol: ", req.Proto)
	response, err := client.Do(req)

	if err != nil {
		return nil, err
	}
	return response, err
}

func FetchUrl(url string) (*http.Response, error) {
	return FetchUrlWithMethod(url, "GET", "")
}

// downloads all images reachable from reddit.com/<path>.json
func TraversePages(path string, handler func(PostData)) {
	target := "https://www.reddit.com/" + strings.TrimSuffix(path, "/")

	after := config.After
	// Handle sort options
	topBy := ""
	switch config.Sort {
	case "hot", "new", "rising":
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
	for {
		var link = target // final link
		if after != "" {
			link += "&after=" + after
		}
		log(config.Debug, "REQUEST: ", link)
		response, err := FetchUrlWithMethod(link, "GET", "application/json")
		log(config.Debug, "Response Headers=", response.Header)
		check(err, "Cannot get JSON response")
		defer response.Body.Close()

		processed := stats.Processed
		after = HandlePosts(response.Body, handler)
		if stats.Processed == processed {
			Finish()
		}
	}
}

func DownloadLink(post PostData) {
	title := strings.TrimSpace(strings.ReplaceAll(post.Title, "/", "|"))
	title = html.UnescapeString(title) // &amp; etc.. are escaped in json
	if len(title) > 194 {
		title = title[:192] + ".."
	}
	// check if Karma limit is met
	if post.Ups < config.MinKarma {
		log(config.Debug, "Skipped Due to Less Karma:", title,
			"| Ups:", post.Ups, "|", post.Url, "\n\n")
		if strings.HasPrefix(config.Sort, "top-") {
			eprintln("Skipping posts with less points, since sort=" + config.Sort)
			Finish()
		}
		return
	}

	if config.PostLinksFile != nil {
		fmt.Fprintln(config.PostLinksFile, post.Url)
	}
	// check if url is image
	url, extension := CheckImage(post.Url)
	if url == "" {
		log(config.Debug, "Skip non-imagelike entry: ", title, " | ", post.Url, "\n")
		return
	}
	filename := title + " [" + strings.TrimPrefix(post.Name, "t3_") +
		"]" + extension
	filename = removeSpecialChars(filename)
	log(config.Debug, "URL: ", post.Url, " | Ups:", post.Ups)
	log(config.Debug && url != post.Url, "->", url)
	if config.MediaLinksFile != nil {
		fmt.Fprintln(config.MediaLinksFile, url)
	}
	eprint(filename)
	// check if already downloaded file
	_, err := os.Stat(filename)
	if err == nil {
		eprint("    [Already Downloaded]\n\n")
		stats.Repeated += 1
		return
	}

	// If dry run, don't fetch media, or create a file
	// but you still have to increase number of files for config.MaxFiles to work
	if config.DryRun {
		eprint("    [Dry Run]\n\n")
		stats.Saved += 1
		if stats.Saved == config.MaxFiles {
			Finish()
		}
		return
	}

	// CHECK: any edge case?
	var output *os.File = nil // don't create until needed

	// Common error handling code
	netError := func(kind string) {
		stats.Failed += 1
		eprintf("    [" + kind + " Error: " + err.Error() + "]\n\n")
		if output != nil {
			// transfer errors when file was already created
			log(config.Debug, "Try remove file: ", filename)
			rmErr := os.Remove(filename)
			if rmErr != nil {
				log(config.Debug, "Error removing file")
			}
		}
		return
	}
	// Fetch
	response, err := FetchUrlWithMethod(url, "HEAD", "")
	if err != nil {
		netError("Request ")
		return
	}
	defer response.Body.Close()

	// check content-type
	// It's generally rare, but few sites send html from urls that end with gif etc..
	contentType := response.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") &&
		!strings.HasPrefix(contentType, "video/") {
		eprintln("    [Unexpected Content-Type: " + contentType + "]")
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
		eprintf("    [Too Large: %s]\n\n", size(length))
		return
	}
	// if file length will go past the storage limit, finish
	if config.MaxStorage != -1 && config.MaxStorage < length+stats.CopiedBytes {
		eprintf("    [%s | Crosses storage limit]\n\n", size(length))
		Finish()
	}

	// Create file
	log(config.Debug, "  [Creating file]")
	downloadingFilename = filename
	defer func() {
		downloadingFilename = ""
	}()
	output, err = os.Create(filename)
	if err != nil {
		eprintf("    [Cannot create file]\n\n")
		stats.Failed += 1
		return
	}
	defer output.Close()

	// do a GET request
	fullResponse, err := FetchUrl(url)
	if err != nil {
		netError("Request ")
		return
	}
	defer fullResponse.Body.Close()

	// Copy
	n, err := io.Copy(output, fullResponse.Body)

	// add n to how much diskspace is consumed even if there's an error
	// because it would give a more appropriate approximation of bandwidth consumption
	// But if you're using that option to limit data usage, give 80% of airtime you can use
	stats.CopiedBytes += n

	if err != nil {
		netError("Transfer ")
		return
	}

	// Transfer success I hope
	// write stats
	eprintf("    [Done: %s]\n\n", size(n))
	stats.Saved += 1
	if stats.Saved == config.MaxFiles {
		Finish()
	}
	return
}

func createLinksFile(filename string) *os.File {
	if filename == "" {
		return nil
	}
	if filename == "-" || filename == "stdout" {
		return os.Stdout
	}
	output, err := os.Create(filename)
	check(err)
	return output
}

func main() {
	help := false
	// whether help option is provided
	flag.BoolVar(&help, "help", false, "Show this help message")
	var logMediaLinksTo, logPostLinksTo string
	var err error
	// option parsing
	flag.BoolVar(&config.Debug, "v", false, "Enable verbose output")
	flag.BoolVar(&config.DryRun, "d", false, "DryRun i.e just print urls and names")
	flag.BoolVar(&config.AllowSpecialChars, "special-chars", false,
		"Allow all characters in filenames except / and \\, " +
		"And windows-special filenames like NUL")
	flag.StringVar(&config.After, "after", "", "Get posts after the given ID")
	flag.StringVar(&config.UserAgent, "useragent", UserAgent, "UserAgent string")
	flag.Int64Var(&config.MaxStorage, "max-storage", -1, "Data usage limit in MB, -1 for no limit")
	flag.Int64Var(&config.MaxSize, "max-size", -1, "Max size of media file in KB, -1 for no limit")
	flag.StringVar(&config.Folder, "folder", "", "Target folder name")
	flag.StringVar(&logMediaLinksTo, "log-media-links", "", "Log media links to given file")
	flag.StringVar(&logPostLinksTo, "log-post-links", "", "Log all links found in posts to given file")
	flag.StringVar(&config.OgType, "og-type", "", "Look Up for a media link in page's og:property"+
		" if link itself is not image/video (experimental) supported: video, image, any")
	flag.StringVar(&config.Sort, "sort", "", "Sort: best|hot|new|rising|top-<all|year|month|week|day>")
	flag.IntVar(&config.MaxFiles, "max", -1, "Max number of files to download (+ve), -1 for no limit")
	flag.IntVar(&config.MinKarma, "min-karma", 0, "Minimum Karma of the post")
	flag.IntVar(&config.Limit, "l", 100, "Number of entries to fetch in one API request (devel)")
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 || help {
		eprintf("Usage: %s <options> <r/subreddit>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	if logPostLinksTo != "" && logPostLinksTo == logMediaLinksTo {
		fatal("Can't log both post and media links to same file")
	}

	config.PostLinksFile = createLinksFile(logPostLinksTo)
	config.MediaLinksFile = createLinksFile(logMediaLinksTo)

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
	// only few values are supported for config.OgType
	og := config.OgType
	if og != "" && og != "video" && og != "image" && og != "any" {
		fatal("Only supported values for -og-type are image, video and any")
	}
	// enable debug output in case of dry run
	config.Debug = config.Debug || config.DryRun

	// when we provide an -after= parameter manually, we expect a t3_ prefix
	if config.After != "" && !strings.HasSuffix(config.After, "t3_") {
		config.After = "t3_" + config.After
	}

	// compute actual MaxStorage in bytes
	if config.MaxStorage != -1 {
		config.MaxStorage *= 1000 * 1000 // MB
	}

	if config.MaxSize != -1 {
		config.MaxSize *= 1000 // KB
	}
	// Create folder
	config.Folder = either(config.Folder,
		strings.TrimPrefix(strings.ReplaceAll(path, "/", "."), "r."))
	_, err = os.Stat(config.Folder)

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

	go TraversePages(path, func(post PostData) {
		DownloadLink(post)
	})

	select {
	case <-interrupt:
		eprintln("Interrupt received, Exiting...")
		if (downloadingFilename != "") {
			eprintf("Removing possibly incomplete file: '%s'\n", downloadingFilename)
			os.Remove(downloadingFilename)
		}
		PrintStat()
	case <-completion:
		os.Exit(0)
	}
}
