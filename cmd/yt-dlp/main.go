package main

import (
	"os"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/cli"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/extractor"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/extractor/generic"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/extractor/youtube"
)

func main() {
	var ies []extractor.IE
	ies = append(ies, youtube.NewYoutubeIE())
	ies = append(ies, youtube.NewYoutubePlaylistIE())
	ies = append(ies, generic.NewGenericIE())
	os.Exit(cli.Main(ies, os.Args[1:]))
}
