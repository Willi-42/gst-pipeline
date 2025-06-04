package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
)

func receiverPipe(transmCh chan []uint8) (*gst.Pipeline, error) {
	gst.Init(nil)

	// Create a pipeline
	pipeline, err := gst.NewPipeline("")
	if err != nil {
		return nil, err
	}

	// Create the elements
	elems, err := gst.NewElementMany("appsrc", "vp8dec", "autovideosink")
	if err != nil {
		return nil, err
	}

	// Add the elements to the pipeline and link them
	pipeline.AddMany(elems...)
	gst.ElementLinkMany(elems...)

	// Get the app sourrce from the first element returned
	src := app.SrcFromElement(elems[0])
	src.SetCaps(gst.NewCapsFromString("video/x-vp8"))

	src.SetCallbacks(&app.SourceCallbacks{
		NeedDataFunc: func(self *app.Source, _ uint) {

			newFrame := <-transmCh
			size := int64(len(newFrame))

			// Create a buffer that can hold exactly one video RGBA frame.
			buffer := gst.NewBufferWithSize(size)

			buffer.Map(gst.MapWrite).WriteData(newFrame)
			defer buffer.Unmap()
			src.PushBuffer(buffer)
		},
	})

	return pipeline, nil
}

func senderPipe(file string, transmCh chan []uint8) (*gst.Pipeline, error) {
	gst.Init(nil)

	// Create a pipeline
	pipeline, err := gst.NewPipeline("")
	if err != nil {
		return nil, err
	}

	src, err := gst.NewElement("filesrc")
	if err != nil {
		return nil, err
	}

	decodebin, err := gst.NewElement("decodebin")
	if err != nil {
		return nil, err
	}

	src.Set("location", file)

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
			fmt.Println("Audi skipped!")

		} else if isVideo {
			sink, err := app.NewAppSink()
			if err != nil {
				return
			}

			// decodebin found a raw videostream, so we build the follow-up pipeline to
			// display it using the autovideosink.
			elements, err := gst.NewElementMany("queue", "videoconvert", "vp8enc")
			if err != nil {
				msg := gst.NewErrorMessage(self, gst.NewGError(2, err), "Could not create elements for video pipeline", nil)
				pipeline.GetPipelineBus().Post(msg)
				return
			}
			pipeline.AddMany(append(elements, sink.Element)...)
			gst.ElementLinkMany(append(elements, sink.Element)...)
			sink.SetCallbacks(&app.SinkCallbacks{
				NewSampleFunc: func(sink *app.Sink) gst.FlowReturn {

					// Pull the sample that triggered this callback
					sample := sink.PullSample()
					if sample == nil {
						return gst.FlowEOS
					}
					buffer := sample.GetBuffer()
					if buffer == nil {
						return gst.FlowError
					}
					samples := buffer.Map(gst.MapRead).AsUint8Slice()
					defer buffer.Unmap()

					// send segment to receiver
					transmCh <- samples // does this copy the array?

					return gst.FlowOK
				},
			})

			// rest is for syncing the elements
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
	file := flag.String("file", "", "source file")
	flag.Parse()

	if *file == "" {
		fmt.Println("No file given!")
		os.Exit(0)
	}

	transmCh := make(chan []uint8, 100)

	sendPipe, err := senderPipe(*file, transmCh)
	if err != nil {
		panic(err)
	}

	recvPipe, err := receiverPipe(transmCh)
	if err != nil {
		panic(err)
	}

	mainLoop := glib.NewMainLoop(glib.MainContextDefault(), false)

	sendPipe.SetState(gst.StatePlaying)
	recvPipe.SetState(gst.StatePlaying)

	mainLoop.Run()
}
