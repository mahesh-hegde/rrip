package main

import "strings"

var windowsSubst = map[rune]string{
	'<':  "&lt;",
	'>':  "&gt;",
	':':  "-",
	'"':  "&quot;",
	'/':  "",
	'\\': "",
	'|':  "",
	'?':  "",
	'*':  "",
}

var minimalSubst = map[rune]string{
	'/':  "",
	'\\': "",
}

var winBan = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true,
	"COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true,
	"LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

func sanitizeWindowsFilename(name string) string {
	name = strings.Trim(name, " .")
	sansExt := strings.SplitN(name, ".", 2)[0]
	if winBan[sansExt] {
		return "__" + name
	}
	if name == "" {
		return "__Blank__"
	}
	return name
}
