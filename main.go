package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

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
	elems, err := gst.NewElementMany("appsrc", "h264parse", "avdec_h264", "autovideosink")
	if err != nil {
		return nil, err
	}

	// Add the elements to the pipeline and link them
	pipeline.AddMany(elems...)
	gst.ElementLinkMany(elems...)

	// Get the app sourrce from the first element returned
	src := app.SrcFromElement(elems[0])
	src.SetCaps(gst.NewCapsFromString("video/x-h264,stream-format=byte-stream"))

	src.SetCallbacks(&app.SourceCallbacks{
		NeedDataFunc: func(self *app.Source, _ uint) {

			newFrame := <-transmCh
			size := int64(len(newFrame))
			buffer := gst.NewBufferWithSize(size)

			buffer.Map(gst.MapWrite).WriteData(newFrame)
			defer buffer.Unmap()
			src.PushBuffer(buffer)
		},
	})

	return pipeline, nil
}

func createEncoder() *gst.Element {
	settings := map[string]interface{}{"tune": 0x00000004} // tune: zero_latency

	encoder, err := gst.NewElementWithProperties("x264enc", settings)
	if err != nil {
		log.Fatal("cannot create encoder: ", err)
	}

	err = encoder.Set("bitrate", uint(500)) // kbps
	if err != nil {
		log.Fatal("cannot set bitrate: ", err)
	}

	// sliced-threads for better performance
	err = encoder.Set("sliced-threads", true)
	if err != nil {
		log.Fatal("cannot set bitrate: ", err)
	}

	return encoder
}

func senderPipe(file string, transmCh chan []uint8) (*gst.Pipeline, *gst.Element, error) {
	gst.Init(nil)

	// Create a pipeline
	pipeline, err := gst.NewPipeline("")
	if err != nil {
		return nil, nil, err
	}

	src, err := gst.NewElement("filesrc")
	if err != nil {
		return nil, nil, err
	}

	decodebin, err := gst.NewElement("decodebin")
	if err != nil {
		return nil, nil, err
	}

	src.Set("location", file)

	pipeline.AddMany(src, decodebin)
	src.Link(decodebin)

	// create ecnoder here, so we can ref it
	encoder := createEncoder()

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
			elements, err := gst.NewElementMany("queue", "videoconvert")
			if err != nil {
				msg := gst.NewErrorMessage(self, gst.NewGError(2, err), "Could not create elements for video pipeline", nil)
				pipeline.GetPipelineBus().Post(msg)
				return
			}

			pipeline.AddMany(append(elements, encoder, sink.Element)...)
			gst.ElementLinkMany(append(elements, encoder, sink.Element)...)
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

	return pipeline, encoder, nil
}

func main() {
	file := flag.String("file", "", "source file")
	flag.Parse()

	if *file == "" {
		fmt.Println("No file given!")
		os.Exit(0)
	}

	// channel to connect sender and receiver pipeline
	transmCh := make(chan []uint8, 100)

	sendPipe, videoEncoder, err := senderPipe(*file, transmCh)
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

	// periodic bitrate adaptation
	go func() {
		for {
			time.Sleep(10 * time.Second)
			err = videoEncoder.Set("bitrate", uint(100)) // kbps
			if err != nil {
				log.Fatal("cannot set bitrate: ", err)
			}
			log.Println("bitrate low")
			time.Sleep(10 * time.Second)
			err = videoEncoder.Set("bitrate", uint(1000)) // kbps
			if err != nil {
				log.Fatal("cannot set bitrate: ", err)
			}
			log.Println("bitrate high")
		}
	}()

	mainLoop.Run()
}
