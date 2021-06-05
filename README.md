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

## Build
Assuming you have Go toolchain installed

There are no dependencies other than standard library

```
git clone --depth=1 https://github.com/mahesh-hegde/reddit-rip.git
cd reddit-rip
go build
```

your executable is "reddit-rip" or "reddit-rip.exe" depending on OS, saved in same folder.

## Prebuilt binaries

If you cannot build or don't want to build, look for binary releases in [releases section](https://github.com/mahesh-hegde/reddit-rip/releases/)

*Those executables may not be up-to-date.*

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
  -max int
        Max number of files to download (+ve), -1 for no limit (default -1)
  -max-size int
        Max size of media file in KB, -1 for no limit (default -1)
  -max-storage int
        Data usage limit in MB, -1 for no limit (default -1)
  -min-karma int
        Minimum Karma of the post
  -sort string
        Sort: best|hot|new|rising|top-<all|year|month|week|day>
  -useragent string
        UserAgent string (default "reddit-dl / Command Line Tool For Linux")
  -v    Enable verbose output
```

**Note:**

* Pressing Ctrl+C will stop the program only after a file is completely downloaded, in order to avoid saving half-downloaded files. Use -max=<num> to limit number of files

* -d implies -v
    
* do not rely on -max-storage or -max-size to save data
    
* I have not tested all combinations of options, you might discover some bugs !!

## Sample Session

```
$ reddit-rip -sort=top-all -max-size=200 -max=15 r/CoolGuides
Five Demands, Not One Less. End Police Brutality. [gvf93v].png    [Too Large: 270KB]

Which waters to avoid by region [kxlyl4].jpg    [Done: 189KB]

Marginal Tax [e1w58y].jpg    [Done: 98KB]

Price and service comparison of the biggest shipping orgs in the US. [ibhb4l].jpg    [Done: 121KB]

How Masks And Social Distancing Works [hpamws].jpg    [Done: 111KB]

How paint can change a room [g8up0b].jpg    [Done: 110KB]

The history of confederate flags. [hakuc7].jpg    [Done: 81KB]

How to resist [dg83vg].png    [Too Large: 565KB]

U.S. Flag but each star is scaled proportionally to their state’s population, in roughly it’s geographical position. [kyeej0].jpg    [Done: 158KB]

How gerrymandering works [j0s9j5].jpg    [Done: 60KB]

From the US holocaust museum [gwpq6q].jpg    [Done: 75KB]

How to pack a hiking bag [eqxs03].jpg    [Done: 31KB]

Epicurean paradox [g2axoj].jpg    [Too Large: 259KB]

Mind Fuck Movies [j6x8jl].jpg    [Done: 168KB]

Recognizing a Mentally Abused Brain [j4nd29].png    [Too Large: 282KB]

Units of measurement [iehqe2].jpg    [Done: 65KB]

Geography Terms [fj0h8a].jpg    [Too Large: 728KB]

How untreated ADHD causes and traps you in depression [fu1kf2].jpg    [Done: 161KB]

Copper through the patina process [gzb8y5].jpg    [Done: 96KB]

A Restaurant Guide For How You Want Your Steak Cooked [esxk8v].jpg    [Done: 38KB]

--------------------
Processed:  21
Already Downloaded:  0
Failed:  0
Saved:  15
Other:  6
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
