package main

import (
	"crypto/tls" // For disabling http/2!
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
)

const (
	UserAgent    = "rrip / Go CLI Tool"
	DefaultLimit = 100
)

var terminalColumns = getTerminalSize();

var horizontalDashedLine = strings.Repeat("-", terminalColumns)

type Stats struct {
	Processed, Saved, Failed, Repeated int
	CopiedBytes                        int64
}

type Options struct {
	After, Sort, UserAgent, Folder   string
	Limit, MaxFiles, MinScore        int
	Debug, DryRun, AllowSpecialChars bool
	MaxStorage, MaxSize              int64
	OgType                           string
	LogLinksFile                     io.Writer
}

type ImagePreviewEntry struct {
	Url    string
	Width  int
	Height int
}

type ImagePreview struct {
	Source      ImagePreviewEntry
	Resolutions []ImagePreviewEntry
}

type PostData struct {
	Url, Name, Title  string
	Ups, Score        int
	Subreddit, Author string
	LinkFlairText     string
	CreatedUtc        int64
	Preview           struct {
		images []ImagePreview
	}
}

type Post struct {
	Data PostData
}

type ApiData struct {
	After    string
	Children []Post
}

type ApiResponse struct {
	Data ApiData
}

var stats Stats
var options Options

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

func coalesce(a, b string) string {
	if a == "" {
		return b
	}
	return a
}

func fatal(val ...interface{}) {
	fmt.Fprintln(os.Stderr, val...)
	os.Exit(1)
}

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

func log(vals ...interface{}) {
	if options.Debug {
		fmt.Fprintln(os.Stderr, vals...)
	}
}

