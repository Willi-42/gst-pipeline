package main

import (
	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/gst"
)

func tutorialPipe() (*gst.Pipeline, error) {
	gst.Init(nil)

	// Create a pipeline
	pipeline, err := gst.NewPipeline("")
	if err != nil {
		return nil, err
	}

	allElements := []*gst.Element{}
	location := "/home/willi/Documents/gr/ts-test/video_test_files/sintel.720p.mkv"
	srcElm, err := gst.NewElement("filesrc")
	if err != nil {
		return nil, err
	}
	// srcElm.Set("location", location)
	srcElm.SetProperty("location", location)
	allElements = append(allElements, srcElm)

	elems, err := gst.NewElementMany("decodebin", "videoconvert", "autovideosink")
	if err != nil {
		return nil, err
	}
	allElements = append(allElements, elems...)

	// Add the elements to the pipeline and link them
	pipeline.AddMany(allElements...)
	gst.ElementLinkMany(allElements...)

	return pipeline, nil
}

func main() {
	pipe, err := tutorialPipe()
	if err != nil {
		panic(err)
	}

	mainLoop := glib.NewMainLoop(glib.MainContextDefault(), false)

	pipe.SetState(gst.StatePlaying)

	mainLoop.Run()
}
