package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"time"
)

const (
	SYN          = 0X01 << 1
	ACK          = 0X01 << 2
	FIN          = 0X01 << 3
	SourceIP     = "127.0.0.1"
	SourcePort   = 20002
	TargetIP     = "127.0.0.1"
	TargetPort   = 8080
	HeaderLength = 18
	MSS          = 1024
)

type Header struct {
	SrcIP   uint32
	SrcPort uint16
	SeqNum  uint32
	AckNum  uint32
	Flags   uint16
	WinSize uint16
}

var syncSeqNum uint32
var lastAckNum uint32
var curSeqNum uint32
var cwndSize uint16
var cwndBase uint32
var ssthresh uint16
var dupAckCount uint8
var isCongestion bool
var culmulativeOffset uint16

var expectedSeqNum uint32
var rwndSize uint16
var rwndBase uint32
var bufSeqNumSet map[uint32]bool

var ch chan uint32

func generateSeqNum() uint32 {
	randSource := rand.NewSource(time.Now().UnixNano())
	r := rand.New(randSource)

	SeqNum := r.Intn(1 << 10)

	return uint32(SeqNum)
}

func sendUDP(packet []byte) {
	conn, err := net.Dial("udp", TargetIP+":"+strconv.Itoa(TargetPort))
	if err != nil {
		fmt.Println(err.Error())
	}
	defer conn.Close()

	conn.Write(packet)
}

func packetListener() {
	pc, err := net.ListenPacket("udp", SourceIP+":"+strconv.Itoa(SourcePort))
	if err != nil {
		fmt.Println(err.Error())
	}
	defer pc.Close()
	ch <- 0 // Block sender until listener

	// Listen loop
	packet := make([]byte, 1536)
	for {
		n, _, err := pc.ReadFrom(packet)
		if err != nil {
			fmt.Println(err.Error())
		}

		// Handle receving packet
		packetHandler(packet[:n])
	}
}

func packetHandler(packet []byte) {
	// Deserialize header
	var header Header
	headerBytes := packet[:HeaderLength]
	headerBuf := bytes.NewReader(headerBytes)
	err := binary.Read(headerBuf, binary.BigEndian, &header)
	if err != nil {
		fmt.Println(err.Error())
	}

	// SrcIP := header.SrcIP
	// SrcPort := header.SrcPort
	// SeqNum := header.SeqNum
	// fmt.Println("Received Packet with IP: ", SrcIP)

	if header.Flags&SYN != 0 {

	}

	if header.Flags&FIN != 0 {

	}

	if header.Flags&ACK != 0 {
		fmt.Println("Received ACK with AckNum: ", header.AckNum)

		if header.AckNum > lastAckNum {
			// New ACK
			dupAckCount = 0 // Reset duplicate ACK count
			offset := header.AckNum - lastAckNum
			lastAckNum = header.AckNum
			// Change congestion window size
			if isCongestion {
				// State transtion from Fast Recovery to Congestion Avodiance
				isCongestion = false
				cwndSize = ssthresh
				fmt.Println("[Slow Start] cw size set to ssthresh", ssthresh/MSS)
			} else if cwndSize <= ssthresh-uint16(offset) && cwndSize <= header.WinSize-uint16(offset) {
				// Slow Start
				cwndSize += uint16(offset)
				fmt.Println("[Slow Start] cw size increase to ", cwndSize/MSS)
			} else if cwndSize > ssthresh-uint16(offset) && cwndSize <= header.WinSize-uint16(offset) {
				// Congestion Avoidance
				culmulativeOffset += uint16(offset)
				if culmulativeOffset >= cwndSize {
					culmulativeOffset = 0
					cwndSize += MSS
					fmt.Println("[Congestion Avoidance] cw size increase to ", cwndSize/MSS)
				}
			}
			// Slide window
			cwndBase += offset
			fmt.Println("cw base increase to ", cwndBase/MSS)
		} else if header.AckNum == lastAckNum {
			// Duplicate ACK
			dupAckCount += 1
			if dupAckCount == 3 {
				isCongestion = true
				// Enter Fast Recovery state
				fmt.Println("[Congestion] receive 3 DupACKs.")
				ssthresh = cwndSize / 2
				cwndSize = ssthresh + 3*MSS
				fmt.Println("[Congestion] cw size decrease to ", cwndSize/MSS)
				// Notify sender goroutine to resend data through channel
				ch <- lastAckNum
			} else if dupAckCount > 3 {
				if cwndSize <= header.WinSize-MSS {
					cwndSize += MSS
					fmt.Println("[Fast Recovery] cw size increase to ", cwndSize/MSS)
				}
			}
		}
	}

	// Receive data
	if len(packet) > HeaderLength {
		if header.SeqNum == expectedSeqNum {
			fmt.Println("[In Order] receive packet with SeqNum: ", header.SeqNum)
			// payload := packet[HeaderLength:]

			AckNum := header.SeqNum + MSS
			_, ok := bufSeqNumSet[expectedSeqNum+MSS]
			delete(bufSeqNumSet, expectedSeqNum+MSS)
			for i := 2; ok; i++ {
				AckNum = expectedSeqNum + uint32(i)*MSS
				_, ok = bufSeqNumSet[expectedSeqNum+uint32(i)*MSS]
				delete(bufSeqNumSet, expectedSeqNum+uint32(i)*MSS)
			}
			sendACK(AckNum)
			expectedSeqNum = AckNum
			// if bufferedAckNum > AckNum {
			// 	expectedSeqNum = bufferedAckNum
			// } else {
			// 	expectedSeqNum = AckNum
			// }
		} else if header.SeqNum > expectedSeqNum {
			fmt.Println("[Out of Order] receive packet with SeqNum: ", header.SeqNum)
			sendACK(expectedSeqNum)
			bufSeqNumSet[header.SeqNum] = true
			// Buffer the packet and send duplicate ACK
			// payload := packet[HeaderLength:]
			// bufferedAckNum := uint32((header.SeqNum + uint32(len(payload))) % (0x01 << 31))
		}
	}
}

