## rip for Reddit

It's a simple image downloader CLI tool for reddit

It downloads images from URLs that look like images

It has a few configurable options as well

## Features
(Note: I have not tested all combinations of features, you might encounter some bugs!)

* Set max size limit for media download (in KBs)

* Fetch top / best / new / rising entries using -sort option

* Assign a karma threshold (default 1)

* If the image / gif is already downloaded, skip it.

* Can log final URLs to a file.

* Primitive support for scraping pages that don't end with media extensions, using -og-type flag.

## Build
Assuming you have Go toolchain installed

```
go install github.com/mahesh-hegde/reddit-rip
```

or

```
git clone --depth=1 https://github.com/mahesh-hegde/reddit-rip.git
cd reddit-rip
go build
```

your executable is "reddit-rip" or "reddit-rip.exe" depending on OS, saved in same folder.

## Note about windows
I wrote this on Linux. May not work well on Windows. A best-effort default option is enabled to sanitize filenames so that they can be saved on Windows / Android. But don't blame me if you face some quirks of Windows OS. 

## Usage
```
Usage: reddit-rip <options> <r/subreddit>
  -after string
        Get posts after the given ID
  -d    DryRun i.e just print urls and names
  -folder string
        Target folder name
  -help
        Show this help message
  -l int
        Number of entries to fetch in one API request (devel) (default 100)
  -log-media-links string
        Log media links to given file
  -log-post-links string
        Log all links found in posts to given file
  -max int
        Max number of files to download (+ve), -1 for no limit (default -1)
  -max-size int
        Max size of media file in KB, -1 for no limit (default -1)
  -max-storage int
        Data usage limit in MB, -1 for no limit (default -1)
  -min-karma int
        Minimum Karma of the post
  -og-type string
        Look Up for a media link in page's og:property if link itself is not image/video (experimental) supported: video, image, any
  -sort string
        Sort: best|hot|new|rising|top-<all|year|month|week|day>
  -special-chars
        Allow all characters in filenames except / and \, And windows-special filenames like NUL
  -useragent string
        UserAgent string (default "rip for Reddit / Command Line Tool")
  -v    Enable verbose output
```

**Note:**

* -d implies -v

* use -log-media-links=filename option with -d (dry run) if you just want to log media URLs and not download them.

* some sites like gfycat don't provide downloadable URLs directly, you might try passing -og-type=video for example, so that the program will try to scrape "og:video" property from the link. Supported options are video, image, or any (first try to find og:video, or fallback to og:image)

* I have not tested all combinations of options, you might discover some bugs !!

## Sample Session

```
$ reddit-rip -max-size=600 -max=20 r/LogicGateMemes              Logic Gates [ffbsit].jpg    [Done: 121.0KB]

Close enough [es6njb].jpg    [Done: 51.2KB]

Well, technically… [er9r6y].jpg    [Done: 158.5KB]

Naming Conventions [epqho9].jpg    [Done: 98.8KB]

Elon Mux [ekysjs].jpg    [Done: 6.2KB]

NOR Flag [ekm2ms].jpg    [Done: 187.0KB]

We have to prepare boys [ekc3d8].png    [Done: 483.2KB]

Romeo and Juliet [eh1m2j].jpg    [Done: 143.0KB]

That extra input is important! [dztgzg].jpg    [Done: 137.9KB]

Hmmmm [d8ge34].jpg    [Done: 42.5KB]

Important Difference [d5wn3e].jpg    [Done: 130.6KB]

What boolean algebraists sound like when they sleep [d4og58].png    [Done: 431.2KB]

If Kira were a boolean algebraist [d4rpim].jpg    [Done: 42.9KB]

high quality facebook meme [d4hl21].png    [Done: 260.3KB]

Classic Thrash Logic [d3t5g3].jpg    [Done: 56.9KB]

The first logic gates [d2nr07].png    [Done: 378.6KB]

This made me giggle... [d1mefi].png    [Done: 61.2KB]

Computer logic! [cwwjs4].jpg    [Done: 152.5KB]

INvestiGATED, and i dont like my meme [ctj56l].png    [Too Large: 1.2MB]

I hope you all appreciate this [bseze4].png    [Done: 87.1KB]

logicdroids  [bs09g3].png    [Done: 211.6KB]

--------------------
Processed Posts:  29
Already Downloaded:  0
Failed:  0
Saved:  20
Other:  9
--------------------
Approx. Storage Used: 3.2MB
--------------------
```

## Another

```
$ reddit-rip -sort=top-all -min-karma=6000 -max-size=600 -max=20 r/PhysicsMemes
Island of stability where [jq6t10].gif    [Too Large: 31MB]

made with paint [kncao1].png    [Too Large: 638KB]

I finally saw it irl :D [mkfvna].png    [Too Large: 918KB]

Made during chemistry class. [k5utnz].jpg    [Done: 51KB]

how to thought experiment [l2n9bw].jpg    [Done: 42KB]

Thermal Physics test tomorrow. Wish me luck. [cv5rgt].jpg    [Done: 235KB]

physics major [kjy78w].jpg    [Done: 17KB]

6 marks [imdell].png    [Too Large: 2MB]

Organic Chemistry books are basically a portfolio of hexagons [epu5ep].jpg    [Done: 87KB]


Skipping posts with less points, since sort=top-all
--------------------
Processed:  10
Already Downloaded:  0
Failed:  0
Saved:  5
Other:  5
--------------------
```
