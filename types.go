package main

import (
	"io"
	"regexp"
	"text/template"
)

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
	DataOutputFile                   io.WriteCloser
	DataOutputFormat                 *template.Template
	TemplateFilter                   *template.Template
	PrintPostData                    bool
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

type PostHandler func(post PostData, postMap map[string]any)
