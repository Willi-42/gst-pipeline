package gstreamer

import (
	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
)

type PullFrameFunction func() []uint8

type Decoder struct {
	pipeline *gst.Pipeline
}

func NewDecoder(pullframe PullFrameFunction) (*Decoder, error) {
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

func (d *Decoder) Run() error {
	err := d.pipeline.SetState(gst.StatePlaying)
	if err != nil {
		return err
	}

	mainLoop := glib.NewMainLoop(glib.MainContextDefault(), false)
	mainLoop.Run()

	return nil
}
