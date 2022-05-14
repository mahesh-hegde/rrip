## rrip

Program to bulk-download image from reddit subreddits.

## Features

* Set max size of file, max total size, minimum score etc..

* Filter by post title or link using regular expression.

* If the image / GIF is already downloaded in same folder, skip it.

* Log final download URLs to a file using a custom format string.

* Download images from Reddit preview links instead of source. (Experimental)

* Scrape images from links that don't end with media extensions. (Experimental)

(Note: I have not tested all combinations of features, you might encounter some bugs!)

## Build
Assuming you have Go toolchain installed

```
go install github.com/mahesh-hegde/rrip@latest
```

or

```
git clone --depth=1 https://github.com/mahesh-hegde/rrip.git
cd rrip
go build && go install
```

or

Download from Release section and unpack the binary executable somewhere in your `PATH`

## Note about windows
I wrote this on Linux. May not work well on Windows. A best-effort default option is enabled to sanitize filenames so that they can be saved on Windows / Android. But don't blame me if you face some quirks of Windows OS. 

## Usage
```
Usage: rrip <options> <r/subreddit>
  -after string
        Get posts after the given ID
  -allow-special-chars
        Allow all characters in filenames except / and \, And windows-special filenames like NUL
  -d    DryRun i.e just print urls and names (devel)
  -download-preview
        download reddit preview image instead of posted URL
  -entries-limit int
        Number of entries to fetch in one API request (devel) (default 100)
  -flair-contains string
        Download if flair contains substring matching given regex
  -flair-not-contains string
        Download if flair does not contain substring matching given regex
  -folder string
        Target folder name
  -help
        Show this help message
  -link-contains string
        Download if posted link contains substring matching given regex
  -link-not-contains string
        Download if posted link does not contain substring matching given regex
  -log-links string
        Log media links to given file
  -log-links-format string
        Format of links logged. allowed placeholders: {{final_url}}, {{posted_url}}, {{id}}, {{author}}, {{title}}, {{score}} (default "{{final_url}}")
  -max-files int
        Max number of files to download (+ve), -1 for no limit (default -1)
  -max-size int
        Max size of media file in KB, -1 for no limit (default -1)
  -max-storage int
        Data usage limit in MB, -1 for no limit (default -1)
  -min-score int
        Minimum score of the post to download
  -og-type string
        Look Up for a media link in page's og:property if link itself is not image/video (experimental). supported values: video, image, any
  -preview-res int
        Width of preview to download, eg: 640, 960, 1080 (default -1)
  -search string
        Search for given term
  -sort string
        Sort: best|hot|new|rising|top-<all|year|month|week|day>
  -title-contains string
        Download if title contains substring matching given regex
  -title-not-contains string
        Download if title does not contain substring matching given regex
  -useragent string
        UserAgent string (default "rrip / Go CLI Tool")
  -v    Enable verbose output (devel)
```

## tl;dr

```sh
## Download only 200KB+ files from r/Wallpaper
rrip -max-size=200 r/Wallpaper

## Download all time top from r/WildLifePhotography, without exceeding 20MB storage or 50 files
rrip -max-storage=20 -max-files=50 -sort=top-all r/WildlifePhotography

## Search "Neon" on r/AMOLEDBackgrounds and download top 20, sorted by top voted in past one year
rrip -search="Neon" -max-files=20 r/AMOLEDBackgrounds

## Download memes from r/LogicGateMemes, download reddit previews (640p)
## instead of original image, for space savings.
## Also log all image links to file called meme.txt along with title

## Note that -preview-res cannot be arbitrary
## Ones that generally work are 1080, 960, 640, 360, 216, 108
## If no suitable preview is found, image won't be downloaded

## use -prefer-preview instead of -download-preview 
## to download original URL if no preview could be found

rrip -download-preview -preview-res=640 -log-links=meme.txt -log-links-format="{{final_url}} {{title}}" r/LogicGateMemes

## Log all image links from r/ImaginaryLandscape
## without downloading files, using -d (dry run) option.
## (Reddit shows last 600 or so.., not really "all")
rrip -d -log-links=imaginary_landscapes.txt -log-links-format="{{score}} {{final_url}} {{quoted_title}} {{author}}" r/ImaginaryLandscapes
```

## Caveats
* Can't handle crossposts etc.. when downloading preview image.
* No support for downloading albums.
* Terminal size detection works only on linux
* Some options don't work together
* Many other caveats I don't remember.