// Initiate a connection
func initConn() {
	curSeqNum = generateSeqNum()

	header := Header{
		SrcIP:   binary.BigEndian.Uint32(net.ParseIP(SourceIP).To4()),
		SrcPort: SourcePort,
		SeqNum:  curSeqNum,
		AckNum:  0x00,
		Flags:   SYN,
		WinSize: 32 * MSS,
	}

	// Serialize the header
	headerBuf := bytes.Buffer{}
	binary.Write(&headerBuf, binary.BigEndian, header)

	sendUDP(headerBuf.Bytes())
}

func closeConn() {

}

func sendData(seqNum uint32, data []byte) {
	header := Header{
		SrcIP:   binary.BigEndian.Uint32(net.ParseIP(SourceIP).To4()),
		SrcPort: SourcePort,
		SeqNum:  seqNum,
		AckNum:  0x00,
		Flags:   0x00,
		WinSize: 32 * MSS,
	}

	// Serialize the header
	buf := bytes.Buffer{}
	binary.Write(&buf, binary.BigEndian, header)

	buf.Write(data)

	sendUDP(buf.Bytes())
	fmt.Println("Send data with SeqNum: ", seqNum)
}

func sendACK(AckNum uint32) {
	header := Header{
		SrcIP:   binary.BigEndian.Uint32(net.ParseIP(SourceIP).To4()),
		SrcPort: SourcePort,
		SeqNum:  0x00,
		AckNum:  AckNum,
		Flags:   ACK,
		WinSize: 32 * MSS,
	}

	// Serialize the header
	buf := bytes.Buffer{}
	binary.Write(&buf, binary.BigEndian, header)

	sendUDP(buf.Bytes())
	fmt.Println("Send ACK with AckNum: ", AckNum)
}

func sendFile() {
	// Init sender's global variables
	syncSeqNum = 0
	curSeqNum = syncSeqNum
	lastAckNum = syncSeqNum
	cwndSize = 1 * MSS
	cwndBase = 0
	ssthresh = 32 * MSS
	dupAckCount = 0

	file := make([]byte, 100*MSS)

	for {
		select {
		case seqNum := <-ch:
			offset := seqNum - syncSeqNum
			sendData(seqNum, file[offset:offset+MSS])
		default:
			offset := curSeqNum - syncSeqNum
			if offset-cwndBase < uint32(cwndSize) && offset < uint32(len(file)) {
				sendData(curSeqNum, file[offset:offset+MSS])
				curSeqNum = syncSeqNum + offset + MSS
			}
		}
		if cwndBase == uint32(len(file)) {
			fmt.Println("finish")
			break
		}
	}
}

func main() {
	ch = make(chan uint32)

	go packetListener()

	<-ch
	sendFile()

	// start := time.Now()
	// for i := 0; i < 100000; i++ {
	// 	sendData(data[i : i+MSS])
	// }
	// end := time.Now()
	// fmt.Println(end.Sub(start))
}
