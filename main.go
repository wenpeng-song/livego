package main

import (
	"net"
	"time"

	"github.com/gwuhaolin/livego/configure"
	"github.com/gwuhaolin/livego/protocol/api"
	"github.com/gwuhaolin/livego/protocol/hls"
	"github.com/gwuhaolin/livego/protocol/httpflv"
	"github.com/gwuhaolin/livego/protocol/rtmp"

	"github.com/gogap/logrus_mate"
	_ "github.com/gogap/logrus_mate/hooks/file"
	log "github.com/sirupsen/logrus"
)

var VERSION = "master"

func startHls() *hls.Server {
	hlsAddr := configure.Config.GetString("hls_addr")
	hlsListen, err := net.Listen("tcp", hlsAddr)
	if err != nil {
		log.Fatal(err)
	}

	hlsServer := hls.NewServer()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("HLS server panic: ", r)
			}
		}()
		log.Info("HLS listen On ", hlsAddr)
		hlsServer.Serve(hlsListen)
	}()
	return hlsServer
}

var rtmpAddr string

func startRtmp(stream *rtmp.RtmpStream, hlsServer *hls.Server) {
	rtmpAddr = configure.Config.GetString("rtmp_addr")

	rtmpListen, err := net.Listen("tcp", rtmpAddr)
	if err != nil {
		log.Fatal(err)
	}

	var rtmpServer *rtmp.Server

	if hlsServer == nil {
		rtmpServer = rtmp.NewRtmpServer(stream, nil)
		log.Info("HLS server disable....")
	} else {
		rtmpServer = rtmp.NewRtmpServer(stream, hlsServer)
		log.Info("HLS server enable....")
	}

	defer func() {
		if r := recover(); r != nil {
			log.Error("RTMP server panic: ", r)
		}
	}()
	log.Info("RTMP Listen On ", rtmpAddr)
	rtmpServer.Serve(rtmpListen)
}

func startHTTPFlv(stream *rtmp.RtmpStream) {
	httpflvAddr := configure.Config.GetString("httpflv_addr")

	flvListen, err := net.Listen("tcp", httpflvAddr)
	if err != nil {
		log.Fatal(err)
	}

	hdlServer := httpflv.NewServer(stream)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("HTTP-FLV server panic: ", r)
			}
		}()
		log.Info("HTTP-FLV listen On ", httpflvAddr)
		hdlServer.Serve(flvListen)
	}()
}

func startAPI(stream *rtmp.RtmpStream) {
	apiAddr := configure.Config.GetString("api_addr")

	if apiAddr != "" {
		opListen, err := net.Listen("tcp", apiAddr)
		if err != nil {
			log.Fatal(err)
		}
		opServer := api.NewServer(stream, rtmpAddr)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Error("HTTP-API server panic: ", r)
				}
			}()
			log.Info("HTTP-API listen On ", apiAddr)
			opServer.Serve(opListen)
		}()
		go func ()  {
			LogStaticsTickerLaunch(opServer)
		}()
	}
}

func LogStaticsTickerLaunch(opServer *api.Server) {
	ticker := time.NewTicker(1 * time.Minute)

	for range ticker.C {
		msgs := opServer.SimpleLiveStaticsData();
		log.Info("statics:", *msgs);
	}
}

func init() {
	mate, _ := logrus_mate.NewLogrusMate(
		logrus_mate.ConfigFile("mate.conf"),
	)
	mate.Hijack(
			log.StandardLogger(),
			"mike",
	)

	// log.SetFormatter(&log.TextFormatter{
	// 	FullTimestamp: true,
	// 	CallerPrettyfier: func(f *runtime.Frame) (string, string) {
	// 		filename := path.Base(f.File)
	// 		return fmt.Sprintf("%s()", f.Function), fmt.Sprintf(" %s:%d", filename, f.Line)
	// 	},
	// })
	// current := time.Now()
	// yyyymmdd := current.Format("20060102")
	// file, err := os.OpenFile("livego_"+ yyyymmdd + ".log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
  // if err != nil {
	// 		fmt.Println("Could Not Open Log File : " + err.Error())
	// }

	// log.SetOutput(io.MultiWriter(file, os.Stdout))
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			log.Error("livego panic: ", r)
			time.Sleep(1 * time.Second)
		}
	}()

	log.Infof(`
		 _     _            ____
		| |   (_)_   _____ / ___| ___
		| |   | \ \ / / _ \ |  _ / _ \
		| |___| |\ V /  __/ |_| | (_) |
		|_____|_| \_/ \___|\____|\___/
				version: %s
	`, VERSION)

	stream := rtmp.NewRtmpStream()
	// hlsServer := startHls()
	startHTTPFlv(stream)
	startAPI(stream)

	startRtmp(stream, nil)

}
