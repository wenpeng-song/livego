package httpflv

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gwuhaolin/livego/av"
	"github.com/gwuhaolin/livego/protocol/amf"
	"github.com/gwuhaolin/livego/utils/pio"
	"github.com/gwuhaolin/livego/utils/uid"

	"nhooyr.io/websocket"
	// "github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

const (
	headerLen   = 11
	maxQueueNum = 1024
)

type FLVWriter struct {
	Uid string
	av.RWBaser
	app, title, url string
	buf             []byte
	closed          bool
	closedChan      chan struct{}
	ctx             http.ResponseWriter
	ws              *websocket.Conn
	ws_ctx          context.Context
	packetQueue     chan *av.Packet
}

func WriteBinary(writer *FLVWriter, data []byte) (int, error) {
	if writer.ws != nil {
		// err := writer.ws.WriteMessage(websocket.BinaryMessage, data)
		err := writer.ws.Write(writer.ws_ctx, websocket.MessageBinary, data)
		// err := wsutil.WriteServerMessage(writer.ws, ws.OpBinary, data)
		return 0, err
	} else {
		return writer.ctx.Write(data)
	}
}

// func NewFLVWriter(app, title, url string, ctx http.ResponseWriter, ws *websocket.Conn) *FLVWriter {
func NewFLVWriter(app, title, url string, ctx http.ResponseWriter, ws *websocket.Conn, ws_ctx context.Context) *FLVWriter {
	// func NewFLVWriter(app, title, url string, ctx http.ResponseWriter, ws net.Conn) *FLVWriter {
	ret := &FLVWriter{
		Uid:         uid.NewId(),
		app:         app,
		title:       title,
		url:         url,
		ctx:         ctx,
		ws:          ws,
		ws_ctx:      ws_ctx,
		RWBaser:     av.NewRWBaser(time.Second * 10),
		closedChan:  make(chan struct{}),
		buf:         make([]byte, headerLen),
		packetQueue: make(chan *av.Packet, maxQueueNum),
	}

	if _, err := WriteBinary(ret, []byte{0x46, 0x4c, 0x56, 0x01, 0x05, 0x00, 0x00, 0x00, 0x09}); err != nil {
		log.Errorf("Error on response writer")
		if !ret.closed {
			ret.Close(err)
		}
	}
	pio.PutI32BE(ret.buf[:4], 0)
	if _, err := WriteBinary(ret, ret.buf[:4]); err != nil {
		log.Errorf("Error on response writer")
		if !ret.closed {
			ret.Close(err)
		}
	}
	go func() {
		err := ret.SendPacket()
		if err != nil {
			log.Error("SendPacket error: ", err)
			if !ret.closed {
				ret.Close(err)
			}
		}

	}()
	return ret
}

func (flvWriter *FLVWriter) DropPacket(pktQue chan *av.Packet, info av.Info) {
	log.Warningf("[%v] packet queue max!!!", info)
	for i := 0; i < maxQueueNum-84; i++ {
		tmpPkt, ok := <-pktQue
		if ok && tmpPkt.IsVideo {
			videoPkt, ok := tmpPkt.Header.(av.VideoPacketHeader)
			// dont't drop sps config and dont't drop key frame
			if ok && (videoPkt.IsSeq() || videoPkt.IsKeyFrame()) {
				log.Debug("insert keyframe to queue")
				pktQue <- tmpPkt
			}

			if len(pktQue) > maxQueueNum-10 {
				<-pktQue
			}
			// drop other packet
			<-pktQue
		}
		// try to don't drop audio
		if ok && tmpPkt.IsAudio {
			log.Debug("insert audio to queue")
			pktQue <- tmpPkt
		}
	}
	log.Debug("packet queue len: ", len(pktQue))
}

func (flvWriter *FLVWriter) Write(p *av.Packet) (err error) {
	err = nil
	if flvWriter.closed {
		err = fmt.Errorf("flvwrite source closed")
		return
	}

	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("FLVWriter has already been closed:%v", e)
		}
	}()

	if len(flvWriter.packetQueue) >= maxQueueNum-24 {
		flvWriter.DropPacket(flvWriter.packetQueue, flvWriter.Info())
	} else {
		flvWriter.packetQueue <- p
	}

	return
}

func (flvWriter *FLVWriter) SendPacket() error {
	for {
		if flvWriter.closed {
			return fmt.Errorf("closed")
		}
		p, ok := <-flvWriter.packetQueue
		if ok {
			flvWriter.RWBaser.SetPreTime()
			h := flvWriter.buf[:headerLen]
			typeID := av.TAG_VIDEO
			if !p.IsVideo {
				if p.IsMetadata {
					var err error
					typeID = av.TAG_SCRIPTDATAAMF0
					p.Data, err = amf.MetaDataReform(p.Data, amf.DEL)
					if err != nil {
						return err
					}
				} else {
					typeID = av.TAG_AUDIO
				}
			}
			dataLen := len(p.Data)
			timestamp := p.TimeStamp
			timestamp += flvWriter.BaseTimeStamp()
			flvWriter.RWBaser.RecTimeStamp(timestamp, uint32(typeID))

			preDataLen := dataLen + headerLen
			timestampbase := timestamp & 0xffffff
			timestampExt := timestamp >> 24 & 0xff

			pio.PutU8(h[0:1], uint8(typeID))
			pio.PutI24BE(h[1:4], int32(dataLen))
			pio.PutI24BE(h[4:7], int32(timestampbase))
			pio.PutU8(h[7:8], uint8(timestampExt))

			if _, err := WriteBinary(flvWriter, h); err != nil {
				return err
			}

			if _, err := WriteBinary(flvWriter, p.Data); err != nil {
				return err
			}

			pio.PutI32BE(h[:4], int32(preDataLen))
			if _, err := WriteBinary(flvWriter, h[:4]); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("closed")
		}

	}
}

func (flvWriter *FLVWriter) Wait() {
	select {
	case <-flvWriter.closedChan:
		return
	}
}

func (flvWriter *FLVWriter) Close(error) {
	if !flvWriter.closed {
		var protocol string
		if flvWriter.ws != nil {
			protocol = "websocket"
		} else {
			protocol = "http"
		}
		log.Info(protocol, " flv writer closed, uid:", flvWriter.Uid, " url:", flvWriter.url)
		close(flvWriter.packetQueue)
		close(flvWriter.closedChan)
	}
	flvWriter.closed = true
}

func (flvWriter *FLVWriter) Info() (ret av.Info) {
	ret.UID = flvWriter.Uid
	ret.URL = flvWriter.url
	ret.Key = flvWriter.app + "/" + flvWriter.title
	ret.Inter = true
	return
}
