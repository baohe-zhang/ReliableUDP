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
	RTO          = 30 * time.Millisecond
)

type Header struct {
	SrcIP   uint32
	SrcPort uint16
	SeqNum  uint32
	AckNum  uint32
	Flags   uint16
	WinSize uint16
}

// Sender's variables
var syncSeqNum uint32
var lastAckNum uint32
var curSeqNum uint32
var cwndSize uint16
var cwndBase uint32
var ssthresh uint16
var dupAckCount uint8
var isCongestion bool
var culmulativeOffset uint16
var timer *time.Timer
var ch chan uint32

// Receiver's variables
var expectedSeqNum uint32
var rwndSize uint16
var rwndBase uint32
var bufSeqNumSet map[uint32]bool
var bufPayloadMap map[uint32][]byte

func generateSeqNum() uint32 {
	randSource := rand.NewSource(time.Now().UnixNano())
	r := rand.New(randSource)

	SeqNum := r.Intn(1000)

	return uint32(SeqNum)
}

func setupConn() net.Conn {
	conn, err := net.Dial("udp", TargetIP+":"+strconv.Itoa(TargetPort))
	if err != nil {
		fmt.Println(err.Error())
	}

	return conn
}

func packetListener() {
	pc, err := net.ListenPacket("udp", SourceIP+":"+strconv.Itoa(SourcePort))
	if err != nil {
		fmt.Println(err.Error())
	}
	defer pc.Close()

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
		fmt.Println("Received ACK with AckNum: ", header.AckNum/MSS)

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
			fmt.Println("[Slide Window] cw base increase to ", cwndBase/MSS)

			// Restart RTO timer
			stop := timer.Stop()
			if stop {
				startTimer()
			}

		} else if header.AckNum == lastAckNum {
			// Duplicate ACK
			dupAckCount += 1
			if dupAckCount == 3 {
				isCongestion = true
				// Enter Fast Recovery state
				fmt.Println("[Congestion] receive 3 DupACKs.")
				ssthresh = cwndSize / 2
				fmt.Println("[Congestion] ssthresh set to ", ssthresh/MSS)
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
			fmt.Println("[In Order] receive packet with SeqNum: ", expectedSeqNum/MSS)
			// Deliver payload
			payload := packet[HeaderLength:]
			fmt.Printf("deliver %d bytes of seqNum %d\n", len(payload), expectedSeqNum/MSS)

			AckNum := expectedSeqNum + MSS

			// Check if there are consecutive buffered ACK
			_, ok := bufSeqNumSet[expectedSeqNum+MSS]
			delete(bufSeqNumSet, expectedSeqNum+MSS)
			for i := 2; ok; i++ {
				// Deliver payload with seqNum expectedSeqNum + uint32(i-1)*MSS
				buf_payload := bufPayloadMap[expectedSeqNum+uint32(i-1)*MSS]
				fmt.Printf("deliver %d bytes of seqNum %d\n", len(buf_payload), (expectedSeqNum+uint32(i-1)*MSS)/MSS)
				delete(bufPayloadMap, expectedSeqNum+uint32(i-1)*MSS)

				AckNum = expectedSeqNum + uint32(i)*MSS
				_, ok = bufSeqNumSet[expectedSeqNum+uint32(i)*MSS]
				delete(bufSeqNumSet, expectedSeqNum+uint32(i)*MSS)
			}

			// Send ACK and update expectedSeqNum
			ch <- AckNum
			expectedSeqNum = AckNum

		} else if header.SeqNum > expectedSeqNum {
			fmt.Println("[Out of Order] receive packet with SeqNum: ", header.SeqNum/MSS)
			// Buffer payload
			payload := packet[HeaderLength:]
			bufPayloadMap[header.SeqNum] = payload

			// Mark the out of order packet
			bufSeqNumSet[header.SeqNum] = true

			// Send duplicate ACK
			ch <- expectedSeqNum

		} else {
			fmt.Println("[Already Acknowledged] Receive Acked packet with SeqNum: ", header.SeqNum/MSS)
		}
	}
}

// Initiate a connection
// func initConn() {
// 	curSeqNum = generateSeqNum()

// 	header := Header{
// 		SrcIP:   binary.BigEndian.Uint32(net.ParseIP(SourceIP).To4()),
// 		SrcPort: SourcePort,
// 		SeqNum:  curSeqNum,
// 		AckNum:  0x00,
// 		Flags:   SYN,
// 		WinSize: 32 * MSS,
// 	}

// 	// Serialize the header
// 	headerBuf := bytes.Buffer{}
// 	binary.Write(&headerBuf, binary.BigEndian, header)

// 	sendUDP(headerBuf.Bytes())
// }

// func closeConn() {

// }

func sendData(conn net.Conn, seqNum uint32, data []byte) {
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

	conn.Write(buf.Bytes())
	fmt.Println("Send data with SeqNum: ", seqNum/MSS)
}

func sendACK(conn net.Conn, AckNum uint32) {
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

	conn.Write(buf.Bytes())
	fmt.Println("Send ACK with AckNum: ", AckNum/MSS)
}

func startTimer() {
	timer = time.NewTimer(RTO)
	go func() {
		<-timer.C
		fmt.Println("[RTO timeout]")
		// Notify sender to retransmit
		ch <- (cwndBase - syncSeqNum)
		// Update variables
		ssthresh = cwndSize / 2
		cwndSize = 1 * MSS
		fmt.Println("[RTO timeout] cw size decrease to ", cwndSize/MSS)
		dupAckCount = 0
		// Start a new timer
		startTimer()
	}()
}

func sendFile() {
	file := make([]byte, 100000*MSS)

	// Init sender's global variables
	syncSeqNum = 0
	curSeqNum = syncSeqNum
	lastAckNum = syncSeqNum
	cwndSize = 1 * MSS
	cwndBase = 0
	ssthresh = 32 * MSS
	dupAckCount = 0
	ch = make(chan uint32)

	go packetListener()

	// Init RTO timer
	startTimer()

	// setup conn
	conn := setupConn()
	defer conn.Close()

	for {
		select {

		case seqNum := <-ch:
			// Retransmit seqNum
			offset := seqNum - syncSeqNum
			sendData(conn, seqNum, file[offset:offset+MSS])

		default:
			offset := curSeqNum - syncSeqNum
			if offset-cwndBase < uint32(cwndSize) && offset < uint32(len(file)) {
				if generateSeqNum() < 990 {
					sendData(conn, curSeqNum, file[offset:offset+MSS])
				}
				// sendData(conn, curSeqNum, file[offset:offset+MSS])
				curSeqNum = syncSeqNum + offset + MSS
			}
		}

		if cwndBase == uint32(len(file)) {
			fmt.Println("finish")
			break
		}
	}
}

func recvFile() {
	syncSeqNum = 0
	expectedSeqNum = syncSeqNum
	bufSeqNumSet = map[uint32]bool{}    // Mark out of order packet
	bufPayloadMap = map[uint32][]byte{} // Buffer out of order packet's payload
	ch = make(chan uint32)

	go packetListener()

	conn := setupConn()
	defer conn.Close()

	for {
		select {
		case AckNum := <-ch:
			sendACK(conn, AckNum)

		default:
		}
	}
}

func main() {
	sendFile()
}

// start := time.Now()
// for i := 0; i < 100000; i++ {
// 	sendData(data[i : i+MSS])
// }
// end := time.Now()
// fmt.Println(end.Sub(start))
