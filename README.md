# gst-pipeline
Simple gst-pipeline for streaming from a source file. Folder `gstreamer` contains a usable go-module.
Videos are encoded with h264.


## Run example

```shell
go build
```
Run the test app, execute the following. Use `-rtp` to run the RTP pipeline. 
```shell
./gst-test -file <path_to_video_file>
```
