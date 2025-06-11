package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-gst/go-glib/glib"
	"github.com/willi-42/gst-pipeline/gstreamer"
)

func senderPipe(file string, transmCh chan []uint8, withRTP bool) (*gstreamer.Encoder, error) {
	callback := func(encFrame []uint8) {
		// send segment to receiver
		transmCh <- encFrame
	}

	encoder, err := gstreamer.NewEncoder(file, callback, withRTP)
	if err != nil {
		return nil, err
	}

	return encoder, nil
}

func receiverPipe(transmCh chan []uint8, withRTP bool) (*gstreamer.Decoder, error) {
	pullFrameFunction := func() []uint8 {
		newFrame := <-transmCh
		return newFrame
	}

	decoder, err := gstreamer.NewDecoder(pullFrameFunction, withRTP)
	if err != nil {
		return nil, err
	}

	return decoder, nil
}

func main() {
	file := flag.String("file", "", "source file")
	withRTP := flag.Bool("rtp", false, "Encapsulate into rtp packets")
	flag.Parse()

	if *file == "" {
		fmt.Println("No file given!")
		os.Exit(0)
	}

	mainLoop := glib.NewMainLoop(glib.MainContextDefault(), false)

	// channel to connect sender and receiver pipeline
	transmCh := make(chan []uint8, 100)

	// create and run receiver
	decoder, err := receiverPipe(transmCh, *withRTP)
	if err != nil {
		log.Fatal(err)
	}
	go decoder.Run()

	// create and run sender
	encoder, err := senderPipe(*file, transmCh, *withRTP)
	if err != nil {
		log.Fatal(err)
	}
	go encoder.Run()

	// periodic bitrate adaptation
	go func() {
		for {
			time.Sleep(10 * time.Second)
			err = encoder.SetBitrate(100) // kbps
			if err != nil {
				log.Fatal("cannot set bitrate: ", err)
			}
			log.Println("bitrate low")
			time.Sleep(10 * time.Second)
			err = encoder.SetBitrate(1000) // kbps
			if err != nil {
				log.Fatal("cannot set bitrate: ", err)
			}
			log.Println("bitrate high")
		}
	}()

	mainLoop.Run()
}
