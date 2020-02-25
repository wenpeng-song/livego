package main

import (
	"flag"
	"log"
	"net"
	"time"

	"github.com/gwuhaolin/livego/configure"
	"github.com/gwuhaolin/livego/protocol/hls"
	"github.com/gwuhaolin/livego/protocol/httpflv"
	"github.com/gwuhaolin/livego/protocol/httpopera"
	"github.com/gwuhaolin/livego/protocol/rtmp"
)

var (
	version        = "master"
	rtmpAddr       = flag.String("rtmp-addr", ":1935", "RTMP server listen address")
	httpFlvAddr    = flag.String("httpflv-addr", ":7001", "HTTP-FLV server listen address")
	hlsAddr        = flag.String("hls-addr", ":7002", "HLS server listen address")
	operaAddr      = flag.String("manage-addr", ":8090", "HTTP manage interface server listen address")
	configfilename = flag.String("cfgfile", ".livego.json", "configure filename")
)

func init() {
	log.SetFlags(log.Lshortfile | log.Ltime | log.Ldate)
	flag.Parse()
}

func startHls() *hls.Server {
	hlsListen, err := net.Listen("tcp", *hlsAddr)
	if err != nil {
		log.Fatal(err)
	}

	hlsServer := hls.NewServer()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Println("HLS server panic: ", r)
			}
		}()
		log.Println("HLS listen On", *hlsAddr)
		hlsServer.Serve(hlsListen)
	}()
	return hlsServer
}

func startRtmp(stream *rtmp.RtmpStream, hlsServer *hls.Server) {
	rtmpListen, err := net.Listen("tcp", *rtmpAddr)
	if err != nil {
		log.Fatal(err)
	}

	var rtmpServer *rtmp.Server

	if hlsServer == nil {
		rtmpServer = rtmp.NewRtmpServer(stream, nil)
		log.Printf("hls server disable....")
	} else {
		rtmpServer = rtmp.NewRtmpServer(stream, hlsServer)
		log.Printf("hls server enable....")
	}

	defer func() {
		if r := recover(); r != nil {
			log.Println("RTMP server panic: ", r)
		}
	}()
	log.Println("RTMP Listen On", *rtmpAddr)
	rtmpServer.Serve(rtmpListen)
}

func startHTTPFlv(stream *rtmp.RtmpStream) {
	flvListen, err := net.Listen("tcp", *httpFlvAddr)
	if err != nil {
		log.Fatal(err)
	}

	hdlServer := httpflv.NewServer(stream)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Println("HTTP-FLV server panic: ", r)
			}
		}()
		log.Println("HTTP-FLV listen On", *httpFlvAddr)
		hdlServer.Serve(flvListen)
	}()
}

func startHTTPOpera(stream *rtmp.RtmpStream) {
	if *operaAddr != "" {
		opListen, err := net.Listen("tcp", *operaAddr)
		if err != nil {
			log.Fatal(err)
		}
		opServer := httpopera.NewServer(stream, *rtmpAddr)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Println("HTTP-Operation server panic: ", r)
				}
			}()
			log.Println("HTTP-Operation listen On", *operaAddr)
			opServer.Serve(opListen)
		}()
	}
}

// func noNameReturn(x, y int) (ret int) {
// 	z := x*y
// 	return z
// }

func main() {

	/* golang try
	var x int
	x = 3
	_ = x
	s := []int{1, 2, 3}     // Result is a slice of length 3.
	s2 := []int {4, 5, 6}
	s2 = append(s, s2...)
	s = append(s, []int{4, 5, 6}...)
	s2 = append(s2, 4, 5, 6);
	m := map[string]int{"a":1, "b":2, "c":3}
	fmt.Println(m);
	_ = make([]int, 20)
	a := [4]int {}
	_ = a
	_ = s2
	file, _ := os.Create("output.txt")
	fmt.Fprint(file, "This is how you write to a file, by the way")
	file.Close()
*/
	defer func() {
		if r := recover(); r != nil {
			log.Println("livego panic: ", r)
			time.Sleep(1 * time.Second)
		}
	}()
	log.Println("start livego, version", version)
	err := configure.LoadConfig(*configfilename)
	if err != nil {
		return
	}

	stream := rtmp.NewRtmpStream()
	// hlsServer := startHls()
	startHTTPFlv(stream)
	// startHTTPOpera(stream)

	// startRtmp(stream, hlsServer)
	startRtmp(stream, nil)
}
