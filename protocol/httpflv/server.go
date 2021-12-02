package httpflv

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/gwuhaolin/livego/av"
	"github.com/gwuhaolin/livego/protocol/rtmp"

	log "github.com/sirupsen/logrus"
	"nhooyr.io/websocket"
)

// var upgrader = websocket.Upgrader{
// 	ReadBufferSize:  1024,
// 	WriteBufferSize: 1024,
// 	CheckOrigin:     checkOrigin,
// }

// func checkOrigin(r *http.Request) bool {
// 	return true
// }

type Server struct {
	handler av.Handler
}

type stream struct {
	Key string `json:"key"`
	Id  string `json:"id"`
}

type streams struct {
	Publishers []stream `json:"publishers"`
	Players    []stream `json:"players"`
}

func IsWebSocketUpgrade(r *http.Request) bool {
	return tokenListContainsValue(r.Header, "Connection", "upgrade") &&
		tokenListContainsValue(r.Header, "Upgrade", "websocket")
}

func NewServer(h av.Handler) *Server {
	return &Server{
		handler: h,
	}
}

func (server *Server) Serve(l net.Listener) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		server.handleConn(w, r)
	})
	mux.HandleFunc("/streams", func(w http.ResponseWriter, r *http.Request) {
		server.getStream(w, r)
	})
	if err := http.Serve(l, mux); err != nil {
		return err
	}
	return nil
}

// 获取发布和播放器的信息
func (server *Server) getStreams(w http.ResponseWriter, r *http.Request) *streams {
	rtmpStream := server.handler.(*rtmp.RtmpStream)
	if rtmpStream == nil {
		return nil
	}
	msgs := new(streams)

	rtmpStream.GetStreams().Range(func(key, val interface{}) bool {
		if s, ok := val.(*rtmp.Stream); ok {
			if s.GetReader() != nil {
				msg := stream{key.(string), s.GetReader().Info().UID}
				msgs.Publishers = append(msgs.Publishers, msg)
			}
		}
		return true
	})

	rtmpStream.GetStreams().Range(func(key, val interface{}) bool {
		ws := val.(*rtmp.Stream).GetWs()

		ws.Range(func(k, v interface{}) bool {
			if pw, ok := v.(*rtmp.PackWriterCloser); ok {
				if pw.GetWriter() != nil {
					msg := stream{key.(string), pw.GetWriter().Info().UID}
					msgs.Players = append(msgs.Players, msg)
				}
			}
			return true
		})
		return true
	})

	return msgs
}

func (server *Server) getStream(w http.ResponseWriter, r *http.Request) {
	msgs := server.getStreams(w, r)
	if msgs == nil {
		return
	}
	resp, _ := json.Marshal(msgs)
	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}

func (server *Server) handleConn(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("http flv handleConn panic: ", r)
		}
	}()

	url := r.URL.String()
	u := r.URL.Path
	if pos := strings.LastIndex(u, "."); pos < 0 || u[pos:] != ".flv" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	path := strings.TrimSuffix(strings.TrimLeft(u, "/"), ".flv")
	paths := strings.SplitN(path, "/", 2)
	log.Debug("url:", u, "path:", path, "paths:", paths)

	if len(paths) != 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	// 判断视屏流是否发布,如果没有发布,直接返回404
	msgs := server.getStreams(w, r)
	if msgs == nil || len(msgs.Publishers) == 0 {
		http.Error(w, "invalid path", http.StatusNotFound)
		return
	} else {
		include := false
		for _, item := range msgs.Publishers {
			if item.Key == path {
				include = true
				break
			}
		}
		if include == false {
			http.Error(w, "invalid path", http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")

	var writer *FLVWriter
	var protocol string
	if IsWebSocketUpgrade(r) {
		protocol = "websocket"
		// ws, err := upgrader.Upgrade(w, r, w.Header())
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"},
		})

		if err != nil {
			log.Errorf("upgrade:", err)
			return
		}
		ctx := ws.CloseRead(r.Context())

		log.Println("ws flv connected: ", url)
		writer = NewFLVWriter(paths[0], paths[1], url, nil, ws, ctx)
		defer func() {
			log.Info("ws flv closed: ", url)
			// ws.Close()
			ws.Close(websocket.StatusNormalClosure, "")
		}()
	} else {
		protocol = "http"
		writer = NewFLVWriter(paths[0], paths[1], url, w, nil, nil)
	}

	log.Info(protocol, " flv start handle writer, id:", writer.Uid, " url:", url)
	server.handler.HandleWriter(writer)
	writer.Wait()
}
