package gstreamer

import (
	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
)

type PullFrameFunction func() (encodedFrame []uint8)

type Decoder struct {
	pipeline *gst.Pipeline
}

func NewDecoder(pullframe PullFrameFunction, withRTP, hideVideo bool) (*Decoder, error) {
	gst.Init(nil)

	// Create a pipeline
	pipeline, err := gst.NewPipeline("")
	if err != nil {
		return nil, err
	}

	sinkName := "autovideosink"
	if hideVideo {
		sinkName = "fakesink"
	}

	// Create the elements
	var elems []*gst.Element
	if withRTP {
		elems, err = gst.NewElementMany("appsrc", "rtpjitterbuffer", "rtph264depay", "avdec_h264", "videoconvert", sinkName)
		if err != nil {
			return nil, err
		}

	} else {
		elems, err = gst.NewElementMany("appsrc", "h264parse", "avdec_h264", "videoconvert", sinkName)
		if err != nil {
			return nil, err
		}
	}

	// Add the elements to the pipeline and link them
	pipeline.AddMany(elems...)
	gst.ElementLinkMany(elems...)

	// Get the app sourrce from the first element returned
	src := app.SrcFromElement(elems[0])

	if withRTP {
		src.SetCaps(gst.NewCapsFromString("application/x-rtp, clock-rate=90000,media=video, encoding-name=H264"))
	} else {
		src.SetCaps(gst.NewCapsFromString("video/x-h264,stream-format=byte-stream"))
	}

	src.SetCallbacks(&app.SourceCallbacks{
		NeedDataFunc: func(self *app.Source, _ uint) {

			// pull new frame
			newFrame := pullframe()

			size := int64(len(newFrame))
			buffer := gst.NewBufferWithSize(size)

			buffer.Map(gst.MapWrite).WriteData(newFrame)
			defer buffer.Unmap()
			src.PushBuffer(buffer)
		},
	})

	return &Decoder{pipeline: pipeline}, nil
}

// Run starts the gstreamer decoder pipeline in the same process
func (d *Decoder) Run() error {
	err := d.pipeline.SetState(gst.StatePlaying)
	if err != nil {
		return err
	}

	mainLoop := glib.NewMainLoop(glib.MainContextDefault(), false)
	mainLoop.Run()

	return nil
}
