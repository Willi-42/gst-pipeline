# gst-pipeline
Simple gst-pipeline for streaming from a source file. Folder `gstreamer` contains a usable go-module.
Videos are encoded with h264.


## Run example

To run the example, execute the following. Use `-rtp` to run the RTP pipeline. 
```shell
go run main.go -file <path_to_video_file>
```
