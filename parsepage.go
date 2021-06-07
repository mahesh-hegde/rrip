// functions to parse html webpage and get a url

package main

import (
	"io"
	"errors"
	"golang.org/x/net/html"
)

func NextMetaTag(tok *html.Tokenizer) (html.Token, error) {
	for {
		tt := tok.Next()
		switch tt {
			case html.ErrorToken:
				return html.Token{}, tok.Err()
			case html.SelfClosingTagToken, html.StartTagToken:
				token := tok.Token()
				if token.Data == "meta" {
					return token, nil
				}
				if token.Data == "body" {
					return token, io.EOF
				}
			default:
				continue;
		}
	}
}

func AttrValue(token html.Token, ns, key string) string {
	for _, attr := range token.Attr {
		if attr.Namespace == ns && attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// ogType can be "video", "image", or "any"

func GetOgUrl(source io.Reader, config *Config) (string, error) {
	tokenizer := html.NewTokenizer(source)
	reqProp := "og:" + config.OgType
	if config.OgType == "any" {
		reqProp = "og:video"
	}
	for {
		metaTag, err := NextMetaTag(tokenizer)
		if err == io.EOF {
			return "", nil
		}
		// unknown error
		if err != nil {
			return "", errors.New("Error Parsing HTML" + err.Error())
		}
		prop := AttrValue(metaTag, "", "property")
		if prop == reqProp ||
				prop == "og:image" && config.OgType == "any" {
			link := AttrValue(metaTag, "", "content")
			return link, nil
		}
		// if any type is preferred, try again with other type
	}
}

