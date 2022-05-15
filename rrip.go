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
	"regexp"
	"strconv"
	"strings"
)

const (
	UserAgent    = "rrip / Go CLI Tool"
	DefaultLimit = 100
)

var terminalColumns = getTerminalSize()

var horizontalDashedLine = strings.Repeat("-", terminalColumns)

type Stats struct {
	Processed, Saved, Failed, Repeated int
	CopiedBytes                        int64
}

type Options struct {
	After, Sort, UserAgent, Folder   string
	EntriesLimit, MaxFiles, MinScore int
	Debug, DryRun, AllowSpecialChars bool
	MaxStorage, MaxSize              int64
	OgType                           string
	LogLinksFile                     io.WriteCloser
	LogLinksFormat                   string
	TitleContains, TitleNotContains  *regexp.Regexp
	FlairContains, FlairNotContains  *regexp.Regexp
	LinkContains, LinkNotContains    *regexp.Regexp
	Search                           string
	DownloadPreview                  bool
	PreferPreview                    bool
	PreviewRes                       int
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
	Score             int
	Subreddit, Author string
	LinkFlairText     string
	CreatedUtc        int64
	Preview           struct {
		Images []ImagePreview
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

// On windows, os.Remove() fails unless we close the open file
// For that, we need to keep a reference for signal handler
var outputFile *os.File

// BugFix: with transparent HTTP/2, sometimes reddit servers send HTML instead of JSON
// So create a custom client
var client = http.Client{
	Transport: &http.Transport{
		TLSNextProto: map[string]func(authority string, c *tls.Conn) http.RoundTripper{},
	},
}

var defaultLogLinkFormat = "{{final_url}}"

func pickPreview(choices ImagePreview, width int) *ImagePreviewEntry {
	if width == -1 {
		return &choices.Source
	}
	for _, preview := range choices.Resolutions {
		if preview.Width == width {
			result := preview
			return &result
		}
	}
	return nil
}

func formatFromPost(format string, post *PostData, finalUrl string) string {
	replacer := strings.NewReplacer(
		"{{posted_url}}", post.Url,
		"{{final_url}}", finalUrl,
		"{{subreddit}}", post.Subreddit,
		"{{id}}", strings.TrimPrefix(post.Name, "t3_"),
		"{{author}}", post.Author,
		"{{score}}", strconv.Itoa(post.Score),
		"{{title}}", post.Title,
		"{{quoted_title}}", strconv.Quote(post.Title),
	)
	return replacer.Replace(format)
}

func coalesce(a, b string) string {
	if a == "" {
		return b
	}
	return a
}

func quote(s string) string {
	return strconv.Quote(s)
}

func fatal(val ...interface{}) {
	fmt.Fprintln(os.Stderr, val...)
	os.Exit(1)
}

func eprintln(vals ...interface{}) (int, error) {
	return fmt.Fprintln(os.Stderr, vals...)
}

func eprintf(format string, vals ...interface{}) (int, error) {
	return fmt.Fprintf(os.Stderr, format, vals...)
}

func eprint(vals ...interface{}) (int, error) {
	return fmt.Fprint(os.Stderr, vals...)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
		log(horizontalDashedLine)
		last = post.Data.Name
	}
	log(horizontalDashedLine)
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

	response, err := client.Do(req)

	if err != nil {
		return nil, err
	}
	return response, err
}

func FetchUrl(url string) (*http.Response, error) {
	return FetchUrlWithMethod(url, "GET", "")
}

func padString(s string, min int) string {
	sl := len(s)
	if sl >= min {
		return s
	}
	return s + strings.Repeat(" ", min-sl)
}

// downloads all images reachable from reddit.com/<path>.json
func Traverse(path string, handler func(PostData)) {
	query := url.Values{}

	unsuffixedPath := strings.TrimSuffix(path, "/")
	target := "https://www.reddit.com/" + unsuffixedPath

	// front page URL
	if unsuffixedPath == "" && options.Search == "" && options.Sort == "" {
		options.Sort = "hot"
	}

	after := options.After

	// Handle sort options
	var sortString, timePeriod string
	switch options.Sort {
	case "hot", "new", "rising":
		sortString = options.Sort
	case "top-hour", "top-day", "top-month", "top-year", "top-all":
		sortString = "top"
		timePeriod = strings.TrimPrefix(options.Sort, "top-")
	case "":
		_ = "best" // do nothing
	default:
		fatal("Invalid option passed to sort")
	}

	if options.Search == "" {
		target += "/" + sortString
	} else {
		target += "/search"
		query.Set("sort", sortString)
	}

	query.Set("limit", fmt.Sprint(options.EntriesLimit))

	if timePeriod != "" {
		query.Set("t", timePeriod)
	}

	if options.Search != "" {
		query.Set("q", options.Search)
		query.Set("restrict_sr", "true")
	}

	target += ".json?" + query.Encode()

	for {
		link := target // final link
		if after != "" {
			link += "&after=" + after
		}
		log("Request: ", link)
		response, err := FetchUrlWithMethod(link, "GET", "application/json")
		check(err, "Cannot get JSON response")
		defer response.Body.Close()

		processed := stats.Processed
		after = HandlePosts(response.Body, handler)
		if stats.Processed == processed {
			Finish()
		}
	}
}

func skipByRegexMatch(re *regexp.Regexp, s string) bool {
	if re != nil {
		return re.MatchString(s)
	}
	// if re = nil, don't skip anything
	return false
}

func chooseByRegexMatch(re *regexp.Regexp, s string) bool {
	if re != nil {
		return re.MatchString(s)
	}
	// if re = nil, choose everything
	return true
}

func DownloadPost(post PostData) {
	title := strings.TrimSpace(strings.ReplaceAll(post.Title, "/", "|"))
	title = html.UnescapeString(title) // &amp; etc.. are escaped in json
	if len(title) > 194 {
		title = title[:192] + ".."
	}

	if !chooseByRegexMatch(options.TitleContains, post.Title) {
		log("Title not match regex:", quote(post.Title))
		return
	}

	if !chooseByRegexMatch(options.FlairContains, post.LinkFlairText) {
		log("Flair not match regex:", quote(post.Title), quote(post.LinkFlairText))
		return
	}

	if !chooseByRegexMatch(options.LinkContains, post.Url) {
		log("Link not match regex:", quote(post.Title), post.Url)
		return
	}

	if skipByRegexMatch(options.TitleNotContains, post.Title) {
		log("Title skipped by regex: ", quote(post.Title))
		return
	}

	if skipByRegexMatch(options.FlairNotContains, post.LinkFlairText) {
		log("Flair skipped by regex: ", quote(post.Title), quote(post.LinkFlairText))
		return
	}

	if skipByRegexMatch(options.LinkNotContains, post.Url) {
		log("Posted link skipped by regex: ", quote(post.Title), post.Url)
		return
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

	url := post.Url

	usePreview := func() bool {
		log("Original URL: ", post.Url)
		log("Choosing preview URL")
		if len(post.Preview.Images) == 0 {
			log("No preview found: ", quote(post.Title))
			return false
		}
		preview := pickPreview(post.Preview.Images[0], options.PreviewRes)
		if preview == nil {
			log("No preview found: ", quote(post.Title))
			return false
		}
		url = html.UnescapeString(preview.Url)
		return true
	}

	if options.DownloadPreview {
		if !usePreview() {
			return
		}
	} else if options.PreferPreview {
		usePreview()
	} else {
		// proceed with URL found in the post
	}

	imageUrl, extension := CheckAndResolveImage(url)
	if imageUrl == "" {
		log("Skip non-imagelike entry: ", title, " | ", url)
		return
	}
	if options.LogLinksFile != nil {
		fmt.Fprintln(options.LogLinksFile,
			formatFromPost(options.LogLinksFormat, &post, imageUrl))
	}
	filename := title + " [" + strings.TrimPrefix(post.Name, "t3_") +
		"]" + extension
	filename = removeSpecialChars(filename)
	log("URL: ", url, " | Score:", post.Score)
	if imageUrl != url {
		log("->", imageUrl)
	}

	printName := func() {
		eprintf("\r%-*.*s", terminalColumns-24, terminalColumns-24,
			filename)
	}

	printName()

	// check if already downloaded file
	_, err := os.Stat(filename)
	if err == nil {
		eprint("    [Already Saved]\n")
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
	netError := func(what string) {
		stats.Failed += 1
		eprintf("    [" + what + " Error: " + err.Error() + "]\n")
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
	downloadingFilename = filename
	defer func() {
		downloadingFilename = ""
	}()
	output, err = os.Create(filename)
	if err != nil {
		eprintf(" [Can't create file]\n")
		stats.Failed += 1
		return
	}
	outputFile = output
	defer func() {
		outputFile = nil
		output.Close()
	}()

	maxCharsOnRight := 0

	out := ProgressWriter{Writer: output, Callback: func(i int64) {
		printName()
		progress := fmt.Sprintf("    [%s/%s]", size(i), size(length))
		_n, _ := eprintf("%-*s", maxCharsOnRight, progress)
		maxCharsOnRight = max(_n, maxCharsOnRight)
	}}

	// do a GET request
	fullResponse, err := FetchUrl(imageUrl)
	if err != nil {
		netError("Request ")
		return
	}
	defer fullResponse.Body.Close()

	n, err := io.Copy(&out, fullResponse.Body)
	printName()
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
	done := fmt.Sprintf("    [Complete: %s]\n", size(n))
	eprintf("%-*s", maxCharsOnRight, done)
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
	var titleContains, titleNotContains string
	var flairContains, flairNotContains string
	var linkContains, linkNotContains string

	// option parsing
	flag.BoolVar(&options.Debug, "v", false, "Enable verbose output (devel)")
	flag.BoolVar(&options.DryRun, "d", false, "DryRun i.e just print urls and names (devel)")
	flag.BoolVar(&options.AllowSpecialChars, "allow-special-chars", false,
		"Allow all characters in filenames except / and \\, "+
			"And windows-special filenames like NUL")
	flag.StringVar(&options.After, "after", "", "Get posts after the given ID")
	flag.StringVar(&options.UserAgent, "useragent", UserAgent, "UserAgent string")
	flag.Int64Var(&options.MaxStorage, "max-storage", -1, "Data usage limit in MB, -1 for no limit")
	flag.Int64Var(&options.MaxSize, "max-size", -1, "Max size of media file in KB, -1 for no limit")
	flag.StringVar(&options.Folder, "folder", "", "Target folder name")
	flag.StringVar(&logLinksTo, "log-links", "", "Log media links to given file")
	flag.StringVar(&options.LogLinksFormat, "log-links-format", "{{final_url}}", "Format of links logged. "+
		"allowed placeholders: {{final_url}}, {{posted_url}}, {{id}}, {{author}}, {{title}}, {{score}}")
	flag.StringVar(&options.OgType, "og-type", "", "Look Up for a media link in page's og:property"+
		" if link itself is not image/video (experimental). supported values: video, image, any")
	flag.StringVar(&options.Sort, "sort", "", "Sort: best|hot|new|rising|top-<all|year|month|week|day>")
	flag.IntVar(&options.MaxFiles, "max-files", -1, "Max number of files to download (+ve), -1 for no limit")
	flag.IntVar(&options.MinScore, "min-score", 0, "Minimum score of the post to download")
	flag.IntVar(&options.EntriesLimit, "entries-limit", 100, "Number of entries to fetch in one API request (devel)")

	flag.StringVar(&titleContains, "title-contains", "", "Download if "+
		"title contains substring matching given regex")
	flag.StringVar(&flairContains, "flair-contains", "", "Download if "+
		"flair contains substring matching given regex")
	flag.StringVar(&linkContains, "link-contains", "", "Download if "+
		"posted link contains substring matching given regex")

	flag.StringVar(&titleNotContains, "title-not-contains", "", "Download if "+
		"title does not contain substring matching given regex")
	flag.StringVar(&flairNotContains, "flair-not-contains", "", "Download if "+
		"flair does not contain substring matching given regex")
	flag.StringVar(&linkNotContains, "link-not-contains", "", "Download if "+
		"posted link does not contain substring matching given regex")

	flag.StringVar(&options.Search, "search", "", "Search for given term")
	flag.BoolVar(&options.PreferPreview, "prefer-preview", false,
		"Prefer reddit preview image when possible")
	flag.BoolVar(&options.DownloadPreview, "download-preview", false,
		"download reddit preview image instead of posted URL")
	flag.IntVar(&options.PreviewRes, "preview-res", -1,
		"Width of preview to download, eg: 640, 960, 1080")

	flag.Parse()
	args := flag.Args()
	if len(args) != 1 || help {
		eprintf("Usage: %s <options> <r/subreddit>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	options.LogLinksFile = createLinksFile(logLinksTo)
	defer options.LogLinksFile.Close()

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

	if options.DryRun {
		if options.MaxSize != -1 || options.MaxStorage != -1 {
			fatal("Can't combine image-size based options with dry run")
		}
	}

	if options.PreviewRes > 0 && !options.DownloadPreview &&
		!options.PreferPreview {
		fatal("-download-preview or -prefer-preview should be used with " +
			"-preview-res")
	}

	if options.PreferPreview && options.DownloadPreview {
		fatal("Use only one of -prefer-preview and -download-preview")
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

	regexVals := map[**regexp.Regexp]string{
		&options.TitleContains:    titleContains,
		&options.TitleNotContains: titleNotContains,
		&options.FlairContains:    flairContains,
		&options.FlairNotContains: flairNotContains,
		&options.LinkContains:     linkContains,
		&options.LinkNotContains:  linkNotContains,
	}

	for re, opt := range regexVals {
		if opt != "" {
			*re = regexp.MustCompile(opt)
		}
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

	go Traverse(path, func(post PostData) {
		DownloadPost(post)
	})

	select {
	case <-interrupt:
		eprintln("Interrupt received, Exiting...")
		if outputFile != nil {
			outputFile.Close()
		}
		if downloadingFilename != "" {
			eprintf("Removing possibly incomplete file: '%s'\n", downloadingFilename)
			os.Remove(downloadingFilename)
		}
		PrintStat()
	case <-completion:
		os.Exit(0)
	}
}
