# rrip - Bulk-download images from subreddits

Program to bulk-download image from reddit subreddits.

## Features

* Set max size of file, max total size, minimum score etc..

* Filter by post title or link using regular expression.

* If the image / GIF is already downloaded in same folder, skip it.

* Log final download URLs to a file using a custom format string.

* Download images from Reddit preview links instead of source, saving some space.

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
Invoke `rrip` without arguments for up-to-date usage output.

## tl;dr

```sh
## Download only <200KB files from r/Wallpaper
rrip --max-size=200 r/Wallpaper

## Download all time top from r/WildLifePhotography, without exceeding 20MB storage or 50 files
rrip --max-storage=20 --max-files=50 --sort=top-all r/WildlifePhotography

## Search "Neon" on r/AMOLEDBackgrounds and download top 20, sorted by top voted in past one year
rrip --search="Neon" --max-files=20 --sort=top-year r/AMOLEDBackgrounds

## Download memes from r/LogicGateMemes, download reddit previews (640p)
## instead of original image, for space savings.
## Also log all image links to file called meme.txt along with title

## Note that -preview-res cannot be arbitrary
## Ones that generally work are 1080, 960, 640, 360, 216, 108
## If no suitable preview is found, image won't be downloaded

## use -prefer-preview instead of -download-preview 
## to download original URL if no preview could be found

rrip --download-preview --preview-res=640 --data-output-file=meme.txt --data-output-format="{{.final_url}} {{.title}}" r/LogicGateMemes

## Log all image links from r/ImaginaryLandscape
## without downloading files, using -d (dry run) option.
## (Reddit shows last 600 or so.., not really "all")
rrip -d --data-output-file=imaginary_landscapes.txt --data-output-format="{{.score}} {{.final_url}} {{.quoted_title}} {{.author}}" r/ImaginaryLandscapes
```

### Using template options
Go `text/template` syntax can be used to do versatile filtering. It can also be used to do formatting of logged links.

```sh
## Inspect the JSON of post using --print-post-data
rrip --print-post-data --max-files=1 r/AMOLEDBackgrounds

## After inspecting the JSON, you can use the field values in `-template-filter` to filter based on any attribute.
## If the template evaluates to "false", "", or "0", the post will be skipped by rrip

## Example: only download gilded posts
rrip --template-filter='{{gt .gilded 0.0}}' --max-files=20 --sort=top-y
ear r/AMOLEDBackgrounds

## Example: only download posts by a given author, say u/temporary_08
rrip --template-filter='{{eq .author "temporary_08"}}' --max-files=20  r/AMOLEDBackgrounds

## Example: skip potentially unsafe content
rrip --template-filter='{{not .over_18}}' --max-files=20  r/AMOLEDBackgrounds

## Example: Log links to a file with author, upvote ratio, and quoted title.
## Use dry run (-d) to skip download
rrip -d --data-output-file=amoled.txt --data-output-format='{{.upvote_ratio}} {{.author}} {{.quoted_title}}' r/AMOLEDBackgrounds
```

## Caveats
* Can't handle crossposts when downloading preview image.
* No support for downloading albums.
* Some options don't work together
* Many other caveats I don't remember.
