package gstreamer

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
)

const MAX_MTU = 1390
const START_ECNODER_RATE = uint(2000) // kbps

type EncoderCallback func(encodedFrame []uint8)

type Encoder struct {
	pipeline       *gst.Pipeline
	encoderElement *gst.Element
}

func createGstEncoderElm() *gst.Element {
	settings := map[string]interface{}{"tune": 0x00000004} // tune: zero_latency

	encoder, err := gst.NewElementWithProperties("x264enc", settings)
	if err != nil {
		log.Fatal("cannot create encoder: ", err)
	}

	err = encoder.Set("bitrate", START_ECNODER_RATE)
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

func NewEncoder(filename string, callback EncoderCallback, withRTP bool) (*Encoder, error) {
	// check if given file exists
	if _, err := os.Stat(filename); errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

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

	// We need the decodebin to know what file we received
	decodebin, err := gst.NewElement("decodebin")
	if err != nil {
		return nil, err
	}

	src.Set("location", filename)

	pipeline.AddMany(src, decodebin)
	src.Link(decodebin)

	// create ecnoder here, so we can ref it
	gstEncoder := createGstEncoderElm()

	// wait for decodebin to receive the first pad and then create rest of pipeline
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
			elements, err := gst.NewElementMany("clocksync", "queue")
			if err != nil {
				msg := gst.NewErrorMessage(self, gst.NewGError(2, err), "Could not create elements for video pipeline", nil)
				pipeline.GetPipelineBus().Post(msg)
				return
			}

			var allElements []*gst.Element
			if withRTP {
				// RTP encapsuling with no aggregation
				rtpEncapuler, err := gst.NewElementWithProperties("rtph264pay", map[string]interface{}{"aggregate-mode": 0, "mtu": MAX_MTU})
				if err != nil {
					msg := gst.NewErrorMessage(self, gst.NewGError(2, err), "Could not create elements for video pipeline", nil)
					pipeline.GetPipelineBus().Post(msg)
				}
				allElements = append(elements, gstEncoder, rtpEncapuler, sink.Element)
			} else {
				// no encapsuling
				allElements = append(elements, gstEncoder, sink.Element)
			}

			pipeline.AddMany(allElements...)
			gst.ElementLinkMany(allElements...)
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
					callback(samples)

					return gst.FlowOK
				},
			})

			// rest is for syncing the elements
			for _, e := range elements {
				e.SyncStateWithParent()
			}

			elmAfterDecodebin := elements[0]
			// Get the queue element's sink pad and link the decodebin's newly created
			// src pad for the video stream to it.
			sinkPad := elmAfterDecodebin.GetStaticPad("sink")
			srcPad.Link(sinkPad)

		}
	})

	return &Encoder{pipeline: pipeline, encoderElement: gstEncoder}, nil
}

// Run starts the gstreamer encoder pipeline in the same process
func (e *Encoder) Run() error {
	err := e.pipeline.SetState(gst.StatePlaying)
	if err != nil {
		return err
	}

	mainLoop := glib.NewMainLoop(glib.MainContextDefault(), false)
	mainLoop.Run()

	return nil
}

func (e *Encoder) SetBitrate(rateKbps uint) error {
	return e.encoderElement.Set("bitrate", rateKbps)
}
