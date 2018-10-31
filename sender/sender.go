package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"time"
	"os"
	"io"
)

const (
	SYN          = 0X01 << 1
	ACK          = 0X01 << 2
	FIN          = 0X01 << 3
	DATA         = 0X01 << 4
	SrcIP        = "127.0.0.1"
	SrcPort      = 20002
	HeaderLength = 18
	MSS          = 1024
	RTO          = 30 * time.Millisecond
	DestAddr     = "127.0.0.1:8080"
	SrcAddr      = "127.0.0.1:20002"
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

// Receiver's variables
var expectedSeqNum uint32
var rwndSize uint16
var rwndBase uint32
var bufSeqNumSet map[uint32]bool
var bufPayloadMap map[uint32][]byte

// Common variables
var ch chan uint32

func BinaryIP(stringIP string) uint32 {
	return binary.BigEndian.Uint32(net.ParseIP(stringIP).To4())
}

func StringIP(binaryIP uint32) string {
	IP := make(net.IP, 4)
	binary.BigEndian.PutUint32(IP, binaryIP)
	return IP.String()
}

func generateSeqNum() uint32 {
	randSource := rand.NewSource(time.Now().UnixNano())
	r := rand.New(randSource)

	SeqNum := r.Intn(1000)

	return uint32(SeqNum)
}

func setupConn(addr string) net.Conn {
	conn, err := net.Dial("udp", addr)
	if err != nil {
		fmt.Println(err.Error())
	}

	return conn
}

func packetListener(addr string) {
	pc, err := net.ListenPacket("udp", addr)
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

	if header.Flags&SYN != 0 {
		fmt.Printf("Receive SYN from (%s:%d) with SeqNum: %d\n", StringIP(header.SrcIP), header.SrcPort, header.SeqNum)

		ch <- header.SrcIP
		ch <- uint32(header.SrcPort)
		ch <- header.SeqNum
	}

	if header.Flags&FIN != 0 {

	}

	if header.Flags&ACK != 0 {
		fmt.Println("Received ACK with AckNum: ", header.AckNum/MSS)

		if header.AckNum == syncSeqNum {
			// Connection request accepted, notify sending goroutine
			ch <- syncSeqNum
		}

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

	if header.Flags&DATA != 0 {

		if header.SeqNum == expectedSeqNum {
			fmt.Println("[In Order] receive packet with SeqNum: ", expectedSeqNum/MSS)

			// Deliver payload
			payload := packet[HeaderLength:]
			payloadSize := uint32(len(payload))
			fmt.Printf("deliver %d bytes of seqNum %d\n", payloadSize, expectedSeqNum/MSS)

			culmulativePayloadSize := payloadSize
			AckNum := expectedSeqNum + culmulativePayloadSize

			// Check if there are consecutive buffered ACK
			_, ok := bufSeqNumSet[AckNum]
			if ok {
				delete(bufSeqNumSet, AckNum)
			}

			for ok {
				// Deliver buffered payload
				bufPayload := bufPayloadMap[AckNum]
				bufPayloadSize := uint32(len(bufPayload))
				fmt.Printf("deliver %d bytes of seqNum %d\n", bufPayloadSize, AckNum/MSS)
				delete(bufPayloadMap, AckNum)

				culmulativePayloadSize += bufPayloadSize
				AckNum = expectedSeqNum + culmulativePayloadSize

				// Check if there are consecutive buffered ACK
				_, ok = bufSeqNumSet[AckNum]
				if ok {
					delete(bufSeqNumSet, AckNum)
				}
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

func sendSYN(conn net.Conn, seqNum uint32) {
	header := Header{
		SrcIP:   BinaryIP(SrcIP),
		SrcPort: SrcPort,
		SeqNum:  seqNum,
		AckNum:  0x00,
		Flags:   SYN,
		WinSize: 32 * MSS,
	}

	// Serialize the header
	buf := bytes.Buffer{}
	binary.Write(&buf, binary.BigEndian, header)

	conn.Write(buf.Bytes())
	fmt.Println("Send SYN with SeqNum: ", seqNum/MSS)
}

func sendDATA(conn net.Conn, seqNum uint32, data []byte) {
	header := Header{
		SrcIP:   BinaryIP(SrcIP),
		SrcPort: SrcPort,
		SeqNum:  seqNum,
		AckNum:  0x00,
		Flags:   DATA,
		WinSize: 32 * MSS,
	}

	// Serialize the header
	buf := bytes.Buffer{}
	binary.Write(&buf, binary.BigEndian, header)

	buf.Write(data)

	conn.Write(buf.Bytes())
	fmt.Println("Send DATA with SeqNum: ", seqNum/MSS)
}

func sendACK(conn net.Conn, AckNum uint32) {
	header := Header{
		SrcIP:   BinaryIP(SrcIP),
		SrcPort: SrcPort,
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

func recvFile() {
	bufSeqNumSet = map[uint32]bool{}    // Mark out of order packet
	bufPayloadMap = map[uint32][]byte{} // Buffer out of order packet's payload
	ch = make(chan uint32)

	// Setup listening conn
	go packetListener(SrcAddr)

	// Receive SYN
	destIP := <-ch
	destPort := <-ch
	syncSeqNum = <-ch
	expectedSeqNum = syncSeqNum

	// Parse DestAddr
	destAddr := StringIP(destIP) + ":" + strconv.Itoa(int(destPort))

	conn := setupConn(destAddr)
	defer conn.Close()

	// Send ACK to accept connection
	sendACK(conn, syncSeqNum)

	for {
		select {
		case AckNum := <-ch:
			sendACK(conn, AckNum)

		default:
		}
	}
}

func sendFile(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		fmt.Println(err.Error())
	}
	defer file.Close()

	var fileBuf bytes.Buffer
	io.Copy(&fileBuf, file)

	fileBytes := fileBuf.Bytes()
	filesize := uint32(len(fileBytes))

	fmt.Printf("%s's size: %d\n", filename, filesize)

	// Init sender's global variables
	syncSeqNum = 0
	curSeqNum = syncSeqNum
	lastAckNum = syncSeqNum
	cwndSize = 1 * MSS
	cwndBase = 0
	ssthresh = 32 * MSS
	dupAckCount = 0
	ch = make(chan uint32)

	// Setup listening conn
	go packetListener(SrcAddr)

	// Setup sending conn
	conn := setupConn(DestAddr)
	defer conn.Close()

	// Initiates a connection and block until receives the other side's ACK
	sendSYN(conn, syncSeqNum)
	code := <-ch
	for code != syncSeqNum {
	}

	// Init RTO timer
	startTimer()

	for {
		select {

		case seqNum := <-ch:
			// Retransmit seqNum
			offset := seqNum - syncSeqNum
			if offset+MSS < filesize {
				sendDATA(conn, seqNum, fileBytes[offset:offset+MSS])
			} else {
				sendDATA(conn, curSeqNum, fileBytes[offset:filesize])
			}

		default:
			offset := curSeqNum - syncSeqNum
			if offset-cwndBase < uint32(cwndSize) {
				if offset+MSS < filesize {
					sendDATA(conn, curSeqNum, fileBytes[offset:offset+MSS])
					curSeqNum = syncSeqNum + offset + MSS
				} else {
					sendDATA(conn, curSeqNum, fileBytes[offset:filesize])
					curSeqNum = syncSeqNum + offset + (filesize - offset)
				}
			}
		}

		if cwndBase == filesize {
			fmt.Println("finish")
			break
		}
	}
}

func main() {
	sendFile("file")
}

// start := time.Now()
// for i := 0; i < 100000; i++ {
// 	sendData(data[i : i+MSS])
// }
// end := time.Now()
// fmt.Println(end.Sub(start))