func size(bytes int64) string {
	sizes := []int64{1000 * 1000 * 1000, 1000 * 1000, 1000}
	names := []string{"GB", "MB", "KB"}

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
	eprintln(horizontalDashedLine)
	eprintln("Processed Posts: ", stats.Processed)
	eprintln("Already Downloaded: ", stats.Repeated)
	eprintln("Failed: ", stats.Failed)
	eprintln("Saved: ", stats.Saved)
	eprintln("Other: ",
		stats.Processed-stats.Failed-stats.Repeated-stats.Saved)
	eprintln(horizontalDashedLine)
	eprintln("Approx. Storage Used:", size(stats.CopiedBytes))
	eprintln(horizontalDashedLine)
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

func removeSpecialChars(filename string) string {
	var b strings.Builder
	var banned map[rune]string
	if options.AllowSpecialChars {
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
	if options.AllowSpecialChars {
		return b.String()
	}
	return sanitizeWindowsFilename(b.String())
}

// Returns whether the image link can be downloaded
// if downloadable, return final URL, else return empty string
// also the extension string that matched
func CheckAndResolveImage(linkString string) (finalLink string, extension string) {
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
	if options.OgType != "" {
		log("REQUEST PAGE: " + linkString)
		response, err := FetchUrl(linkString)
		log("Response Headers=", response.Header)
		if err != nil {
			log(err.Error())
			return "", ""
		}
		defer response.Body.Close()
		contentType := response.Header.Get("Content-Type")
		if strings.ToLower(contentType) != "text/html; charset=utf-8" {
			log("Unsupported ContentType when looking for og: url")
			return "", ""
		}
		ogUrl, err := GetOgUrl(response.Body)
		if ogUrl != "" {
			return CheckAndResolveImage(ogUrl)
		}
	}
	return "", ""
}

// pass acceptMimeType = "" if no restriction
func FetchUrlWithMethod(url, method string, acceptMimeType string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	check(err)
	req.Header.Add("User-Agent", options.UserAgent)

	if acceptMimeType != "" {
		req.Header.Add("Accept", acceptMimeType)
	}

	log("Headers: ", req.Header)
	log("Protocol: ", req.Proto)
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
	unsuffixedPath := strings.TrimSuffix(path, "/")
	if unsuffixedPath == "" {
		options.Sort = "hot"
	}
	target := "https://www.reddit.com/" + unsuffixedPath

	after := options.After
	// Handle sort options
	topBy := ""
	switch options.Sort {
	case "hot", "new", "rising":
		target += "/" + options.Sort
	case "top-hour", "top-day", "top-month", "top-year", "top-all":
		target += "/top"
		topBy = strings.TrimPrefix(options.Sort, "top-")
	case "":
		_ = "best" // do nothing
	default:
		fatal("Invalid option passed to sort")
	}
	target += ".json?limit=" + strconv.Itoa(options.Limit)
	// for sort by top voted
	if topBy != "" {
		target += "&t=" + topBy
	}
	for {
		var link = target // final link
		if after != "" {
			link += "&after=" + after
		}
		log("REQUEST: ", link)
		response, err := FetchUrlWithMethod(link, "GET", "application/json")
		log("Response Headers=", response.Header)
		check(err, "Cannot get JSON response")
		defer response.Body.Close()

		processed := stats.Processed
		after = HandlePosts(response.Body, handler)
		if stats.Processed == processed {
			Finish()
		}
	}
}

func DownloadPost(post PostData) {
	title := strings.TrimSpace(strings.ReplaceAll(post.Title, "/", "|"))
	title = html.UnescapeString(title) // &amp; etc.. are escaped in json
	if len(title) > 194 {
		title = title[:192] + ".."
	}
	if post.Score < options.MinScore {
		log("Skipped due to less score:", title,
			"| Score:", post.Score, "|", post.Url, "\n")
		if strings.HasPrefix(options.Sort, "top-") {
			eprintln("Skipping posts with less points, since sort=" + options.Sort)
			Finish()
		}
		return
	}

	if options.LogLinksFile != nil {
		fmt.Fprintln(options.LogLinksFile, post.Url)
	}
	imageUrl, extension := CheckAndResolveImage(post.Url)
	if imageUrl == "" {
		log("Skip non-imagelike entry: ", title, " | ", post.Url)
		return
	}
	filename := title + " [" + strings.TrimPrefix(post.Name, "t3_") +
		"]" + extension
	filename = removeSpecialChars(filename)
	log("URL: ", post.Url, " | Ups:", post.Ups)
	if imageUrl != post.Url {
		log("->", imageUrl)
	}
	eprintf("%-*.*s", terminalColumns-24, terminalColumns-24, filename)

	// check if already downloaded file
	_, err := os.Stat(filename)
	if err == nil {
		eprint("    [Already Downloaded]\n")
		stats.Repeated += 1
		return
	}

	// If dry run, don't fetch media, or create a file
	// but you still have to increase number of files for config.MaxFiles to work
	if options.DryRun {
		eprint("    [Dry Run]\n")
		stats.Saved += 1
		if stats.Saved == options.MaxFiles {
			Finish()
		}
		return
	}

	// CHECK: any edge case?
	var output *os.File = nil // don't create until needed

	// Common error handling code
	netError := func(kind string) {
		stats.Failed += 1
		eprintf("    [" + kind + " Error: " + err.Error() + "]\n")
		if output != nil {
			// transfer errors when file was already created
			log("Try remove file: ", filename)
			rmErr := os.Remove(filename)
			if rmErr != nil {
				log("Error removing file")
			}
		}
		return
	}
	// Fetch
	response, err := FetchUrlWithMethod(imageUrl, "HEAD", "")
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
	skipDueToSize := (options.MaxSize != -1) &&
		(options.MaxSize < length || length == -1)
	// if file length unknown and there is storage limit, skip
	skipDueToSize = skipDueToSize ||
		(options.MaxStorage != -1 && length == -1)
	if skipDueToSize {
		eprintf("    [Too Large: %s]\n", size(length))
		return
	}
	// if file length will go past the storage limit, finish
	if options.MaxStorage != -1 && options.MaxStorage < length+stats.CopiedBytes {
		eprintf("    [%s | Crosses storage limit]\n\n", size(length))
		Finish()
	}

	// Create file
	log("  [Creating file]")
	downloadingFilename = filename
	defer func() {
		downloadingFilename = ""
	}()
	output, err = os.Create(filename)
	if err != nil {
		eprintf("    [Cannot create file]\n")
		stats.Failed += 1
		return
	}
	defer output.Close()

	// do a GET request
	fullResponse, err := FetchUrl(imageUrl)
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
	eprintf("    [Done: %s]\n", size(n))
	stats.Saved += 1
	if stats.Saved == options.MaxFiles {
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
	var logLinksTo string
	var err error
	// option parsing
	flag.BoolVar(&options.Debug, "v", false, "Enable verbose output")
	flag.BoolVar(&options.DryRun, "d", false, "DryRun i.e just print urls and names")
	flag.BoolVar(&options.AllowSpecialChars, "allow-special-chars", false,
		"Allow all characters in filenames except / and \\, "+
			"And windows-special filenames like NUL")
	flag.StringVar(&options.After, "after", "", "Get posts after the given ID")
	flag.StringVar(&options.UserAgent, "useragent", UserAgent, "UserAgent string")
	flag.Int64Var(&options.MaxStorage, "max-storage", -1, "Data usage limit in MB, -1 for no limit")
	flag.Int64Var(&options.MaxSize, "max-size", -1, "Max size of media file in KB, -1 for no limit")
	flag.StringVar(&options.Folder, "folder", "", "Target folder name")
	flag.StringVar(&logLinksTo, "log-links", "", "Log media links to given file")
	flag.StringVar(&options.OgType, "og-type", "", "Look Up for a media link in page's og:property"+
		" if link itself is not image/video (experimental) supported: video, image, any")
	flag.StringVar(&options.Sort, "sort", "", "Sort: best|hot|new|rising|top-<all|year|month|week|day>")
	flag.IntVar(&options.MaxFiles, "max-files", -1, "Max number of files to download (+ve), -1 for no limit")
	flag.IntVar(&options.MinScore, "min-score", 0, "Minimum score of the post to download")
	flag.IntVar(&options.Limit, "l", 100, "Number of entries to fetch in one API request (devel)")
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 || help {
		eprintf("Usage: %s <options> <r/subreddit>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	options.LogLinksFile = createLinksFile(logLinksTo)

	path := strings.TrimSuffix(args[0], "/")

	// validate some arguments
	toCheck := map[string]int64{
		"-max":         int64(options.MaxFiles),
		"-max-storage": options.MaxStorage,
		"-max-size":    options.MaxSize,
	}
	for option, value := range toCheck {
		if value < 1 && value != -1 {
			fatal("Invalid value for option " + option)
		}
	}

	og := options.OgType
	if og != "" && og != "video" && og != "image" && og != "any" {
		fatal("Only supported values for -og-type are image, video and any")
	}

	// enable debug output in case of dry run
	options.Debug = options.Debug || options.DryRun

	if options.After != "" && !strings.HasPrefix(options.After, "t3_") {
		options.After = "t3_" + options.After
	}

	// compute actual MaxStorage in bytes
	if options.MaxStorage != -1 {
		options.MaxStorage *= 1000 * 1000 // MB
	}

	if options.MaxSize != -1 {
		options.MaxSize *= 1000 // KB
	}

	// Create folder
	options.Folder = coalesce(options.Folder,
		strings.TrimPrefix(strings.ReplaceAll(path, "/", "."), "r."))
	_, err = os.Stat(options.Folder)

	// Note: not creating folder anew if dry run
	if os.IsNotExist(err) && !options.DryRun {
		check(os.MkdirAll(options.Folder, 0755))
	}

	// if dry run, change to folder only if folder already existed
	if err == nil || !options.DryRun {
		check(os.Chdir(options.Folder))
	}

	// to properly handle Ctrl+C, notify os.Interrupt
	interrupt = make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	go TraversePages(path, func(post PostData) {
		DownloadPost(post)
	})

	select {
	case <-interrupt:
		eprintln("Interrupt received, Exiting...")
		if downloadingFilename != "" {
			eprintf("Removing possibly incomplete file: '%s'\n", downloadingFilename)
			os.Remove(downloadingFilename)
		}
		PrintStat()
	case <-completion:
		os.Exit(0)
	}
}
