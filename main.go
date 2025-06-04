package main

import (
	"errors"
	"fmt"
	"strings"

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

	location := "/Users/fabianfromwald/private/91_copy_usb/F/Videos/Parachuteing_Video.mp4"
	src, err := gst.NewElement("filesrc")
	if err != nil {
		return nil, err
	}

	decodebin, err := gst.NewElement("decodebin")
	if err != nil {
		return nil, err
	}

	src.Set("location", location)

	pipeline.AddMany(src, decodebin)
	src.Link(decodebin)

	decodebin.Connect("pad-added", func(self *gst.Element, srcPad *gst.Pad) {

		// Try to detect whether this is video or audio
		var isAudio, isVideo bool
		caps := srcPad.GetCurrentCaps()
		for i := 0; i < caps.GetSize(); i++ {
			st := caps.GetStructureAt(i)
			if strings.HasPrefix(st.Name(), "audio/") {
				isAudio = true
			}
			if strings.HasPrefix(st.Name(), "video/") {
				isVideo = true
			}
		}

		fmt.Printf("New pad added, is_audio=%v, is_video=%v\n", isAudio, isVideo)

		if !isAudio && !isVideo {
			err := errors.New("could not detect media stream type")
			// We can send errors directly to the pipeline bus if they occur.
			// These will be handled downstream.
			msg := gst.NewErrorMessage(self, gst.NewGError(1, err), fmt.Sprintf("Received caps: %s", caps.String()), nil)
			pipeline.GetPipelineBus().Post(msg)
			return
		}

		if isAudio {
			// decodebin found a raw audiostream, so we build the follow-up pipeline to
			// play it on the default audio playback device (using autoaudiosink).
			elements, err := gst.NewElementMany("queue", "audioconvert", "audioresample", "autoaudiosink")
			if err != nil {
				// We can create custom errors (with optional structures) and send them to the pipeline bus.
				// The first argument reflects the source of the error, the second is the error itself, followed by a debug string.
				msg := gst.NewErrorMessage(self, gst.NewGError(2, err), "Could not create elements for audio pipeline", nil)
				pipeline.GetPipelineBus().Post(msg)
				return
			}
			pipeline.AddMany(elements...)
			gst.ElementLinkMany(elements...)

			// !!ATTENTION!!:
			// This is quite important and people forget it often. Without making sure that
			// the new elements have the same state as the pipeline, things will fail later.
			// They would still be in Null state and can't process data.
			for _, e := range elements {
				e.SyncStateWithParent()
			}

			// The queue was the first element returned above
			queue := elements[0]
			// Get the queue element's sink pad and link the decodebin's newly created
			// src pad for the audio stream to it.
			sinkPad := queue.GetStaticPad("sink")
			srcPad.Link(sinkPad)

		} else if isVideo {
			// decodebin found a raw videostream, so we build the follow-up pipeline to
			// display it using the autovideosink.
			elements, err := gst.NewElementMany("queue", "videoconvert", "videoscale", "autovideosink")
			if err != nil {
				msg := gst.NewErrorMessage(self, gst.NewGError(2, err), "Could not create elements for video pipeline", nil)
				pipeline.GetPipelineBus().Post(msg)
				return
			}
			pipeline.AddMany(elements...)
			gst.ElementLinkMany(elements...)

			for _, e := range elements {
				e.SyncStateWithParent()
			}

			queue := elements[0]
			// Get the queue element's sink pad and link the decodebin's newly created
			// src pad for the video stream to it.
			sinkPad := queue.GetStaticPad("sink")
			srcPad.Link(sinkPad)
		}
	})

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
